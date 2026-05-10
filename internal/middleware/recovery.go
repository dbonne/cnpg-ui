package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
)

// Recovery returns middleware that catches panics from downstream handlers,
// logs the stack trace, and returns an appropriate 500 response.
//
// Response format:
//   - /api/* and all non-UI paths → application/json
//   - /ui/* paths → text/html (minimal error page)
func Recovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()

					slog.ErrorContext(r.Context(), "panic recovered",
						"panic", fmt.Sprintf("%v", rec),
						"path", r.URL.Path,
						"stack", string(stack),
					)

					if strings.HasPrefix(r.URL.Path, "/ui/") {
						respondPanicHTML(w)
					} else {
						respondPanicJSON(w)
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func respondPanicJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	body, _ := json.Marshal(map[string]string{
		"error": "internal server error",
		"code":  "INTERNAL_ERROR",
	})
	_, _ = w.Write(body)
}

func respondPanicHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>500 Internal Server Error</title></head>
<body>
  <h1>500 Internal Server Error</h1>
  <p>An unexpected error occurred. Please try again later.</p>
</body>
</html>`)
}
