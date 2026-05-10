package hub

import (
	"encoding/json"
	"fmt"
	"log/slog"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	toolscache "k8s.io/client-go/tools/cache"

	"github.com/dbonne/cnpg-ui/internal/k8s"
)

// clusterEventPayload is the JSON payload published to the hub for each cluster event.
// It contains the minimal fields needed by browser clients to update their UI state.
type clusterEventPayload struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Status         string `json:"status"`
	Phase          string `json:"phase,omitempty"`
	CurrentPrimary string `json:"currentPrimary,omitempty"`
	ReadyInstances int    `json:"readyInstances"`
	Instances      int    `json:"instances"`
}

// Watcher handles K8s informer events for CNPG Cluster CRs and publishes
// normalized events to the Hub. It implements the three informer event handler
// functions: OnAdd, OnUpdate, OnDelete.
//
// The Watcher is stateless — it maps incoming events to hub publications without
// maintaining its own state. Reconnection is handled by the informer framework.
type Watcher struct {
	hub *Hub
}

// NewWatcher creates a Watcher that publishes events to the given Hub.
func NewWatcher(h *Hub) *Watcher {
	return &Watcher{hub: h}
}

// AsResourceEventHandler returns a toolscache.ResourceEventHandler that delegates
// to this watcher's OnAdd/OnUpdate/OnDelete methods. Use this to register the
// watcher with the informer cache via InformerManager.AddClusterEventHandler.
func (w *Watcher) AsResourceEventHandler() toolscache.ResourceEventHandler {
	return toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.OnAdd(obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { w.OnUpdate(oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { w.OnDelete(obj) },
	}
}

// OnAdd is called when a new Cluster CR is observed by the informer.
// It publishes a "cluster.created" event to both "clusters" and "cluster/{name}".
func (w *Watcher) OnAdd(obj interface{}) {
	cl, ok := toCluster(obj)
	if !ok {
		return
	}
	w.publish(cl, "cluster.created")
}

// OnUpdate is called when an existing Cluster CR is updated.
// It publishes a "cluster.updated" event to both "clusters" and "cluster/{name}".
//
// Both oldObj and newObj are provided by the informer; the watcher uses only
// newObj to build the event payload.
func (w *Watcher) OnUpdate(oldObj, newObj interface{}) {
	cl, ok := toCluster(newObj)
	if !ok {
		return
	}
	w.publish(cl, "cluster.updated")
}

// OnDelete is called when a Cluster CR is removed.
// It publishes a "cluster.deleted" event to both "clusters" and "cluster/{name}".
func (w *Watcher) OnDelete(obj interface{}) {
	cl, ok := toCluster(obj)
	if !ok {
		return
	}
	w.publish(cl, "cluster.deleted")
}

// publish encodes the cluster state and publishes it to the hub on both the
// "clusters" broadcast topic and the "cluster/{name}" filtered topic.
func (w *Watcher) publish(cl *cnpgv1.Cluster, eventType string) {
	payload := buildPayload(cl)
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("watcher: failed to marshal cluster event", "err", err, "cluster", cl.Name)
		return
	}

	evt := Event{
		Type: eventType,
		Data: data,
	}

	broadcastTopic := "clusters"
	specificTopic := fmt.Sprintf("cluster/%s", cl.Name)

	w.hub.Publish(broadcastTopic, evt)
	w.hub.Publish(specificTopic, evt)
}

// buildPayload converts a CNPG Cluster CR to the SSE event payload.
// This is a pure function — it has no side effects and is easy to test.
func buildPayload(cl *cnpgv1.Cluster) clusterEventPayload {
	return clusterEventPayload{
		Name:           cl.Name,
		Namespace:      cl.Namespace,
		Status:         string(k8s.MapPhaseToStatus(cl.Status.Phase)),
		Phase:          cl.Status.Phase,
		CurrentPrimary: cl.Status.CurrentPrimary,
		ReadyInstances: cl.Status.ReadyInstances,
		Instances:      cl.Spec.Instances,
	}
}

// toCluster safely casts obj to *cnpgv1.Cluster.
// Returns (nil, false) if the cast fails (e.g. tombstone objects from the informer).
func toCluster(obj interface{}) (*cnpgv1.Cluster, bool) {
	cl, ok := obj.(*cnpgv1.Cluster)
	if !ok {
		slog.Warn("watcher: unexpected object type in cluster event", "type", fmt.Sprintf("%T", obj))
	}
	return cl, ok
}
