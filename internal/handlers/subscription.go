package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type SubscriptionHandler struct {
	DB *pgxpool.Pool
}

type subscribeRequest struct {
	Email     string `json:"email"`      // used when the visitor isn't logged in
	ProductID string `json:"product_id"` // empty = notify for any flash sale
}

// Subscribe lets a customer opt in to flash-sale alerts. Logged-in users
// are tied to their account (so we always have a channel to notify them
// on); a logged-out visitor can subscribe with just an email. Either way,
// leaving product_id empty means "tell me about any flash sale", while
// setting it means "only this product."
func (h *SubscriptionHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)

	userID, isAuthed := appmiddleware.UserIDFromContext(r.Context())
	if !isAuthed && req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required when not signed in")
		return
	}

	var productID any
	if req.ProductID != "" {
		productID = req.ProductID
	}
	var userIDArg, emailArg any
	if isAuthed {
		userIDArg = userID
	} else {
		emailArg = req.Email
	}

	if _, err := h.DB.Exec(context.Background(),
		`INSERT INTO flashsale_subscriptions (user_id, email, product_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		userIDArg, emailArg, productID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to subscribe")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}
