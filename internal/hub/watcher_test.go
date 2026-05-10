package hub_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dbonne/cnpg-ui/internal/hub"
)

// makeCluster is a test helper that builds a minimal CNPG Cluster.
func makeCluster(name, phase string) *cnpgv1.Cluster {
	return &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: cnpgv1.ClusterStatus{
			Phase: phase,
		},
	}
}

// TestWatcher_AddPublishesToClustersAndSpecificTopic verifies that when a cluster
// Add event is fired, the watcher publishes to both "clusters" and "cluster/{name}".
func TestWatcher_AddPublishesToClustersAndSpecificTopic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	chAll := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", chAll)

	chSpecific := h.Subscribe("cluster/pg-main")
	defer h.Unsubscribe("cluster/pg-main", chSpecific)

	w := hub.NewWatcher(h)
	w.OnAdd(makeCluster("pg-main", "Cluster in healthy state"))

	expectEvent(t, chAll, "cluster.created", "pg-main")
	expectEvent(t, chSpecific, "cluster.created", "pg-main")
}

// TestWatcher_UpdatePublishesCorrectEventType verifies that an Update event
// produces a "cluster.updated" event type on both topics.
func TestWatcher_UpdatePublishesCorrectEventType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	chAll := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", chAll)

	w := hub.NewWatcher(h)
	w.OnUpdate(
		makeCluster("pg-main", "Cluster in healthy state"),
		makeCluster("pg-main", "Switchover in progress"),
	)

	expectEvent(t, chAll, "cluster.updated", "pg-main")
}

// TestWatcher_DeletePublishesCorrectEventType verifies that a Delete event
// produces a "cluster.deleted" event type.
func TestWatcher_DeletePublishesCorrectEventType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	chAll := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", chAll)

	w := hub.NewWatcher(h)
	w.OnDelete(makeCluster("pg-replica", "Cluster in healthy state"))

	expectEvent(t, chAll, "cluster.deleted", "pg-replica")
}

// TestWatcher_EventDataContainsNormalizedStatus verifies that the published
// event data includes the normalized status field derived from the CNPG phase.
func TestWatcher_EventDataContainsNormalizedStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New()
	go h.Run(ctx)

	chAll := h.Subscribe("clusters")
	defer h.Unsubscribe("clusters", chAll)

	w := hub.NewWatcher(h)
	// PhaseHealthy → HEALTHY status
	w.OnAdd(makeCluster("pg-main", "Cluster in healthy state"))

	select {
	case evt := <-chAll:
		var payload map[string]interface{}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			t.Fatalf("event data is not valid JSON: %v", err)
		}
		status, ok := payload["status"].(string)
		if !ok {
			t.Fatal("event data missing 'status' field")
		}
		if status != "HEALTHY" {
			t.Errorf("event status = %q, want %q", status, "HEALTHY")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: no event received")
	}
}

// expectEvent is a test helper that reads from ch and asserts the event type and
// cluster name in the data payload.
func expectEvent(t *testing.T, ch <-chan hub.Event, wantType, wantName string) {
	t.Helper()
	select {
	case evt := <-ch:
		if evt.Type != wantType {
			t.Errorf("event type = %q, want %q", evt.Type, wantType)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			t.Fatalf("event data is not valid JSON: %v", err)
		}
		name, _ := payload["name"].(string)
		if name != wantName {
			t.Errorf("event data.name = %q, want %q", name, wantName)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout: no event of type %q received on channel", wantType)
	}
}
