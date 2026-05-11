package hub_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/hub"
)

// TestSSEHandler_AllClusters_ResponseHeaders verifies that the /api/v1/events/clusters
// endpoint sets the correct SSE headers.
func TestSSEHandler_AllClusters_ResponseHeaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	handler := hub.NewSSEHandler(h)

	// Use a pipe-backed test server so we can cancel the request after
	// reading headers without waiting for the stream to close.
	reqCtx, reqCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer reqCancel()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/events/clusters", nil).WithContext(reqCtx)
	w := httptest.NewRecorder()

	// Run handler in goroutine — it blocks until request context is done.
	done := make(chan struct{})
	go func() {
		handler.AllClustersEvents(w, req)
		close(done)
	}()

	<-done

	resp := w.Result()
	assertHeader(t, resp, "Content-Type", "text/event-stream")
	assertHeader(t, resp, "Cache-Control", "no-cache")
	assertHeader(t, resp, "Connection", "keep-alive")
}

// TestSSEHandler_AllClusters_DeliverEvent verifies that an event published to
// the hub is received by a connected SSE client on /api/v1/events/clusters.
func TestSSEHandler_AllClusters_DeliverEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	handler := hub.NewSSEHandler(h)

	// Use a real HTTP test server so we can stream the response body.
	srv := httptest.NewServer(http.HandlerFunc(handler.AllClustersEvents))
	defer srv.Close()

	// Open the SSE stream
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET SSE stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Give the handler goroutine time to subscribe to the hub
	time.Sleep(50 * time.Millisecond)

	// Publish an event
	evt := hub.Event{Type: "cluster.updated", Data: []byte(`{"name":"pg-main"}`)}
	h.Publish("clusters", evt)

	// Read one SSE message from the response body
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			// SSE messages end with an empty line
			if line == "" {
				break
			}
		}
	}

	if len(lines) == 0 {
		t.Fatal("no SSE lines received from stream")
	}

	// Verify we got the event type line
	found := false
	for _, l := range lines {
		if strings.Contains(l, "cluster.updated") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("SSE output lines %v do not contain event type 'cluster.updated'", lines)
	}
}

// TestSSEHandler_SingleCluster_TopicFiltering verifies that the
// /api/v1/clusters/{name}/events endpoint subscribes to the correct topic.
func TestSSEHandler_SingleCluster_TopicFiltering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	handler := hub.NewSSEHandler(h)

	// Mount the handler under a chi router to enable URL param extraction
	r := chi.NewRouter()
	r.Get("/api/v1/clusters/{name}/events", handler.ClusterEvents)

	srv := httptest.NewServer(r)
	defer srv.Close()

	req2, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/clusters/pg-main/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("GET single cluster SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Give handler time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Publish to the specific topic only
	h.Publish("cluster/pg-main", hub.Event{
		Type: "cluster.updated",
		Data: []byte(`{"name":"pg-main"}`),
	})

	// Also publish to a different cluster — should NOT appear
	h.Publish("cluster/pg-replica", hub.Event{
		Type: "cluster.updated",
		Data: []byte(`{"name":"pg-replica"}`),
	})

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" {
				break
			}
		}
	}

	// We must have received the pg-main event
	foundPgMain := false
	for _, l := range lines {
		if strings.Contains(l, "pg-main") {
			foundPgMain = true
		}
		// pg-replica must NOT appear
		if strings.Contains(l, "pg-replica") {
			t.Errorf("single-cluster stream received event for wrong cluster: %q", l)
		}
	}
	if !foundPgMain {
		t.Errorf("single-cluster stream did not receive event for pg-main, lines: %v", lines)
	}
}

// assertHeader checks that a response header has the expected value.
func assertHeader(t *testing.T, resp *http.Response, key, want string) {
	t.Helper()
	got := resp.Header.Get(key)
	if got != want {
		t.Errorf("header %q = %q, want %q", key, got, want)
	}
}
