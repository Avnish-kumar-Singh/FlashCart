package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type SupportHandler struct {
	DB *pgxpool.Pool
}

type supportTicketRequest struct {
	OrderID string `json:"order_id,omitempty"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

func (h *SupportHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	ctx := context.Background()

	var req supportTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "subject and message are required")
		return
	}

	if strings.TrimSpace(req.OrderID) != "" {
		var exists bool
		if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE id = $1::uuid AND user_id = $2)`, req.OrderID, userID).Scan(&exists); err != nil || !exists {
			writeError(w, http.StatusNotFound, "order not found")
			return
		}
	}

	var ticketID string
	if err := h.DB.QueryRow(ctx, `
		INSERT INTO support_tickets (user_id, order_id, subject, message, status)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, 'OPEN')
		RETURNING id::text
	`, userID, strings.TrimSpace(req.OrderID), strings.TrimSpace(req.Subject), strings.TrimSpace(req.Message)).Scan(&ticketID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create support ticket")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket_id": ticketID,
		"status":    "OPEN",
	})
}

func (h *SupportHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	ctx := context.Background()

	rows, err := h.DB.Query(ctx, `
		SELECT id::text, order_id::text, subject, message, status, created_at::text
		FROM support_tickets
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support tickets")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, orderID, subject, message, status, createdAt string
		if err := rows.Scan(&id, &orderID, &subject, &message, &status, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read support tickets")
			return
		}
		items = append(items, map[string]any{
			"ticket_id":  id,
			"order_id":   orderID,
			"subject":    subject,
			"message":    message,
			"status":     status,
			"created_at": createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"tickets": items})
}
