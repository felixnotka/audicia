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
| `cloud.clusterIdentity` | AKS resource ID | Used to filter events from shared Event Hubs |
| `cloud.azure.eventHubNamespace` | FQDN | Fully qualified Event Hub namespace |
| `cloud.azure.eventHubName` | Hub name | Event Hub instance receiving diagnostic logs |
| `cloud.azure.consumerGroup` | `$Default` | Consumer group for partition reads |

## Authentication

Authentication uses Azure Workload Identity. Annotate the operator ServiceAccount:

```yaml
serviceAccount:
  annotations:
    azure.workload.identity/client-id: "<MANAGED_IDENTITY_CLIENT_ID>"
```

The managed identity needs the `Azure Event Hubs Data Receiver` role on the Event Hub namespace.

## Related

- [AKS Setup Guide](../guides/aks-setup.md) — Step-by-step walkthrough
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture overview
- [AudiciaSource CRD Reference](../reference/crd-audiciasource.md) — Full field reference
