package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// contextKey is the unexported context key type for this package.
type contextKey string

// UsernameContextKey is the exported context key used to pass the authenticated
// username from the auth middleware into handlers. It is exported so the
// middleware package adapter can inject it, and so tests can inject it directly.
const UsernameContextKey contextKey = "username"

// Handler holds all HTTP handlers for auth endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a new Handler backed by the given Service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ── API handlers ─────────────────────────────────────────────────────────────

// loginRequest is the JSON body for POST /api/v1/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// APILogin handles POST /api/v1/auth/login.
// It validates credentials and returns a session token + sets a cookie.
func (h *Handler) APILogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body","code":"VALIDATION_ERROR"}`,
			http.StatusBadRequest)
		return
	}

	sess, err := h.svc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		http.Error(w, `{"error":"invalid credentials","code":"UNAUTHORIZED"}`,
			http.StatusUnauthorized)
		return
	}

	setSessionCookie(w, sess.ID, sess.ExpiresAt)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token":    sess.ID,
		"username": sess.Username,
	})
}

// APILogout handles POST /api/v1/auth/logout.
// It reads the Bearer token from the Authorization header and invalidates it.
func (h *Handler) APILogout(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token != "" {
		h.svc.Logout(token)
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// changePasswordRequest is the JSON body for POST /api/v1/auth/password.
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword handles POST /api/v1/auth/password.
// It expects the username to be injected into the context by the auth middleware.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	username := usernameFromCtx(r.Context())
	if username == "" {
		http.Error(w, `{"error":"not authenticated","code":"UNAUTHORIZED"}`,
			http.StatusUnauthorized)
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body","code":"VALIDATION_ERROR"}`,
			http.StatusBadRequest)
		return
	}

	if err := h.svc.ChangePassword(r.Context(), username, req.OldPassword, req.NewPassword); err != nil {
		http.Error(w, `{"error":"invalid credentials","code":"UNAUTHORIZED"}`,
			http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ── UI handlers ──────────────────────────────────────────────────────────────

// UILogin handles POST /ui/login (form submission).
func (h *Handler) UILogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/ui/login?error=bad_request", http.StatusSeeOther)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	sess, err := h.svc.Login(r.Context(), username, password)
	if err != nil {
		http.Redirect(w, r, "/ui/login?error=invalid_credentials", http.StatusSeeOther)
		return
	}

	setSessionCookie(w, sess.ID, sess.ExpiresAt)
	http.Redirect(w, r, "/ui/clusters", http.StatusSeeOther)
}

// UILogout handles POST /ui/logout.
func (h *Handler) UILogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("cnpg-ui-session"); err == nil {
		h.svc.Logout(cookie.Value)
	}
	clearSessionCookie(w)
	http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// setSessionCookie writes the cnpg-ui-session cookie to the response.
func setSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cnpg-ui-session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie overwrites the cookie with an expired one.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cnpg-ui-session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// extractBearerToken reads the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
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

// usernameFromCtx retrieves the username from the request context.
func usernameFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(UsernameContextKey).(string)
	return v
}
