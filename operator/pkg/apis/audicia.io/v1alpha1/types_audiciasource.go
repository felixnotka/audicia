package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceType defines the type of audit log source.
// +kubebuilder:validation:Enum=K8sAuditLog;Webhook;CloudAuditLog
type SourceType string

const (
	SourceTypeK8sAuditLog   SourceType = "K8sAuditLog"
	SourceTypeWebhook       SourceType = "Webhook"
	SourceTypeCloudAuditLog SourceType = "CloudAuditLog"
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

	// Cloud configures cloud-based audit log ingestion (AKS Event Hub, EKS CloudWatch, GKE Pub/Sub).
	// +optional
	Cloud *CloudConfig `json:"cloud,omitempty"`

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

// CloudProvider defines supported cloud providers for audit log ingestion.
// +kubebuilder:validation:Enum=AzureEventHub;AWSCloudWatch;GCPPubSub
type CloudProvider string

const (
	CloudProviderAzureEventHub CloudProvider = "AzureEventHub"
	CloudProviderAWSCloudWatch CloudProvider = "AWSCloudWatch"
	CloudProviderGCPPubSub     CloudProvider = "GCPPubSub"
)

// CloudConfig configures cloud-based audit log ingestion.
type CloudConfig struct {
	// Provider specifies the cloud platform.
	// +kubebuilder:validation:Required
	Provider CloudProvider `json:"provider"`

	// CredentialSecretName is the name of a Kubernetes Secret containing cloud
	// credentials (e.g., connection string). The Secret must exist in the same
	// namespace as the AudiciaSource. Leave empty for managed identity / workload identity.
	// +optional
	CredentialSecretName string `json:"credentialSecretName,omitempty"`

	// ClusterIdentity is used to verify that received audit events belong to
	// the cluster where this operator is running. Format varies by provider
	// (e.g., AKS resource ID, EKS cluster ARN, GKE resource name).
	// +kubebuilder:validation:Required
	ClusterIdentity string `json:"clusterIdentity"`

	// Azure contains Azure Event Hub-specific configuration.
	// +optional
	Azure *AzureEventHubConfig `json:"azure,omitempty"`

	// AWS contains AWS CloudWatch-specific configuration.
	// +optional
	AWS *AWSCloudWatchConfig `json:"aws,omitempty"`

	// GCP contains GCP Pub/Sub-specific configuration.
	// +optional
	GCP *GCPPubSubConfig `json:"gcp,omitempty"`
}

// AzureEventHubConfig configures Azure Event Hub-based ingestion.
type AzureEventHubConfig struct {
	// EventHubNamespace is the fully qualified Event Hub namespace
	// (e.g., "myns.servicebus.windows.net").
	// +kubebuilder:validation:Required
	EventHubNamespace string `json:"eventHubNamespace"`

	// EventHubName is the name of the Event Hub instance.
	// +kubebuilder:validation:Required
	EventHubName string `json:"eventHubName"`

	// ConsumerGroup is the consumer group name.
	// +kubebuilder:default="$Default"
	// +optional
	ConsumerGroup string `json:"consumerGroup,omitempty"`

	// StorageAccountURL is the Azure Blob Storage URL used for checkpoint
	// persistence by the Event Hub processor. If empty, checkpoints are
	// stored in AudiciaSource status only.
	// +optional
	StorageAccountURL string `json:"storageAccountURL,omitempty"`

	// StorageContainerName is the blob container name for checkpoints.
	// +optional
	StorageContainerName string `json:"storageContainerName,omitempty"`
}

// AWSCloudWatchConfig configures AWS CloudWatch-based ingestion (placeholder).
type AWSCloudWatchConfig struct {
	// LogGroupName is the CloudWatch Logs group containing audit logs.
	// +kubebuilder:validation:Required
	LogGroupName string `json:"logGroupName"`

	// LogStreamPrefix is an optional stream name prefix filter.
	// +optional
	LogStreamPrefix string `json:"logStreamPrefix,omitempty"`
}

// GCPPubSubConfig configures GCP Pub/Sub-based ingestion (placeholder).
type GCPPubSubConfig struct {
	// ProjectID is the GCP project ID.
	// +kubebuilder:validation:Required
	ProjectID string `json:"projectID"`

	// SubscriptionID is the Pub/Sub subscription name.
	// +kubebuilder:validation:Required
	SubscriptionID string `json:"subscriptionID"`
}

// CloudCheckpointStatus stores cloud-specific checkpoint data.
type CloudCheckpointStatus struct {
	// PartitionOffsets maps partition/shard IDs to their last-acknowledged
	// sequence numbers. Used to resume consumption after restart.
	// +optional
	PartitionOffsets map[string]string `json:"partitionOffsets,omitempty"`
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

	// CloudCheckpoint stores resumption state for cloud audit log sources.
	// +optional
	CloudCheckpoint *CloudCheckpointStatus `json:"cloudCheckpoint,omitempty"`

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
