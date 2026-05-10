// Package pooler provides PoolerService for read-only CNPG Pooler CR access.
package pooler

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dbonne/cnpg-ui/internal/api"
)

// Service defines the pooler read operations exposed to handlers.
type Service interface {
	List(ctx context.Context, clusterName string) ([]api.PoolerSummary, error)
	Get(ctx context.Context, clusterName, name string) (*api.PoolerSummary, error)
}

// service implements Service using a controller-runtime client.
type service struct {
	client    client.Client
	namespace string
}

// NewService creates a pooler Service.
func NewService(c client.Client, namespace string) Service {
	return &service{client: c, namespace: namespace}
}

// List returns all Pooler CRs for the given cluster.
func (s *service) List(ctx context.Context, clusterName string) ([]api.PoolerSummary, error) {
	var list cnpgv1.PoolerList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("list poolers: %w", err)
	}

	result := make([]api.PoolerSummary, 0)
	for _, p := range list.Items {
		if p.Spec.Cluster.Name != clusterName {
			continue
		}
		result = append(result, toSummary(&p))
	}
	return result, nil
}

// Get returns a single Pooler CR by cluster name and pooler name.
func (s *service) Get(ctx context.Context, clusterName, name string) (*api.PoolerSummary, error) {
	var p cnpgv1.Pooler
	key := client.ObjectKey{Namespace: s.namespace, Name: name}
	if err := s.client.Get(ctx, key, &p); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("pooler %q not found", name)
		}
		return nil, fmt.Errorf("get pooler %q: %w", name, err)
	}
	if p.Spec.Cluster.Name != clusterName {
		return nil, fmt.Errorf("pooler %q not found", name)
	}
	summary := toSummary(&p)
	return &summary, nil
}

// toSummary maps a CNPG Pooler CR to an API PoolerSummary.
func toSummary(p *cnpgv1.Pooler) api.PoolerSummary {
	s := api.PoolerSummary{
		Name:        p.Name,
		ClusterName: p.Spec.Cluster.Name,
		Type:        string(p.Spec.Type),
	}
	if p.Spec.Instances != nil {
		s.Instances = int(*p.Spec.Instances)
	}
	if p.Spec.PgBouncer != nil {
		s.PoolMode = string(p.Spec.PgBouncer.PoolMode)
		if p.Spec.PgBouncer.Parameters != nil {
			if v, ok := p.Spec.PgBouncer.Parameters["max_client_conn"]; ok {
				// Best-effort parse — skip if not a valid integer
				var n int
				if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
					s.MaxClientConn = n
				}
			}
		}
	}
	return s
}
