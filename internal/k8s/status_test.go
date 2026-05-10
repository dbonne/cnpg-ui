package k8s_test

import (
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"

	"github.com/dbonne/cnpg-ui/internal/k8s"
)

// TestNormalizedStatus verifies that CNPG Cluster.Status.Phase strings map
// to the four normalized states: HEALTHY, TRANSIENT, FAULT, HIBERNATED.
func TestNormalizedStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		phase string
		want  k8s.NormalizedStatus
	}{
		// HEALTHY
		{
			name:  "healthy_phase",
			phase: cnpgv1.PhaseHealthy,
			want:  k8s.StatusHealthy,
		},
		{
			name:  "empty_phase_is_healthy",
			phase: "",
			want:  k8s.StatusHealthy,
		},

		// TRANSIENT — cluster is doing work but expected to stabilize
		{
			name:  "switchover_is_transient",
			phase: cnpgv1.PhaseSwitchover,
			want:  k8s.StatusTransient,
		},
		{
			name:  "first_primary_is_transient",
			phase: cnpgv1.PhaseFirstPrimary,
			want:  k8s.StatusTransient,
		},
		{
			name:  "creating_replica_is_transient",
			phase: cnpgv1.PhaseCreatingReplica,
			want:  k8s.StatusTransient,
		},
		{
			name:  "upgrade_is_transient",
			phase: cnpgv1.PhaseUpgrade,
			want:  k8s.StatusTransient,
		},
		{
			name:  "major_upgrade_is_transient",
			phase: cnpgv1.PhaseMajorUpgrade,
			want:  k8s.StatusTransient,
		},
		{
			name:  "upgrade_delayed_is_transient",
			phase: cnpgv1.PhaseUpgradeDelayed,
			want:  k8s.StatusTransient,
		},
		{
			name:  "waiting_for_user_is_transient",
			phase: cnpgv1.PhaseWaitingForUser,
			want:  k8s.StatusTransient,
		},
		{
			name:  "inplace_primary_restart_is_transient",
			phase: cnpgv1.PhaseInplacePrimaryRestart,
			want:  k8s.StatusTransient,
		},
		{
			name:  "inplace_delete_primary_restart_is_transient",
			phase: cnpgv1.PhaseInplaceDeletePrimaryRestart,
			want:  k8s.StatusTransient,
		},
		{
			name:  "waiting_for_instances_is_transient",
			phase: cnpgv1.PhaseWaitingForInstancesToBeActive,
			want:  k8s.StatusTransient,
		},
		{
			name:  "online_upgrading_is_transient",
			phase: cnpgv1.PhaseOnlineUpgrading,
			want:  k8s.StatusTransient,
		},
		{
			name:  "applying_configuration_is_transient",
			phase: cnpgv1.PhaseApplyingConfiguration,
			want:  k8s.StatusTransient,
		},
		{
			name:  "replica_cluster_promotion_is_transient",
			phase: cnpgv1.PhaseReplicaClusterPromotion,
			want:  k8s.StatusTransient,
		},

		// FAULT — cluster is in an error state
		{
			name:  "failover_is_fault",
			phase: cnpgv1.PhaseFailOver,
			want:  k8s.StatusFault,
		},
		{
			name:  "unrecoverable_is_fault",
			phase: cnpgv1.PhaseUnrecoverable,
			want:  k8s.StatusFault,
		},
		{
			name:  "unknown_plugin_is_fault",
			phase: cnpgv1.PhaseUnknownPlugin,
			want:  k8s.StatusFault,
		},
		{
			name:  "failure_plugin_is_fault",
			phase: cnpgv1.PhaseFailurePlugin,
			want:  k8s.StatusFault,
		},
		{
			name:  "image_catalog_error_is_fault",
			phase: cnpgv1.PhaseImageCatalogError,
			want:  k8s.StatusFault,
		},
		{
			name:  "architecture_binary_missing_is_fault",
			phase: cnpgv1.PhaseArchitectureBinaryMissing,
			want:  k8s.StatusFault,
		},
		{
			name:  "cannot_create_cluster_objects_is_fault",
			phase: cnpgv1.PhaseCannotCreateClusterObjects,
			want:  k8s.StatusFault,
		},
		{
			name:  "unknown_phase_is_fault",
			phase: "some unknown future phase string",
			want:  k8s.StatusFault,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := k8s.MapPhaseToStatus(tc.phase)
			if got != tc.want {
				t.Errorf("MapPhaseToStatus(%q) = %q, want %q", tc.phase, got, tc.want)
			}
		})
	}
}

// TestNormalizedStatusString verifies the String() representation.
func TestNormalizedStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status k8s.NormalizedStatus
		want   string
	}{
		{k8s.StatusHealthy, "HEALTHY"},
		{k8s.StatusTransient, "TRANSIENT"},
		{k8s.StatusFault, "FAULT"},
		{k8s.StatusHibernated, "HIBERNATED"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.String(); got != tc.want {
				t.Errorf("NormalizedStatus.String() = %q, want %q", got, tc.want)
			}
		})
	}
}
