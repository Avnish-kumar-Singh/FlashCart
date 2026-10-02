package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type ReturnHandler struct {
	DB *pgxpool.Pool
}

type returnRequest struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

func (h *ReturnHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	ctx := context.Background()

	var req returnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.OrderID) == "" {
		writeError(w, http.StatusBadRequest, "order_id is required")
		return
	}

	var status string
	var totalPaise int64
	if err := h.DB.QueryRow(ctx, `SELECT status, total_paise FROM orders WHERE id = $1::uuid AND user_id = $2`, req.OrderID, userID).Scan(&status, &totalPaise); err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if status != "CONFIRMED" && status != "PAID" {
		writeError(w, http.StatusConflict, "returns are only available for confirmed orders")
		return
	}

	var returnID string
	if err := h.DB.QueryRow(ctx, `
		INSERT INTO order_returns (order_id, user_id, reason, status, amount_paise)
		VALUES ($1::uuid, $2, $3, 'REQUESTED', $4)
		RETURNING id::text
	`, req.OrderID, userID, strings.TrimSpace(req.Reason), totalPaise).Scan(&returnID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create return request")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"return_id":    returnID,
		"order_id":     req.OrderID,
		"status":       "REQUESTED",
		"amount_paise": totalPaise,
	})
}

func (h *ReturnHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	ctx := context.Background()

	rows, err := h.DB.Query(ctx, `
		SELECT id::text, order_id::text, reason, status, amount_paise, created_at::text
		FROM order_returns
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load returns")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, orderID, reason, status, createdAt string
		var amountPaise int64
		if err := rows.Scan(&id, &orderID, &reason, &status, &amountPaise, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read return records")
			return
		}
		items = append(items, map[string]any{
			"return_id":    id,
			"order_id":     orderID,
			"reason":       reason,
			"status":       status,
			"amount_paise": amountPaise,
			"created_at":   createdAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"returns": items})
}
