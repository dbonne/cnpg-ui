// Package cluster provides the ClusterService for CNPG Cluster CR operations.
// It wraps controller-runtime client calls and maps CNPG types to API response types.
package cluster

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/k8s"
)

// Service defines the cluster operations exposed to handlers.
type Service interface {
	List(ctx context.Context) ([]api.ClusterSummary, error)
	Get(ctx context.Context, name string) (*api.ClusterDetail, error)
	Create(ctx context.Context, req *api.CreateClusterRequest) (*api.ClusterDetail, error)
	Update(ctx context.Context, name string, req api.UpdateClusterRequest) (*api.ClusterDetail, error)
	Scale(ctx context.Context, name string, req api.ScaleClusterRequest) (*api.ClusterDetail, error)
	Delete(ctx context.Context, name string) error
}

// service implements Service using a controller-runtime client.
type service struct {
	client    client.Client
	namespace string
}

// NewService creates a Service backed by the given controller-runtime client.
func NewService(c client.Client, namespace string) Service {
	return &service{client: c, namespace: namespace}
}

// List returns all CNPG Cluster CRs in the configured namespace,
// mapped to API summary types with normalized status.
func (s *service) List(ctx context.Context) ([]api.ClusterSummary, error) {
	var list cnpgv1.ClusterList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}

	result := make([]api.ClusterSummary, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, toSummary(&list.Items[i]))
	}
	return result, nil
}

// Get returns the full detail for a single cluster by name.
func (s *service) Get(ctx context.Context, name string) (*api.ClusterDetail, error) {
	cl, err := s.fetchCluster(ctx, name)
	if err != nil {
		return nil, err
	}
	detail := toDetail(cl)
	return &detail, nil
}

// Create creates a new CNPG Cluster CR from the request body.
func (s *service) Create(ctx context.Context, req *api.CreateClusterRequest) (*api.ClusterDetail, error) {
	ns := req.Namespace
	if ns == "" {
		ns = s.namespace
	}

	storageSize, err := resource.ParseQuantity(req.StorageSize)
	if err != nil {
		return nil, fmt.Errorf("invalid storageSize %q: %w", req.StorageSize, err)
	}

	cl := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: ns,
		},
		Spec: cnpgv1.ClusterSpec{
			Instances: req.Instances,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				Size: storageSize.String(),
			},
		},
	}

	if req.PgVersion > 0 {
		cl.Spec.ImageCatalogRef = &cnpgv1.ImageCatalogRef{
			Major: req.PgVersion,
		}
	}

	if req.ImageName != "" {
		cl.Spec.ImageName = req.ImageName
	}

	if err := s.client.Create(ctx, cl); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("cluster %q already exists", req.Name)
		}
		return nil, fmt.Errorf("create cluster: %w", err)
	}

	detail := toDetail(cl)
	return &detail, nil
}

// Update patches the spec fields of an existing Cluster CR from the request body.
// Only non-zero fields in the request are applied.
func (s *service) Update(ctx context.Context, name string, req api.UpdateClusterRequest) (*api.ClusterDetail, error) {
	cl, err := s.fetchCluster(ctx, name)
	if err != nil {
		return nil, err
	}

	patch := client.MergeFrom(cl.DeepCopy())

	if req.Instances > 0 {
		cl.Spec.Instances = req.Instances
	}
	if req.StorageSize != "" {
		storageSize, err := resource.ParseQuantity(req.StorageSize)
		if err != nil {
			return nil, fmt.Errorf("invalid storageSize %q: %w", req.StorageSize, err)
		}
		cl.Spec.StorageConfiguration.Size = storageSize.String()
	}
	if req.ImageName != "" {
		cl.Spec.ImageName = req.ImageName
	}

	if err := s.client.Patch(ctx, cl, patch); err != nil {
		return nil, fmt.Errorf("update cluster %q: %w", name, err)
	}

	detail := toDetail(cl)
	return &detail, nil
}

// Scale updates the instances field of the named cluster.
func (s *service) Scale(ctx context.Context, name string, req api.ScaleClusterRequest) (*api.ClusterDetail, error) {
	cl, err := s.fetchCluster(ctx, name)
	if err != nil {
		return nil, err
	}

	patch := client.MergeFrom(cl.DeepCopy())
	cl.Spec.Instances = req.Instances

	if err := s.client.Patch(ctx, cl, patch); err != nil {
		return nil, fmt.Errorf("scale cluster %q: %w", name, err)
	}

	detail := toDetail(cl)
	return &detail, nil
}

// Delete removes a Cluster CR by name.
func (s *service) Delete(ctx context.Context, name string) error {
	cl, err := s.fetchCluster(ctx, name)
	if err != nil {
		return err
	}
	if err := s.client.Delete(ctx, cl); err != nil {
		return fmt.Errorf("delete cluster %q: %w", name, err)
	}
	return nil
}

// fetchCluster retrieves a single Cluster CR and wraps not-found errors.
func (s *service) fetchCluster(ctx context.Context, name string) (*cnpgv1.Cluster, error) {
	var cl cnpgv1.Cluster
	key := client.ObjectKey{Namespace: s.namespace, Name: name}
	if err := s.client.Get(ctx, key, &cl); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("cluster %q not found", name)
		}
		return nil, fmt.Errorf("get cluster %q: %w", name, err)
	}
	return &cl, nil
}

// toSummary maps a CNPG Cluster to an API ClusterSummary.
func toSummary(cl *cnpgv1.Cluster) api.ClusterSummary {
	return api.ClusterSummary{
		Name:           cl.Name,
		Namespace:      cl.Namespace,
		Instances:      cl.Spec.Instances,
		ReadyInstances: cl.Status.ReadyInstances,
		Status:         mapStatus(cl.Status.Phase),
		Phase:          cl.Status.Phase,
		CurrentPrimary: cl.Status.CurrentPrimary,
	}
}

// toDetail maps a CNPG Cluster to an API ClusterDetail.
func toDetail(cl *cnpgv1.Cluster) api.ClusterDetail {
	detail := api.ClusterDetail{
		ClusterSummary: toSummary(cl),
		ImageName:      cl.Spec.ImageName,
	}
	if cl.Spec.StorageConfiguration.Size != "" {
		detail.StorageSize = cl.Spec.StorageConfiguration.Size
	}
	if cl.Spec.ImageCatalogRef != nil {
		detail.PgVersion = cl.Spec.ImageCatalogRef.Major
	}
	if !cl.CreationTimestamp.IsZero() {
		detail.CreatedAt = cl.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	return detail
}

// mapStatus converts a CNPG phase string to the API NormalizedStatus.
// It delegates to the k8s package's pure MapPhaseToStatus function.
func mapStatus(phase string) api.NormalizedStatus {
	return api.NormalizedStatus(k8s.MapPhaseToStatus(phase))
}
