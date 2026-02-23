package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceType defines the type of audit log source.
// +kubebuilder:validation:Enum=K8sAuditLog;Webhook
type SourceType string

const (
	SourceTypeK8sAuditLog SourceType = "K8sAuditLog"
	SourceTypeWebhook     SourceType = "Webhook"
)

// ScopeMode controls whether ClusterRoles are generated.
// +kubebuilder:validation:Enum=NamespaceStrict;ClusterScopeAllowed
type ScopeMode string

const (
	ScopeModeNamespaceStrict     ScopeMode = "NamespaceStrict"
	ScopeModeClusterScopeAllowed ScopeMode = "ClusterScopeAllowed"
)

// VerbMerge controls verb merging behavior.
// +kubebuilder:validation:Enum=Smart;Exact
type VerbMerge string

const (
	VerbMergeSmart VerbMerge = "Smart"
	VerbMergeExact VerbMerge = "Exact"
)

// WildcardMode controls wildcard generation.
// +kubebuilder:validation:Enum=Forbidden;Safe
type WildcardMode string

const (
	WildcardModeForbidden WildcardMode = "Forbidden"
	WildcardModeSafe      WildcardMode = "Safe"
)

// FilterAction defines whether a filter allows or denies.
// +kubebuilder:validation:Enum=Allow;Deny
type FilterAction string

const (
	FilterActionAllow FilterAction = "Allow"
	FilterActionDeny  FilterAction = "Deny"
)

// AudiciaSourceSpec defines the desired state of an AudiciaSource.
type AudiciaSourceSpec struct {
	// SourceType is the type of audit log source (K8sAuditLog or Webhook).
	// +kubebuilder:validation:Required
	SourceType SourceType `json:"sourceType"`

	// Location configures the file-based audit log source.
	// +optional
	Location *FileLocation `json:"location,omitempty"`

	// Webhook configures the webhook-based audit event receiver.
	// +optional
	Webhook *WebhookConfig `json:"webhook,omitempty"`

	// PolicyStrategy configures how policies are generated.
	// +optional
	PolicyStrategy PolicyStrategy `json:"policyStrategy,omitempty"`

	// Filters defines an ordered allow/deny chain for events. First match wins.
	// +optional
	Filters []Filter `json:"filters,omitempty"`

	// IgnoreSystemUsers filters out known system users (e.g., system:kube-controller-manager).
	// +optional
	// +kubebuilder:default=true
	IgnoreSystemUsers bool `json:"ignoreSystemUsers,omitempty"`

	// Checkpoint configures processing checkpoint behavior.
	// +optional
	Checkpoint CheckpointConfig `json:"checkpoint,omitempty"`

	// Limits configures object size and retention limits.
	// +optional
	Limits LimitsConfig `json:"limits,omitempty"`
}

// FileLocation configures file-based audit log ingestion.
type FileLocation struct {
	// Path is the filesystem path to the audit log file.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
}

// WebhookConfig configures webhook-based audit event ingestion.
type WebhookConfig struct {
	// Port is the HTTPS port for the webhook receiver.
	// +kubebuilder:default=8443
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// TLSSecretName is the name of the Secret containing TLS cert and key.
	// +kubebuilder:validation:Required
	TLSSecretName string `json:"tlsSecretName"`

	// ClientCASecretName is the name of the Secret containing the CA bundle
	// for mTLS client certificate verification. Optional but recommended.
	// +optional
	ClientCASecretName string `json:"clientCASecretName,omitempty"`

	// RateLimitPerSecond is the maximum number of requests per second.
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	RateLimitPerSecond int32 `json:"rateLimitPerSecond,omitempty"`

	// MaxRequestBodyBytes is the maximum size of a request body in bytes.
	// +kubebuilder:default=1048576
	// +kubebuilder:validation:Minimum=1024
	MaxRequestBodyBytes int64 `json:"maxRequestBodyBytes,omitempty"`
}

// PolicyStrategy configures how RBAC policies are generated.
type PolicyStrategy struct {
	// ScopeMode controls whether ClusterRoles are generated.
	// +kubebuilder:default=NamespaceStrict
	ScopeMode ScopeMode `json:"scopeMode,omitempty"`

	// VerbMerge controls whether similar verbs (get/list/watch) are merged.
	// +kubebuilder:default=Smart
	VerbMerge VerbMerge `json:"verbMerge,omitempty"`

	// Wildcards controls whether wildcard (*) permissions are generated.
	// +kubebuilder:default=Forbidden
	Wildcards WildcardMode `json:"wildcards,omitempty"`

	// ResourceNames controls whether resourceNames are included in rules.
	// "Explicit" includes observed resource names; default omits them.
	// +optional
	// +kubebuilder:validation:Enum=Omit;Explicit
	// +kubebuilder:default=Omit
	ResourceNames string `json:"resourceNames,omitempty"`
}

// Filter defines a single allow/deny filter rule.
type Filter struct {
	// Action is whether this filter allows or denies matching events.
	// +kubebuilder:validation:Required
	Action FilterAction `json:"action"`

	// UserPattern is a regex matched against the event username.
	// +optional
	UserPattern string `json:"userPattern,omitempty"`

	// NamespacePattern is a regex matched against the event namespace.
	// +optional
	NamespacePattern string `json:"namespacePattern,omitempty"`
}

// CheckpointConfig configures processing checkpoint behavior.
type CheckpointConfig struct {
	// IntervalSeconds is the minimum interval between status checkpoint updates.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=5
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`

	// BatchSize is the number of events processed per batch.
	// +kubebuilder:default=500
	// +kubebuilder:validation:Minimum=1
	BatchSize int32 `json:"batchSize,omitempty"`
}

// LimitsConfig configures object size and retention limits.
type LimitsConfig struct {
	// MaxRulesPerReport is the maximum number of rules in a single AudiciaPolicyReport.
	// +kubebuilder:default=200
	// +kubebuilder:validation:Minimum=1
	MaxRulesPerReport int32 `json:"maxRulesPerReport,omitempty"`

	// RetentionDays is the number of days to retain rules that haven't been seen.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	RetentionDays int32 `json:"retentionDays,omitempty"`
}

// AudiciaSourceStatus defines the observed state of an AudiciaSource.
type AudiciaSourceStatus struct {
	// FileOffset is the byte offset of the last processed position in the audit log file.
	// +optional
	FileOffset int64 `json:"fileOffset,omitempty"`

	// LastTimestamp is the timestamp of the last processed audit event.
	// +optional
	LastTimestamp *metav1.Time `json:"lastTimestamp,omitempty"`

	// Inode is the inode number of the audit log file (for rotation detection).
	// +optional
	Inode uint64 `json:"inode,omitempty"`

	// Conditions represent the latest available observations of the source's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={as,asrc}
// +kubebuilder:printcolumn:name="Source Type",type=string,JSONPath=`.spec.sourceType`
// +kubebuilder:printcolumn:name="Scope Mode",type=string,JSONPath=`.spec.policyStrategy.scopeMode`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AudiciaSource defines the input configuration for the Audicia operator.
// It specifies where to read audit events from and how to generate policies.
type AudiciaSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AudiciaSourceSpec   `json:"spec,omitempty"`
	Status AudiciaSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AudiciaSourceList contains a list of AudiciaSource resources.
type AudiciaSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AudiciaSource `json:"items"`
}
