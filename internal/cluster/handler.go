package cluster

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	apierr "github.com/dbonne/cnpg-ui/internal/errors"
)

// Handler holds HTTP handlers for cluster endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a cluster Handler backed by the given Service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ListClusters handles GET /api/v1/clusters.
func (h *Handler) ListClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, clusters)
}

// GetCluster handles GET /api/v1/clusters/{name}.
func (h *Handler) GetCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	detail, err := h.svc.Get(r.Context(), name)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// CreateCluster handles POST /api/v1/clusters.
func (h *Handler) CreateCluster(w http.ResponseWriter, r *http.Request) {
	var req api.CreateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, apierr.CodeValidation, "invalid request body: "+err.Error())
		return
	}

	detail, err := h.svc.Create(r.Context(), req)
	if err != nil {
		if isConflict(err) {
			writeError(w, http.StatusConflict, apierr.CodeConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

// UpdateCluster handles PUT /api/v1/clusters/{name}.
func (h *Handler) UpdateCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req api.UpdateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, apierr.CodeValidation, "invalid request body: "+err.Error())
		return
	}

	detail, err := h.svc.Update(r.Context(), name, req)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// ScaleCluster handles PATCH /api/v1/clusters/{name}/scale.
func (h *Handler) ScaleCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req api.ScaleClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, apierr.CodeValidation, "invalid request body: "+err.Error())
		return
	}
	if req.Instances < 1 {
		writeError(w, http.StatusUnprocessableEntity, apierr.CodeValidation,
			"instances must be at least 1")
		return
	}

	detail, err := h.svc.Scale(r.Context(), name, req)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// DeleteCluster handles DELETE /api/v1/clusters/{name}.
func (h *Handler) DeleteCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.svc.Delete(r.Context(), name); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// writeJSON serializes v as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a canonical API error response.
func writeError(w http.ResponseWriter, status int, code apierr.ErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.APIError{
		Error: message,
		Code:  api.ErrorCode(code),
	})
}

// isNotFound returns true if the error message contains "not found".
// This is a simple convention used across service implementations.
func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// isConflict returns true if the error message contains "already exists".
func isConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}
