package hub

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// SSEHandler provides HTTP handlers for SSE event streams.
// Each handler subscribes to a hub topic and streams events to the connected client
// using the text/event-stream wire format.
type SSEHandler struct {
	hub *Hub
}

// NewSSEHandler creates an SSEHandler backed by the given Hub.
func NewSSEHandler(h *Hub) *SSEHandler {
	return &SSEHandler{hub: h}
}

// AllClustersEvents handles GET /api/v1/events/clusters.
// It opens an SSE stream that delivers events for ALL cluster changes.
// The connection stays open until the client disconnects or the server shuts down.
func (h *SSEHandler) AllClustersEvents(w http.ResponseWriter, r *http.Request) {
	h.streamTopic(w, r, "clusters")
}

// ClusterEvents handles GET /api/v1/clusters/{name}/events.
// It opens an SSE stream that delivers events for a SINGLE named cluster.
// The connection stays open until the client disconnects or the server shuts down.
func (h *SSEHandler) ClusterEvents(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	topic := fmt.Sprintf("cluster/%s", name)
	h.streamTopic(w, r, topic)
}

// streamTopic is the shared SSE streaming implementation.
// It:
//  1. Asserts that the ResponseWriter supports http.Flusher (required for streaming)
//  2. Sets SSE response headers
//  3. Subscribes to the given hub topic
//  4. Streams events until the client disconnects (request context cancelled)
//  5. Unsubscribes on return
func (h *SSEHandler) streamTopic(w http.ResponseWriter, r *http.Request, topic string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE required headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Disable buffering in proxies/nginx that support X-Accel-Buffering.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Subscribe to the topic. Unsubscribe when we return (client disconnect or shutdown).
	ch := h.hub.Subscribe(topic)
	defer h.hub.Unsubscribe(topic, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected — stop streaming.
			return
		case evt, ok := <-ch:
			if !ok {
				// Channel closed — hub shut down.
				return
			}
			_, _ = fmt.Fprint(w, evt.Format())
			flusher.Flush()
		}
	}
}
