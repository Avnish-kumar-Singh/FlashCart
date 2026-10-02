package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/yourname/flashcart/internal/auth"
)

type contextKey string

const (
	userIDKey contextKey = "userID"
	roleKey   contextKey = "role"
)

// RequireAuth validates the Authorization: Bearer <token> header, rejects
// anything that isn't a valid, unexpired *access* token (a refresh token
// used here is rejected too — it has its own single-purpose endpoint),
// and stores the authenticated user ID on the request context.
func RequireAuth(issuer *auth.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimPrefix(header, "Bearer ")

			claims, err := issuer.Parse(tokenString)
			if err != nil || claims.Type != auth.AccessToken {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, roleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth populates the user ID on context if a valid access token is
// present, but never rejects the request otherwise — for endpoints like
// flash-sale subscription that work for both logged-in and anonymous
// visitors, but should recognize a logged-in user when there is one.
func OptionalAuth(issuer *auth.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}
			tokenString := strings.TrimPrefix(header, "Bearer ")
			claims, err := issuer.Parse(tokenString)
			if err != nil || claims.Type != auth.AccessToken {
				next.ServeHTTP(w, r) // invalid token on an optional-auth route: proceed as anonymous
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, roleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext reads the authenticated user ID set by RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

// RoleFromContext reads the role set by RequireAuth.
func RoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(roleKey).(string)
	return role, ok
}

// RequireRole allows only authenticated users with one of the requested roles.
func RequireRole(issuer *auth.Issuer, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireAuth(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, _ := RoleFromContext(r.Context())
			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		}))
	}
}
