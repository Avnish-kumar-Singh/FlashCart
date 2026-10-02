package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductHandler struct {
	DB *pgxpool.Pool
}

type productRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
	Category    string `json:"category"`
	PricePaise  int64  `json:"price_paise"`
	TaxRateBPS  int    `json:"tax_rate_bps"`
	Stock       int    `json:"stock"`
	SellerID    string `json:"seller_id"`
	ImageURL    string `json:"image_url"`
}

// List returns products, optionally filtered by category/brand and/or a
// free-text search (`q`) matched against name, brand, and description.
// This is a plain ILIKE query for now; Phase 4's roadmap calls out
// Elasticsearch for real relevance ranking once catalog size warrants it.
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	brand := r.URL.Query().Get("brand")
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	query := `SELECT id, name, description, brand, category, price_paise, tax_rate_bps, stock, seller_id, image_url, created_at::text
	          FROM products
	          WHERE ($1 = '' OR category = $1)
	            AND ($2 = '' OR brand = $2)
	            AND ($3 = '' OR name ILIKE '%' || $3 || '%' OR brand ILIKE '%' || $3 || '%' OR description ILIKE '%' || $3 || '%')
		          ORDER BY created_at DESC LIMIT 250`

	rows, err := h.DB.Query(context.Background(), query, category, brand, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch products")
		return
	}
	defer rows.Close()

	type product struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Brand       string  `json:"brand"`
		Category    string  `json:"category"`
		PricePaise  int64   `json:"price_paise"`
		TaxRateBPS  int     `json:"tax_rate_bps"`
		Stock       int     `json:"stock"`
		SellerID    *string `json:"seller_id"`
		ImageURL    string  `json:"image_url"`
		CreatedAt   string  `json:"created_at"`
	}

	products := []product{}
	for rows.Next() {
		var p product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Brand, &p.Category, &p.PricePaise, &p.TaxRateBPS, &p.Stock, &p.SellerID, &p.ImageURL, &p.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read product row")
			return
		}
		products = append(products, p)
	}

	writeJSON(w, http.StatusOK, products)
}

func (h *ProductHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var p struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Brand       string  `json:"brand"`
		Category    string  `json:"category"`
		PricePaise  int64   `json:"price_paise"`
		TaxRateBPS  int     `json:"tax_rate_bps"`
		Stock       int     `json:"stock"`
		SellerID    *string `json:"seller_id"`
		ImageURL    string  `json:"image_url"`
	}

	err := h.DB.QueryRow(context.Background(),
		`SELECT id, name, description, brand, category, price_paise, tax_rate_bps, stock, seller_id, image_url FROM products WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Brand, &p.Category, &p.PricePaise, &p.TaxRateBPS, &p.Stock, &p.SellerID, &p.ImageURL)

	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch product")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req productRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.PricePaise < 0 || req.Stock < 0 || req.TaxRateBPS < 0 || req.TaxRateBPS > 10000 {
		writeError(w, http.StatusBadRequest, "name required; price and stock must be non-negative and tax rate must be between 0 and 100 percent")
		return
	}

	var id string
	var sellerID any
	if strings.TrimSpace(req.SellerID) != "" {
		sellerID = req.SellerID
	}
	err := h.DB.QueryRow(context.Background(),
		`INSERT INTO products (name, description, brand, category, price_paise, tax_rate_bps, stock, seller_id, image_url)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		req.Name, req.Description, req.Brand, req.Category, req.PricePaise, req.TaxRateBPS, req.Stock, sellerID, req.ImageURL,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create product")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req productRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.PricePaise < 0 || req.Stock < 0 || req.TaxRateBPS < 0 || req.TaxRateBPS > 10000 {
		writeError(w, http.StatusBadRequest, "name required; price and stock must be non-negative and tax rate must be between 0 and 100 percent")
		return
	}

	tag, err := h.DB.Exec(context.Background(),
		`UPDATE products SET name=$1, description=$2, brand=$3, category=$4, price_paise=$5, tax_rate_bps=$6, stock=$7, image_url=$8 WHERE id=$9`,
		req.Name, req.Description, req.Brand, req.Category, req.PricePaise, req.TaxRateBPS, req.Stock, req.ImageURL, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update product")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := h.DB.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete product")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
