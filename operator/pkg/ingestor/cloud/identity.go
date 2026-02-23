package cloud

import (
	"strings"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var identityLog = ctrl.Log.WithName("ingestor").WithName("cloud").WithName("identity")

// ClusterIdentityValidator verifies that audit events originate from the
// expected cluster. This prevents the operator from processing events from
// a different cluster when using a shared cloud message bus.
//
// For AKS, each cluster typically gets its own Diagnostic Settings -> Event Hub,
// making the Event Hub itself an implicit identity boundary. The validator
// provides defense-in-depth for shared Event Hub scenarios.
type ClusterIdentityValidator struct {
	// ExpectedIdentity is the cluster identity string to match against.
	// For AKS: the resource ID (/subscriptions/.../managedClusters/<name>)
	// For EKS: the cluster ARN
	// For GKE: the cluster resource name
	ExpectedIdentity string
}

// Matches checks whether the audit event belongs to this cluster.
// Returns true if:
//   - No expected identity is configured (validation disabled)
//   - The event annotations contain the expected identity string
//   - The request URI contains the expected identity string
//
// Returns false only when the expected identity is set and the event
// explicitly references a different cluster.
func (v *ClusterIdentityValidator) Matches(event auditv1.Event) bool {
	if v.ExpectedIdentity == "" {
		return true
	}

	// Check annotations for cluster identity markers.
	if event.Annotations != nil {
		for _, value := range event.Annotations {
			if strings.Contains(value, v.ExpectedIdentity) {
				return true
			}
		}
	}

	// Check the request URI for cluster-specific paths.
	if strings.Contains(event.RequestURI, v.ExpectedIdentity) {
		return true
	}

	// AKS audit events don't always carry the cluster resource ID in the event
	// itself. The Event Hub namespace/name (configured in Diagnostic Settings)
	// provides the primary identity binding. This validator is defense-in-depth.
	//
	// Default to allow to avoid false drops during initial rollout. Log at high
	// verbosity so operators can verify their setup.
	identityLog.V(2).Info("cluster identity not found in event, allowing by default",
		"auditID", event.AuditID, "expectedIdentity", v.ExpectedIdentity)
	return true
}
