package backup

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	apierr "github.com/dbonne/cnpg-ui/internal/errors"
)

// Handler holds HTTP handlers for backup endpoints.
type Handler struct {
	svc Service
}

// NewHandler creates a backup Handler backed by the given Service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ListBackups handles GET /api/v1/clusters/{name}/backups.
func (h *Handler) ListBackups(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")
	backups, err := h.svc.ListBackups(r.Context(), clusterName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, backups)
}

// TriggerBackup handles POST /api/v1/clusters/{name}/backups.
func (h *Handler) TriggerBackup(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")

	var req api.TriggerBackupRequest
	// Request body is optional — ignore decode errors
	_ = json.NewDecoder(r.Body).Decode(&req)

	result, err := h.svc.TriggerBackup(r.Context(), clusterName, req)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

// ListScheduledBackups handles GET /api/v1/clusters/{name}/scheduled-backups.
func (h *Handler) ListScheduledBackups(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")
	scheds, err := h.svc.ListScheduledBackups(r.Context(), clusterName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, scheds)
}

// GetScheduledBackup handles GET /api/v1/clusters/{name}/scheduled-backups/{id}.
func (h *Handler) GetScheduledBackup(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")
	id := chi.URLParam(r, "id")

	sched, err := h.svc.GetScheduledBackup(r.Context(), clusterName, id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

// CreateScheduledBackup handles POST /api/v1/clusters/{name}/scheduled-backups.
func (h *Handler) CreateScheduledBackup(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")

	var req api.CreateScheduledBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, apierr.CodeValidation, "invalid request body: "+err.Error())
		return
	}

	result, err := h.svc.CreateScheduledBackup(r.Context(), clusterName, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// DeleteScheduledBackup handles DELETE /api/v1/clusters/{name}/scheduled-backups/{id}.
func (h *Handler) DeleteScheduledBackup(w http.ResponseWriter, r *http.Request) {
	clusterName := chi.URLParam(r, "name")
	id := chi.URLParam(r, "id")

	if err := h.svc.DeleteScheduledBackup(r.Context(), clusterName, id); err != nil {
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
