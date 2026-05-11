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
	"k8s.io/client-go/kubernetes"

	"github.com/dbonne/cnpg-ui/internal/auth"
	"github.com/dbonne/cnpg-ui/internal/backup"
	"github.com/dbonne/cnpg-ui/internal/cluster"
	"github.com/dbonne/cnpg-ui/internal/config"
	"github.com/dbonne/cnpg-ui/internal/hub"
	"github.com/dbonne/cnpg-ui/internal/k8s"
	"github.com/dbonne/cnpg-ui/internal/middleware"
	openapipkg "github.com/dbonne/cnpg-ui/internal/openapi"
	"github.com/dbonne/cnpg-ui/internal/pooler"
	"github.com/dbonne/cnpg-ui/internal/ui"
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

	k8sClient, restCfg, err := k8s.NewClientWithConfig(scheme)
	if err != nil {
		logger.Warn("K8s client unavailable — running without cluster access", "err", err)
	}

	// ── SSE Hub ───────────────────────────────────────────────────────────────
	// The hub is the central SSE broker. It is always created and started so that
	// SSE endpoints work even in dev mode (without a live K8s cluster). When a
	// K8s client is available, the watcher is wired into the informer to feed the hub.
	sseHub := hub.New()
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go sseHub.Run(hubCtx)

	if k8sClient != nil {
		// Pass the real REST config so the informer cache can connect to the API server.
		// When restCfg is nil (should not happen here since k8sClient != nil implies
		// a successful NewClientWithConfig call), NewInformerManager falls back to noop.
		informerMgr := k8s.NewInformerManager(restCfg, scheme)

		// Wire the watcher event handlers BEFORE starting the informer so we
		// receive the initial list events.
		watcher := hub.NewWatcher(sseHub)
		if err := informerMgr.AddClusterEventHandler(context.Background(), watcher.AsResourceEventHandler()); err != nil {
			logger.Warn("failed to register cluster event handler", "err", err)
		}

		go informerMgr.Start(context.Background())
	}
	// ─────────────────────────────────────────────────────────────────────────

	// TLS is enabled when both cert and key paths are configured.
	// This flag is used to select ListenAndServeTLS vs ListenAndServe and to
	// set the Secure flag on session cookies.
	tlsEnabled := cfg.TLSCertPath != "" && cfg.TLSKeyPath != ""

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

	authHandler := auth.NewHandler(authSvc, tlsEnabled)
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

	// ── SSE and log handlers ──────────────────────────────────────────────────
	sseHandler := hub.NewSSEHandler(sseHub)

	var logSvc cluster.LogService
	if k8sClient != nil {
		// Build a typed kubernetes.Interface from the same REST config used for the
		// controller-runtime client. This is required for pod log streaming because
		// controller-runtime client.Client does not support the logs API.
		typedClient, typedErr := kubernetes.NewForConfig(restCfg)
		if typedErr != nil {
			logger.Warn("failed to build typed K8s client for log streaming", "err", typedErr)
			typedClient = nil
		}
		logSvc = cluster.NewLogService(k8sClient, typedClient, ns)
	}
	logHandler := cluster.NewLogHandler(logSvc)
	// ─────────────────────────────────────────────────────────────────────────

	r := chi.NewRouter()
	// Middleware chain order (per design): Recovery → Logging → CORS → Router → Auth (per-group).
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.CORS(cfg.CORSOrigins))

	// Health check — no auth required.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// OpenAPI spec — no auth required.
	// /api/v1/openapi.yaml serves the embedded spec as YAML.
	// /api/v1/openapi.json serves the same spec converted to JSON.
	r.Get("/api/v1/openapi.yaml", serveOpenAPISpec)
	r.Get("/api/v1/openapi.json", serveOpenAPISpecJSON)

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
		r.Put("/api/v1/clusters/{name}", requireService(clusterSvc, clusterHandler.UpdateCluster))
		r.Delete("/api/v1/clusters/{name}", requireService(clusterSvc, clusterHandler.DeleteCluster))
		r.Patch("/api/v1/clusters/{name}/scale", requireService(clusterSvc, clusterHandler.ScaleCluster))

		// Postgres config
		r.Get("/api/v1/clusters/{name}/config", requireService(configSvc, configHandler.GetPostgresConfig))
		r.Put("/api/v1/clusters/{name}/config", requireService(configSvc, configHandler.UpdatePostgresConfig))

		// Backups
		r.Get("/api/v1/clusters/{name}/backups", requireService(backupSvc, backupHandler.ListBackups))
		r.Post("/api/v1/clusters/{name}/backups", requireService(backupSvc, backupHandler.TriggerBackup))

		// Scheduled backups
		r.Get("/api/v1/clusters/{name}/scheduled-backups", requireService(backupSvc, backupHandler.ListScheduledBackups))
		r.Post("/api/v1/clusters/{name}/scheduled-backups", requireService(backupSvc, backupHandler.CreateScheduledBackup))
		r.Get("/api/v1/clusters/{name}/scheduled-backups/{id}", requireService(backupSvc, backupHandler.GetScheduledBackup))
		r.Put("/api/v1/clusters/{name}/scheduled-backups/{id}", requireService(backupSvc, backupHandler.UpdateScheduledBackup))
		r.Delete("/api/v1/clusters/{name}/scheduled-backups/{id}", requireService(backupSvc, backupHandler.DeleteScheduledBackup))

		// Poolers (read-only)
		r.Get("/api/v1/clusters/{name}/poolers", requireService(poolerSvc, poolerHandler.ListPoolers))
		r.Get("/api/v1/clusters/{name}/poolers/{poolerName}", requireService(poolerSvc, poolerHandler.GetPooler))

		// SSE — realtime cluster events
		r.Get("/api/v1/events/clusters", sseHandler.AllClustersEvents)
		r.Get("/api/v1/clusters/{name}/events", sseHandler.ClusterEvents)

		// Log streaming
		r.Get("/api/v1/clusters/{name}/logs", requireService(logSvc, logHandler.GetLogs))
		r.Get("/api/v1/clusters/{name}/logs/stream", requireService(logSvc, logHandler.StreamLogs))
	})

	// ── UI handler (templates embedded at build time) ────────────────────────
	uiHandler, err := ui.NewUIHandler(clusterSvc, clusterSvc, backupSvc, poolerSvc, configSvc)
	if err != nil {
		return fmt.Errorf("initializing UI handler: %w", err)
	}

	// ── Login page (no auth) ─────────────────────────────────────────────────
	r.Get("/ui/login", uiHandler.LoginPage)
	// ─────────────────────────────────────────────────────────────────────────

	// ── Protected UI routes ───────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.UIAuth(sessionValidator))
		r.Post("/ui/logout", authHandler.UILogout)

		// Cluster UI pages
		r.Get("/ui/clusters", uiHandler.ListClusters)
		r.Get("/ui/clusters/{name}", uiHandler.ClusterDetail)

		// Panel partials — HTMX fragments loaded by tab switching in ClusterDetail
		r.Get("/ui/clusters/{name}/overview", uiHandler.OverviewPanel)
		r.Get("/ui/clusters/{name}/logs-panel", uiHandler.LogsPanel)
		r.Get("/ui/clusters/{name}/config-panel", uiHandler.ConfigPanel)
		r.Get("/ui/clusters/{name}/backups-panel", uiHandler.BackupsPanel)
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
		if tlsEnabled {
			logger.Info("server starting (TLS)", "addr", cfg.ServerAddr)
			if err := srv.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		} else {
			logger.Info("server starting", "addr", cfg.ServerAddr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down gracefully")
		// hubCancel is called via defer above; no explicit call needed here.
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

// serveOpenAPISpec serves the embedded OpenAPI specification as YAML.
// The spec is embedded at build time from internal/openapi/openapi.yaml
// (which mirrors api/openapi.yaml in the repository root).
func serveOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapipkg.SpecYAML)
}

// serveOpenAPISpecJSON serves the embedded OpenAPI specification as JSON.
// The YAML spec is converted to JSON once at startup in the openapi package.
func serveOpenAPISpecJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapipkg.SpecJSON())
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


