package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type AnalyticsHandler struct {
	DB    *pgxpool.Pool
	Redis *redis.Client
}

// FlashSale reports views, clicks, and conversions (confirmed orders +
// revenue) for one sale — the funnel an admin needs to judge whether a
// drop actually worked: views -> clicks -> orders.
func (h *AnalyticsHandler) FlashSale(w http.ResponseWriter, r *http.Request) {
	saleID := chi.URLParam(r, "saleID")
	ctx := context.Background()

	views, _ := h.Redis.Get(ctx, "flashsale:views:"+saleID).Int64()
	clicks, _ := h.Redis.Get(ctx, "flashsale:clicks:"+saleID).Int64()

	var orders int64
	var confirmedOrders int64
	var revenuePaise int64
	h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE flash_sale_id = $1`, saleID).Scan(&orders)
	h.DB.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(total_paise), 0) FROM orders WHERE flash_sale_id = $1 AND status = 'CONFIRMED'`,
		saleID,
	).Scan(&confirmedOrders, &revenuePaise)

	var conversionRate float64
	if views > 0 {
		conversionRate = float64(confirmedOrders) / float64(views) * 100
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sale_id": saleID, "views": views, "clicks": clicks,
		"orders_created": orders, "orders_confirmed": confirmedOrders,
		"revenue_paise": revenuePaise, "conversion_rate_percent": conversionRate,
	})
}

// Overview lists every sale that's ever collected any analytics data
// (active or ended), sorted by revenue, for an at-a-glance leaderboard.
func (h *AnalyticsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	rows, err := h.DB.Query(ctx, `
		SELECT flash_sale_id, COUNT(*) FILTER (WHERE status = 'CONFIRMED'),
		       COALESCE(SUM(total_paise) FILTER (WHERE status = 'CONFIRMED'), 0)
		FROM orders WHERE flash_sale_id IS NOT NULL
		GROUP BY flash_sale_id ORDER BY 3 DESC LIMIT 50`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load analytics overview")
		return
	}
	defer rows.Close()

	type row struct {
		SaleID          string `json:"sale_id"`
		ConfirmedOrders int64  `json:"confirmed_orders"`
		RevenuePaise    int64  `json:"revenue_paise"`
		Views           int64  `json:"views"`
		Clicks          int64  `json:"clicks"`
	}
	out := []row{}
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.SaleID, &rr.ConfirmedOrders, &rr.RevenuePaise); err != nil {
			continue
		}
		rr.Views, _ = h.Redis.Get(ctx, "flashsale:views:"+rr.SaleID).Int64()
		rr.Clicks, _ = h.Redis.Get(ctx, "flashsale:clicks:"+rr.SaleID).Int64()
		out = append(out, rr)
	}
	writeJSON(w, http.StatusOK, out)
}
