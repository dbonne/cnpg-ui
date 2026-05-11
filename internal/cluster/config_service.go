package cluster

import (
	"context"
	"fmt"
	"strings"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dbonne/cnpg-ui/internal/api"
)

// ConfigService defines the Postgres configuration operations.
type ConfigService interface {
	GetConfig(ctx context.Context, clusterName string) (*api.PgConfigResponse, error)
	UpdateConfig(ctx context.Context, clusterName string, req api.UpdatePgConfigRequest) (*api.PgConfigResponse, error)
}

// configService implements ConfigService.
type configService struct {
	client    client.Client
	namespace string
}

// NewConfigService creates a ConfigService backed by the given controller-runtime client.
func NewConfigService(c client.Client, namespace string) ConfigService {
	return &configService{client: c, namespace: namespace}
}

// restartParams is the set of well-known Postgres parameters that require a restart.
// This is a representative subset used for the UI metadata — not exhaustive.
var restartParams = map[string]struct{}{
	"shared_buffers":           {},
	"max_connections":          {},
	"wal_level":                {},
	"archive_mode":             {},
	"max_wal_senders":          {},
	"max_replication_slots":    {},
	"hot_standby":              {},
	"shared_preload_libraries": {},
	"listen_addresses":         {},
	"port":                     {},
}

// GetConfig returns the current Postgres parameters for a cluster, enriched
// with type metadata and requires_restart annotations.
func (s *configService) GetConfig(ctx context.Context, clusterName string) (*api.PgConfigResponse, error) {
	cl, err := s.fetchCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	params := buildParamList(cl.Spec.PostgresConfiguration.Parameters)
	resp := &api.PgConfigResponse{
		Parameters:      params,
		RequiresRestart: false,
	}
	return resp, nil
}

// UpdateConfig patches the Postgres parameters in the Cluster CR spec.
func (s *configService) UpdateConfig(ctx context.Context, clusterName string, req api.UpdatePgConfigRequest) (*api.PgConfigResponse, error) {
	if err := validateParams(req.Parameters); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	cl, err := s.fetchCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	patch := client.MergeFrom(cl.DeepCopy())
	if cl.Spec.PostgresConfiguration.Parameters == nil {
		cl.Spec.PostgresConfiguration.Parameters = make(map[string]string)
	}
	for k, v := range req.Parameters {
		cl.Spec.PostgresConfiguration.Parameters[k] = v
	}

	if err := s.client.Patch(ctx, cl, patch); err != nil {
		return nil, fmt.Errorf("patch cluster config: %w", err)
	}

	params := buildParamList(cl.Spec.PostgresConfiguration.Parameters)
	requiresRestart := anyRequiresRestart(req.Parameters)

	return &api.PgConfigResponse{
		Parameters:      params,
		RequiresRestart: requiresRestart,
	}, nil
}

// fetchCluster retrieves a Cluster CR.
func (s *configService) fetchCluster(ctx context.Context, name string) (*cnpgv1.Cluster, error) {
	var cl cnpgv1.Cluster
	key := client.ObjectKey{Namespace: s.namespace, Name: name}
	if err := s.client.Get(ctx, key, &cl); err != nil {
		return nil, fmt.Errorf("cluster %q not found", name)
	}
	return &cl, nil
}

// buildParamList converts the raw parameters map to typed PgParamMetadata slice.
func buildParamList(params map[string]string) []api.PgParamMetadata {
	result := make([]api.PgParamMetadata, 0, len(params))
	for name, value := range params {
		_, restart := restartParams[name]
		result = append(result, api.PgParamMetadata{
			Name:            name,
			Type:            inferParamType(name, value),
			CurrentValue:    value,
			RequiresRestart: restart,
		})
	}
	return result
}

// inferParamType returns a simple type annotation for well-known parameters.
// Unknown parameters default to "string".
func inferParamType(name, _ string) string {
	switch name {
	case "max_connections", "work_mem", "shared_buffers",
		"effective_cache_size", "max_wal_size", "min_wal_size",
		"wal_buffers", "maintenance_work_mem", "temp_buffers":
		return "string" // Memory/size params are strings (e.g. "128MB")
	case "log_min_duration_statement", "checkpoint_completion_target":
		return "real"
	case "log_statement", "wal_level", "synchronous_commit":
		return "enum"
	case "log_connections", "log_disconnections", "hot_standby":
		return "bool"
	default:
		return "string"
	}
}

// validateParams validates the incoming parameter map.
// Currently only validates that values are non-empty strings.
func validateParams(params map[string]string) error {
	for k, v := range params {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("parameter %q cannot be empty", k)
		}
	}
	return nil
}

// anyRequiresRestart returns true if any of the given parameter names require a restart.
func anyRequiresRestart(params map[string]string) bool {
	for k := range params {
		if _, ok := restartParams[k]; ok {
			return true
		}
	}
	return false
}
