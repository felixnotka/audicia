# AudiciaSource: Cloud (AKS)

An AudiciaSource configured for cloud-based ingestion from an AKS cluster via Azure Event Hub.

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: aks-cloud-audit
  namespace: audicia-system
spec:
  sourceType: CloudAuditLog
  cloud:
    provider: AzureEventHub
    credentialSecretName: cloud-credentials
    clusterIdentity: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/my-rg/providers/Microsoft.ContainerService/managedClusters/my-cluster"
    azure:
      eventHubNamespace: "my-namespace.servicebus.windows.net"
      eventHubName: "aks-audit-logs"
      consumerGroup: "$Default"
      storageAccountURL: ""
      storageContainerName: ""
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

## Key Fields

| Field | Value | Notes |
|-------|-------|-------|
| `sourceType` | `CloudAuditLog` | Selects the cloud ingestion path |
| `cloud.provider` | `AzureEventHub` | Azure Event Hub adapter |
| `cloud.credentialSecretName` | `cloud-credentials` | Secret with `connection-string` key. Omit for workload identity |
| `cloud.clusterIdentity` | AKS resource ID | Used to filter events from shared Event Hubs |
| `cloud.azure.eventHubNamespace` | FQDN | Fully qualified Event Hub namespace |
| `cloud.azure.eventHubName` | Hub name | Event Hub instance receiving diagnostic logs |
| `cloud.azure.consumerGroup` | `$Default` | Consumer group for partition reads |

## Credential Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloud-credentials
  namespace: audicia-system
type: Opaque
stringData:
  connection-string: "Endpoint=sb://my-namespace.servicebus.windows.net/;SharedAccessKeyName=audicia-reader;SharedAccessKey=...;EntityPath=aks-audit-logs"
```

## Related

- [AKS Setup Guide](../guides/aks-setup.md) — Step-by-step walkthrough
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture overview
- [AudiciaSource CRD Reference](../reference/crd-audiciasource.md) — Full field reference
