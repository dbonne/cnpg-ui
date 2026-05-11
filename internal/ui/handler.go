// Package ui provides HTTP handlers for the HTMX-powered web UI layer.
// Templates are embedded via embed.FS so the binary is self-contained.
package ui

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/middleware"
)

//go:embed templates
var templateFS embed.FS

// ── Service interfaces ────────────────────────────────────────────────────────

// ClusterLister abstracts the cluster listing operation for the UI layer.
// This avoids a direct dependency on the cluster package.
type ClusterLister interface {
	List(ctx context.Context) ([]api.ClusterSummary, error)
}

// ClusterGetter abstracts the cluster detail fetch for the UI layer.
type ClusterGetter interface {
	Get(ctx context.Context, name string) (*api.ClusterDetail, error)
}

// BackupLister abstracts the backup listing operation for the UI layer.
type BackupLister interface {
	ListBackups(ctx context.Context, clusterName string) ([]api.BackupSummary, error)
}

// PoolerLister abstracts the pooler listing operation for the UI layer.
type PoolerLister interface {
	List(ctx context.Context, clusterName string) ([]api.PoolerSummary, error)
}

// ConfigGetter abstracts the Postgres config fetch for the UI layer.
type ConfigGetter interface {
	GetConfig(ctx context.Context, clusterName string) (*api.PgConfigResponse, error)
}

// Handler renders HTMX-powered HTML pages for the web UI.
type Handler struct {
	// templates stores one compiled template set per page.
	// Each page gets layout.html + its own page template, avoiding
	// block name collisions between pages.
	templates map[string]*template.Template
	clusters  ClusterLister
	getter    ClusterGetter
	backups   BackupLister
	poolers   PoolerLister
	config    ConfigGetter
}

// NewUIHandler parses all embedded templates and returns a ready Handler.
// All service parameters may be nil (graceful degradation — panels show empty state).
// Returns an error if any template file fails to parse.
func NewUIHandler(
	clusterLister ClusterLister,
	clusterGetter ClusterGetter,
	backupLister BackupLister,
	poolerLister PoolerLister,
	configGetter ConfigGetter,
) (*Handler, error) {
	funcs := templateFuncs()
	pages := map[string][]string{
		"login":                   {"templates/login.html"},
		"clusters/list":           {"templates/layout.html", "templates/clusters/list.html"},
		"clusters/detail":         {"templates/layout.html", "templates/clusters/detail.html"},
		"clusters/overview_panel": {"templates/clusters/overview_panel.html"},
		"clusters/logs_panel":     {"templates/clusters/logs_panel.html"},
		"clusters/config_panel":   {"templates/clusters/config_panel.html"},
		"clusters/backups_panel":  {"templates/clusters/backups_panel.html"},
	}

	templates := make(map[string]*template.Template, len(pages))
	for name, files := range pages {
		t, err := template.New("").Funcs(funcs).ParseFS(templateFS, files...)
		if err != nil {
			return nil, fmt.Errorf("parsing template %q: %w", name, err)
		}
		templates[name] = t
	}

	return &Handler{
		templates: templates,
		clusters:  clusterLister,
		getter:    clusterGetter,
		backups:   backupLister,
		poolers:   poolerLister,
		config:    configGetter,
	}, nil
}

// ── Page handlers ──────────────────────────────────────────────────────────────

// LoginPage handles GET /ui/login — renders the login form.
// The handler always renders the form. Error messages are injected via
// query param ?error=invalid+credentials when the POST handler redirects back.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Error": r.URL.Query().Get("error"),
	}
	h.renderPage(w, "login", data)
}

// ListClusters handles GET /ui/clusters — renders the cluster list page.
// Loads clusters from K8s on server-side render; HTMX will trigger refreshes
// from SSE events for live updates.
func (h *Handler) ListClusters(w http.ResponseWriter, r *http.Request) {
	username := middleware.UsernameFromContext(r.Context())

	var rows []clusterRow
	if h.clusters != nil {
		clusters, err := h.clusters.List(r.Context())
		if err != nil {
			slog.Error("failed to list clusters", "error", err)
		} else {
			for _, c := range clusters {
				rows = append(rows, clusterRow{
					Name:        c.Name,
					Status:      c.Status,
					StatusClass: StatusBadgeClass(c.Status),
					Instances:   c.Instances,
				})
			}
		}
	}

	data := map[string]interface{}{
		"Username":  username,
		"ActiveNav": "clusters",
		"Clusters":  rows,
	}
	h.renderPage(w, "clusters/list", data)
}

// ClusterDetail handles GET /ui/clusters/{name} — renders the cluster detail page.
// When ClusterGetter is configured, it loads real cluster data; otherwise it
// degrades gracefully showing the cluster name with an unknown status.
func (h *Handler) ClusterDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	username := middleware.UsernameFromContext(r.Context())

	// Default to minimal fallback data (graceful degradation when no getter).
	status := api.StatusHealthy
	data := map[string]interface{}{
		"Username":    username,
		"ActiveNav":   "clusters",
		"ClusterName": name,
		"Status":      status,
		"StatusClass": StatusBadgeClass(status),
	}

	if h.getter != nil {
		detail, err := h.getter.Get(r.Context(), name)
		if err != nil {
			slog.Error("failed to get cluster detail", "cluster", name, "error", err)
		} else if detail != nil {
			data["Status"] = detail.Status
			data["StatusClass"] = StatusBadgeClass(detail.Status)
			data["Instances"] = detail.Instances
			data["ReadyInstances"] = detail.ReadyInstances
			data["CurrentPrimary"] = detail.CurrentPrimary
			data["StorageSize"] = detail.StorageSize
			data["PgVersion"] = detail.PgVersion
		}
	}

	h.renderPage(w, "clusters/detail", data)
}

// ── Panel handlers (HTMX partials) ────────────────────────────────────────────

// OverviewPanel handles GET /ui/clusters/{name}/overview — renders the overview HTML partial.
// Shows cluster instances, status, primary node, and storage information.
func (h *Handler) OverviewPanel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	data := map[string]interface{}{
		"ClusterName":    name,
		"Status":         api.StatusHealthy,
		"StatusClass":    StatusBadgeClass(api.StatusHealthy),
		"Instances":      0,
		"ReadyInstances": 0,
		"CurrentPrimary": "",
		"StorageSize":    "",
		"PgVersion":      0,
	}

	if h.getter != nil {
		detail, err := h.getter.Get(r.Context(), name)
		if err != nil {
			slog.Error("overview panel: failed to get cluster", "cluster", name, "error", err)
		} else if detail != nil {
			data["Status"] = detail.Status
			data["StatusClass"] = StatusBadgeClass(detail.Status)
			data["Instances"] = detail.Instances
			data["ReadyInstances"] = detail.ReadyInstances
			data["CurrentPrimary"] = detail.CurrentPrimary
			data["StorageSize"] = detail.StorageSize
			data["PgVersion"] = detail.PgVersion
		}
	}

	h.renderPanel(w, "clusters/overview_panel", data)
}

// LogsPanel handles GET /ui/clusters/{name}/logs-panel — renders the log viewer HTML partial.
// The partial sets up an SSE connection to the log stream endpoint.
func (h *Handler) LogsPanel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	data := map[string]interface{}{
		"ClusterName": name,
	}
	h.renderPanel(w, "clusters/logs_panel", data)
}

// ConfigPanel handles GET /ui/clusters/{name}/config-panel — renders the Postgres config HTML partial.
// Lists postgres parameters with their type metadata from the ConfigGetter.
func (h *Handler) ConfigPanel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var params []api.PgParamMetadata
	if h.config != nil {
		resp, err := h.config.GetConfig(r.Context(), name)
		if err != nil {
			slog.Error("config panel: failed to get config", "cluster", name, "error", err)
		} else if resp != nil {
			params = resp.Parameters
		}
	}

	data := map[string]interface{}{
		"ClusterName": name,
		"Parameters":  params,
	}
	h.renderPanel(w, "clusters/config_panel", data)
}

// BackupsPanel handles GET /ui/clusters/{name}/backups-panel — renders the backups HTML partial.
// Lists backups for the cluster from the BackupLister.
func (h *Handler) BackupsPanel(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var backups []api.BackupSummary
	if h.backups != nil {
		list, err := h.backups.ListBackups(r.Context(), name)
		if err != nil {
			slog.Error("backups panel: failed to list backups", "cluster", name, "error", err)
		} else {
			backups = list
		}
	}

	data := map[string]interface{}{
		"ClusterName": name,
		"Backups":     backups,
	}
	h.renderPanel(w, "clusters/backups_panel", data)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// renderPage renders a named template, writing HTML to w.
// name is the page key (e.g. "login", "clusters/list", "clusters/detail").
func (h *Handler) renderPage(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	t, ok := h.templates[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Standalone pages (login) render by their own filename.
	// Layout-wrapped pages render "layout.html" which invokes {{block "content"}}.
	execName := "layout.html"
	if name == "login" {
		execName = "login.html"
	}

	if err := t.ExecuteTemplate(w, execName, data); err != nil {
		http.Error(w, "template rendering error: "+err.Error(), http.StatusInternalServerError)
	}
}

// renderPanel renders an HTML fragment partial (no layout wrapper).
// Panel templates are the filename within the embed.FS.
func (h *Handler) renderPanel(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	t, ok := h.templates[name]
	if !ok {
		http.Error(w, "panel template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Panel templates are single-file fragments; execute by the last path segment.
	execName := templateFilename(name)
	if err := t.ExecuteTemplate(w, execName, data); err != nil {
		http.Error(w, "panel rendering error: "+err.Error(), http.StatusInternalServerError)
	}
}

// templateFilename extracts the base filename for panel template execution.
// e.g. "clusters/overview_panel" → "overview_panel.html".
func templateFilename(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[i+1:] + ".html"
		}
	}
	return name + ".html"
}

// StatusBadgeClass maps a NormalizedStatus to the CSS class for the badge element.
// Pure function — no side effects, no dependencies.
func StatusBadgeClass(status api.NormalizedStatus) string {
	switch status {
	case api.StatusHealthy:
		return "badge-green"
	case api.StatusTransient:
		return "badge-yellow"
	case api.StatusFault:
		return "badge-red"
	case api.StatusHibernated:
		return "badge-blue"
	default:
		return "badge-gray"
	}
}

// templateFuncs returns custom template functions available in all templates.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// age formats a time.Time as a human-readable age string (e.g. "3d", "2h").
		"age": func(t time.Time) string {
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return fmt.Sprintf("%ds", int(d.Seconds()))
			case d < time.Hour:
				return fmt.Sprintf("%dm", int(d.Minutes()))
			case d < 24*time.Hour:
				return fmt.Sprintf("%dh", int(d.Hours()))
			default:
				return fmt.Sprintf("%dd", int(d.Hours()/24))
			}
		},
		"statusBadgeClass": StatusBadgeClass,
	}
}

// ── View models ───────────────────────────────────────────────────────────────

// clusterRow is the view model for a single row in the cluster list table.
type clusterRow struct {
	Name        string
	Status      api.NormalizedStatus
	StatusClass string
	Instances   int
	Age         string
}
