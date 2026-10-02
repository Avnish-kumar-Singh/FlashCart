package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourname/flashcart/internal/auth"
	appmiddleware "github.com/yourname/flashcart/internal/middleware"
)

type UserHandler struct {
	DB     *pgxpool.Pool
	Issuer *auth.Issuer
}

type tokenPairResponse struct {
	UserID         string `json:"user_id"`
	AccessToken    string `json:"access_token"`
	RefreshToken   string `json:"refresh_token"`
	Role           string `json:"role"`
	Theme          string `json:"theme"`
	WelcomeMessage string `json:"welcome_message,omitempty"`
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register creates a new user with a bcrypt-hashed password.
// Phase 1 has no JWT issuance yet — that's added in Phase 2 alongside
// access/refresh tokens. For now, login just verifies credentials.
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "name, email required; password must be >= 8 chars")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var id string
	err = h.DB.QueryRow(context.Background(),
		`INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		req.Name, req.Email, string(hash),
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusConflict, "email already registered or invalid data")
		return
	}

	access, refresh, err := h.Issuer.IssuePair(id, "USER")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	writeJSON(w, http.StatusCreated, tokenPairResponse{
		UserID: id, Role: "USER", Theme: "light", AccessToken: access, RefreshToken: refresh,
		WelcomeMessage: "Welcome to FlashCart, " + req.Name + "! 🎉 Check out today's flash sales before they're gone.",
	})
}

// Login verifies email/password and returns a signed access/refresh token pair.
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var id, hash, role, theme string
	err := h.DB.QueryRow(context.Background(),
		`SELECT id, password_hash, role, theme FROM users WHERE email = $1`, req.Email,
	).Scan(&id, &hash, &role, &theme)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	access, refresh, err := h.Issuer.IssuePair(id, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	writeJSON(w, http.StatusOK, tokenPairResponse{UserID: id, Role: role, Theme: theme, AccessToken: access, RefreshToken: refresh})
}

// Refresh exchanges a valid, unexpired refresh token for a brand new
// access/refresh pair. The old refresh token is not explicitly revoked
// here — that requires a Redis-backed denylist/allowlist, which is a
// Phase 3 addition once session revocation actually matters (e.g. logout,
// compromised account). For now, tokens simply expire on their own TTL.
func (h *UserHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims, err := h.Issuer.ParseRefresh(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	var role, theme string
	if err := h.DB.QueryRow(context.Background(), `SELECT role, theme FROM users WHERE id = $1`, claims.UserID).Scan(&role, &theme); err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	access, refresh, err := h.Issuer.IssuePair(claims.UserID, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	writeJSON(w, http.StatusOK, tokenPairResponse{UserID: claims.UserID, Role: role, Theme: theme, AccessToken: access, RefreshToken: refresh})
}

var allowedThemes = map[string]bool{
	"light": true, "dark": true, "gradient": true,
	"aurora": true, "neon": true, "particles": true,
}

func (h *UserHandler) GetTheme(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var theme string
	if err := h.DB.QueryRow(r.Context(), `SELECT theme FROM users WHERE id=$1`, userID).Scan(&theme); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"theme": theme})
}

func (h *UserHandler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	userID, _ := appmiddleware.UserIDFromContext(r.Context())
	var req struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !allowedThemes[req.Theme] {
		writeError(w, http.StatusBadRequest, "theme must be one of light, dark, gradient, aurora, neon, or particles")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `UPDATE users SET theme=$1 WHERE id=$2`, req.Theme, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save theme preference")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"theme": req.Theme})
}
