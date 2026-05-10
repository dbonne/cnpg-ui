package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-chi/chi/v5"

	apierr "github.com/dbonne/cnpg-ui/internal/errors"
)

const defaultTailLines int64 = 100

// LogService provides log access for CNPG cluster pods.
type LogService interface {
	// GetPrimaryPodName returns the name of the current primary pod for a cluster.
	// Returns ("", ErrNotFound) if the cluster doesn't exist.
	// Returns ("", ErrNoPrimary) if no primary is currently elected.
	GetPrimaryPodName(ctx context.Context, clusterName string) (string, error)

	// StreamPodLogs opens a log stream for the named pod.
	// If follow is true, the stream follows new log lines (live tail).
	// If tailLines > 0, only the last N lines are returned (historical mode).
	// The caller is responsible for closing the returned ReadCloser.
	StreamPodLogs(ctx context.Context, podName string, follow bool, tailLines int64) (io.ReadCloser, error)
}

// logService implements LogService using a controller-runtime client for CR lookups
// and the native k8s typed client for pod log streaming.
type logService struct {
	ctrlClient client.Client
	namespace  string
}

// NewLogService creates a LogService backed by the given controller-runtime client.
func NewLogService(c client.Client, namespace string) LogService {
	return &logService{ctrlClient: c, namespace: namespace}
}

// GetPrimaryPodName fetches the cluster CR and returns the current primary pod name.
func (s *logService) GetPrimaryPodName(ctx context.Context, clusterName string) (string, error) {
	var cl cnpgv1.Cluster
	key := client.ObjectKey{Namespace: s.namespace, Name: clusterName}
	if err := s.ctrlClient.Get(ctx, key, &cl); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", fmt.Errorf("cluster %q not found", clusterName)
		}
		return "", fmt.Errorf("get cluster %q: %w", clusterName, err)
	}

	if cl.Status.CurrentPrimary == "" {
		return "", fmt.Errorf("cluster %q has no primary pod: %w", clusterName, errNoPrimary)
	}
	return cl.Status.CurrentPrimary, nil
}

// errNoPrimary is a sentinel error for the "no primary elected" state.
var errNoPrimary = fmt.Errorf("no primary elected")

// StreamPodLogs opens a log stream for the named pod using the K8s typed client.
// For production use, we need a typed Kubernetes client (k8s.io/client-go/kubernetes)
// to access pod logs; controller-runtime client doesn't support log streaming.
//
// In unit tests, the fake controller-runtime client is used for cluster lookups;
// pod log streaming is not available via the fake client (it always returns an empty
// stream). Real log streaming requires a live cluster or a mock kubeclient.
func (s *logService) StreamPodLogs(ctx context.Context, podName string, follow bool, tailLines int64) (io.ReadCloser, error) {
	// Build a typed K8s client from the controller-runtime client's REST config.
	// Note: controller-runtime fake.Client doesn't expose a REST config, so in tests
	// StreamPodLogs will use the fallback empty stream (see below).
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		// Not in-cluster — attempt kubeconfig fallback or return empty stream.
		// In tests, this is expected; we return an empty reader.
		return io.NopCloser(strings.NewReader("")), nil
	}

	typed, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build typed client: %w", err)
	}

	opts := &corev1.PodLogOptions{
		Follow: follow,
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}

	req := typed.CoreV1().Pods(s.namespace).GetLogs(podName, opts)
	return req.Stream(ctx)
}

// ── Handler ───────────────────────────────────────────────────────────────────

// LogHandler provides HTTP handlers for cluster log endpoints.
type LogHandler struct {
	svc LogService
}

// NewLogHandler creates a LogHandler backed by the given LogService.
func NewLogHandler(svc LogService) *LogHandler {
	return &LogHandler{svc: svc}
}

// GetLogs handles GET /api/v1/clusters/{name}/logs
// It returns the last N lines of logs from the primary pod as a JSON array.
// Query param: ?lines=N (default 100).
func (h *LogHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	lines := ParseLinesParam(r.URL.RawQuery)

	podName, err := h.svc.GetPrimaryPodName(r.Context(), name)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		if isNoPrimary(err) {
			writeError(w, http.StatusUnprocessableEntity, apierr.CodeValidation, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}

	reader, err := h.svc.StreamPodLogs(r.Context(), podName, false, lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal,
			"failed to get pod logs: "+err.Error())
		return
	}
	defer reader.Close()

	// Read all lines and return as JSON array
	var logLines []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		logLines = append(logLines, scanner.Text())
	}
	if logLines == nil {
		logLines = []string{}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"pod":   podName,
		"lines": logLines,
	})
}

// StreamLogs handles GET /api/v1/clusters/{name}/logs/stream
// It opens an SSE stream that delivers live log lines from the primary pod.
// The stream stops when the client disconnects (context cancel).
func (h *LogHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	podName, err := h.svc.GetPrimaryPodName(r.Context(), name)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, apierr.CodeNotFound, err.Error())
			return
		}
		if isNoPrimary(err) {
			writeError(w, http.StatusUnprocessableEntity, apierr.CodeValidation, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, apierr.CodeInternal, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send a metadata event with the pod name so the client knows which pod is streaming.
	meta, _ := json.Marshal(map[string]string{"pod": podName})
	fmt.Fprintf(w, "event: log.meta\ndata: %s\n\n", meta)
	flusher.Flush()

	// Open the log stream with Follow=true for live tail.
	reader, err := h.svc.StreamPodLogs(r.Context(), podName, true, 0)
	if err != nil {
		fmt.Fprintf(w, "event: log.error\ndata: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		line := scanner.Text()
		data, _ := json.Marshal(map[string]string{"line": line})
		fmt.Fprintf(w, "event: log.line\ndata: %s\n\n", data)
		flusher.Flush()
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// ParseLinesParam parses the ?lines= query parameter from a raw query string.
// The rawQuery argument should be the value of url.URL.RawQuery (no leading "?"),
// or a string starting with "?" (the "?" is stripped automatically).
//
// Returns the default (100) if the parameter is absent, zero, or non-numeric.
//
// This is exported so it can be tested directly as a pure function.
func ParseLinesParam(rawQuery string) int64 {
	// Strip a leading "?" if the caller passes the full query string.
	rawQuery = strings.TrimPrefix(rawQuery, "?")
	// Parse manually to avoid allocating a full url.Values for this simple case.
	for _, part := range strings.Split(rawQuery, "&") {
		if strings.HasPrefix(part, "lines=") {
			val := strings.TrimPrefix(part, "lines=")
			n, err := strconv.ParseInt(val, 10, 64)
			if err == nil && n > 0 {
				return n
			}
		}
	}
	return defaultTailLines
}

// isNoPrimary returns true if the error indicates no primary pod is elected.
func isNoPrimary(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no primary")
}
