package errors_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/dbonne/cnpg-ui/internal/errors"
)

// ---- Code enum ----

func TestErrorCode_Values(t *testing.T) {
	codes := []apperrors.ErrorCode{
		apperrors.CodeNotFound,
		apperrors.CodeUnauthorized,
		apperrors.CodeForbidden,
		apperrors.CodeConflict,
		apperrors.CodeValidation,
		apperrors.CodeInternal,
	}

	seen := make(map[apperrors.ErrorCode]bool)
	for _, c := range codes {
		if string(c) == "" {
			t.Errorf("ErrorCode must not be empty string")
		}
		if seen[c] {
			t.Errorf("duplicate ErrorCode value: %q", c)
		}
		seen[c] = true
	}
}

// ---- Constructor ----

func TestNew_SetsFields(t *testing.T) {
	err := apperrors.New(http.StatusNotFound, apperrors.CodeNotFound, "cluster not found")

	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus: got %d, want %d", err.HTTPStatus, http.StatusNotFound)
	}
	if err.Code != apperrors.CodeNotFound {
		t.Errorf("Code: got %q, want %q", err.Code, apperrors.CodeNotFound)
	}
	if err.Message != "cluster not found" {
		t.Errorf("Message: got %q, want %q", err.Message, "cluster not found")
	}
}

func TestNew_DifferentCodes(t *testing.T) {
	tests := []struct {
		status int
		code   apperrors.ErrorCode
		msg    string
	}{
		{http.StatusUnauthorized, apperrors.CodeUnauthorized, "not authenticated"},
		{http.StatusForbidden, apperrors.CodeForbidden, "access denied"},
		{http.StatusConflict, apperrors.CodeConflict, "resource conflict"},
		{http.StatusBadRequest, apperrors.CodeValidation, "invalid input"},
		{http.StatusInternalServerError, apperrors.CodeInternal, "internal failure"},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			err := apperrors.New(tc.status, tc.code, tc.msg)
			if err.HTTPStatus != tc.status {
				t.Errorf("HTTPStatus: got %d, want %d", err.HTTPStatus, tc.status)
			}
			if err.Code != tc.code {
				t.Errorf("Code: got %q, want %q", err.Code, tc.code)
			}
			if err.Message != tc.msg {
				t.Errorf("Message: got %q, want %q", err.Message, tc.msg)
			}
		})
	}
}

// ---- Error() interface ----

func TestAppError_ImplementsError(t *testing.T) {
	appErr := apperrors.New(http.StatusNotFound, apperrors.CodeNotFound, "not found")
	// Verify *AppError satisfies the error interface at compile time via assignment.
	var err error = appErr
	_ = err
	if appErr.Error() == "" {
		t.Error("Error() must return non-empty string")
	}
	if !strings.Contains(appErr.Error(), "not found") {
		t.Errorf("Error() should contain message, got: %q", appErr.Error())
	}
}

// ---- RenderJSON ----

func TestRenderJSON_WritesCorrectPayload(t *testing.T) {
	w := httptest.NewRecorder()
	appErr := apperrors.New(http.StatusNotFound, apperrors.CodeNotFound, "cluster alpha not found")

	appErr.RenderJSON(w)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status code: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if body["error"] != "cluster alpha not found" {
		t.Errorf("error field: got %v, want %q", body["error"], "cluster alpha not found")
	}
	if body["code"] != string(apperrors.CodeNotFound) {
		t.Errorf("code field: got %v, want %q", body["code"], apperrors.CodeNotFound)
	}
}

func TestRenderJSON_500Response(t *testing.T) {
	w := httptest.NewRecorder()
	appErr := apperrors.New(http.StatusInternalServerError, apperrors.CodeInternal, "unexpected failure")

	appErr.RenderJSON(w)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status code: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if body["code"] != string(apperrors.CodeInternal) {
		t.Errorf("code field: got %v, want %q", body["code"], apperrors.CodeInternal)
	}
}

// ---- RenderHTML ----

func TestRenderHTML_WritesHTMLPartial(t *testing.T) {
	w := httptest.NewRecorder()
	appErr := apperrors.New(http.StatusForbidden, apperrors.CodeForbidden, "access denied")

	appErr.RenderHTML(w)

	resp := w.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status code: got %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}

	buf := new(strings.Builder)
	// Read body into string
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	buf.Write(body[:n])
	html := buf.String()

	if !strings.Contains(html, "access denied") {
		t.Errorf("HTML body should contain message %q, got:\n%s", "access denied", html)
	}
	if !strings.Contains(html, string(apperrors.CodeForbidden)) {
		t.Errorf("HTML body should contain code %q, got:\n%s", apperrors.CodeForbidden, html)
	}
}

func TestRenderHTML_DifferentErrors(t *testing.T) {
	tests := []struct {
		code    apperrors.ErrorCode
		status  int
		message string
	}{
		{apperrors.CodeNotFound, http.StatusNotFound, "resource missing"},
		{apperrors.CodeValidation, http.StatusBadRequest, "invalid field"},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			w := httptest.NewRecorder()
			apperrors.New(tc.status, tc.code, tc.message).RenderHTML(w)

			resp := w.Result()
			if resp.StatusCode != tc.status {
				t.Errorf("status: got %d, want %d", resp.StatusCode, tc.status)
			}
			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			html := string(body[:n])
			if !strings.Contains(html, tc.message) {
				t.Errorf("HTML should contain %q", tc.message)
			}
		})
	}
}
