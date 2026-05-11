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

// ClusterLister abstracts the cluster listing operation for the UI layer.
// This avoids a direct dependency on the cluster package.
type ClusterLister interface {
	List(ctx context.Context) ([]api.ClusterSummary, error)
}

// UIHandler renders HTMX-powered HTML pages for the web UI.
type UIHandler struct {
	// tmpl is the parsed template set (layout + all pages).
	// Parsing once at startup ensures startup-time errors surface early.
	tmpl     *template.Template
	clusters ClusterLister
}

// NewUIHandler parses all embedded templates and returns a ready UIHandler.
// clusterLister may be nil (clusters will show as empty).
// Returns an error if any template file fails to parse.
func NewUIHandler(clusterLister ClusterLister) (*UIHandler, error) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/login.html",
		"templates/clusters/list.html",
		"templates/clusters/detail.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parsing UI templates: %w", err)
	}
	return &UIHandler{tmpl: tmpl, clusters: clusterLister}, nil
}

// ── Page handlers ──────────────────────────────────────────────────────────────

// LoginPage handles GET /ui/login — renders the login form.
// The handler always renders the form. Error messages are injected via
// query param ?error=invalid+credentials when the POST handler redirects back.
func (h *UIHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Error": r.URL.Query().Get("error"),
	}
	h.renderPage(w, "login.html", data)
}

// ListClusters handles GET /ui/clusters — renders the cluster list page.
// Loads clusters from K8s on server-side render; HTMX will trigger refreshes
// from SSE events for live updates.
func (h *UIHandler) ListClusters(w http.ResponseWriter, r *http.Request) {
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
	h.renderPage(w, "clusters/list.html", data)
}

// ClusterDetail handles GET /ui/clusters/{name} — renders the cluster detail page.
func (h *UIHandler) ClusterDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	username := middleware.UsernameFromContext(r.Context())
	data := map[string]interface{}{
		"Username":    username,
		"ActiveNav":   "clusters",
		"ClusterName": name,
		"Status":      api.StatusHealthy,
		"StatusClass": StatusBadgeClass(api.StatusHealthy),
	}
	h.renderPage(w, "clusters/detail.html", data)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// renderPage renders a named template from the compiled set, writing HTML to w.
// The template name matches the filename relative to the templates/ dir.
// On error a 500 is returned; the error is logged to stderr.
func (h *UIHandler) renderPage(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// For standalone pages (login) we execute the template by its base name.
	// For layout-wrapped pages the template defines "content" and "layout" is
	// invoked via "{{template "layout" .}}" in the page file.
	// html/template.ParseFS registers every file by its path relative to the FS
	// root; we look up by basename to keep call sites simple.
	var tmplName string
	switch name {
	case "login.html":
		tmplName = "login.html"
	default:
		// layout-wrapped pages: execute the layout template which calls {{block "content" .}}
		tmplName = "layout.html"
	}

	if err := h.tmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		// Header may already be written; best-effort 500.
		http.Error(w, "template rendering error: "+err.Error(), http.StatusInternalServerError)
	}
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
