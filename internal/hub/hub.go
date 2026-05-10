// Package hub implements a central SSE broker with topic-based pub/sub routing.
//
// Architecture: one Hub instance, one K8s watch, N browser subscribers.
// Topics follow a two-level scheme:
//   - "clusters" — receives all cluster events (broadcast)
//   - "cluster/{name}" — receives events for a single cluster (filtered)
//
// The hub uses a drop-oldest buffering strategy: when a subscriber's channel is
// full, the oldest event is discarded to make room for the new one. This prevents
// a slow consumer from blocking K8s event processing.
package hub

import (
	"fmt"
	"strings"
	"sync"
)

const subscriberBufferCap = 64

// Event is a single SSE message delivered to browser clients.
// It maps directly to the SSE text/event-stream wire format.
type Event struct {
	// Type is the SSE event name (e.g. "cluster.updated", "cluster.deleted").
	// Sent as "event: {Type}\n".
	Type string

	// Data is the JSON-encoded payload for this event.
	// Sent as "data: {Data}\n".
	Data []byte

	// ID is the SSE event id for client-side reconnection tracking.
	// Optional — omitted when empty.
	ID string

	// Retry is the suggested reconnection interval in milliseconds.
	// Sent as "retry: {Retry}\n" when non-zero.
	Retry int
}

// Format renders the Event as a standards-compliant SSE wire message.
// The returned string ends with a double newline (\n\n) as required by the SSE spec.
func (e Event) Format() string {
	var sb strings.Builder
	if e.Retry > 0 {
		fmt.Fprintf(&sb, "retry: %d\n", e.Retry)
	}
	if e.ID != "" {
		fmt.Fprintf(&sb, "id: %s\n", e.ID)
	}
	if e.Type != "" {
		fmt.Fprintf(&sb, "event: %s\n", e.Type)
	}
	fmt.Fprintf(&sb, "data: %s\n\n", e.Data)
	return sb.String()
}

// subscriber is an internal subscription record held by the hub.
type subscriber struct {
	ch    chan Event
	topic string
}

// Hub is the central SSE broker.
// All operations are goroutine-safe. Use New() to create a Hub.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscriber // topic → list of subscribers

	publish     chan publishMsg
	subscribe   chan subscribeMsg
	unsubscribe chan unsubscribeMsg
}

type publishMsg struct {
	topic string
	event Event
}

type subscribeMsg struct {
	topic string
	sub   *subscriber
	ack   chan struct{} // closed by hub once subscriber is registered
}

type unsubscribeMsg struct {
	topic string
	recv  <-chan Event // matches the <-chan returned by Subscribe
	ack   chan struct{} // closed by hub once subscriber is removed
}

// New creates a Hub ready to be started with Run(ctx).
func New() *Hub {
	return &Hub{
		subscribers: make(map[string][]*subscriber),
		publish:     make(chan publishMsg, 256),
		subscribe:   make(chan subscribeMsg, 64),
		unsubscribe: make(chan unsubscribeMsg, 64),
	}
}

// Subscribe registers a new subscriber for the given topic and returns a channel
// on which events will be delivered. The caller must eventually call Unsubscribe
// to clean up resources.
//
// Subscribe blocks until the hub has registered the subscriber, guaranteeing
// that any subsequent Publish call will reach this subscriber.
//
// The returned channel has a buffer of 64 events. Events are delivered via
// the hub's internal loop; Subscribe is safe to call from any goroutine.
func (h *Hub) Subscribe(topic string) <-chan Event {
	ch := make(chan Event, subscriberBufferCap)
	sub := &subscriber{ch: ch, topic: topic}
	ack := make(chan struct{})
	h.subscribe <- subscribeMsg{topic: topic, sub: sub, ack: ack}
	<-ack // wait until hub has registered us
	return ch
}

// Unsubscribe removes the subscriber identified by ch from the given topic.
// The ch parameter must be the value returned by Subscribe (same underlying
// channel). Unsubscribe blocks until the hub has removed the subscriber,
// guaranteeing that no further events will be delivered to ch after it returns.
func (h *Hub) Unsubscribe(topic string, ch <-chan Event) {
	ack := make(chan struct{})
	h.unsubscribe <- unsubscribeMsg{topic: topic, recv: ch, ack: ack}
	<-ack
}

// Publish sends an event to all current subscribers of the given topic.
// If a subscriber's buffer is full, the oldest buffered event is dropped
// to make room for the new one (drop-oldest strategy).
//
// Publish is safe to call from any goroutine and never blocks.
func (h *Hub) Publish(topic string, evt Event) {
	h.publish <- publishMsg{topic: topic, event: evt}
}

// Run starts the hub's event loop. It blocks until ctx is cancelled.
// Call Run in a dedicated goroutine: go hub.Run(ctx).
func (h *Hub) Run(ctx interface{ Done() <-chan struct{} }) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-h.subscribe:
			h.mu.Lock()
			h.subscribers[msg.topic] = append(h.subscribers[msg.topic], msg.sub)
			h.mu.Unlock()
			close(msg.ack) // signal caller that subscriber is registered

		case msg := <-h.unsubscribe:
			h.mu.Lock()
			subs := h.subscribers[msg.topic]
			newSubs := make([]*subscriber, 0, len(subs))
			for _, s := range subs {
				// Compare the receive-direction view of each subscriber channel
				// against the receive-direction channel from the caller.
				if (<-chan Event)(s.ch) != msg.recv {
					newSubs = append(newSubs, s)
				}
			}
			h.subscribers[msg.topic] = newSubs
			h.mu.Unlock()
			close(msg.ack) // signal caller that subscriber is removed

		case msg := <-h.publish:
			h.mu.RLock()
			subs := h.subscribers[msg.topic]
			h.mu.RUnlock()
			for _, s := range subs {
				deliverDropOldest(s.ch, msg.event)
			}
		}
	}
}

// deliverDropOldest attempts to send evt to ch. If ch is full, one event is
// discarded from the front (oldest) to make room for the new one.
// This keeps the subscriber channel from blocking the hub loop.
func deliverDropOldest(ch chan Event, evt Event) {
	select {
	case ch <- evt:
		// delivered without dropping
	default:
		// Channel full — discard the oldest event
		select {
		case <-ch:
		default:
		}
		// Try once more; still non-blocking in case another goroutine drained
		select {
		case ch <- evt:
		default:
		}
	}
}


