// Command cnpg-ui starts the CNPG Web UI server.
// It loads configuration from environment variables, sets up the HTTP server,
// and wires dependencies following a 12-factor application approach.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/dbonne/cnpg-ui/internal/config"
	"github.com/dbonne/cnpg-ui/internal/k8s"
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
	// Build the runtime scheme with all CNPG CRD types registered.
	scheme := k8s.NewScheme()

	// Attempt to create a real K8s client (in-cluster config with kubeconfig fallback).
	// In dev environments without a running cluster this will log a warning and continue.
	k8sClient, err := k8s.NewClient(scheme)
	if err != nil {
		logger.Warn("K8s client unavailable — running without cluster access", "err", err)
	}

	if k8sClient != nil {
		clusterReader := k8s.NewClusterReader(k8sClient)
		_ = clusterReader // wired in PR 4 when service layer is added

		// Informer manager — syncs CRD caches in background.
		// Production wiring: pass rest.Config separately (done when cfg is threaded through).
		// For now, no-op is fine (no real cluster in dev).
		informerMgr := k8s.NewInformerManager(nil, scheme)
		go informerMgr.Start(context.Background())
		_ = informerMgr.WaitForSync
	}
	// ─────────────────────────────────────────────────────────────────────────

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health check — no auth required.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	srv := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
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
