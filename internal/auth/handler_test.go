package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/auth"
	"github.com/dbonne/cnpg-ui/internal/config"
	"github.com/dbonne/cnpg-ui/internal/k8s"
)

// newTestService builds a real auth.Service backed by a fake K8s client.
func newTestService(t *testing.T, username, password string) auth.Service {
	t.Helper()
	scheme := k8s.NewScheme()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cnpg-ui-credentials"},
		Data: map[string][]byte{
			"username":      []byte(username),
			"password_hash": []byte(hash),
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	cfg := &config.Config{
		K8sNamespace: "default",
		SecretName:   "cnpg-ui-credentials",
		SessionTTL:   30 * time.Minute,
	}
	store := auth.NewStore(cfg.SessionTTL)
	return auth.NewService(cfg, fc, store)
}

// ── POST /api/v1/auth/login ─────────────────────────────────────────────────

func TestHandler_APILogin_ValidJSON(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.APILogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Response body must contain a token field.
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("response must include non-empty 'token' field")
	}

	// Session cookie must be set.
	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "cnpg-ui-session" {
			found = true
			if c.Value != resp["token"] {
				t.Errorf("cookie value %q != token %q", c.Value, resp["token"])
			}
			if !c.HttpOnly {
				t.Error("session cookie must be HttpOnly")
			}
		}
	}
	if !found {
		t.Error("cnpg-ui-session cookie must be set")
	}
}

func TestHandler_APILogin_InvalidPassword_Returns401(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.APILogin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandler_APILogin_MalformedJSON_Returns400(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.APILogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// ── POST /ui/login ──────────────────────────────────────────────────────────

func TestHandler_UILogin_ValidForm(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	form := url.Values{"username": {"admin"}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/ui/login",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.UILogin(rr, req)

	// Must redirect to /ui/clusters on success.
	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/ui/clusters" {
		t.Errorf("Location = %q, want /ui/clusters", loc)
	}

	// Cookie must be set.
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "cnpg-ui-session" {
			found = true
		}
	}
	if !found {
		t.Error("cnpg-ui-session cookie must be set on UI login")
	}
}

func TestHandler_UILogin_InvalidPassword_RedirectsWithError(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	form := url.Values{"username": {"admin"}, "password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, "/ui/login",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.UILogin(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect to login", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/ui/login") {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── POST /api/v1/auth/logout ────────────────────────────────────────────────

func TestHandler_APILogout_InvalidatesSession(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	// First login to get a session.
	body := `{"username":"admin","password":"secret"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	h.APILogin(loginRR, loginReq)

	var loginResp map[string]string
	_ = json.NewDecoder(loginRR.Body).Decode(&loginResp)
	token := loginResp["token"]

	// Now logout.
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+token)
	logoutRR := httptest.NewRecorder()
	h.APILogout(logoutRR, logoutReq)

	if logoutRR.Code != http.StatusNoContent {
		t.Errorf("logout status = %d, want 204", logoutRR.Code)
	}
}

func TestHandler_UILogout_ClearsCookieAndRedirects(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false)

	// Login first.
	form := url.Values{"username": {"admin"}, "password": {"secret"}}
	loginReq := httptest.NewRequest(http.MethodPost, "/ui/login",
		strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRR := httptest.NewRecorder()
	h.UILogin(loginRR, loginReq)

	var sessionCookie *http.Cookie
	for _, c := range loginRR.Result().Cookies() {
		if c.Name == "cnpg-ui-session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}

	// Logout.
	logoutReq := httptest.NewRequest(http.MethodPost, "/ui/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutRR := httptest.NewRecorder()
	h.UILogout(logoutRR, logoutReq)

	if logoutRR.Code != http.StatusSeeOther {
		t.Errorf("logout status = %d, want 303", logoutRR.Code)
	}
	loc := logoutRR.Header().Get("Location")
	if loc != "/ui/login" {
		t.Errorf("Location = %q, want /ui/login", loc)
	}
}

// ── POST /api/v1/auth/password ──────────────────────────────────────────────

func TestHandler_ChangePassword_ValidOld(t *testing.T) {
	svc := newTestService(t, "admin", "oldpass")
	h := auth.NewHandler(svc, false)

	// Login to get a session.
	loginBody := `{"username":"admin","password":"oldpass"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	h.APILogin(loginRR, loginReq)

	var loginResp map[string]string
	_ = json.NewDecoder(loginRR.Body).Decode(&loginResp)

	// Change password — new password meets minimum 12-char requirement.
	cpBody := `{"old_password":"oldpass","new_password":"newpassword1234"}`
	cpReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(cpBody))
	cpReq.Header.Set("Content-Type", "application/json")
	cpReq.Header.Set("Authorization", "Bearer "+loginResp["token"])
	// Inject username into context (as auth middleware would).
	ctx := context.WithValue(cpReq.Context(), auth.UsernameContextKey, "admin")
	cpReq = cpReq.WithContext(ctx)

	cpRR := httptest.NewRecorder()
	h.ChangePassword(cpRR, cpReq)

	if cpRR.Code != http.StatusOK {
		t.Errorf("change password status = %d, want 200; body: %s", cpRR.Code, cpRR.Body.String())
	}
}

func TestHandler_ChangePassword_WrongOld_Returns401(t *testing.T) {
	svc := newTestService(t, "admin", "oldpass")
	h := auth.NewHandler(svc, false)

	cpBody := `{"old_password":"wrongold","new_password":"newpass_long_enough"}`
	cpReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(cpBody))
	cpReq.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(cpReq.Context(), auth.UsernameContextKey, "admin")
	cpReq = cpReq.WithContext(ctx)

	cpRR := httptest.NewRecorder()
	h.ChangePassword(cpRR, cpReq)

	if cpRR.Code != http.StatusUnauthorized {
		t.Errorf("change password status = %d, want 401", cpRR.Code)
	}
}

// TestHandler_ChangePassword_TooShort_Returns422 verifies that a new password
// shorter than 12 characters is rejected with 422 Unprocessable Entity.
func TestHandler_ChangePassword_TooShort_Returns422(t *testing.T) {
	svc := newTestService(t, "admin", "oldpass")
	h := auth.NewHandler(svc, false)

	cpBody := `{"old_password":"oldpass","new_password":"short"}`
	cpReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(cpBody))
	cpReq.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(cpReq.Context(), auth.UsernameContextKey, "admin")
	cpReq = cpReq.WithContext(ctx)

	cpRR := httptest.NewRecorder()
	h.ChangePassword(cpRR, cpReq)

	if cpRR.Code != http.StatusUnprocessableEntity {
		t.Errorf("change password (too short) status = %d, want 422", cpRR.Code)
	}
}

// TestHandler_ChangePassword_ExactMinLength_Succeeds verifies a 12-char password is accepted.
func TestHandler_ChangePassword_ExactMinLength_Succeeds(t *testing.T) {
	svc := newTestService(t, "admin", "oldpassword1")
	h := auth.NewHandler(svc, false)

	// Login first
	loginBody := `{"username":"admin","password":"oldpassword1"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	h.APILogin(loginRR, loginReq)

	// Change to exactly 12-char password
	cpBody := `{"old_password":"oldpassword1","new_password":"newpassword1"}`
	cpReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(cpBody))
	cpReq.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(cpReq.Context(), auth.UsernameContextKey, "admin")
	cpReq = cpReq.WithContext(ctx)

	cpRR := httptest.NewRecorder()
	h.ChangePassword(cpRR, cpReq)

	if cpRR.Code != http.StatusOK {
		t.Errorf("change password (exact min length) status = %d, want 200; body: %s", cpRR.Code, cpRR.Body.String())
	}
}

// TestHandler_SecureCookie_HTTP verifies that Secure is false when secureCookie=false.
func TestHandler_SecureCookie_HTTP(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, false) // HTTP mode — no TLS

	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.APILogin(rr, req)

	for _, c := range rr.Result().Cookies() {
		if c.Name == "cnpg-ui-session" && c.Secure {
			t.Error("Secure flag must be false when server is not using TLS")
		}
	}
}

// TestHandler_SecureCookie_TLS verifies that Secure is true when secureCookie=true.
func TestHandler_SecureCookie_TLS(t *testing.T) {
	svc := newTestService(t, "admin", "secret")
	h := auth.NewHandler(svc, true) // TLS mode

	body := `{"username":"admin","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.APILogin(rr, req)

	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "cnpg-ui-session" {
			found = true
			if !c.Secure {
				t.Error("Secure flag must be true when server is using TLS")
			}
		}
	}
	if !found {
		t.Error("cnpg-ui-session cookie not set")
	}
}
