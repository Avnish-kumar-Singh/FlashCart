package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	couponrules "github.com/yourname/flashcart/internal/coupons"
	"github.com/yourname/flashcart/internal/events"
	"github.com/yourname/flashcart/internal/kafka"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"github.com/yourname/flashcart/internal/orderstate"
)

type OrderHandler struct {
	DB       *pgxpool.Pool
	Redis    *redis.Client
	Producer *kafka.Producer
}

const idempotencyTTL = 24 * time.Hour

func normalizePaymentMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "CARD", "UPI", "NETBANKING", "WALLET", "QR", "RAZORPAY":
		return "RAZORPAY"
	case "COD":
		return "COD"
	default:
		return strings.ToUpper(strings.TrimSpace(method))
	}
}

// CreateFromCart converts the authenticated user's cart into an order and
// hands it off to the async payment/confirmation saga (see
// cmd/orderworker). The API's job stops at "reserve inventory and durably
// record intent to buy" — it does not wait for payment to complete.
//
// Three correctness properties are enforced here:
//
//  1. No overselling. Everything runs inside a single DB transaction, and
//     `SELECT ... FOR UPDATE` takes a row lock on each product being
//     purchased, so a concurrent checkout for the same product blocks
//     until this transaction commits or rolls back. Stock can never be
//     read stale and decremented below zero. (Phase 4 replaces this with
//     Redis atomic decrement + reservation for flash-sale scale; the
//     guarantee is established here first with the simpler mechanism.)
//
//  2. No duplicate orders. The client sends an `Idempotency-Key` header.
//     A slow network causing the user to double-tap "Buy Now" produces
//     two HTTP requests with the *same* key — the second one returns the
//     first request's order instead of creating a second order and
//     double-charging inventory/payment.
//
//  3. No blocking on payment. Once inventory is reserved and the order
//     row exists, an `order.created` event goes to Kafka and the API
//     responds `202 Accepted` immediately. The order worker consumes that
//     event, calls the payment simulator, and drives the order to
//     CONFIRMED or ORDER_CANCELLED — see internal/orderstate and
//     cmd/orderworker for the saga itself.
func (h *OrderHandler) CreateFromCart(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	ctx := context.Background()

	var body struct {
		PaymentMethod     string `json:"payment_method"`
		PaymentInstrument string `json:"payment_instrument"`
		PaymentIssuer     string `json:"payment_issuer"`
		CouponCode        string `json:"coupon_code"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	paymentMethod := normalizePaymentMethod(body.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = "COD"
	}
	if !validPaymentMethods[paymentMethod] {
		writeError(w, http.StatusBadRequest, "payment_method must be one of COD, CARD, QR, UPI, NETBANKING, WALLET, RAZORPAY")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}

	redisKey := "idem:" + userID + ":" + idempotencyKey

	// Claim the key. Only the first request for a given key wins the SETNX;
	// everyone else finds it already set and short-circuits below.
	claimed, err := h.Redis.SetNX(ctx, redisKey, "PENDING", idempotencyTTL).Result()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check idempotency key")
		return
	}

	if !claimed {
		existing, err := h.Redis.Get(ctx, redisKey).Result()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read idempotency key")
			return
		}
		if existing == "PENDING" {
			// The original request for this key is still being processed.
			writeError(w, http.StatusConflict, "a request with this idempotency key is already in progress")
			return
		}
		// existing holds a previously created order ID — return it as-is,
		// which is exactly what makes this a safe retry rather than a duplicate.
		writeJSON(w, http.StatusOK, map[string]any{"order_id": existing, "status": "already_processed"})
		return
	}

	orderID, totalPaise, items, discountPaise, err := h.createOrder(ctx, userID, paymentMethod,
		strings.ToUpper(strings.TrimSpace(body.CouponCode)), strings.ToUpper(strings.TrimSpace(body.PaymentInstrument)),
		strings.ToUpper(strings.TrimSpace(body.PaymentIssuer)))
	if err != nil {
		// Release the key so the user can legitimately retry (e.g. after
		// fixing an empty cart) instead of being stuck behind a dead PENDING marker.
		h.Redis.Del(ctx, redisKey)

		var status int
		switch {
		case errors.Is(err, errEmptyCart):
			status = http.StatusBadRequest
		case errors.Is(err, errInsufficientStock):
			status = http.StatusConflict
		case errors.Is(err, errInvalidCoupon):
			status = http.StatusBadRequest
		default:
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}

	// Now that the order exists, overwrite PENDING with the real order ID
	// so future retries of this key resolve to it instead of re-running checkout.
	h.Redis.Set(ctx, redisKey, orderID, idempotencyTTL)

	// Move CREATED -> PAYMENT_PENDING before handing off to the saga, so the
	// order worker never has to guess whether inventory reservation finished.
	if err := orderstate.Transition(ctx, h.DB, orderID, orderstate.PaymentPending); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance order status")
		return
	}

	event := events.OrderCreated{
		OrderID:    orderID,
		UserID:     userID,
		TotalPaise: totalPaise,
		Items:      items,
		CreatedAt:  time.Now(),
		Source:     events.SourceCart,
	}
	if err := h.Producer.Publish(ctx, events.TopicOrderCreated, orderID, event); err != nil {
		// The order and its reserved inventory already exist and are durable
		// in Postgres — a publish failure here must not silently strand the
		// order in PAYMENT_PENDING forever. In production this would retry
		// with backoff or fall back to an outbox table; logging + surfacing
		// the error is the honest thing to do for this simulator.
		writeError(w, http.StatusInternalServerError, "order created but failed to enqueue for payment processing")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"order_id":       orderID,
		"total_paise":    totalPaise,
		"discount_paise": discountPaise,
		"status":         string(orderstate.PaymentPending),
	})
}

var (
	errEmptyCart         = errors.New("cart is empty")
	errInsufficientStock = errors.New("insufficient stock for one or more items")
	errInvalidCoupon     = errors.New("invalid coupon")
)

// createOrder holds the actual locking/transaction logic, kept separate
// from HTTP concerns so the idempotency wrapper above stays readable.
// Returns the order ID, total, and line items (the latter needed to build
// the order.created event for the saga).
func (h *OrderHandler) createOrder(ctx context.Context, userID, paymentMethod, couponCode, paymentInstrument, paymentIssuer string) (string, int64, []events.OrderItemPayload, int64, error) {
	cartItems, err := h.Redis.HGetAll(ctx, cartKey(userID)).Result()
	if err != nil {
		return "", 0, nil, 0, err
	}
	if len(cartItems) == 0 {
		return "", 0, nil, 0, errEmptyCart
	}

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return "", 0, nil, 0, err
	}
	defer tx.Rollback(ctx) // no-op if committed

	var totalPaise int64
	type orderLine struct {
		productID string
		name      string
		category  string
		quantity  int
		unitPrice int64
		taxRate   int
	}
	var orderLines []orderLine

	for productID, qtyStr := range cartItems {
		quantity, _ := strconv.Atoi(qtyStr)
		if quantity <= 0 {
			continue
		}

		var stock int
		var price int64
		var productName string
		var category string
		var taxRate int
		// FOR UPDATE locks this product row until commit/rollback.
		err := tx.QueryRow(ctx,
			`SELECT stock, price_paise, name, category, tax_rate_bps FROM products WHERE id = $1 FOR UPDATE`, productID,
		).Scan(&stock, &price, &productName, &category, &taxRate)
		if err != nil {
			return "", 0, nil, 0, errors.New("product not found: " + productID)
		}

		if stock < quantity {
			return "", 0, nil, 0, errInsufficientStock
		}

		if _, err := tx.Exec(ctx, `UPDATE products SET stock = stock - $1 WHERE id = $2`, quantity, productID); err != nil {
			return "", 0, nil, 0, err
		}

		orderLines = append(orderLines, orderLine{productID: productID, name: productName, category: category, quantity: quantity, unitPrice: price, taxRate: taxRate})
		totalPaise += price * int64(quantity)
	}

	var discountPaise int64
	var couponID string
	couponCode = strings.ToUpper(strings.TrimSpace(couponCode))
	if couponCode != "" {
		var rule couponrules.Rule
		var expiresAt *time.Time
		err = tx.QueryRow(ctx, `
			SELECT id::text, code, description, offer_type, discount_type, discount_value,
			       min_order_paise, COALESCE(max_discount_paise,0), max_uses, used_count,
			       eligible_category, required_payment_method, required_issuer, new_user_only, active, expires_at
			FROM coupons WHERE code=$1 FOR UPDATE`, couponCode).Scan(&rule.ID, &rule.Code,
			&rule.Description, &rule.OfferType, &rule.DiscountType, &rule.DiscountValue,
			&rule.MinOrderPaise, &rule.MaxDiscountPaise, &rule.MaxUses, &rule.UsedCount,
			&rule.EligibleCategory, &rule.RequiredPaymentMethod, &rule.RequiredIssuer,
			&rule.NewUserOnly, &rule.Active, &expiresAt)
		if err != nil || (expiresAt != nil && expiresAt.Before(time.Now())) {
			return "", 0, nil, 0, fmt.Errorf("%w: invalid or expired", errInvalidCoupon)
		}
		var isNewUser, alreadyUsed bool
		if err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM orders WHERE user_id=$1 AND status IN ('PAID','CONFIRMED'))`, userID).Scan(&isNewUser); err != nil {
			return "", 0, nil, 0, err
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coupon_redemptions WHERE coupon_id=$1 AND user_id=$2 AND status IN ('RESERVED','APPLIED'))`, rule.ID, userID).Scan(&alreadyUsed); err != nil {
			return "", 0, nil, 0, err
		}
		if alreadyUsed {
			return "", 0, nil, 0, fmt.Errorf("%w: already used by this customer", errInvalidCoupon)
		}
		couponLines := make([]couponrules.Line, 0, len(orderLines))
		for _, line := range orderLines {
			couponLines = append(couponLines, couponrules.Line{Category: line.category, UnitPricePaise: line.unitPrice, Quantity: line.quantity})
		}
		result := couponrules.Evaluate(rule, couponLines, totalPaise, isNewUser, paymentInstrument, paymentIssuer)
		if !result.Eligible {
			return "", 0, nil, 0, fmt.Errorf("%w: %s", errInvalidCoupon, result.Reason)
		}
		discountPaise = result.DiscountPaise
		couponID = rule.ID
		if _, err := tx.Exec(ctx, `UPDATE coupons SET used_count=used_count+1 WHERE id=$1`, couponID); err != nil {
			return "", 0, nil, 0, err
		}
		totalPaise -= discountPaise
	}

	var orderID string
	err = tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, total_paise, status, payment_method, coupon_code, discount_paise, payment_instrument, payment_issuer) VALUES ($1, $2, 'CREATED', $3, $4, $5, $6, $7) RETURNING id`,
		userID, totalPaise, paymentMethod, couponCode, discountPaise, paymentInstrument, paymentIssuer,
	).Scan(&orderID)
	if err != nil {
		return "", 0, nil, 0, err
	}
	if couponID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO coupon_redemptions (coupon_id,order_id,user_id,discount_paise) VALUES ($1,$2,$3,$4)`, couponID, orderID, userID, discountPaise); err != nil {
			return "", 0, nil, 0, err
		}
	}

	eventItems := make([]events.OrderItemPayload, 0, len(orderLines))
	for _, ol := range orderLines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, unit_price_paise, product_name_snapshot, tax_rate_bps) VALUES ($1, $2, $3, $4, $5, $6)`,
			orderID, ol.productID, ol.quantity, ol.unitPrice, ol.name, ol.taxRate,
		); err != nil {
			return "", 0, nil, 0, err
		}
		eventItems = append(eventItems, events.OrderItemPayload{ProductID: ol.productID, Quantity: ol.quantity})
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, nil, 0, err
	}

	// Cart is cleared only after the order transaction commits successfully,
	// so a mid-checkout failure leaves the cart intact for the user to retry.
	h.Redis.Del(ctx, cartKey(userID))

	return orderID, totalPaise, eventItems, discountPaise, nil
}

func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	ctx := context.Background()

	var userID, status, paymentMethod string
	var flashSaleID *string
	var totalPaise int64
	err := h.DB.QueryRow(ctx,
		`SELECT user_id, total_paise, status, payment_method, flash_sale_id::text FROM orders WHERE id = $1`, orderID,
	).Scan(&userID, &totalPaise, &status, &paymentMethod, &flashSaleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	requesterID, _ := appmiddleware.UserIDFromContext(r.Context())
	if requesterID != userID {
		// Don't leak order existence/details to a different authenticated user.
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	rows, err := h.DB.Query(ctx,
		`SELECT product_id, quantity, unit_price_paise FROM order_items WHERE order_id = $1`, orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch order items")
		return
	}
	defer rows.Close()

	type item struct {
		ProductID string `json:"product_id"`
		Quantity  int    `json:"quantity"`
		UnitPrice int64  `json:"unit_price_paise"`
	}
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ProductID, &it.Quantity, &it.UnitPrice); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read order item")
			return
		}
		items = append(items, it)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":       orderID,
		"user_id":        userID,
		"status":         status,
		"total_paise":    totalPaise,
		"payment_method": paymentMethod,
		"flash_sale_id":  flashSaleID,
		"items":          items,
	})
}

func (h *OrderHandler) GetTracking(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	ctx := context.Background()

	var userID, status string
	if err := h.DB.QueryRow(ctx, `SELECT user_id, status FROM orders WHERE id = $1`, orderID).Scan(&userID, &status); err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	requesterID, _ := appmiddleware.UserIDFromContext(r.Context())
	if requesterID != userID {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	trackingSteps := []map[string]any{
		{"key": "CREATED", "label": "Processing", "done": true},
		{"key": "PAYMENT_PENDING", "label": "Payment pending", "done": false},
		{"key": "PAID", "label": "Packed", "done": false},
		{"key": "CONFIRMED", "label": "Out for delivery", "done": false},
		{"key": "DELIVERED", "label": "Delivered", "done": false},
	}

	currentStatus := strings.ToUpper(strings.TrimSpace(status))
	currentIndex := 0
	switch currentStatus {
	case "CREATED":
		currentIndex = 0
	case "PAYMENT_PENDING":
		currentIndex = 1
	case "PAID":
		currentIndex = 2
	case "CONFIRMED":
		currentIndex = 3
	case "ORDER_CANCELLED":
		currentIndex = -1
		trackingSteps = []map[string]any{{"key": "CANCELLED", "label": "Cancelled", "done": true, "current": true}}
	case "PAYMENT_FAILED":
		currentIndex = -1
		trackingSteps = []map[string]any{{"key": "PAYMENT_FAILED", "label": "Payment failed", "done": true, "current": true}}
	default:
		currentIndex = 0
	}

	if currentIndex >= 0 {
		for i := range trackingSteps {
			trackingSteps[i]["done"] = i < currentIndex
			trackingSteps[i]["current"] = i == currentIndex
		}
		trackingSteps[currentIndex]["done"] = true
		trackingSteps[currentIndex]["current"] = true
	}

	trackingNumber := "FLC" + strings.ToUpper(strings.ReplaceAll(orderID[:8], "-", ""))
	courierName := "BlueDart"
	if strings.HasPrefix(orderID, "flash-") {
		courierName = "Delhivery"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":        orderID,
		"status":          status,
		"courier_name":    courierName,
		"tracking_number": trackingNumber,
		"timeline":        trackingSteps,
	})
}

func (h *OrderHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text,total_paise,status,payment_method,COALESCE(coupon_code,''),discount_paise,created_at::text
		FROM orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		writeError(w, 500, "failed to fetch orders")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, status, method, coupon, created string
		var total, discount int64
		if err := rows.Scan(&id, &total, &status, &method, &coupon, &discount, &created); err != nil {
			writeError(w, 500, "failed to read orders")
			return
		}
		out = append(out, map[string]any{"id": id, "total_paise": total, "status": status, "payment_method": method, "coupon_code": coupon, "discount_paise": discount, "created_at": created})
	}
	writeJSON(w, 200, out)
}
