package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	razorpay "github.com/razorpay/razorpay-go"

	"github.com/yourname/flashcart/internal/events"
	"github.com/yourname/flashcart/internal/kafka"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"github.com/yourname/flashcart/internal/orderstate"
)

type PaymentHandler struct {
	DB            *pgxpool.Pool
	Client        *razorpay.Client
	KeyID         string
	KeySecret     string
	WebhookSecret string
	Producer      *kafka.Producer
}

func NewPaymentHandler(db *pgxpool.Pool, keyID, keySecret, webhookSecret string, producer *kafka.Producer) *PaymentHandler {
	var client *razorpay.Client
	if keyID != "" && keySecret != "" {
		client = razorpay.NewClient(keyID, keySecret)
	}
	return &PaymentHandler{DB: db, Client: client, KeyID: keyID, KeySecret: keySecret, WebhookSecret: webhookSecret, Producer: producer}
}

func (h *PaymentHandler) CreateRazorpayOrder(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req struct {
		OrderID string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.OrderID) == "" {
		writeError(w, http.StatusBadRequest, "order_id is required")
		return
	}
	if h.Client == nil {
		writeError(w, http.StatusServiceUnavailable, "Razorpay is not configured. Add RAZORPAY_KEY_ID and RAZORPAY_KEY_SECRET.")
		return
	}

	var owner, status, paymentMethod string
	var amount int64
	err := h.DB.QueryRow(r.Context(),
		`SELECT user_id::text, status, payment_method, total_paise FROM orders WHERE id=$1`,
		req.OrderID).Scan(&owner, &status, &paymentMethod, &amount)
	if err != nil || owner != userID {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	paymentMethod = normalizePaymentMethod(paymentMethod)
	if paymentMethod != "RAZORPAY" {
		writeError(w, http.StatusBadRequest, "order is not a Razorpay order")
		return
	}
	if status != string(orderstate.PaymentPending) {
		writeError(w, http.StatusConflict, "order is not awaiting Razorpay payment")
		return
	}

	var existingID, existingStatus string
	err = h.DB.QueryRow(r.Context(),
		`SELECT razorpay_order_id, status FROM payments WHERE order_id=$1`, req.OrderID).
		Scan(&existingID, &existingStatus)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"key_id": h.KeyID, "razorpay_order_id": existingID,
			"amount": amount, "currency": "INR", "status": existingStatus,
		})
		return
	}

	data := map[string]interface{}{
		"amount":   amount,
		"currency": "INR",
		"receipt":  "fc_" + req.OrderID,
		"notes":    map[string]interface{}{"flashcart_order_id": req.OrderID, "user_id": userID},
	}
	order, err := h.Client.Order.Create(data, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to create Razorpay order")
		return
	}
	razorpayOrderID, ok := order["id"].(string)
	if !ok || razorpayOrderID == "" {
		writeError(w, http.StatusBadGateway, "Razorpay returned an invalid order")
		return
	}

	_, err = h.DB.Exec(r.Context(),
		`INSERT INTO payments (order_id, razorpay_order_id, amount_paise, currency, method, status)
		 VALUES ($1,$2,$3,'INR','RAZORPAY','CREATED')`,
		req.OrderID, razorpayOrderID, amount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save payment")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"key_id": h.KeyID, "razorpay_order_id": razorpayOrderID,
		"amount": amount, "currency": "INR", "status": "CREATED",
	})
}

func (h *PaymentHandler) Verify(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req struct {
		OrderID           string `json:"order_id"`
		RazorpayOrderID   string `json:"razorpay_order_id"`
		RazorpayPaymentID string `json:"razorpay_payment_id"`
		RazorpaySignature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrderID == "" || req.RazorpayOrderID == "" || req.RazorpayPaymentID == "" || req.RazorpaySignature == "" {
		writeError(w, http.StatusBadRequest, "all Razorpay verification fields are required")
		return
	}

	var owner, dbRazorpayOrderID, status string
	err := h.DB.QueryRow(r.Context(),
		`SELECT o.user_id::text, p.razorpay_order_id, p.status
		 FROM orders o JOIN payments p ON p.order_id=o.id WHERE o.id=$1`,
		req.OrderID).Scan(&owner, &dbRazorpayOrderID, &status)
	if err != nil || owner != userID {
		writeError(w, http.StatusNotFound, "payment order not found")
		return
	}
	if dbRazorpayOrderID != req.RazorpayOrderID {
		writeError(w, http.StatusBadRequest, "Razorpay order mismatch")
		return
	}
	if status == "CAPTURED" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "order_id": req.OrderID})
		return
	}

	expected := sign(req.RazorpayOrderID+"|"+req.RazorpayPaymentID, h.KeySecret)
	if !hmac.Equal([]byte(expected), []byte(req.RazorpaySignature)) {
		writeError(w, http.StatusBadRequest, "payment signature verification failed")
		return
	}
	if status == "REFUNDED" {
		writeError(w, http.StatusConflict, "payment was refunded because it did not meet the coupon requirements")
		return
	}
	if mismatch, err := h.couponPaymentMismatch(r.Context(), req.OrderID, req.RazorpayPaymentID, req.RazorpayOrderID); err != nil {
		writeError(w, http.StatusBadGateway, "could not verify coupon payment eligibility with Razorpay")
		return
	} else if mismatch != "" {
		if err := h.refundCouponMismatch(r.Context(), req.OrderID, req.RazorpayPaymentID, req.RazorpaySignature); err != nil {
			writeError(w, http.StatusBadGateway, "coupon did not match the payment and automatic refund needs support review")
			return
		}
		writeError(w, http.StatusConflict, "the payment was refunded because the selected bank or wallet did not match the coupon; retry with an eligible payment method")
		return
	}

	_, err = h.DB.Exec(r.Context(),
		`UPDATE payments SET razorpay_payment_id=$1, razorpay_signature=$2, status='CAPTURED', updated_at=now()
		 WHERE order_id=$3`,
		req.RazorpayPaymentID, req.RazorpaySignature, req.OrderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record payment")
		return
	}

	if err := orderstate.Transition(r.Context(), h.DB, req.OrderID, orderstate.Paid); err != nil {
		writeError(w, http.StatusConflict, "payment recorded but order could not be marked paid")
		return
	}
	if err := orderstate.Transition(r.Context(), h.DB, req.OrderID, orderstate.Confirmed); err != nil {
		writeError(w, http.StatusConflict, "payment recorded but order could not be confirmed")
		return
	}
	_ = h.Producer.Publish(r.Context(), events.TopicOrderConfirmed, req.OrderID, events.OrderConfirmed{
		OrderID: req.OrderID, UserID: userID,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success", "order_id": req.OrderID,
		"razorpay_payment_id": req.RazorpayPaymentID,
	})
}

func (h *PaymentHandler) couponPaymentMismatch(ctx context.Context, orderID, paymentID, razorpayOrderID string) (string, error) {
	var requiredMethod, requiredIssuer, selectedMethod, selectedIssuer string
	err := h.DB.QueryRow(ctx, `
		SELECT COALESCE(c.required_payment_method,''), COALESCE(c.required_issuer,''),
		       COALESCE(o.payment_instrument,''), COALESCE(o.payment_issuer,'')
		FROM orders o LEFT JOIN coupons c ON c.code=o.coupon_code WHERE o.id=$1`, orderID).
		Scan(&requiredMethod, &requiredIssuer, &selectedMethod, &selectedIssuer)
	if err != nil || (requiredMethod == "" && requiredIssuer == "") {
		return "", err
	}
	if h.Client == nil {
		return "", errors.New("Razorpay client is unavailable")
	}
	payment, err := h.Client.Payment.Fetch(paymentID, nil, nil)
	if err != nil {
		return "", err
	}
	if gatewayOrderID, ok := payment["order_id"].(string); ok && gatewayOrderID != razorpayOrderID {
		return "", errors.New("payment belongs to another gateway order")
	}
	actualMethod := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payment["method"])))
	if requiredMethod != "" && actualMethod != requiredMethod {
		return "payment method mismatch", nil
	}
	if requiredIssuer == "" {
		return "", nil
	}
	actualIssuer := ""
	if requiredMethod == "CARD" {
		card, err := h.Client.Payment.FetchCardDetails(paymentID, nil, nil)
		if err != nil {
			return "", err
		}
		actualIssuer = normalizeCouponIssuer(fmt.Sprint(card["issuer"]))
	} else if requiredMethod == "WALLET" {
		actualIssuer = normalizeCouponIssuer(fmt.Sprint(payment["wallet"]))
	}
	if actualIssuer == "" || actualIssuer != normalizeCouponIssuer(requiredIssuer) || actualIssuer != normalizeCouponIssuer(selectedIssuer) {
		return "payment issuer mismatch", nil
	}
	return "", nil
}

func normalizeCouponIssuer(issuer string) string {
	issuer = strings.ToUpper(strings.TrimSpace(issuer))
	switch issuer {
	case "SBIN", "STATE BANK OF INDIA":
		return "SBI"
	case "HDFC BANK":
		return "HDFC"
	case "ICIC", "ICICI BANK":
		return "ICICI"
	case "UTIB", "AXIS BANK":
		return "AXIS"
	case "PHONEPE WALLET":
		return "PHONEPE"
	case "PAYTM WALLET":
		return "PAYTM"
	default:
		return issuer
	}
}

func (h *PaymentHandler) refundCouponMismatch(ctx context.Context, orderID, paymentID, signature string) error {
	var amount int64
	var couponCode string
	if err := h.DB.QueryRow(ctx, `SELECT p.amount_paise,o.coupon_code FROM payments p JOIN orders o ON o.id=p.order_id WHERE o.id=$1`, orderID).Scan(&amount, &couponCode); err != nil {
		return err
	}
	if _, err := h.Client.Payment.Refund(paymentID, int(amount), nil, nil); err != nil {
		return err
	}
	if _, err := h.DB.Exec(ctx, `UPDATE payments SET razorpay_payment_id=$1,razorpay_signature=$2,status='REFUNDED',updated_at=now() WHERE order_id=$3`, paymentID, signature, orderID); err != nil {
		return err
	}
	if err := orderstate.Transition(ctx, h.DB, orderID, orderstate.PaymentFailed); err != nil {
		return err
	}
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE products p SET stock=p.stock+oi.quantity FROM order_items oi WHERE oi.order_id=$1 AND p.id=oi.product_id`, orderID); err != nil {
		return err
	}
	if couponCode != "" {
		if _, err := tx.Exec(ctx, `UPDATE coupons SET used_count=GREATEST(used_count-1,0) WHERE code=$1`, couponCode); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE coupon_redemptions SET status='RELEASED',updated_at=now() WHERE order_id=$1 AND status='RESERVED'`, orderID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return orderstate.Transition(ctx, h.DB, orderID, orderstate.Cancelled)
}

func (h *PaymentHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	orderID := chi.URLParam(r, "orderID")
	var owner, rpOrder, rpPayment, status string
	var amount int64
	err := h.DB.QueryRow(r.Context(),
		`SELECT o.user_id::text, p.razorpay_order_id, COALESCE(p.razorpay_payment_id,''), p.status, p.amount_paise
		 FROM orders o JOIN payments p ON p.order_id=o.id WHERE o.id=$1`,
		orderID).Scan(&owner, &rpOrder, &rpPayment, &status, &amount)
	if err != nil || owner != userID {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"order_id": orderID, "razorpay_order_id": rpOrder,
		"razorpay_payment_id": rpPayment, "amount": amount,
		"currency": "INR", "status": status,
	})
}

func (h *PaymentHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook body")
		return
	}
	if h.WebhookSecret != "" {
		sig := r.Header.Get("X-Razorpay-Signature")
		expected := sign(string(body), h.WebhookSecret)
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			writeError(w, http.StatusUnauthorized, "invalid webhook signature")
			return
		}
	}
	// The checkout Verify endpoint is the primary demo path. Webhooks provide
	// a durable fallback/observability hook for future retries and live mode.
	var evt struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID      string `json:"id"`
					OrderID string `json:"order_id"`
					Status  string `json:"status"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if json.Unmarshal(body, &evt) == nil && evt.Event == "payment.captured" {
		var orderID, userID, orderStatus, razorpayOrderID string
		err := h.DB.QueryRow(r.Context(), `
			SELECT o.id::text,o.user_id::text,o.status,p.razorpay_order_id
			FROM orders o JOIN payments p ON p.order_id=o.id WHERE p.razorpay_order_id=$1`,
			evt.Payload.Payment.Entity.OrderID).Scan(&orderID, &userID, &orderStatus, &razorpayOrderID)
		if err == nil && orderStatus == string(orderstate.PaymentPending) {
			if mismatch, checkErr := h.couponPaymentMismatch(r.Context(), orderID, evt.Payload.Payment.Entity.ID, razorpayOrderID); checkErr != nil {
				writeError(w, http.StatusBadGateway, "could not verify coupon payment with Razorpay")
				return
			} else if mismatch != "" {
				if err := h.refundCouponMismatch(r.Context(), orderID, evt.Payload.Payment.Entity.ID, ""); err != nil {
					writeError(w, http.StatusBadGateway, "coupon mismatch refund requires support review")
					return
				}
				w.WriteHeader(http.StatusOK)
				return
			}
			if _, err := h.DB.Exec(r.Context(),
				`UPDATE payments SET razorpay_payment_id=COALESCE(razorpay_payment_id,$1), status='CAPTURED', updated_at=now() WHERE razorpay_order_id=$2`,
				evt.Payload.Payment.Entity.ID, razorpayOrderID); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to record captured payment")
				return
			}
			if err := orderstate.Transition(r.Context(), h.DB, orderID, orderstate.Paid); err == nil {
				if err := orderstate.Transition(r.Context(), h.DB, orderID, orderstate.Confirmed); err == nil {
					_ = h.Producer.Publish(r.Context(), events.TopicOrderConfirmed, orderID, events.OrderConfirmed{OrderID: orderID, UserID: userID})
				}
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func sign(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *PaymentHandler) Refund(w http.ResponseWriter, r *http.Request) {
	if h.Client == nil {
		writeError(w, http.StatusServiceUnavailable, "Razorpay is not configured")
		return
	}
	paymentID := chi.URLParam(r, "paymentID")
	var req struct {
		AmountPaise int64 `json:"amount_paise"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var razorpayPaymentID, status string
	var amount int64
	err := h.DB.QueryRow(r.Context(), `SELECT razorpay_payment_id,status,amount_paise FROM payments WHERE id=$1`, paymentID).Scan(&razorpayPaymentID, &status, &amount)
	if err != nil {
		writeError(w, 404, "payment not found")
		return
	}
	if razorpayPaymentID == "" || status != "CAPTURED" {
		writeError(w, 409, "only captured payments can be refunded")
		return
	}
	if req.AmountPaise <= 0 || req.AmountPaise > amount {
		req.AmountPaise = amount
	}

	if _, err := h.Client.Payment.Refund(razorpayPaymentID, int(req.AmountPaise), nil, nil); err != nil {
		writeError(w, 502, "Razorpay refund failed")
		return
	}
	_, err = h.DB.Exec(r.Context(), `UPDATE payments SET status='REFUNDED',updated_at=now() WHERE id=$1`, paymentID)
	if err != nil {
		writeError(w, 500, "refund succeeded but local status update failed")
		return
	}
	writeJSON(w, 200, map[string]any{"status": "REFUNDED", "amount_paise": req.AmountPaise, "razorpay_payment_id": razorpayPaymentID})
}
