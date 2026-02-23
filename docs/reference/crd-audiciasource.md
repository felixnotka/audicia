# AudiciaSource CRD

Complete field reference for the AudiciaSource Custom Resource Definition.

---

**API Group:** `audicia.io/v1alpha1`
**Scope:** Namespaced
**Short names:** `as`, `asrc`
**kubectl columns:** Source Type, Scope Mode, Age

## Example

A complete AudiciaSource for webhook mode with mTLS:

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: realtime-audit
  namespace: audicia-system
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
    clientCASecretName: kube-apiserver-client-ca
    rateLimitPerSecond: 100
    maxRequestBodyBytes: 1048576
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
  checkpoint:
    intervalSeconds: 30
    batchSize: 500
  limits:
    maxRulesPerReport: 200
    retentionDays: 30
```

For more examples, see the [Examples](../examples/audicia-source-hardened.md) section.

---

## spec

| Field               | Type    | Default | Description                                                 |
|---------------------|---------|---------|-------------------------------------------------------------|
| `sourceType`        | string  | -       | Ingestion backend: `K8sAuditLog`, `Webhook`, or `CloudAuditLog` |
| `ignoreSystemUsers` | boolean | `true`  | Drop events from `system:*` users (except service accounts) |

## spec.location

| Field           | Type   | Default | Description                                                                |
|-----------------|--------|---------|----------------------------------------------------------------------------|
| `location.path` | string | -       | Filesystem path to the audit log file. Used with `sourceType: K8sAuditLog` |

## spec.webhook

| Field                         | Type    | Default   | Description                                                                   |
|-------------------------------|---------|-----------|-------------------------------------------------------------------------------|
| `webhook.port`                | integer | `8443`    | TCP port for the webhook HTTPS server (1-65535)                               |
| `webhook.tlsSecretName`       | string  | -         | Name of a `kubernetes.io/tls` Secret for the webhook TLS certificate          |
| `webhook.clientCASecretName`  | string  | -         | Name of a Secret containing `ca.crt` for mTLS client certificate verification |
| `webhook.rateLimitPerSecond`  | integer | `100`     | Maximum requests per second (excess returns HTTP 429)                         |
| `webhook.maxRequestBodyBytes` | integer | `1048576` | Maximum request body size in bytes (1MB default)                              |

## spec.cloud

Configuration for cloud-based audit log ingestion. Used with `sourceType: CloudAuditLog`.

| Field                        | Type   | Default | Description                                                                                                                |
|------------------------------|--------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `cloud.provider`             | string | -       | Cloud platform: `AzureEventHub`, `AWSCloudWatch`, or `GCPPubSub`                                                          |
| `cloud.credentialSecretName` | string | -       | Name of a Secret containing cloud credentials (e.g., `connection-string` key). Leave empty for managed/workload identity    |
| `cloud.clusterIdentity`     | string | -       | Identity string for cluster event validation. Format varies by provider (AKS resource ID, EKS ARN, GKE resource name)      |

### spec.cloud.azure

| Field                              | Type   | Default    | Description                                                                      |
|------------------------------------|--------|------------|----------------------------------------------------------------------------------|
| `cloud.azure.eventHubNamespace`    | string | -          | Fully qualified Event Hub namespace (e.g., `myns.servicebus.windows.net`)        |
| `cloud.azure.eventHubName`        | string | -          | Event Hub instance name                                                          |
| `cloud.azure.consumerGroup`       | string | `$Default` | Consumer group for partition reads                                               |
| `cloud.azure.storageAccountURL`   | string | -          | Azure Blob Storage URL for checkpoint persistence. Empty = in-status checkpoints |
| `cloud.azure.storageContainerName`| string | -          | Blob container name for checkpoints                                              |

### spec.cloud.aws

| Field                         | Type   | Default | Description                                           |
|-------------------------------|--------|---------|-------------------------------------------------------|
| `cloud.aws.logGroupName`     | string | -       | CloudWatch Logs group containing audit logs            |
| `cloud.aws.logStreamPrefix`  | string | -       | Optional stream name prefix filter                     |

### spec.cloud.gcp

| Field                        | Type   | Default | Description                                 |
|------------------------------|--------|---------|---------------------------------------------|
| `cloud.gcp.projectID`       | string | -       | GCP project ID                              |
| `cloud.gcp.subscriptionID`  | string | -       | Pub/Sub subscription ID for audit log topic |

## spec.policyStrategy

| Field                          | Type   | Default           | Description                                                                   |
|--------------------------------|--------|-------------------|-------------------------------------------------------------------------------|
| `policyStrategy.scopeMode`     | string | `NamespaceStrict` | `NamespaceStrict` (Roles only) or `ClusterScopeAllowed` (allows ClusterRoles) |
| `policyStrategy.verbMerge`     | string | `Smart`           | `Smart` (merge same-resource rules) or `Exact` (one rule per verb)            |
| `policyStrategy.wildcards`     | string | `Forbidden`       | `Forbidden` (never emit `*`) or `Safe` (allow when all 8 verbs observed)      |
| `policyStrategy.resourceNames` | string | `Omit`            | `Omit` (no resourceNames) or `Explicit` (include observed resource names)     |

## spec.filters[]

Ordered allow/deny chain. First match wins. Default: allow.

| Field                        | Type   | Description                                       |
|------------------------------|--------|---------------------------------------------------|
| `filters[].action`           | string | `Allow` or `Deny`                                 |
| `filters[].userPattern`      | string | Regex matched against `event.User.Username`       |
| `filters[].namespacePattern` | string | Regex matched against `event.ObjectRef.Namespace` |

## spec.checkpoint

| Field                        | Type    | Default | Description                                        |
|------------------------------|---------|---------|----------------------------------------------------|
| `checkpoint.intervalSeconds` | integer | `30`    | Seconds between status checkpoint updates (min: 5) |
| `checkpoint.batchSize`       | integer | `500`   | Maximum events per processing batch (min: 1)       |

## spec.limits

| Field                      | Type    | Default | Description                                                              |
|----------------------------|---------|---------|--------------------------------------------------------------------------|
| `limits.maxRulesPerReport` | integer | `200`   | Maximum rules per AudiciaPolicyReport (oldest by lastSeen dropped first) |
| `limits.retentionDays`     | integer | `30`    | Rules not seen within this window are dropped during flush               |

## status

| Field                                         | Type        | Description                                                 |
|-----------------------------------------------|-------------|-------------------------------------------------------------|
| `status.fileOffset`                           | int64       | Byte offset in the audit log at last checkpoint             |
| `status.lastTimestamp`                        | date-time   | Timestamp of the last processed event                       |
| `status.inode`                                | int64       | Inode number for log rotation detection (Linux only)        |
| `status.cloudCheckpoint.partitionOffsets`     | map         | Per-partition sequence numbers for cloud sources            |
| `status.conditions[]`                         | Condition[] | Standard Kubernetes conditions (`Ready`)                    |
