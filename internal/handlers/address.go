package handlers

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
	"net/http"
)

type AddressHandler struct{ DB *pgxpool.Pool }

type addressRequest struct {
	Label     string `json:"label"`
	FullName  string `json:"full_name"`
	Phone     string `json:"phone"`
	Line1     string `json:"line1"`
	Line2     string `json:"line2"`
	City      string `json:"city"`
	State     string `json:"state"`
	Pincode   string `json:"pincode"`
	IsDefault bool   `json:"is_default"`
}

func (h *AddressHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	rows, err := h.DB.Query(r.Context(), `SELECT id::text,label,full_name,phone,line1,line2,city,state,pincode,is_default,created_at::text FROM user_addresses WHERE user_id=$1 ORDER BY is_default DESC, created_at DESC`, userID)
	if err != nil {
		writeError(w, 500, "failed to fetch addresses")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, label, fullName, phone, line1, line2, city, state, pincode, created string
		var isDefault bool
		if err := rows.Scan(&id, &label, &fullName, &phone, &line1, &line2, &city, &state, &pincode, &isDefault, &created); err != nil {
			writeError(w, 500, "failed to read address")
			return
		}
		out = append(out, map[string]any{"id": id, "label": label, "full_name": fullName, "phone": phone, "line1": line1, "line2": line2, "city": city, "state": state, "pincode": pincode, "is_default": isDefault, "created_at": created})
	}
	writeJSON(w, 200, out)
}

func (h *AddressHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req addressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FullName == "" || req.Phone == "" || req.Line1 == "" || req.City == "" || req.Pincode == "" {
		writeError(w, 400, "name, phone, address, city and pincode are required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to save address")
		return
	}
	defer tx.Rollback(r.Context())
	if req.IsDefault {
		_, _ = tx.Exec(r.Context(), `UPDATE user_addresses SET is_default=false WHERE user_id=$1`, userID)
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO user_addresses(user_id,label,full_name,phone,line1,line2,city,state,pincode,is_default) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`, userID, defaultLabel(req.Label), req.FullName, req.Phone, req.Line1, req.Line2, req.City, req.State, req.Pincode, req.IsDefault).Scan(&id)
	if err != nil {
		writeError(w, 500, "failed to save address")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to save address")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "status": "created"})
}

func (h *AddressHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")
	tag, err := h.DB.Exec(r.Context(), `DELETE FROM user_addresses WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		writeError(w, 500, "failed to delete address")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "address not found")
		return
	}
	w.WriteHeader(204)
}
func defaultLabel(v string) string {
	if v == "" {
		return "Home"
	}
	return v
}
