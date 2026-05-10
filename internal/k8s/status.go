// Package k8s provides the Kubernetes client layer for the CNPG Web UI.
// It initializes the K8s client, manages informers for CNPG CRDs, and
// maps CNPG cluster phase strings to normalized status values.
package k8s

import cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"

// NormalizedStatus is the four-value status model exposed by the UI.
// It abstracts the many CNPG cluster phase strings into a single enum
// that drives status badges, SSE events, and alerting rules.
type NormalizedStatus string

const (
	// StatusHealthy means the cluster is running normally with no active operations.
	StatusHealthy NormalizedStatus = "HEALTHY"

	// StatusTransient means the cluster is in a temporary, expected operation
	// (e.g. scaling, upgrade, switchover) that should resolve on its own.
	StatusTransient NormalizedStatus = "TRANSIENT"

	// StatusFault means the cluster is in an error state that may require attention.
	// Unknown phases are also mapped to FAULT to surface unexpected states.
	StatusFault NormalizedStatus = "FAULT"

	// StatusHibernated means the cluster has been deliberately paused/hibernated.
	StatusHibernated NormalizedStatus = "HIBERNATED"
)

// String returns the string representation of a NormalizedStatus.
// It satisfies the fmt.Stringer interface and is used in JSON/SSE serialization.
func (s NormalizedStatus) String() string {
	return string(s)
}

// transientPhases is the set of CNPG phase strings that represent temporary,
// expected operations. Using a map[string]struct{} gives O(1) lookup.
var transientPhases = map[string]struct{}{
	cnpgv1.PhaseSwitchover:                {},
	cnpgv1.PhaseFirstPrimary:              {},
	cnpgv1.PhaseCreatingReplica:           {},
	cnpgv1.PhaseUpgrade:                   {},
	cnpgv1.PhaseMajorUpgrade:              {},
	cnpgv1.PhaseUpgradeDelayed:            {},
	cnpgv1.PhaseWaitingForUser:            {},
	cnpgv1.PhaseInplacePrimaryRestart:     {},
	cnpgv1.PhaseInplaceDeletePrimaryRestart: {},
	cnpgv1.PhaseWaitingForInstancesToBeActive: {},
	cnpgv1.PhaseOnlineUpgrading:           {},
	cnpgv1.PhaseApplyingConfiguration:     {},
	cnpgv1.PhaseReplicaClusterPromotion:   {},
}

// faultPhases is the set of CNPG phase strings that indicate an error state.
var faultPhases = map[string]struct{}{
	cnpgv1.PhaseFailOver:                  {},
	cnpgv1.PhaseUnrecoverable:             {},
	cnpgv1.PhaseUnknownPlugin:             {},
	cnpgv1.PhaseFailurePlugin:             {},
	cnpgv1.PhaseImageCatalogError:         {},
	cnpgv1.PhaseArchitectureBinaryMissing: {},
	cnpgv1.PhaseCannotCreateClusterObjects: {},
}

// MapPhaseToStatus converts a CNPG Cluster.Status.Phase string to a NormalizedStatus.
//
// Mapping rules (in priority order):
//  1. PhaseHealthy or empty string → HEALTHY
//  2. Known transient phases → TRANSIENT
//  3. Known fault phases → FAULT
//  4. Unknown/future phases → FAULT (safe default — surface unexpected states)
//
// Note: HIBERNATED is not yet a first-class CNPG phase string; it is reserved
// for future CNPG hibernation support and can be mapped when the API adds it.
func MapPhaseToStatus(phase string) NormalizedStatus {
	switch {
	case phase == "" || phase == cnpgv1.PhaseHealthy:
		return StatusHealthy

	case isTransient(phase):
		return StatusTransient

	case isFault(phase):
		return StatusFault

	default:
		// Unknown phase — treat as fault to surface unexpected states.
		return StatusFault
	}
}

func isTransient(phase string) bool {
	_, ok := transientPhases[phase]
	return ok
}

func isFault(phase string) bool {
	_, ok := faultPhases[phase]
	return ok
}
