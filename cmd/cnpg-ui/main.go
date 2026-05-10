// Command cnpg-ui starts the CNPG Web UI server.
// It loads configuration from environment variables, sets up the HTTP server,
// and wires dependencies following a 12-factor application approach.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/dbonne/cnpg-ui/internal/auth"
	"github.com/dbonne/cnpg-ui/internal/backup"
	"github.com/dbonne/cnpg-ui/internal/cluster"
	"github.com/dbonne/cnpg-ui/internal/config"
	"github.com/dbonne/cnpg-ui/internal/k8s"
	"github.com/dbonne/cnpg-ui/internal/middleware"
	"github.com/dbonne/cnpg-ui/internal/pooler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// ── K8s client layer ──────────────────────────────────────────────────────
	scheme := k8s.NewScheme()

	k8sClient, err := k8s.NewClient(scheme)
	if err != nil {
		logger.Warn("K8s client unavailable — running without cluster access", "err", err)
	}

	if k8sClient != nil {
		informerMgr := k8s.NewInformerManager(nil, scheme)
		go informerMgr.Start(context.Background())
		_ = informerMgr.WaitForSync
	}
	// ─────────────────────────────────────────────────────────────────────────

	// ── Auth layer ────────────────────────────────────────────────────────────
	sessionStore := auth.NewStore(cfg.SessionTTL)

	var authSvc auth.Service
	if k8sClient != nil {
		authSvc = auth.NewService(cfg, k8sClient, sessionStore)
	}

	var sessionValidator middleware.SessionValidator
	if authSvc != nil {
		sessionValidator = auth.NewSessionValidatorAdapter(authSvc)
	} else {
		sessionValidator = &rejectAllValidator{}
	}

	authHandler := auth.NewHandler(authSvc)
	// ─────────────────────────────────────────────────────────────────────────

	// ── Domain services ───────────────────────────────────────────────────────
	ns := cfg.K8sNamespace
	var (
		clusterSvc cluster.Service
		backupSvc  backup.Service
		poolerSvc  pooler.Service
		configSvc  cluster.ConfigService
	)
	if k8sClient != nil {
		clusterSvc = cluster.NewService(k8sClient, ns)
		backupSvc = backup.NewService(k8sClient, ns)
		poolerSvc = pooler.NewService(k8sClient, ns)
		configSvc = cluster.NewConfigService(k8sClient, ns)
	}

	clusterHandler := cluster.NewHandler(clusterSvc)
	backupHandler := backup.NewHandler(backupSvc)
	poolerHandler := pooler.NewHandler(poolerSvc)
	configHandler := cluster.NewConfigHandler(configSvc)
	// ─────────────────────────────────────────────────────────────────────────

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	// Health check — no auth required.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// OpenAPI spec — no auth required.
	r.Get("/api/v1/openapi.json", serveOpenAPISpec)

	// ── Auth endpoints (no session required) ──────────────────────────────────
	r.Post("/api/v1/auth/login", authHandler.APILogin)
	r.Post("/ui/login", authHandler.UILogin)

	// ── Protected API routes ──────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.APIAuth(sessionValidator))

		// Auth management
		r.Post("/api/v1/auth/logout", authHandler.APILogout)
		r.Put("/api/v1/auth/password", authHandler.ChangePassword)

		// Cluster CRUD
		r.Get("/api/v1/clusters", requireService(clusterSvc, clusterHandler.ListClusters))
		r.Post("/api/v1/clusters", requireService(clusterSvc, clusterHandler.CreateCluster))
		r.Get("/api/v1/clusters/{name}", requireService(clusterSvc, clusterHandler.GetCluster))
		r.Delete("/api/v1/clusters/{name}", requireService(clusterSvc, clusterHandler.DeleteCluster))
		r.Patch("/api/v1/clusters/{name}/scale", requireService(clusterSvc, clusterHandler.ScaleCluster))

		// Postgres config
		r.Get("/api/v1/clusters/{name}/postgres-config", requireService(configSvc, configHandler.GetPostgresConfig))
		r.Put("/api/v1/clusters/{name}/postgres-config", requireService(configSvc, configHandler.UpdatePostgresConfig))

		// Backups
		r.Get("/api/v1/clusters/{name}/backups", requireService(backupSvc, backupHandler.ListBackups))
		r.Post("/api/v1/clusters/{name}/backups", requireService(backupSvc, backupHandler.TriggerBackup))

		// Scheduled backups
		r.Get("/api/v1/clusters/{name}/scheduled-backups", requireService(backupSvc, backupHandler.ListScheduledBackups))
		r.Post("/api/v1/clusters/{name}/scheduled-backups", requireService(backupSvc, backupHandler.CreateScheduledBackup))
		r.Get("/api/v1/clusters/{name}/scheduled-backups/{id}", requireService(backupSvc, backupHandler.GetScheduledBackup))
		r.Delete("/api/v1/clusters/{name}/scheduled-backups/{id}", requireService(backupSvc, backupHandler.DeleteScheduledBackup))

		// Poolers (read-only)
		r.Get("/api/v1/clusters/{name}/poolers", requireService(poolerSvc, poolerHandler.ListPoolers))
		r.Get("/api/v1/clusters/{name}/poolers/{poolerName}", requireService(poolerSvc, poolerHandler.GetPooler))
	})

	// ── Protected UI routes ───────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.UIAuth(sessionValidator))
		r.Post("/ui/logout", authHandler.UILogout)
		// PR 6: UI routes for HTMX partials added here
	})

	// ── Login page (no auth) ─────────────────────────────────────────────────
	r.Get("/ui/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, loginPageHTML)
	})
	// ─────────────────────────────────────────────────────────────────────────

	srv := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down gracefully")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
	}

	return nil
}

// requireService returns a handler that responds 503 if the service is nil
// (i.e. no K8s client is available). This prevents nil pointer panics.
func requireService(svc interface{}, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Kubernetes client not available",
				"code":  "SERVICE_UNAVAILABLE",
			})
			return
		}
		h(w, r)
	}
}

// serveOpenAPISpec serves the OpenAPI specification as JSON.
// The YAML spec is embedded at build time from api/openapi.yaml.
func serveOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// The spec is served as a simple redirect message in this phase.
	// In PR 7, the openapi.yaml will be embedded with //go:embed and
	// converted to JSON. For now, we return the spec location.
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"spec": "/api/v1/openapi.json",
		"yaml": "api/openapi.yaml",
		"note": "Full OpenAPI spec available in api/openapi.yaml; JSON conversion added in PR7",
	})
}

// rejectAllValidator is a SessionValidator that rejects every session.
// It is used when no K8s client is available (dev / no-cluster mode).
type rejectAllValidator struct{}

func (r *rejectAllValidator) ValidateSession(_ string) (string, error) {
	return "", middleware.ErrUnauthorized
}

// newLogger constructs a structured slog.Logger at the requested level.
func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

// loginPageHTML is a minimal login page served at GET /ui/login.
// The full template will be replaced in PR 6 (UI layer).
const loginPageHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>CNPG UI — Login</title></head>
<body>
<h1>CNPG Web UI</h1>
<form method="POST" action="/ui/login">
  <label>Username: <input type="text" name="username" required autofocus></label><br>
  <label>Password: <input type="password" name="password" required></label><br>
  <button type="submit">Login</button>
</form>
</body>
</html>`
