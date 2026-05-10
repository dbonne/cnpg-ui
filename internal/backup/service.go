// Package backup provides BackupService for CNPG Backup and ScheduledBackup CR operations.
package backup

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dbonne/cnpg-ui/internal/api"
)

// Service defines backup operations exposed to handlers.
type Service interface {
	ListBackups(ctx context.Context, clusterName string) ([]api.BackupSummary, error)
	TriggerBackup(ctx context.Context, clusterName string, req api.TriggerBackupRequest) (*api.BackupSummary, error)
	ListScheduledBackups(ctx context.Context, clusterName string) ([]api.ScheduledBackupSummary, error)
	GetScheduledBackup(ctx context.Context, clusterName, name string) (*api.ScheduledBackupSummary, error)
	CreateScheduledBackup(ctx context.Context, clusterName string, req api.CreateScheduledBackupRequest) (*api.ScheduledBackupSummary, error)
	DeleteScheduledBackup(ctx context.Context, clusterName, name string) error
}

// service implements Service using a controller-runtime client.
type service struct {
	client    client.Client
	namespace string
}

// NewService creates a backup Service.
func NewService(c client.Client, namespace string) Service {
	return &service{client: c, namespace: namespace}
}

// ListBackups returns all Backup CRs for the given cluster.
func (s *service) ListBackups(ctx context.Context, clusterName string) ([]api.BackupSummary, error) {
	var list cnpgv1.BackupList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	result := make([]api.BackupSummary, 0)
	for _, b := range list.Items {
		if b.Spec.Cluster.Name != clusterName {
			continue
		}
		result = append(result, toBackupSummary(&b))
	}
	return result, nil
}

// TriggerBackup creates an on-demand Backup CR for the given cluster.
func (s *service) TriggerBackup(ctx context.Context, clusterName string, req api.TriggerBackupRequest) (*api.BackupSummary, error) {
	method := cnpgv1.BackupMethodBarmanObjectStore
	if req.Method != "" {
		method = cnpgv1.BackupMethod(req.Method)
	}

	b := &cnpgv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: clusterName + "-",
			Namespace:    s.namespace,
		},
		Spec: cnpgv1.BackupSpec{
			Cluster: cnpgv1.LocalObjectReference{Name: clusterName},
			Method:  method,
		},
	}

	if err := s.client.Create(ctx, b); err != nil {
		return nil, fmt.Errorf("trigger backup for cluster %q: %w", clusterName, err)
	}

	summary := toBackupSummary(b)
	return &summary, nil
}

// ListScheduledBackups returns all ScheduledBackup CRs for the given cluster.
func (s *service) ListScheduledBackups(ctx context.Context, clusterName string) ([]api.ScheduledBackupSummary, error) {
	var list cnpgv1.ScheduledBackupList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("list scheduled backups: %w", err)
	}

	result := make([]api.ScheduledBackupSummary, 0)
	for _, sb := range list.Items {
		if sb.Spec.Cluster.Name != clusterName {
			continue
		}
		result = append(result, toScheduledSummary(&sb))
	}
	return result, nil
}

// GetScheduledBackup retrieves a single ScheduledBackup CR by name.
func (s *service) GetScheduledBackup(ctx context.Context, clusterName, name string) (*api.ScheduledBackupSummary, error) {
	var sb cnpgv1.ScheduledBackup
	key := client.ObjectKey{Namespace: s.namespace, Name: name}
	if err := s.client.Get(ctx, key, &sb); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("scheduled backup %q not found", name)
		}
		return nil, fmt.Errorf("get scheduled backup %q: %w", name, err)
	}
	if sb.Spec.Cluster.Name != clusterName {
		return nil, fmt.Errorf("scheduled backup %q not found", name)
	}
	summary := toScheduledSummary(&sb)
	return &summary, nil
}

// CreateScheduledBackup creates a new ScheduledBackup CR.
func (s *service) CreateScheduledBackup(ctx context.Context, clusterName string, req api.CreateScheduledBackupRequest) (*api.ScheduledBackupSummary, error) {
	sb := &cnpgv1.ScheduledBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: s.namespace,
		},
		Spec: cnpgv1.ScheduledBackupSpec{
			Schedule:  req.Schedule,
			Cluster:   cnpgv1.LocalObjectReference{Name: clusterName},
			Immediate: &req.Immediate,
		},
	}

	if err := s.client.Create(ctx, sb); err != nil {
		return nil, fmt.Errorf("create scheduled backup: %w", err)
	}

	summary := toScheduledSummary(sb)
	return &summary, nil
}

// DeleteScheduledBackup removes a ScheduledBackup CR by name.
func (s *service) DeleteScheduledBackup(ctx context.Context, clusterName, name string) error {
	var sb cnpgv1.ScheduledBackup
	key := client.ObjectKey{Namespace: s.namespace, Name: name}
	if err := s.client.Get(ctx, key, &sb); err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("scheduled backup %q not found", name)
		}
		return fmt.Errorf("get scheduled backup %q: %w", name, err)
	}
	if err := s.client.Delete(ctx, &sb); err != nil {
		return fmt.Errorf("delete scheduled backup %q: %w", name, err)
	}
	return nil
}

// toBackupSummary maps a CNPG Backup CR to an API BackupSummary.
func toBackupSummary(b *cnpgv1.Backup) api.BackupSummary {
	s := api.BackupSummary{
		Name:        b.Name,
		ClusterName: b.Spec.Cluster.Name,
		Phase:       string(b.Status.Phase),
		Method:      string(b.Spec.Method),
	}
	if b.Status.StartedAt != nil {
		s.StartedAt = b.Status.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if b.Status.StoppedAt != nil {
		s.StoppedAt = b.Status.StoppedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return s
}

// toScheduledSummary maps a CNPG ScheduledBackup CR to an API ScheduledBackupSummary.
func toScheduledSummary(sb *cnpgv1.ScheduledBackup) api.ScheduledBackupSummary {
	s := api.ScheduledBackupSummary{
		Name:        sb.Name,
		ClusterName: sb.Spec.Cluster.Name,
		Schedule:    sb.Spec.Schedule,
	}
	if sb.Spec.Immediate != nil {
		s.Immediate = *sb.Spec.Immediate
	}
	if sb.Spec.Suspend != nil {
		s.Suspended = *sb.Spec.Suspend
	}
	return s
}
