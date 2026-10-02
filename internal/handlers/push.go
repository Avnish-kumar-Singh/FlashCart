package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type PushHandler struct {
	DB        *pgxpool.Pool
	PublicKey string
}

type pushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (h *PushHandler) PublicVAPIDKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"public_key": h.PublicKey})
}

func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req pushSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid push subscription")
		return
	}
	parsed, err := url.ParseRequestURI(req.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(req.Endpoint) > 2048 || req.Keys.P256DH == "" || req.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "a valid HTTPS push subscription is required")
		return
	}
	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO web_push_subscriptions (user_id,endpoint,p256dh,auth_secret)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT(endpoint) DO UPDATE SET user_id=EXCLUDED.user_id,p256dh=EXCLUDED.p256dh,
		                          auth_secret=EXCLUDED.auth_secret,updated_at=now()`,
		userID, req.Endpoint, req.Keys.P256DH, req.Keys.Auth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save push subscription")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}

func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Endpoint) == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	_, err := h.DB.Exec(r.Context(), `DELETE FROM web_push_subscriptions WHERE user_id=$1 AND endpoint=$2`, userID, req.Endpoint)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove push subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
