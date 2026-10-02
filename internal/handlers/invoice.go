package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourname/flashcart/internal/invoice"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type InvoiceHandler struct{ DB *pgxpool.Pool }

func (h *InvoiceHandler) Get(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.load(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *InvoiceHandler) PDF(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.load(w, r)
	if !ok {
		return
	}
	content, err := invoice.PDF(doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render invoice PDF")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+doc.InvoiceNumber+`.pdf"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *InvoiceHandler) load(w http.ResponseWriter, r *http.Request) (invoice.Invoice, bool) {
	userID, ok := appmiddleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return invoice.Invoice{}, false
	}
	doc, err := invoice.Generate(r.Context(), h.DB, chi.URLParam(r, "id"), userID)
	if errors.Is(err, invoice.ErrNotFound) {
		writeError(w, http.StatusNotFound, "invoice not found")
		return invoice.Invoice{}, false
	}
	if errors.Is(err, invoice.ErrNotConfirmed) {
		writeError(w, http.StatusConflict, "invoice is available after payment confirmation")
		return invoice.Invoice{}, false
	}
	if errors.Is(err, invoice.ErrAmount) {
		writeError(w, http.StatusConflict, "order totals are not consistent; contact support")
		return invoice.Invoice{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load invoice")
		return invoice.Invoice{}, false
	}
	return doc, true
}
