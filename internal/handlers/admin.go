package handlers

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminHandler struct{ DB *pgxpool.Pool }

func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var products, users, orders, revenue int64
	if err := h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM products`).Scan(&products); err != nil {
		writeError(w, 500, "failed to load stats")
		return
	}
	if err := h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		writeError(w, 500, "failed to load stats")
		return
	}
	if err := h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&orders); err != nil {
		writeError(w, 500, "failed to load stats")
		return
	}
	if err := h.DB.QueryRow(ctx, `SELECT COALESCE(SUM(total_paise),0) FROM orders WHERE status='CONFIRMED'`).Scan(&revenue); err != nil {
		writeError(w, 500, "failed to load stats")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products, "users": users, "orders": orders, "revenue_paise": revenue})
}

type adminOrder struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	Email         string `json:"email"`
	TotalPaise    int64  `json:"total_paise"`
	Status        string `json:"status"`
	Source        string `json:"source"`
	PaymentMethod string `json:"payment_method"`
	CreatedAt     string `json:"created_at"`
}

func (h *AdminHandler) Orders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(context.Background(), `SELECT o.id::text, o.user_id::text, u.email, o.total_paise, o.status, o.source, o.payment_method, o.created_at::text FROM orders o JOIN users u ON u.id=o.user_id ORDER BY o.created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, "failed to fetch orders")
		return
	}
	defer rows.Close()
	out := []adminOrder{}
	for rows.Next() {
		var item adminOrder
		if err := rows.Scan(&item.ID, &item.UserID, &item.Email, &item.TotalPaise, &item.Status, &item.Source, &item.PaymentMethod, &item.CreatedAt); err != nil {
			writeError(w, 500, "failed to read orders")
			return
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "failed to read orders")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AdminHandler) Payments(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT p.id::text, p.order_id::text, u.email, p.razorpay_order_id,
		       COALESCE(p.razorpay_payment_id,''), p.amount_paise, p.method,
		       p.status, p.created_at::text
		FROM payments p
		JOIN orders o ON o.id=p.order_id
		JOIN users u ON u.id=o.user_id
		ORDER BY p.created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, "failed to fetch payments")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, orderID, email, rpOrder, rpPayment, method, status, created string
		var amount int64
		if err := rows.Scan(&id, &orderID, &email, &rpOrder, &rpPayment, &amount, &method, &status, &created); err != nil {
			writeError(w, 500, "failed to read payments")
			return
		}
		out = append(out, map[string]any{"id": id, "order_id": orderID, "email": email, "razorpay_order_id": rpOrder, "razorpay_payment_id": rpPayment, "amount_paise": amount, "method": method, "status": status, "created_at": created})
	}
	writeJSON(w, 200, out)
}
