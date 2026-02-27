package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubjectKind represents the kind of RBAC subject.
// +kubebuilder:validation:Enum=ServiceAccount;User;Group
type SubjectKind string

const (
	SubjectKindServiceAccount SubjectKind = "ServiceAccount"
	SubjectKindUser           SubjectKind = "User"
	SubjectKindGroup          SubjectKind = "Group"
)

// AudiciaPolicyReportSpec defines the identity context for a policy report.
// This contains the subject the report covers. Set once when created.
type AudiciaPolicyReportSpec struct {
	// Subject identifies who this report is about.
	// +kubebuilder:validation:Required
	Subject Subject `json:"subject"`
}

// Subject identifies a Kubernetes RBAC subject (ServiceAccount, User, or Group).
type Subject struct {
	// Kind is the type of subject (ServiceAccount, User, or Group).
	// +kubebuilder:validation:Required
	Kind SubjectKind `json:"kind"`

	// Name is the name of the subject.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the subject (only for ServiceAccount).
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// AudiciaPolicyReportStatus contains all operator-generated output.
type AudiciaPolicyReportStatus struct {
	// ObservedRules is the structured list of observed RBAC rules for this subject.
	// +optional
	ObservedRules []ObservedRule `json:"observedRules,omitempty"`

	// SuggestedPolicy contains rendered RBAC manifests ready for kubectl apply.
	// +optional
	SuggestedPolicy *SuggestedPolicy `json:"suggestedPolicy,omitempty"`

	// Compliance contains the RBAC drift analysis comparing observed usage
	// against the subject's effective permissions in the cluster.
	// +optional
	Compliance *ComplianceReport `json:"compliance,omitempty"`

	// EventsProcessed is the total number of audit events that contributed to this report.
	// +optional
	EventsProcessed int64 `json:"eventsProcessed,omitempty"`

	// LastProcessedTime is the timestamp of the last processed event for this subject.
	// +optional
	LastProcessedTime *metav1.Time `json:"lastProcessedTime,omitempty"`

	// Conditions represent the latest available observations of the report's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ComplianceSeverity represents the compliance level.
// +kubebuilder:validation:Enum=Green;Yellow;Red
type ComplianceSeverity string

const (
	ComplianceSeverityGreen  ComplianceSeverity = "Green"
	ComplianceSeverityYellow ComplianceSeverity = "Yellow"
	ComplianceSeverityRed    ComplianceSeverity = "Red"
)

// ComplianceReport contains the result of comparing observed usage against
// the effective RBAC permissions for a subject.
type ComplianceReport struct {
	// Score is the ratio of used effective rules to total effective rules,
	// expressed as a percentage (0-100). A score of 100 means every granted
	// permission was actually exercised by at least one observed action.
	Score int32 `json:"score"`

	// Severity is the compliance level: Green (score >= 80), Yellow (>= 50), Red (< 50).
	Severity ComplianceSeverity `json:"severity"`

	// UsedCount is the number of effective RBAC rules that were exercised by
	// at least one observed action.
	UsedCount int32 `json:"usedCount"`

	// ExcessCount is the number of effective RBAC rules that were never observed in use.
	ExcessCount int32 `json:"excessCount"`

	// UncoveredCount is the number of observed rules NOT covered by any existing RBAC grant.
	// These represent permissions being used without explicit RBAC authorization
	// (possible via aggregated ClusterRoles or other mechanisms not yet resolved).
	UncoveredCount int32 `json:"uncoveredCount"`

	// HasSensitiveExcess is true when excess RBAC grants include sensitive
	// resources (secrets, nodes, webhookconfigurations, etc.).
	// +optional
	HasSensitiveExcess bool `json:"hasSensitiveExcess,omitempty"`

	// SensitiveExcess lists excess RBAC grants on sensitive resources
	// (e.g., secrets, nodes, webhookconfigurations).
	// +optional
	SensitiveExcess []string `json:"sensitiveExcess,omitempty"`

	// ExcessRules lists effective RBAC rules that were never observed in use.
	// +optional
	ExcessRules []ComplianceRule `json:"excessRules,omitempty"`

	// UncoveredRules lists observed actions not covered by any effective RBAC grant.
	// +optional
	UncoveredRules []ComplianceRule `json:"uncoveredRules,omitempty"`

	// LastEvaluatedTime is when the compliance check was last run.
	LastEvaluatedTime metav1.Time `json:"lastEvaluatedTime"`
}

// ComplianceRule describes a single RBAC permission used in excess/uncovered lists.
type ComplianceRule struct {
	// APIGroups is the list of API groups for this rule.
	APIGroups []string `json:"apiGroups"`

	// Resources is the list of resources.
	Resources []string `json:"resources"`

	// Verbs is the list of verbs.
	Verbs []string `json:"verbs"`

	// NonResourceURLs is the list of non-resource URLs (e.g., "/metrics").
	// +optional
	NonResourceURLs []string `json:"nonResourceURLs,omitempty"`

	// Namespace is the namespace this rule applies in.
	// Empty for cluster-scoped rules.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ObservedRule represents a single observed RBAC rule with metadata.
type ObservedRule struct {
	// APIGroups is the list of API groups for this rule.
	APIGroups []string `json:"apiGroups"`

	// Resources is the list of resources (including subresources like "pods/exec").
	Resources []string `json:"resources"`

	// Verbs is the list of verbs observed.
	Verbs []string `json:"verbs"`

	// NonResourceURLs is the list of non-resource URLs (e.g., "/metrics").
	// Mutually exclusive with APIGroups/Resources.
	// +optional
	NonResourceURLs []string `json:"nonResourceURLs,omitempty"`

	// Namespace is the namespace where this rule was observed.
	// Empty for cluster-scoped resources or non-resource URLs.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// FirstSeen is when this rule was first observed.
	FirstSeen metav1.Time `json:"firstSeen"`

	// LastSeen is when this rule was last observed.
	LastSeen metav1.Time `json:"lastSeen"`

	// Count is the number of times this rule was observed.
	// +kubebuilder:validation:Minimum=1
	Count int64 `json:"count"`
}

// SuggestedPolicy contains rendered RBAC manifests.
type SuggestedPolicy struct {
	// Manifests is a list of rendered YAML strings, each containing a complete
	// Role, ClusterRole, RoleBinding, or ClusterRoleBinding manifest.
	Manifests []string `json:"manifests"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={apr,apreport}
// +kubebuilder:printcolumn:name="Subject",type=string,JSONPath=`.spec.subject.name`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.subject.kind`
// +kubebuilder:printcolumn:name="Compliance",type=string,JSONPath=`.status.compliance.severity`
// +kubebuilder:printcolumn:name="Score",type=integer,JSONPath=`.status.compliance.score`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Needed",type=integer,JSONPath=`.status.compliance.usedCount`,priority=1,description="RBAC rules actually exercised"
// +kubebuilder:printcolumn:name="Excess",type=integer,JSONPath=`.status.compliance.excessCount`,priority=1,description="RBAC rules granted but never used"
// +kubebuilder:printcolumn:name="Ungranted",type=integer,JSONPath=`.status.compliance.uncoveredCount`,priority=1,description="observed actions without RBAC grant"
// +kubebuilder:printcolumn:name="Sensitive",type=boolean,JSONPath=`.status.compliance.hasSensitiveExcess`,priority=1,description="excess grants on sensitive resources"
// +kubebuilder:printcolumn:name="Audit Events",type=integer,JSONPath=`.status.eventsProcessed`,priority=1,description="total audit events processed"

// AudiciaPolicyReport contains the observed RBAC rules and suggested policies
// for a single subject, generated by the Audicia operator.
type AudiciaPolicyReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AudiciaPolicyReportSpec   `json:"spec,omitempty"`
	Status AudiciaPolicyReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AudiciaPolicyReportList contains a list of AudiciaPolicyReport resources.
type AudiciaPolicyReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AudiciaPolicyReport `json:"items"`
}
