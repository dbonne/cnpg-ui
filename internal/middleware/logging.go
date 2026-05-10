package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const requestIDKey contextKey = "request-id"

// RequestIDFromContext retrieves the request ID injected by RequestLogger.
// Returns "" if not present.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// responseWriter is a wrapper around http.ResponseWriter that captures the
// response status code written by downstream handlers.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestLogger returns middleware that:
//  1. Assigns or propagates an X-Request-ID header
//  2. Injects the request ID into the context
//  3. Emits a structured slog log entry after the handler returns:
//     method, path, status, duration_ms, request_id
func RequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Reuse client-provided ID, or generate a new one.
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.NewString()
			}

			// Propagate ID back to client and into context.
			w.Header().Set("X-Request-ID", requestID)
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			r = r.WithContext(ctx)

			// Wrap response writer to capture status code.
			rw := newResponseWriter(w)
			start := time.Now()

			next.ServeHTTP(rw, r)

			slog.InfoContext(ctx, "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", requestID,
			)
		})
	}
}
