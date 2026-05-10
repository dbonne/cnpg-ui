// Package hub provides the central SSE broker that connects K8s watches to browser subscribers.
package hub_test

import (
	"context"
	"testing"
	"time"

	"github.com/dbonne/cnpg-ui/internal/hub"
)

// ── Event formatting ──────────────────────────────────────────────────────────

func TestEventFormat_WithAllFields(t *testing.T) {
	evt := hub.Event{
		Type:  "cluster.updated",
		Data:  []byte(`{"name":"pg-main","status":"HEALTHY"}`),
		ID:    "evt-1",
		Retry: 3000,
	}
	got := evt.Format()
	want := "retry: 3000\nid: evt-1\nevent: cluster.updated\ndata: {\"name\":\"pg-main\",\"status\":\"HEALTHY\"}\n\n"
	if got != want {
		t.Errorf("Event.Format() =\n%q\nwant\n%q", got, want)
	}
}

func TestEventFormat_WithoutOptionalFields(t *testing.T) {
	evt := hub.Event{
		Data: []byte(`{"name":"pg-replica","status":"FAULT"}`),
	}
	got := evt.Format()
	// No retry, no id, no event type — only data + blank line.
	want := "data: {\"name\":\"pg-replica\",\"status\":\"FAULT\"}\n\n"
	if got != want {
		t.Errorf("Event.Format() without optionals =\n%q\nwant\n%q", got, want)
	}
}

// ── Hub pub/sub ───────────────────────────────────────────────────────────────

func TestHub_SubscribeReceivesPublishedEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	ch := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", ch)

	evt := hub.Event{Type: "cluster.updated", Data: []byte(`{}`)}
	h.Publish("clusters", evt)

	select {
	case received := <-ch:
		if received.Type != evt.Type {
			t.Errorf("received event type %q, want %q", received.Type, evt.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: subscriber did not receive published event")
	}
}

func TestHub_TopicRouting_DoesNotDeliverToOtherTopics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	// Subscribe to a specific cluster topic
	chSpecific := h.Subscribe("cluster/pg-main")
	defer h.Unsubscribe("cluster/pg-main", chSpecific)

	// Subscribe to all-clusters topic
	chAll := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", chAll)

	// Publish only to "clusters" topic
	evt := hub.Event{Type: "cluster.updated", Data: []byte(`{}`)}
	h.Publish("clusters", evt)

	// "clusters" subscriber should receive it
	select {
	case <-chAll:
		// correct
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: all-clusters subscriber did not receive event")
	}

	// "cluster/pg-main" subscriber should NOT receive it
	select {
	case unexpected := <-chSpecific:
		t.Errorf("specific subscriber received unexpected event: %+v", unexpected)
	case <-time.After(50 * time.Millisecond):
		// correct — no cross-topic delivery
	}
}

func TestHub_MultipleSubscribersSameTopic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	ch1 := h.Subscribe("clusters")
	ch2 := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", ch1)
	defer h.Unsubscribe("clusters", ch2)

	evt := hub.Event{Type: "cluster.created", Data: []byte(`{}`)}
	h.Publish("clusters", evt)

	for i, ch := range []<-chan hub.Event{ch1, ch2} {
		select {
		case received := <-ch:
			if received.Type != evt.Type {
				t.Errorf("subscriber %d: received type %q, want %q", i+1, received.Type, evt.Type)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timeout: subscriber %d did not receive event", i+1)
		}
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	ch := h.Subscribe("clusters")
	h.Unsubscribe("clusters", ch)

	// Give hub goroutine time to process the unsubscribe
	time.Sleep(20 * time.Millisecond)

	evt := hub.Event{Type: "cluster.updated", Data: []byte(`{}`)}
	h.Publish("clusters", evt)

	select {
	case unexpected := <-ch:
		t.Errorf("unsubscribed channel still received event: %+v", unexpected)
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

func TestHub_ContextCancelStopsHub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := hub.New()

	done := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// correct — hub exited on ctx cancel
	case <-time.After(500 * time.Millisecond):
		t.Fatal("hub did not stop after context cancel")
	}
}

func TestHub_DropOldestOnBufferFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	ch := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", ch)

	// Fill the subscriber buffer (capacity 64) + a few extra that trigger drop-oldest
	for i := range 70 {
		h.Publish("clusters", hub.Event{
			Type: "cluster.updated",
			Data: []byte(`{}`),
			ID:   string(rune('A' + i%26)),
		})
	}

	// Give the hub time to process all publishes
	time.Sleep(50 * time.Millisecond)

	// We should be able to drain the channel (it didn't block/deadlock)
	count := 0
	drain:
	for {
		select {
		case <-ch:
			count++
		default:
			break drain
		}
	}

	// We published 70 events into a buffer of 64; we should have at most 64 events
	// (oldest were dropped). We should have received some events.
	if count == 0 {
		t.Error("subscriber received 0 events after 70 publishes")
	}
	if count > 64 {
		t.Errorf("subscriber received %d events, but buffer capacity is 64", count)
	}
}
