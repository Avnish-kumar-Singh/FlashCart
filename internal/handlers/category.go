package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CategoryHandler struct{ DB *pgxpool.Pool }

type categoryRequest struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	ParentID       *string `json:"parent_id"`
	Description    string  `json:"description"`
	BannerImageURL string  `json:"banner_image_url"`
	SEOTitle       string  `json:"seo_title"`
	SEODescription string  `json:"seo_description"`
	SortOrder      int     `json:"sort_order"`
	Active         bool    `json:"active"`
}

var categorySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT c.id::text, c.name, c.slug, c.parent_id::text, c.description,
		       c.banner_image_url, c.seo_title, c.seo_description, c.sort_order,
		       c.active, c.created_at::text, c.updated_at::text,
		       (SELECT COUNT(*) FROM categories child WHERE child.parent_id=c.id)
		FROM categories c
		ORDER BY c.sort_order, c.name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch categories")
		return
	}
	defer rows.Close()

	type category struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		Slug           string  `json:"slug"`
		ParentID       *string `json:"parent_id"`
		Description    string  `json:"description"`
		BannerImageURL string  `json:"banner_image_url"`
		SEOTitle       string  `json:"seo_title"`
		SEODescription string  `json:"seo_description"`
		SortOrder      int     `json:"sort_order"`
		Active         bool    `json:"active"`
		CreatedAt      string  `json:"created_at"`
		UpdatedAt      string  `json:"updated_at"`
		Children       int64   `json:"children"`
	}

	result := []category{}
	for rows.Next() {
		var item category
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.ParentID, &item.Description,
			&item.BannerImageURL, &item.SEOTitle, &item.SEODescription, &item.SortOrder,
			&item.Active, &item.CreatedAt, &item.UpdatedAt, &item.Children); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read category")
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read categories")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validCategory(&req, w) {
		return
	}
	var id string
	err := h.DB.QueryRow(r.Context(), `
		INSERT INTO categories (name, slug, parent_id, description, banner_image_url,
		                        seo_title, seo_description, sort_order, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`,
		req.Name, req.Slug, req.ParentID, req.Description, req.BannerImageURL,
		req.SEOTitle, req.SEODescription, req.SortOrder, req.Active).Scan(&id)
	if err != nil {
		writeCategoryDBError(w, err, "failed to create category")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *CategoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validCategory(&req, w) {
		return
	}
	id := chi.URLParam(r, "id")
	if req.ParentID != nil {
		var createsCycle bool
		err := h.DB.QueryRow(r.Context(), `
			WITH RECURSIVE descendants AS (
				SELECT id FROM categories WHERE parent_id=$1
				UNION ALL
				SELECT c.id FROM categories c JOIN descendants d ON c.parent_id=d.id
			)
			SELECT $2::uuid = $1::uuid OR EXISTS(SELECT 1 FROM descendants WHERE id=$2::uuid)`,
			id, *req.ParentID).Scan(&createsCycle)
		if err != nil {
			writeCategoryDBError(w, err, "invalid category or parent")
			return
		}
		if createsCycle {
			writeError(w, http.StatusBadRequest, "a category cannot be its own ancestor")
			return
		}
	}
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE categories SET name=$1, slug=$2, parent_id=$3, description=$4,
		       banner_image_url=$5, seo_title=$6, seo_description=$7,
		       sort_order=$8, active=$9, updated_at=now()
		WHERE id=$10`, req.Name, req.Slug, req.ParentID, req.Description,
		req.BannerImageURL, req.SEOTitle, req.SEODescription, req.SortOrder, req.Active, id)
	if err != nil {
		writeCategoryDBError(w, err, "failed to update category")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *CategoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tag, err := h.DB.Exec(r.Context(), `DELETE FROM categories WHERE id=$1`, chi.URLParam(r, "id"))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			writeError(w, http.StatusConflict, "remove or reassign child categories before deleting")
			return
		}
		writeCategoryDBError(w, err, "failed to delete category")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validCategory(req *categoryRequest, w http.ResponseWriter) bool {
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	if req.Name == "" || !categorySlugPattern.MatchString(req.Slug) || req.SortOrder < 0 {
		writeError(w, http.StatusBadRequest, "name, a lowercase URL-safe slug, and a non-negative sort order are required")
		return false
	}
	return true
}

func writeCategoryDBError(w http.ResponseWriter, err error, fallback string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			writeError(w, http.StatusConflict, "category slug already exists")
			return
		case "23503", "22P02":
			writeError(w, http.StatusBadRequest, "invalid category or parent")
			return
		}
	}
	writeError(w, http.StatusInternalServerError, fallback)
}
