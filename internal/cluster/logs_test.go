package cluster_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/cluster"

	"github.com/go-chi/chi/v5"
)

// buildScheme returns a scheme with CNPG + core types registered.
func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = cnpgv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

// makeTestCluster builds a CNPG Cluster with the given primary pod name.
func makeTestCluster(name, primaryPod string) *cnpgv1.Cluster {
	return &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: cnpgv1.ClusterStatus{
			CurrentPrimary: primaryPod,
		},
	}
}

// ── LogsHandler ───────────────────────────────────────────────────────────────

// TestLogsHandler_HistoricalLogs_ClusterNotFound verifies 404 when cluster doesn't exist.
func TestLogsHandler_HistoricalLogs_ClusterNotFound(t *testing.T) {
	scheme := buildScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	logSvc := cluster.NewLogService(fakeClient, nil, "default")
	handler := cluster.NewLogHandler(logSvc)

	r := chi.NewRouter()
	r.Get("/api/v1/clusters/{name}/logs", handler.GetLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/nonexistent/logs", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GetLogs missing cluster: status = %d, want 404", w.Code)
	}
}

// TestLogsHandler_HistoricalLogs_NoPrimaryPod verifies 422 when cluster exists
// but has no current primary pod set.
func TestLogsHandler_HistoricalLogs_NoPrimaryPod(t *testing.T) {
	scheme := buildScheme()
	cl := makeTestCluster("pg-main", "") // no primary pod
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	logSvc := cluster.NewLogService(fakeClient, nil, "default")
	handler := cluster.NewLogHandler(logSvc)

	r := chi.NewRouter()
	r.Get("/api/v1/clusters/{name}/logs", handler.GetLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/pg-main/logs", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("GetLogs no primary: status = %d, want 422", w.Code)
	}
}

// TestLogsHandler_StreamLogs_SetsSseHeaders verifies that the streaming endpoint
// sets the correct SSE/streaming response headers.
func TestLogsHandler_StreamLogs_SetsSseHeaders(t *testing.T) {
	scheme := buildScheme()
	cl := makeTestCluster("pg-main", "pg-main-1")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pg-main-1",
			Namespace: "default",
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl, pod).
		Build()

	logSvc := cluster.NewLogService(fakeClient, nil, "default")
	handler := cluster.NewLogHandler(logSvc)

	r := chi.NewRouter()
	r.Get("/api/v1/clusters/{name}/logs/stream", handler.StreamLogs)

	// Use a context with a short timeout so the stream handler exits.
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer reqCancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/pg-main/logs/stream", nil).
		WithContext(reqCtx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(w, req)
		close(done)
	}()
	<-done

	resp := w.Result()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want \"text/event-stream\"", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want \"no-cache\"", cc)
	}
}

// TestLogsHandler_StreamLogs_ClusterNotFound verifies 404 for unknown cluster.
func TestLogsHandler_StreamLogs_ClusterNotFound(t *testing.T) {
	scheme := buildScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	logSvc := cluster.NewLogService(fakeClient, nil, "default")
	handler := cluster.NewLogHandler(logSvc)

	r := chi.NewRouter()
	r.Get("/api/v1/clusters/{name}/logs/stream", handler.StreamLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/unknown/logs/stream", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("StreamLogs missing cluster: status = %d, want 404", w.Code)
	}
}

// TestLogsHandler_LinesParam_DefaultAndCustom verifies that the ?lines= query parameter
// is parsed correctly (default 100, custom 50).
func TestLogsHandler_LinesParam_DefaultAndCustom(t *testing.T) {
	// We test the pure parameter-parsing function directly to avoid K8s pod logs API.
	tests := []struct {
		query string
		want  int64
	}{
		{"", 100},
		{"?lines=50", 50},
		{"?lines=abc", 100}, // invalid → default
		{"?lines=0", 100},   // zero → default
		{"?lines=500", 500},
	}
	for _, tt := range tests {
		got := cluster.ParseLinesParam(tt.query)
		if got != tt.want {
			t.Errorf("ParseLinesParam(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// TestLogsHandler_StreamOutput_ContainsPodName verifies that the streaming endpoint
// includes the pod name in its SSE output (integration-level check using fake server).
func TestLogsHandler_StreamOutput_ContainsPodName(t *testing.T) {
	scheme := buildScheme()
	cl := makeTestCluster("pg-main", "pg-main-1")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pg-main-1",
			Namespace: "default",
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl, pod).
		Build()

	logSvc := cluster.NewLogService(fakeClient, nil, "default")
	handler := cluster.NewLogHandler(logSvc)

	r := chi.NewRouter()
	r.Get("/api/v1/clusters/{name}/logs/stream", handler.StreamLogs)

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/clusters/pg-main/logs/stream")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()

	// The fake K8s client doesn't produce real pod logs, but the handler should
	// at minimum emit a metadata event with the pod name.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) && scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.Contains(line, "pg-main-1") {
			break
		}
	}

	found := false
	for _, l := range lines {
		if strings.Contains(l, "pg-main-1") {
			found = true
			break
		}
	}
	if !found {
		t.Logf("lines received: %v", lines)
		t.Error("stream output does not contain pod name 'pg-main-1'")
	}
}
