package cluster

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	apierr "github.com/dbonne/cnpg-ui/internal/errors"
)

// ConfigHandler holds HTTP handlers for Postgres configuration endpoints.
type ConfigHandler struct {
	svc ConfigService
}

// NewConfigHandler creates a ConfigHandler backed by the given ConfigService.
func NewConfigHandler(svc ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

// GetPostgresConfig handles GET /api/v1/clusters/{name}/postgres-config.
func (h *ConfigHandler) GetPostgresConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	resp, err := h.svc.GetConfig(r.Context(), name)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// UpdatePostgresConfig handles PUT /api/v1/clusters/{name}/postgres-config.
func (h *ConfigHandler) UpdatePostgresConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req api.UpdatePgConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, apierr.CodeValidation, "invalid request body: "+err.Error())
		return
	}

	resp, err := h.svc.UpdateConfig(r.Context(), name, req)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		if isValidationError(err) {
			writeError(w, http.StatusUnprocessableEntity, apierr.CodeValidation, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// isValidationError returns true if the error is a validation error from ConfigService.
func isValidationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "validation")
}
