package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type WishlistHandler struct {
	DB *pgxpool.Pool
}

type wishlistItem struct {
	ProductID  string `json:"product_id"`
	Name       string `json:"name"`
	PricePaise int64  `json:"price_paise"`
	ImageURL   string `json:"image_url"`
	Stock      int    `json:"stock"`
	AddedAt    string `json:"added_at"`
}

// List returns the signed-in user's saved products, joined with current
// product data so price/stock changes since saving are reflected — this
// doubles as the "reminder" surface: come back here to see the price you
// were waiting on and whether it's still in stock.
func (h *WishlistHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())

	rows, err := h.DB.Query(context.Background(), `
		SELECT p.id, p.name, p.price_paise, p.image_url, p.stock, w.created_at::text
		FROM wishlist_items w JOIN products p ON p.id = w.product_id
		WHERE w.user_id = $1 ORDER BY w.created_at DESC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load wishlist")
		return
	}
	defer rows.Close()

	out := []wishlistItem{}
	for rows.Next() {
		var it wishlistItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.PricePaise, &it.ImageURL, &it.Stock, &it.AddedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read wishlist item")
			return
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *WishlistHandler) Add(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "productID")

	if _, err := h.DB.Exec(context.Background(),
		`INSERT INTO wishlist_items (user_id, product_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, productID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add to wishlist")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (h *WishlistHandler) Remove(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "productID")

	if _, err := h.DB.Exec(context.Background(),
		`DELETE FROM wishlist_items WHERE user_id = $1 AND product_id = $2`, userID, productID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove from wishlist")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
