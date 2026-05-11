// Package config loads and validates application configuration from environment
// variables and command-line flags (12-factor style).
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration for cnpg-ui.
type Config struct {
	// ServerAddr is the TCP address the HTTP server listens on (e.g. ":8080").
	ServerAddr string

	// K8sNamespace is the Kubernetes namespace to watch for CNPG resources.
	K8sNamespace string

	// SessionTTL is how long an authenticated session remains valid.
	SessionTTL time.Duration

	// SecretName is the name of the K8s Secret holding the auth credentials.
	SecretName string

	// LogLevel controls the verbosity of structured logging (debug, info, warn, error).
	LogLevel string

	// TLSCertPath is the path to the TLS certificate file. Empty means HTTP only.
	TLSCertPath string

	// TLSKeyPath is the path to the TLS private key file. Empty means HTTP only.
	TLSKeyPath string

	// CORSOrigins is the list of allowed CORS origins parsed from
	// CNPG_UI_CORS_ORIGINS (comma-separated). Empty means same-origin only.
	CORSOrigins []string
}

// Load reads configuration from environment variables, applying defaults for
// missing values. It returns an error if any value cannot be parsed.
func Load() (*Config, error) {
	port := getEnvOrDefault("CNPG_UI_LISTEN_PORT", "8080")

	sessionTTLRaw := getEnvOrDefault("CNPG_UI_SESSION_TTL", "24h")
	sessionTTL, err := time.ParseDuration(sessionTTLRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid CNPG_UI_SESSION_TTL %q: %w", sessionTTLRaw, err)
	}

	var corsOrigins []string
	if raw := os.Getenv("CNPG_UI_CORS_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				corsOrigins = append(corsOrigins, trimmed)
			}
		}
	}

	return &Config{
		ServerAddr:   ":" + port,
		K8sNamespace: getEnvOrDefault("CNPG_UI_NAMESPACE", "default"),
		SessionTTL:   sessionTTL,
		SecretName:   getEnvOrDefault("CNPG_UI_SECRET_NAME", "cnpg-ui-credentials"),
		LogLevel:     getEnvOrDefault("CNPG_UI_LOG_LEVEL", "info"),
		TLSCertPath:  os.Getenv("CNPG_UI_TLS_CERT"),
		TLSKeyPath:   os.Getenv("CNPG_UI_TLS_KEY"),
		CORSOrigins:  corsOrigins,
	}, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
