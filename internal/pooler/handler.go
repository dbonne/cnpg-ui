package pooler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	apierr "github.com/dbonne/cnpg-ui/internal/errors"
)

// Handler holds HTTP handlers for pooler endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a pooler Handler backed by the given Service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ListPoolers handles GET /api/v1/clusters/{name}/poolers.
func (h *Handler) ListPoolers(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")
	poolers, err := h.svc.List(r.Context(), clusterName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, poolers)
}

// GetPooler handles GET /api/v1/clusters/{name}/poolers/{poolerName}.
func (h *Handler) GetPooler(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")
	poolerName := chi.URLParam(r, "poolerName")

	p, err := h.svc.Get(r.Context(), clusterName, poolerName)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code apierr.ErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.APIError{
		Error: message,
		Code:  api.ErrorCode(code),
	})
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}
