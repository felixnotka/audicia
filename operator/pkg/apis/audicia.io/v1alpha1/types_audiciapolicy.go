package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyState represents the lifecycle state of a suggested RBAC policy.
// +kubebuilder:validation:Enum=Pending;Approved;Applied;Outdated
type PolicyState string

const (
	PolicyStatePending  PolicyState = "Pending"
	PolicyStateApproved PolicyState = "Approved"
	PolicyStateApplied  PolicyState = "Applied"
	PolicyStateOutdated PolicyState = "Outdated"
)

// AudiciaPolicySpec defines the suggested RBAC policy for a subject.
type AudiciaPolicySpec struct {
	// Subject identifies who this policy is for.
	// +kubebuilder:validation:Required
	Subject Subject `json:"subject"`

	// SourceRef is the name of the AudiciaSource that generated this policy.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SourceRef string `json:"sourceRef"`

	// Manifests is a list of rendered YAML strings, each containing a complete
	// Role, ClusterRole, RoleBinding, or ClusterRoleBinding manifest.
	Manifests []string `json:"manifests"`
}

// AudiciaPolicyStatus contains the approval state and metadata.
type AudiciaPolicyStatus struct {
	// State is the lifecycle state of this policy.
	// +kubebuilder:default=Pending
	State PolicyState `json:"state,omitempty"`

	// RuleCount is the number of RBAC rules in the suggested manifests.
	// +optional
	RuleCount int32 `json:"ruleCount,omitempty"`

	// ApprovedBy is the identity of the user who approved this policy.
	// +optional
	ApprovedBy string `json:"approvedBy,omitempty"`

	// ApprovedTime is when this policy was approved.
	// +optional
	ApprovedTime *metav1.Time `json:"approvedTime,omitempty"`

	// Conditions represent the latest available observations of the policy's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={ap,apolicy}
// +kubebuilder:printcolumn:name="Subject",type=string,JSONPath=`.spec.subject.name`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.subject.kind`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AudiciaPolicy contains the suggested RBAC manifests for a single subject,
// with an approval workflow for applying them to the cluster.
type AudiciaPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AudiciaPolicySpec   `json:"spec,omitempty"`
	Status AudiciaPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AudiciaPolicyList contains a list of AudiciaPolicy resources.
type AudiciaPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AudiciaPolicy `json:"items"`
}
