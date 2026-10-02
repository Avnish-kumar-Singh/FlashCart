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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/flashcart/internal/events"
	"github.com/yourname/flashcart/internal/inventory"
	"github.com/yourname/flashcart/internal/kafka"
	"github.com/yourname/flashcart/internal/metrics"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"github.com/yourname/flashcart/internal/notify"
)

type FlashSaleHandler struct {
	DB       *pgxpool.Pool
	Redis    *redis.Client
	Producer *kafka.Producer
	Notifier notify.Sender
}

type activateFlashSaleRequest struct {
	ProductID       string    `json:"product_id"`
	DiscountPercent int       `json:"discount_percent"`
	Stock           int       `json:"stock"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
}

type buyFlashSaleRequest struct {
	Quantity      int    `json:"quantity"`
	PaymentMethod string `json:"payment_method"`
}

var validPaymentMethods = map[string]bool{"COD": true, "CARD": true, "QR": true, "UPI": true, "NETBANKING": true, "WALLET": true, "RAZORPAY": true}

// Activate starts a new flash sale. Multiple sales — for different
// products, or even the same product run again later — can be active at
// once: each gets its own saleID and is added to the active-sales set
// (internal/inventory.AddActiveSale) rather than overwriting a single
// "current sale" pointer, which was the root cause of the storefront only
// ever showing one product on sale.
func (h *FlashSaleHandler) Activate(w http.ResponseWriter, r *http.Request) {
	var req activateFlashSaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProductID == "" || req.DiscountPercent <= 0 || req.DiscountPercent >= 100 || req.Stock <= 0 || req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.EndsAt.After(req.StartsAt) {
		writeError(w, http.StatusBadRequest, "product_id, discount_percent, positive stock, and valid start/end times are required")
		return
	}

	ctx := r.Context()
	var basePrice int64
	var productStock int
	var productName, productImage string
	if err := h.DB.QueryRow(ctx, `SELECT price_paise, stock, name, image_url FROM products WHERE id = $1`, req.ProductID).
		Scan(&basePrice, &productStock, &productName, &productImage); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load product")
		return
	}
	if req.Stock > productStock {
		writeError(w, http.StatusConflict, "flash-sale stock cannot exceed product stock")
		return
	}

	// Move the sale allocation out of normal product stock at activation time.
	// Successful flash-sale orders consume this allocation; cancelled reservations
	// return to the flash-sale pool, while non-sale inventory remains untouched.
	if tag, err := h.DB.Exec(ctx, `UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1`, req.Stock, req.ProductID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reserve flash-sale inventory")
		return
	} else if tag.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "flash-sale inventory changed; try again")
		return
	}

	saleID := uuid.NewString()
	salePrice := basePrice * int64(100-req.DiscountPercent) / 100
	pipe := h.Redis.TxPipeline()
	pipe.Set(ctx, inventory.Key(saleID, "product_id"), req.ProductID, 0)
	pipe.Set(ctx, inventory.Key(saleID, "stock"), req.Stock, 0)
	pipe.Set(ctx, inventory.Key(saleID, "price"), salePrice, 0)
	pipe.Set(ctx, inventory.Key(saleID, "discount_percent"), req.DiscountPercent, 0)
	pipe.Set(ctx, inventory.Key(saleID, "start_ms"), req.StartsAt.UnixMilli(), 0)
	pipe.Set(ctx, inventory.Key(saleID, "end_ms"), req.EndsAt.UnixMilli(), 0)
	pipe.SAdd(ctx, inventory.ActiveSalesSetKey, saleID)
	if _, err := pipe.Exec(ctx); err != nil {
		// Redis activation failed after the DB allocation; compensate the DB
		// allocation so stock is not permanently lost.
		_, _ = h.DB.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id = $2`, req.Stock, req.ProductID)
		writeError(w, http.StatusInternalServerError, "failed to activate flash sale")
		return
	}

	go h.notifySubscribers(context.Background(), req.ProductID, productName, salePrice)

	writeJSON(w, http.StatusCreated, map[string]any{
		"sale_id": saleID, "product_id": req.ProductID, "discount_percent": req.DiscountPercent,
		"sale_price_paise": salePrice, "stock": req.Stock, "starts_at": req.StartsAt, "ends_at": req.EndsAt,
		"product_name": productName, "product_image": productImage,
	})
}

// Deactivate ends a sale early: whatever stock is still unsold in Redis is
// returned to the product's normal Postgres stock, the sale is removed
// from the active set, and its end time is pulled to now so any client
// still polling its status sees ENDED rather than the sale just vanishing.
func (h *FlashSaleHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	saleID := chi.URLParam(r, "saleID")
	ctx := r.Context()

	productID, err := h.Redis.Get(ctx, inventory.Key(saleID, "product_id")).Result()
	if err != nil {
		writeError(w, http.StatusNotFound, "flash sale not found")
		return
	}
	remaining, _ := h.Redis.Get(ctx, inventory.Key(saleID, "stock")).Int()

	if remaining > 0 {
		if _, err := h.DB.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id = $2`, remaining, productID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to return unsold flash-sale stock")
			return
		}
	}

	pipe := h.Redis.TxPipeline()
	pipe.Set(ctx, inventory.Key(saleID, "stock"), 0, 0)
	pipe.Set(ctx, inventory.Key(saleID, "end_ms"), time.Now().UnixMilli(), 0)
	pipe.SRem(ctx, inventory.ActiveSalesSetKey, saleID)
	if _, err := pipe.Exec(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to deactivate flash sale")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sale_id": saleID, "status": "DEACTIVATED", "stock_returned": remaining})
}

func (h *FlashSaleHandler) Status(w http.ResponseWriter, r *http.Request) {
	saleID := chi.URLParam(r, "saleID")
	body, err := h.saleStatus(r.Context(), saleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "flash sale not found")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// Current keeps backward compatibility for anything still calling the
// single-sale endpoint: it returns whichever active sale ended most
// recently-created among the set. New frontend code should use Active
// instead, which returns all of them.
func (h *FlashSaleHandler) Current(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ids, err := inventory.ActiveSaleIDs(ctx, h.Redis)
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusNotFound, "no active flash sale")
		return
	}
	body, err := h.saleStatus(ctx, ids[0])
	if err != nil {
		writeError(w, http.StatusNotFound, "no active flash sale")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// Active returns every currently-live flash sale — this is the fix the
// storefront's Flash Sale page now calls to render one card per product
// instead of a single hardcoded slot. Sales whose window has ended are
// dropped from the response and lazily swept out of the active set, so
// admins don't need a separate cleanup job.
func (h *FlashSaleHandler) Active(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ids, err := inventory.ActiveSaleIDs(ctx, h.Redis)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list active flash sales")
		return
	}

	sales := []map[string]any{}
	for _, id := range ids {
		body, err := h.saleStatus(ctx, id)
		if err != nil {
			inventory.RemoveActiveSale(ctx, h.Redis, id) // stale/expired entry, sweep it
			continue
		}
		if body["state"] == "ENDED" {
			inventory.RemoveActiveSale(ctx, h.Redis, id)
			continue
		}
		sales = append(sales, body)
	}

	writeJSON(w, http.StatusOK, sales)
}

func (h *FlashSaleHandler) saleStatus(ctx context.Context, saleID string) (map[string]any, error) {
	values, err := h.Redis.MGet(ctx,
		inventory.Key(saleID, "product_id"), inventory.Key(saleID, "stock"),
		inventory.Key(saleID, "price"), inventory.Key(saleID, "start_ms"), inventory.Key(saleID, "end_ms"),
		inventory.Key(saleID, "discount_percent"),
	).Result()
	if err != nil || len(values) != 6 || values[0] == nil {
		return nil, fmt.Errorf("flash sale not found")
	}

	productID := fmt.Sprint(values[0])
	stock, _ := strconv.Atoi(fmt.Sprint(values[1]))
	price, _ := strconv.ParseInt(fmt.Sprint(values[2]), 10, 64)
	start, _ := strconv.ParseInt(fmt.Sprint(values[3]), 10, 64)
	end, _ := strconv.ParseInt(fmt.Sprint(values[4]), 10, 64)
	discountPercent, _ := strconv.Atoi(fmt.Sprint(values[5]))
	now := time.Now().UnixMilli()
	state := "ACTIVE"
	if now < start {
		state = "NOT_STARTED"
	}
	if end > 0 && now >= end {
		state = "ENDED"
	}

	views, _ := h.Redis.Get(ctx, "flashsale:views:"+saleID).Int64()
	clicks, _ := h.Redis.Get(ctx, "flashsale:clicks:"+saleID).Int64()

	return map[string]any{
		"sale_id": saleID, "product_id": productID, "stock_remaining": stock,
		"sale_price_paise": price, "discount_percent": discountPercent,
		"starts_at_ms": start, "ends_at_ms": end, "state": state,
		"views": views, "clicks": clicks,
	}, nil
}

// Track records a lightweight analytics event (a page view of the sale
// card, or a click on "Buy now") for the admin analytics dashboard.
// Intentionally unauthenticated and fire-and-forget: a failed/blocked
// beacon should never break the shopping experience.
func (h *FlashSaleHandler) Track(w http.ResponseWriter, r *http.Request) {
	saleID := chi.URLParam(r, "saleID")
	var req struct {
		Event string `json:"event"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var key string
	switch req.Event {
	case "view":
		key = "flashsale:views:" + saleID
	case "click":
		key = "flashsale:clicks:" + saleID
	default:
		writeError(w, http.StatusBadRequest, "event must be 'view' or 'click'")
		return
	}
	h.Redis.Incr(r.Context(), key)
	w.WriteHeader(http.StatusNoContent)
}

func (h *FlashSaleHandler) Buy(w http.ResponseWriter, r *http.Request) {
	userID, ok := appmiddleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	saleID := chi.URLParam(r, "saleID")
	idem := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idem == "" {
		writeError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}

	var req buyFlashSaleRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Quantity == 0 {
		req.Quantity = 1
	}
	if req.Quantity < 1 || req.Quantity > 5 {
		writeError(w, http.StatusBadRequest, "quantity must be between 1 and 5")
		return
	}
	req.PaymentMethod = normalizePaymentMethod(req.PaymentMethod)
	if req.PaymentMethod == "" {
		req.PaymentMethod = "COD"
	}
	if !validPaymentMethods[req.PaymentMethod] {
		writeError(w, http.StatusBadRequest, "payment_method must be one of COD, CARD, QR, UPI, NETBANKING, WALLET, RAZORPAY")
		return
	}

	ctx := r.Context()
	idemKey := "flash-idem:" + userID + ":" + saleID + ":" + idem
	claimed, err := h.Redis.SetNX(ctx, idemKey, "PENDING", 24*time.Hour).Result()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "idempotency store unavailable")
		return
	}
	if !claimed {
		value, getErr := h.Redis.Get(ctx, idemKey).Result()
		if getErr == nil && value != "PENDING" {
			writeJSON(w, http.StatusOK, map[string]any{"order_id": value, "status": "already_processed"})
			return
		}
		writeError(w, http.StatusConflict, "request with this idempotency key is already in progress")
		return
	}

	productID, err := h.Redis.Get(ctx, inventory.Key(saleID, "product_id")).Result()
	if err != nil {
		h.Redis.Del(ctx, idemKey)
		writeError(w, http.StatusNotFound, "flash sale not found")
		return
	}
	salePrice, err := h.Redis.Get(ctx, inventory.Key(saleID, "price")).Int64()
	if err != nil {
		h.Redis.Del(ctx, idemKey)
		writeError(w, http.StatusNotFound, "flash sale price not found")
		return
	}

	reservationID := uuid.NewString()
	reserved, reason, err := inventory.Reserve(ctx, h.Redis, saleID, userID, reservationID, req.Quantity)
	if err != nil {
		metrics.FlashSaleReservations.WithLabelValues("error").Inc()
		h.Redis.Del(ctx, idemKey)
		writeError(w, http.StatusServiceUnavailable, "flash-sale reservation unavailable")
		return
	}
	if !reserved {
		metrics.FlashSaleReservations.WithLabelValues(strings.ToLower(reason)).Inc()
		h.Redis.Del(ctx, idemKey)
		status := http.StatusConflict
		if reason == "NOT_STARTED" {
			status = http.StatusTooEarly
		}
		if reason == "ENDED" {
			status = http.StatusGone
		}
		writeError(w, status, reason)
		return
	}

	metrics.FlashSaleReservations.WithLabelValues("reserved").Inc()
	orderID, err := h.persistFlashSaleOrder(ctx, userID, productID, saleID, req.PaymentMethod, req.Quantity, salePrice)
	if err != nil {
		_ = inventory.Release(ctx, h.Redis, saleID, userID, reservationID)
		h.Redis.Del(ctx, idemKey)
		writeError(w, http.StatusServiceUnavailable, "failed to create flash-sale order")
		return
	}

	event := events.OrderCreated{
		OrderID: orderID, UserID: userID, TotalPaise: salePrice * int64(req.Quantity),
		Items:     []events.OrderItemPayload{{ProductID: productID, Quantity: req.Quantity}},
		CreatedAt: time.Now(), Source: events.SourceFlashSale, FlashSaleID: saleID, ReservationID: reservationID,
	}
	if err := h.Producer.Publish(ctx, events.TopicOrderCreated, orderID, event); err != nil {
		// Keep the reservation until its TTL expires rather than returning stock
		// immediately: the durable order exists and can be recovered/requeued.
		writeError(w, http.StatusServiceUnavailable, "order created but failed to enqueue payment processing")
		return
	}

	_ = h.Redis.Set(ctx, idemKey, orderID, 24*time.Hour).Err()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"order_id": orderID, "sale_id": saleID, "status": "PAYMENT_PENDING",
		"reservation_id": reservationID, "total_paise": salePrice * int64(req.Quantity),
		"payment_method": req.PaymentMethod,
	})
}

func (h *FlashSaleHandler) persistFlashSaleOrder(ctx context.Context, userID, productID, saleID, paymentMethod string, quantity int, unitPrice int64) (string, error) {
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var orderID string
	total := unitPrice * int64(quantity)
	var productName string
	var taxRate int
	if err := tx.QueryRow(ctx, `SELECT name, tax_rate_bps FROM products WHERE id=$1`, productID).Scan(&productName, &taxRate); err != nil {
		return "", err
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO orders (user_id,total_paise,status,source,flash_sale_id,payment_method) VALUES ($1,$2,'PAYMENT_PENDING','FLASH_SALE',$3,$4) RETURNING id`,
		userID, total, saleID, paymentMethod,
	).Scan(&orderID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO order_items (order_id,product_id,quantity,unit_price_paise,product_name_snapshot,tax_rate_bps) VALUES ($1,$2,$3,$4,$5,$6)`, orderID, productID, quantity, unitPrice, productName, taxRate); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return orderID, nil
}

// notifySubscribers simulates the "customer notification system": anyone
// subscribed to this product specifically, or to flash sales in general,
// gets a simulated Email/WhatsApp alert the moment the sale goes live.
// Runs in its own goroutine off the request path so a slow/large
// subscriber list never delays the admin's activation response.
func (h *FlashSaleHandler) notifySubscribers(ctx context.Context, productID, productName string, salePrice int64) {
	rows, err := h.DB.Query(ctx, `
		SELECT COALESCE(u.email, s.email) AS email, COALESCE(u.phone, '') AS phone
		FROM flashsale_subscriptions s
		LEFT JOIN users u ON u.id = s.user_id
		WHERE s.product_id = $1 OR s.product_id IS NULL`, productID)
	if err != nil {
		return
	}
	defer rows.Close()

	message := fmt.Sprintf("Flash sale is live: %s now at Rs.%.2f! Grab it before stock runs out.", productName, float64(salePrice)/100)
	count := 0
	for rows.Next() {
		var email, phone string
		if err := rows.Scan(&email, &phone); err != nil {
			continue
		}
		_ = h.Notifier.Send(ctx, notify.ChannelBoth, notify.Recipient{Email: email, Phone: phone}, message)
		count++
	}
	if count > 0 {
		metrics.NotificationsSent.WithLabelValues("flashsale_activated").Add(float64(count))
	}
}
