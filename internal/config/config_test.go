package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/dbonne/cnpg-ui/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any env vars that might interfere
	clearEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.ServerAddr != ":8080" {
		t.Errorf("ServerAddr: got %q, want %q", cfg.ServerAddr, ":8080")
	}
	if cfg.K8sNamespace != "default" {
		t.Errorf("K8sNamespace: got %q, want %q", cfg.K8sNamespace, "default")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL: got %v, want %v", cfg.SessionTTL, 24*time.Hour)
	}
	if cfg.SecretName != "cnpg-ui-credentials" {
		t.Errorf("SecretName: got %q, want %q", cfg.SecretName, "cnpg-ui-credentials")
	}
	if cfg.TLSCertPath != "" {
		t.Errorf("TLSCertPath: got %q, want empty", cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != "" {
		t.Errorf("TLSKeyPath: got %q, want empty", cfg.TLSKeyPath)
	}
}

func TestLoad_OverridesFromEnv(t *testing.T) {
	clearEnv(t)

	t.Setenv("CNPG_UI_PORT", "9090")
	t.Setenv("CNPG_UI_NAMESPACE", "production")
	t.Setenv("CNPG_UI_LOG_LEVEL", "debug")
	t.Setenv("CNPG_UI_SESSION_TTL", "12h")
	t.Setenv("CNPG_UI_SECRET_NAME", "my-secret")
	t.Setenv("CNPG_UI_TLS_CERT", "/path/to/cert.pem")
	t.Setenv("CNPG_UI_TLS_KEY", "/path/to/key.pem")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.ServerAddr != ":9090" {
		t.Errorf("ServerAddr: got %q, want %q", cfg.ServerAddr, ":9090")
	}
	if cfg.K8sNamespace != "production" {
		t.Errorf("K8sNamespace: got %q, want %q", cfg.K8sNamespace, "production")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("SessionTTL: got %v, want %v", cfg.SessionTTL, 12*time.Hour)
	}
	if cfg.SecretName != "my-secret" {
		t.Errorf("SecretName: got %q, want %q", cfg.SecretName, "my-secret")
	}
	if cfg.TLSCertPath != "/path/to/cert.pem" {
		t.Errorf("TLSCertPath: got %q, want %q", cfg.TLSCertPath, "/path/to/cert.pem")
	}
	if cfg.TLSKeyPath != "/path/to/key.pem" {
		t.Errorf("TLSKeyPath: got %q, want %q", cfg.TLSKeyPath, "/path/to/key.pem")
	}
}

func TestLoad_InvalidSessionTTL(t *testing.T) {
	clearEnv(t)
	t.Setenv("CNPG_UI_SESSION_TTL", "notaduration")

	_, err := config.Load()
	if err == nil {
		t.Error("Load() should return error for invalid SESSION_TTL, got nil")
	}
}

func TestLoad_ServerAddrFormat(t *testing.T) {
	tests := []struct {
		port     string
		wantAddr string
	}{
		{port: "8080", wantAddr: ":8080"},
		{port: "3000", wantAddr: ":3000"},
		{port: "443", wantAddr: ":443"},
	}

	for _, tc := range tests {
		t.Run("port="+tc.port, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("CNPG_UI_PORT", tc.port)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.ServerAddr != tc.wantAddr {
				t.Errorf("ServerAddr: got %q, want %q", cfg.ServerAddr, tc.wantAddr)
			}
		})
	}
}

// TestLoad_CORSOrigins verifies that CNPG_UI_CORS_ORIGINS is parsed into
// a slice of trimmed origin strings.
func TestLoad_CORSOrigins(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    []string
	}{
		{
			name:   "empty env var means same-origin only",
			envVal: "",
			want:   nil,
		},
		{
			name:   "single origin",
			envVal: "https://example.com",
			want:   []string{"https://example.com"},
		},
		{
			name:   "multiple origins comma-separated",
			envVal: "https://example.com, https://app.example.com",
			want:   []string{"https://example.com", "https://app.example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			if tc.envVal != "" {
				t.Setenv("CNPG_UI_CORS_ORIGINS", tc.envVal)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load(): %v", err)
			}
			if len(cfg.CORSOrigins) != len(tc.want) {
				t.Errorf("CORSOrigins len = %d, want %d (%v)", len(cfg.CORSOrigins), len(tc.want), cfg.CORSOrigins)
				return
			}
			for i, o := range cfg.CORSOrigins {
				if o != tc.want[i] {
					t.Errorf("CORSOrigins[%d] = %q, want %q", i, o, tc.want[i])
				}
			}
		})
	}
}

// clearEnv removes all CNPG_UI_ environment variables for a clean test state.
func clearEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"CNPG_UI_PORT",
		"CNPG_UI_NAMESPACE",
		"CNPG_UI_LOG_LEVEL",
		"CNPG_UI_SESSION_TTL",
		"CNPG_UI_SECRET_NAME",
		"CNPG_UI_TLS_CERT",
		"CNPG_UI_TLS_KEY",
		"CNPG_UI_CORS_ORIGINS",
	}
	for _, v := range vars {
		t.Setenv(v, "") // t.Setenv restores on cleanup
		os.Unsetenv(v)  //nolint:errcheck
	}
}
