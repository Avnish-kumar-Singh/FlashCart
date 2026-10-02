package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/flashcart/internal/coupons"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type CouponHandler struct {
	DB    *pgxpool.Pool
	Redis *redis.Client
}

type couponOfferResponse struct {
	Code             string `json:"code"`
	Description      string `json:"description"`
	OfferType        string `json:"offer_type"`
	DiscountType     string `json:"discount_type"`
	DiscountValue    int64  `json:"discount_value"`
	MinOrderPaise    int64  `json:"min_order_paise"`
	MaxDiscountPaise int64  `json:"max_discount_paise"`
	EligibleCategory string `json:"eligible_category"`
	PaymentMethod    string `json:"required_payment_method"`
	Issuer           string `json:"required_issuer"`
	NewUserOnly      bool   `json:"new_user_only"`
	Eligible         bool   `json:"eligible"`
	Reason           string `json:"reason,omitempty"`
	DiscountPaise    int64  `json:"discount_paise"`
	EligiblePaise    int64  `json:"eligible_paise"`
}

type couponCart struct {
	Lines    []coupons.Line
	Subtotal int64
}

func (h *CouponHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	cart, err := h.loadCart(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to calculate cart offers")
		return
	}
	offers, err := h.evaluateOffers(r.Context(), userID, cart, r.URL.Query().Get("payment_method"), r.URL.Query().Get("issuer"), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load coupon offers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subtotal_paise":       cart.Subtotal,
		"minimum_unlock_paise": int64(50000),
		"offers":               offers,
	})
}

func (h *CouponHandler) Validate(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req struct {
		Code          string `json:"code"`
		PaymentMethod string `json:"payment_method"`
		Issuer        string `json:"issuer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "coupon code is required")
		return
	}
	cart, err := h.loadCart(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to calculate cart offers")
		return
	}
	offers, err := h.evaluateOffers(r.Context(), userID, cart, req.PaymentMethod, req.Issuer, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate coupon")
		return
	}
	if len(offers) == 0 {
		writeError(w, http.StatusNotFound, "coupon code not found or inactive")
		return
	}
	if !offers[0].Eligible {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":          offers[0].Reason,
			"offer":          offers[0],
			"subtotal_paise": cart.Subtotal,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"offer":          offers[0],
		"subtotal_paise": cart.Subtotal,
		"discount_paise": offers[0].DiscountPaise,
		"total_paise":    cart.Subtotal - offers[0].DiscountPaise,
	})
}

func (h *CouponHandler) Analytics(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT c.code, c.description, c.offer_type, c.used_count,
		       count(r.id) FILTER (WHERE r.status='APPLIED'),
	       COALESCE(sum(r.discount_paise) FILTER (WHERE r.status='APPLIED'),0),
	       count(r.id) FILTER (WHERE r.status='RESERVED'),
	       count(r.id) FILTER (WHERE r.status='RELEASED')
		FROM coupons c LEFT JOIN coupon_redemptions r ON r.coupon_id=c.id
		GROUP BY c.id ORDER BY c.code`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load coupon analytics")
		return
	}
	defer rows.Close()
	type couponStats struct {
		Code          string `json:"code"`
		Description   string `json:"description"`
		OfferType     string `json:"offer_type"`
		UsedCount     int    `json:"used_count"`
		AppliedCount  int64  `json:"applied_count"`
		DiscountPaise int64  `json:"discount_paise"`
		ReservedCount int64  `json:"reserved_count"`
		ReleasedCount int64  `json:"released_count"`
	}
	result := []couponStats{}
	for rows.Next() {
		var item couponStats
		if err := rows.Scan(&item.Code, &item.Description, &item.OfferType, &item.UsedCount,
			&item.AppliedCount, &item.DiscountPaise, &item.ReservedCount, &item.ReleasedCount); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read coupon analytics")
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read coupon analytics")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CouponHandler) loadCart(ctx context.Context, userID string) (couponCart, error) {
	raw, err := h.Redis.HGetAll(ctx, cartKey(userID)).Result()
	if err != nil {
		return couponCart{}, err
	}
	cart := couponCart{Lines: []coupons.Line{}}
	for productID, quantityText := range raw {
		quantity, err := strconv.Atoi(quantityText)
		if err != nil || quantity <= 0 {
			continue
		}
		var line coupons.Line
		if err := h.DB.QueryRow(ctx, `SELECT category, price_paise FROM products WHERE id=$1`, productID).Scan(&line.Category, &line.UnitPricePaise); err != nil {
			return couponCart{}, err
		}
		line.Quantity = quantity
		cart.Subtotal += line.UnitPricePaise * int64(quantity)
		cart.Lines = append(cart.Lines, line)
	}
	return cart, nil
}

func (h *CouponHandler) evaluateOffers(ctx context.Context, userID string, cart couponCart, paymentMethod, issuer, onlyCode string) ([]couponOfferResponse, error) {
	var isNewUser bool
	if err := h.DB.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM orders WHERE user_id=$1 AND status IN ('PAID','CONFIRMED'))`, userID).Scan(&isNewUser); err != nil {
		return nil, err
	}
	query := `
		SELECT id::text, code, description, offer_type, discount_type, discount_value,
		       min_order_paise, COALESCE(max_discount_paise,0), max_uses, used_count,
		       eligible_category, required_payment_method, required_issuer, new_user_only, active
		FROM coupons
		WHERE active AND (expires_at IS NULL OR expires_at > now())
		  AND ($1='' OR code=$1)
		ORDER BY min_order_paise, code`
	rows, err := h.DB.Query(ctx, query, onlyCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	offers := []couponOfferResponse{}
	for rows.Next() {
		var rule coupons.Rule
		if err := rows.Scan(&rule.ID, &rule.Code, &rule.Description, &rule.OfferType,
			&rule.DiscountType, &rule.DiscountValue, &rule.MinOrderPaise, &rule.MaxDiscountPaise,
			&rule.MaxUses, &rule.UsedCount, &rule.EligibleCategory, &rule.RequiredPaymentMethod,
			&rule.RequiredIssuer, &rule.NewUserOnly, &rule.Active); err != nil {
			return nil, err
		}
		var alreadyUsed bool
		if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coupon_redemptions WHERE coupon_id=$1 AND user_id=$2 AND status IN ('RESERVED','APPLIED'))`, rule.ID, userID).Scan(&alreadyUsed); err != nil {
			return nil, err
		}
		result := coupons.Evaluate(rule, cart.Lines, cart.Subtotal, isNewUser, strings.ToUpper(paymentMethod), strings.ToUpper(issuer))
		if alreadyUsed {
			result.Eligible = false
			result.DiscountPaise = 0
			result.Reason = "You have already used this coupon."
		}
		offers = append(offers, couponOfferResponse{
			Code: rule.Code, Description: rule.Description, OfferType: rule.OfferType,
			DiscountType: rule.DiscountType, DiscountValue: rule.DiscountValue,
			MinOrderPaise: rule.MinOrderPaise, MaxDiscountPaise: rule.MaxDiscountPaise,
			EligibleCategory: rule.EligibleCategory, PaymentMethod: rule.RequiredPaymentMethod,
			Issuer: rule.RequiredIssuer, NewUserOnly: rule.NewUserOnly, Eligible: result.Eligible,
			Reason: result.Reason, DiscountPaise: result.DiscountPaise, EligiblePaise: result.EligiblePaise,
		})
	}
	return offers, rows.Err()
}
