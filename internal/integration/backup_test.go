package integration_test

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/backup"
)

// TestBackupTrigger_WithEnvtest verifies that triggering a backup via
// backup.Service creates a CNPG Backup CR in the K8s API server.
func TestBackupTrigger_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-backup"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a minimal Cluster CR that the Backup will reference.
	cluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-test-cluster",
			Namespace: ns,
		},
		Spec: cnpgv1.ClusterSpec{
			Instances: 1,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				Size: "1Gi",
			},
		},
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create Cluster CR for backup test: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), cluster) })

	svc := backup.NewService(k8sClient, ns)

	// Trigger a backup for the cluster.
	req := api.TriggerBackupRequest{Method: "barmanObjectStore"}
	backupSummary, err := svc.TriggerBackup(ctx, "backup-test-cluster", req)
	if err != nil {
		t.Fatalf("TriggerBackup: %v", err)
	}
	if backupSummary.ClusterName != "backup-test-cluster" {
		t.Errorf("TriggerBackup.ClusterName = %q, want %q", backupSummary.ClusterName, "backup-test-cluster")
	}
	if backupSummary.Name == "" {
		t.Error("TriggerBackup: expected non-empty backup name")
	}

	// List backups for the cluster — expect 1.
	backups, err := svc.ListBackups(ctx, "backup-test-cluster")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("ListBackups: got %d, want 1", len(backups))
	}
}

// TestBackupList_NoBackups_WithEnvtest verifies that listing backups for a cluster
// with no backups returns an empty slice, not an error.
func TestBackupList_NoBackups_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-backup-empty"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	svc := backup.NewService(k8sClient, ns)
	backups, err := svc.ListBackups(ctx, "nonexistent-cluster")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("ListBackups: got %d backups, want 0", len(backups))
	}
}
