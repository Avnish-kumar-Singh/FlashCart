package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type AnnouncementHandler struct {
	DB *pgxpool.Pool
}

type announcementRequest struct {
	Message string `json:"message"`
	Link    string `json:"link"`
}

type announcement struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Link      string `json:"link"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

// Active returns the single most recent active announcement, if any — the
// "Boom Link" banner the storefront shows with a bell icon. Public/no auth:
// this is meant for every visitor, logged in or not.
func (h *AnnouncementHandler) Active(w http.ResponseWriter, r *http.Request) {
	var a announcement
	err := h.DB.QueryRow(context.Background(),
		`SELECT id, message, link, active, created_at::text FROM announcements WHERE active = true ORDER BY created_at DESC LIMIT 1`,
	).Scan(&a.ID, &a.Message, &a.Link, &a.Active, &a.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusOK, nil) // no active announcement is a normal state, not an error
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// List returns every announcement (active or not) for the admin panel's
// history view.
func (h *AnnouncementHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(context.Background(),
		`SELECT id, message, link, active, created_at::text FROM announcements ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load announcements")
		return
	}
	defer rows.Close()

	out := []announcement{}
	for rows.Next() {
		var a announcement
		if err := rows.Scan(&a.ID, &a.Message, &a.Link, &a.Active, &a.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read announcement")
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

// Create adds a new announcement. Only one announcement is shown at a
// time (Active picks the newest), so creating a new one effectively
// replaces the banner without needing a separate "deactivate old one" step
// — but old ones stay in the table, active=true, for history/reuse. Admins
// wanting a clean single-banner state should deactivate the old one first.
func (h *AnnouncementHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req announcementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var id string
	if err := h.DB.QueryRow(context.Background(),
		`INSERT INTO announcements (message, link, created_by) VALUES ($1, $2, $3) RETURNING id`,
		req.Message, req.Link, userID,
	).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create announcement")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Deactivate turns off a specific announcement's banner without deleting
// its history.
func (h *AnnouncementHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := h.DB.Exec(context.Background(), `UPDATE announcements SET active = false WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to deactivate announcement")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "announcement not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}
