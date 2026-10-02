package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type CartHandler struct {
	Redis *redis.Client
}

type cartItemRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// Phase 2 moves the cart from Postgres to Redis:
//
//	User -> Cart API -> Redis
//
// Carts are read/written far more often than orders are placed, and don't
// need Postgres's durability guarantees — losing an in-progress cart on a
// Redis restart is an acceptable trade for the latency win. Each user's
// cart is a single Redis hash (cart:{userID}), field = product_id,
// value = quantity, so add/update/remove/view are all O(1) hash ops.
//
// The user ID is now taken from the authenticated JWT (via RequireAuth),
// not a URL param — a user can only ever read or modify their own cart.

func cartKey(userID string) string {
	return "cart:" + userID
}

func (h *CartHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())

	raw, err := h.Redis.HGetAll(context.Background(), cartKey(userID)).Result()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch cart")
		return
	}

	items := []cartItemRequest{}
	for productID, qtyStr := range raw {
		qty, _ := strconv.Atoi(qtyStr)
		items = append(items, cartItemRequest{ProductID: productID, Quantity: qty})
	}

	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "items": items})
}

func (h *CartHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())

	var req cartItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Quantity <= 0 || req.ProductID == "" {
		writeError(w, http.StatusBadRequest, "product_id required; quantity must be positive")
		return
	}

	// HIncrBy is atomic: two concurrent "add 1" requests for the same
	// product both land correctly instead of racing on a read-modify-write.
	if _, err := h.Redis.HIncrBy(context.Background(), cartKey(userID), req.ProductID, int64(req.Quantity)).Result(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add item to cart")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (h *CartHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "productID")

	var req struct {
		Quantity int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := context.Background()
	if req.Quantity <= 0 {
		if err := h.Redis.HDel(ctx, cartKey(userID), productID).Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove item")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
		return
	}

	if err := h.Redis.HSet(ctx, cartKey(userID), productID, req.Quantity).Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update item")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *CartHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "productID")

	if err := h.Redis.HDel(context.Background(), cartKey(userID), productID).Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove item")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
