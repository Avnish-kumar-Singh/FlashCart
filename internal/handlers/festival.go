package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FestivalHandler struct{ DB *pgxpool.Pool }

func (h *FestivalHandler) Current(w http.ResponseWriter, r *http.Request) {
	var festival struct {
		ID              string          `json:"id"`
		Name            string          `json:"name"`
		LocalName       string          `json:"local_name"`
		HolidayDate     string          `json:"holiday_date"`
		StartsAt        string          `json:"starts_at"`
		EndsAt          string          `json:"ends_at"`
		Status          string          `json:"status"`
		DiscountPercent int             `json:"discount_percent"`
		Global          bool            `json:"global"`
		Regions         json.RawMessage `json:"-"`
	}
	err := h.DB.QueryRow(r.Context(), `
		SELECT id::text,name,local_name,holiday_date::text,starts_at::text,ends_at::text,status,discount_percent,global_holiday, counties
		FROM festival_events
		WHERE status IN ('SCHEDULED','ACTIVATING','ACTIVE') AND ends_at>now()
		ORDER BY CASE WHEN status='ACTIVE' THEN 0 ELSE 1 END,starts_at
		LIMIT 1`).Scan(&festival.ID, &festival.Name, &festival.LocalName, &festival.HolidayDate,
		&festival.StartsAt, &festival.EndsAt, &festival.Status, &festival.DiscountPercent, &festival.Global, &festival.Regions)
	if err != nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": festival.ID, "name": festival.Name, "local_name": festival.LocalName,
		"holiday_date": festival.HolidayDate, "starts_at": festival.StartsAt, "ends_at": festival.EndsAt,
		"status": festival.Status, "discount_percent": festival.DiscountPercent,
		"global": festival.Global, "regions": json.RawMessage(festival.Regions),
	})
}
