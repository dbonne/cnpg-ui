package backup_test

import (
	"context"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/backup"
	"github.com/dbonne/cnpg-ui/internal/k8s"
)

func newScheme() *runtime.Scheme { return k8s.NewScheme() }

func fakeBackup(name, ns, clusterName, phase string) *cnpgv1.Backup {
	return &cnpgv1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: cnpgv1.BackupSpec{
			Cluster: cnpgv1.LocalObjectReference{Name: clusterName},
		},
		Status: cnpgv1.BackupStatus{
			Phase: cnpgv1.BackupPhase(phase),
		},
	}
}

func fakeScheduledBackup(name, ns, clusterName, schedule string) *cnpgv1.ScheduledBackup {
	return &cnpgv1.ScheduledBackup{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: cnpgv1.ScheduledBackupSpec{
			Schedule: schedule,
			Cluster:  cnpgv1.LocalObjectReference{Name: clusterName},
		},
	}
}

// TestListBackups_ReturnsBackupsForCluster verifies that ListBackups returns
// only backups belonging to the given cluster.
func TestListBackups_ReturnsBackupsForCluster(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	b1 := fakeBackup("backup-1", "default", "prod", cnpgv1.BackupPhaseCompleted)
	b2 := fakeBackup("backup-2", "default", "prod", cnpgv1.BackupPhaseRunning)
	b3 := fakeBackup("backup-3", "default", "other-cluster", cnpgv1.BackupPhaseCompleted)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(b1, b2, b3).
		Build()

	svc := backup.NewService(fakeClient, "default")
	backups, err := svc.ListBackups(context.Background(), "prod")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}

	if len(backups) != 2 {
		t.Fatalf("got %d backups, want 2", len(backups))
	}
	// Verify cluster name is set on each
	for _, b := range backups {
		if b.ClusterName != "prod" {
			t.Errorf("ClusterName: got %q, want %q", b.ClusterName, "prod")
		}
	}
}

// TestListBackups_Empty verifies ListBackups returns empty (not nil) when no backups exist.
func TestListBackups_Empty(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := backup.NewService(fakeClient, "default")

	backups, err := svc.ListBackups(context.Background(), "prod")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if backups == nil {
		t.Error("ListBackups returned nil, want empty slice")
	}
	if len(backups) != 0 {
		t.Errorf("got %d backups, want 0", len(backups))
	}
}

// TestTriggerBackup_CreatesBackupCR verifies TriggerBackup creates a Backup CR.
func TestTriggerBackup_CreatesBackupCR(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := backup.NewService(fakeClient, "default")

	result, err := svc.TriggerBackup(context.Background(), "prod", api.TriggerBackupRequest{})
	if err != nil {
		t.Fatalf("TriggerBackup: %v", err)
	}

	if result.ClusterName != "prod" {
		t.Errorf("ClusterName: got %q, want %q", result.ClusterName, "prod")
	}

	// Verify the CR was created
	var list cnpgv1.BackupList
	if err := fakeClient.List(context.Background(), &list); err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("CR count: got %d, want 1", len(list.Items))
	}
	if list.Items[0].Spec.Cluster.Name != "prod" {
		t.Errorf("Backup.Spec.Cluster.Name: got %q, want %q",
			list.Items[0].Spec.Cluster.Name, "prod")
	}
}

// TestListScheduledBackups_ReturnsForCluster verifies ListScheduledBackups filters by cluster.
func TestListScheduledBackups_ReturnsForCluster(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	sb1 := fakeScheduledBackup("sched-1", "default", "prod", "0 2 * * *")
	sb2 := fakeScheduledBackup("sched-2", "default", "other", "0 3 * * *")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sb1, sb2).
		Build()

	svc := backup.NewService(fakeClient, "default")
	scheds, err := svc.ListScheduledBackups(context.Background(), "prod")
	if err != nil {
		t.Fatalf("ListScheduledBackups: %v", err)
	}

	if len(scheds) != 1 {
		t.Fatalf("got %d scheduled backups, want 1", len(scheds))
	}
	if scheds[0].Schedule != "0 2 * * *" {
		t.Errorf("Schedule: got %q, want %q", scheds[0].Schedule, "0 2 * * *")
	}
}

// TestCreateScheduledBackup_CreatesCR verifies CreateScheduledBackup creates the CR.
func TestCreateScheduledBackup_CreatesCR(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := backup.NewService(fakeClient, "default")

	req := api.CreateScheduledBackupRequest{
		Name:     "daily",
		Schedule: "0 2 * * *",
	}
	result, err := svc.CreateScheduledBackup(context.Background(), "prod", req)
	if err != nil {
		t.Fatalf("CreateScheduledBackup: %v", err)
	}

	if result.Name != "daily" {
		t.Errorf("Name: got %q, want %q", result.Name, "daily")
	}
	if result.Schedule != "0 2 * * *" {
		t.Errorf("Schedule: got %q, want %q", result.Schedule, "0 2 * * *")
	}
	if result.ClusterName != "prod" {
		t.Errorf("ClusterName: got %q, want %q", result.ClusterName, "prod")
	}

	// Verify CR exists
	var list cnpgv1.ScheduledBackupList
	if err := fakeClient.List(context.Background(), &list); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("CR count: got %d, want 1", len(list.Items))
	}
}

// TestDeleteScheduledBackup_Existing verifies deletion removes the CR.
func TestDeleteScheduledBackup_Existing(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	sb := fakeScheduledBackup("daily", "default", "prod", "0 2 * * *")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sb).
		Build()

	svc := backup.NewService(fakeClient, "default")
	if err := svc.DeleteScheduledBackup(context.Background(), "prod", "daily"); err != nil {
		t.Fatalf("DeleteScheduledBackup: %v", err)
	}

	var list cnpgv1.ScheduledBackupList
	if err := fakeClient.List(context.Background(), &list); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("CR count after delete: got %d, want 0", len(list.Items))
	}
}

// TestDeleteScheduledBackup_NotFound verifies error for non-existent scheduled backup.
func TestDeleteScheduledBackup_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := backup.NewService(fakeClient, "default")

	err := svc.DeleteScheduledBackup(context.Background(), "prod", "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent scheduled backup, got nil")
	}
}
