// Package middleware provides HTTP middleware for the CNPG Web UI.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// ErrUnauthorized is returned by SessionValidator.ValidateSession when the
// session token is invalid or expired.
var ErrUnauthorized = errors.New("unauthorized")

// contextKey is an unexported type used as context keys to avoid collisions.
type contextKey string

const usernameKey contextKey = "username"

// SessionValidator is a subset of auth.Service that the middleware needs.
// Defining it here avoids an import cycle (middleware → auth → middleware).
type SessionValidator interface {
	// ValidateSession returns the username for a valid session ID, or an error
	// if the session does not exist or has expired.
	ValidateSession(sessionID string) (string, error)
}

// APIAuth returns middleware that checks for a Bearer token in the
// Authorization header. On success it injects the username into the context.
// On failure it returns 401 JSON.
func APIAuth(svc SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				http.Error(w, `{"error":"missing or malformed Authorization header","code":"UNAUTHORIZED"}`,
					http.StatusUnauthorized)
				return
			}

			username, err := svc.ValidateSession(token)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired session","code":"UNAUTHORIZED"}`,
					http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), usernameKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UIAuth returns middleware that checks for the cnpg-ui-session cookie.
// On success it injects the username into the context.
// On failure it redirects to /ui/login with 303 See Other.
func UIAuth(svc SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("cnpg-ui-session")
			if err != nil {
				http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
				return
			}

			username, err := svc.ValidateSession(cookie.Value)
			if err != nil {
				http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), usernameKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UsernameFromContext retrieves the authenticated username injected by
// APIAuth or UIAuth. Returns "" if not present.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(usernameKey).(string)
	return v
}

// extractBearer parses the Authorization header and returns the Bearer token.
// Returns "" if the header is missing or not in "Bearer <token>" form.
func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
