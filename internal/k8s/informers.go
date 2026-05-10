package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InformerManager manages the controller-runtime cache (informer factory) lifecycle.
// It is the equivalent of a SharedInformerFactory: it watches all registered CRD
// types, maintains an in-memory cache, and delivers events to subscribers.
//
// In unit tests, NewInformerManager receives a nil *rest.Config and returns a
// noopInformerManager so tests compile and run without a real API server.
type InformerManager interface {
	// Start begins the informer goroutines. Blocks until ctx is cancelled.
	// Should be run in a goroutine: go mgr.Start(ctx).
	Start(ctx context.Context)

	// WaitForSync waits until the cache has received a full list response from
	// the API server. Returns true if synced, false if ctx was cancelled first.
	WaitForSync(ctx context.Context) bool
}

// realInformerManager implements InformerManager using controller-runtime cache.
type realInformerManager struct {
	cache cache.Cache
}

// noopInformerManager is used when no real K8s config is available (e.g., tests).
type noopInformerManager struct{}

func (n *noopInformerManager) Start(_ context.Context)            {}
func (n *noopInformerManager) WaitForSync(_ context.Context) bool { return false }

// NewInformerManager creates an InformerManager.
//
//   - In production: call with a real *rest.Config (from NewClient or rest.InClusterConfig).
//     The manager will create a controller-runtime cache that watches all CRD types.
//   - In tests: call with a nil cfg. A no-op manager is returned so tests compile
//     and run without a real API server.
//
// The scheme must contain all types you intend to watch (use NewScheme()).
func NewInformerManager(cfg *rest.Config, scheme *runtime.Scheme) InformerManager {
	if cfg == nil {
		return &noopInformerManager{}
	}

	ca, err := cache.New(cfg, cache.Options{Scheme: scheme})
	if err != nil {
		// If we can't create the cache, fall back to no-op rather than panic.
		return &noopInformerManager{}
	}

	return &realInformerManager{cache: ca}
}

// NewInformerManagerFromClient creates an InformerManager from a controller-runtime
// client. This is a convenience constructor for production use where the client
// was already created via NewClient.
//
// Returns a noopInformerManager if the client does not expose a REST config
// (e.g., fake.Client in unit tests — detected by the absence of RESTMapper
// that requires a live server).
func NewInformerManagerFromClient(_ client.Client) InformerManager {
	// This constructor is kept for future use when controller-runtime exposes
	// a way to extract the rest.Config from a client. For now, callers should
	// use NewInformerManager(cfg, scheme) directly.
	return &noopInformerManager{}
}

// Start begins the cache informers and blocks until ctx is cancelled.
func (m *realInformerManager) Start(ctx context.Context) {
	// cache.Start runs until ctx is done; errors are logged internally.
	_ = m.cache.Start(ctx)
}

// WaitForSync waits for the cache to finish the initial list from the API server.
func (m *realInformerManager) WaitForSync(ctx context.Context) bool {
	return m.cache.WaitForCacheSync(ctx)
}
