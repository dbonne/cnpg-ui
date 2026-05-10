package middleware

import (
	"net/http"
	"strings"
)

// allowedMethods lists the HTTP methods the CORS middleware advertises in
// preflight responses.
const allowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"

// allowedHeaders lists the headers clients may send.
const allowedHeaders = "Authorization, Content-Type, X-Request-ID"

// CORS returns middleware that implements Cross-Origin Resource Sharing.
//
// allowedOrigins is the whitelist. When nil or empty, the middleware is
// effectively same-origin: it does not add any Access-Control-* headers.
//
// Preflight OPTIONS requests for an allowed origin receive 204 No Content with
// the standard CORS headers. Requests from disallowed origins pass through
// without CORS headers — the browser will block them.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.TrimSpace(o)] = struct{}{}
	}

	isAllowed := func(origin string) bool {
		if len(originSet) == 0 {
			return false
		}
		_, ok := originSet[origin]
		return ok
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && isAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}

			// Handle preflight — always return 204 (with or without CORS headers).
			if r.Method == http.MethodOptions {
				if origin != "" && isAllowed(origin) {
					w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
					w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
