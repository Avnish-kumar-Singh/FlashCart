package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"github.com/yourname/flashcart/internal/notify"
)

type BroadcastHandler struct {
	DB       *pgxpool.Pool
	Notifier notify.Sender
}

type broadcastRequest struct {
	Channel string `json:"channel"` // EMAIL | WHATSAPP | BOTH
	Message string `json:"message"`
}

type broadcastRecord struct {
	ID             string `json:"id"`
	Channel        string `json:"channel"`
	Message        string `json:"message"`
	RecipientCount int    `json:"recipient_count"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

// Create is the "Message Queue Box": an admin writes a pre-festival
// announcement ("Diwali mega sale starts tomorrow 9 AM!"), picks a
// channel, and every registered user gets a simulated Email/WhatsApp
// message. It runs synchronously and returns the final recipient count —
// for a large user base this would move to the Kafka worker pipeline
// (same shape as order processing), but the volumes here don't need that
// yet, and keeping it synchronous means the admin sees the real result
// immediately instead of polling a status endpoint.
func (h *BroadcastHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req broadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.Channel != "EMAIL" && req.Channel != "WHATSAPP" && req.Channel != "BOTH" {
		writeError(w, http.StatusBadRequest, "channel must be EMAIL, WHATSAPP, or BOTH")
		return
	}

	ctx := context.Background()
	userID, _ := appmiddleware.UserIDFromContext(r.Context())

	var id string
	if err := h.DB.QueryRow(ctx,
		`INSERT INTO broadcast_messages (channel, message, created_by) VALUES ($1, $2, $3) RETURNING id`,
		req.Channel, req.Message, userID,
	).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue broadcast")
		return
	}

	rows, err := h.DB.Query(ctx, `SELECT email, phone FROM users`)
	if err != nil {
		h.DB.Exec(ctx, `UPDATE broadcast_messages SET status = 'FAILED' WHERE id = $1`, id)
		writeError(w, http.StatusInternalServerError, "failed to load recipients")
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var email, phone string
		if err := rows.Scan(&email, &phone); err != nil {
			continue
		}
		_ = h.Notifier.Send(ctx, notify.Channel(req.Channel), notify.Recipient{Email: email, Phone: phone}, req.Message)
		count++
	}

	h.DB.Exec(ctx, `UPDATE broadcast_messages SET status = 'SENT', recipient_count = $1 WHERE id = $2`, count, id)

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "recipient_count": count, "status": "SENT"})
}

// List shows recent broadcasts in the admin panel — what was sent, to how
// many people, and when.
func (h *BroadcastHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(context.Background(),
		`SELECT id, channel, message, recipient_count, status, created_at::text FROM broadcast_messages ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load broadcasts")
		return
	}
	defer rows.Close()

	out := []broadcastRecord{}
	for rows.Next() {
		var b broadcastRecord
		if err := rows.Scan(&b.ID, &b.Channel, &b.Message, &b.RecipientCount, &b.Status, &b.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read broadcast")
			return
		}
		out = append(out, b)
	}
	writeJSON(w, http.StatusOK, out)
}
