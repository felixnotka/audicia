# AKS Setup (Event Hub)

This guide walks through configuring Audicia to ingest audit logs from an Azure Kubernetes Service (AKS) cluster via
Azure Event Hub.

## Prerequisites

- An AKS cluster with [Diagnostic Settings](https://learn.microsoft.com/en-us/azure/aks/monitor-aks#resource-logs)
  enabled, routing `kube-audit` or `kube-audit-admin` logs to an Event Hub
- An Azure Event Hub namespace and instance receiving the diagnostic logs
- The Audicia operator image built with the `azure` build tag
- Helm 3

## Step 1: Enable AKS Diagnostic Settings

In the Azure portal or via CLI, configure your AKS cluster to send audit logs to an Event Hub:

```bash
az monitor diagnostic-settings create \
  --resource /subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.ContainerService/managedClusters/<CLUSTER> \
  --name aks-audit-to-eventhub \
  --event-hub <EVENT_HUB_NAME> \
  --event-hub-rule /subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.EventHub/namespaces/<NAMESPACE>/authorizationRules/RootManageSharedAccessKey \
  --logs '[{"category": "kube-audit-admin", "enabled": true}]'
```

Use `kube-audit-admin` to reduce volume (excludes read-only events) or `kube-audit` for complete coverage.

## Step 2: Create a Credential Secret

### Option A: Connection String

Create a Secret containing the Event Hub connection string:

```bash
kubectl create secret generic cloud-credentials \
  --from-literal=connection-string="Endpoint=sb://<NAMESPACE>.servicebus.windows.net/;SharedAccessKeyName=...;SharedAccessKey=...;EntityPath=<EVENT_HUB_NAME>" \
  -n audicia-system
```

### Option B: Workload Identity (Recommended)

For production, use Azure Workload Identity. No credential Secret is needed — annotate the ServiceAccount instead:

```yaml
serviceAccount:
  annotations:
    azure.workload.identity/client-id: "<MANAGED_IDENTITY_CLIENT_ID>"
```

The managed identity needs the `Azure Event Hubs Data Receiver` role on the Event Hub namespace.

## Step 3: Install with Helm

```bash
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set cloudAuditLog.enabled=true \
  --set cloudAuditLog.provider=AzureEventHub \
  --set cloudAuditLog.credentialSecretName=cloud-credentials \
  --set cloudAuditLog.clusterIdentity="/subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.ContainerService/managedClusters/<CLUSTER>" \
  --set cloudAuditLog.azure.eventHubNamespace="<NAMESPACE>.servicebus.windows.net" \
  --set cloudAuditLog.azure.eventHubName="<EVENT_HUB_NAME>"
```

## Step 4: Create an AudiciaSource

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
    clusterIdentity: "/subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.ContainerService/managedClusters/<CLUSTER>"
    azure:
      eventHubNamespace: "<NAMESPACE>.servicebus.windows.net"
      eventHubName: "<EVENT_HUB_NAME>"
      consumerGroup: "$Default"
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  ignoreSystemUsers: true
  checkpoint:
    intervalSeconds: 30
    batchSize: 500
```

## Step 5: Verify

Check that the operator is ingesting events:

```bash
# Check AudiciaSource status
kubectl get audiciasource aks-cloud-audit -n audicia-system -o yaml

# Check operator logs
kubectl logs -l app.kubernetes.io/name=audicia -n audicia-system

# Check metrics
kubectl port-forward svc/audicia-metrics 8080:8080 -n audicia-system
curl localhost:8080/metrics | grep audicia_cloud
```

You should see `audicia_cloud_messages_received_total` incrementing and `AudiciaPolicyReport` resources being created.

## Optional: Blob Storage Checkpoints

For production workloads, configure Azure Blob Storage for distributed checkpoint persistence:

```yaml
cloudAuditLog:
  azure:
    storageAccountURL: "https://<ACCOUNT>.blob.core.windows.net"
    storageContainerName: "audicia-checkpoints"
```

This enables the Event Hub processor to manage partition ownership across multiple replicas and persist offsets
independently of AudiciaSource status.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| No messages received | Diagnostic Settings not routing to Event Hub | Verify `az monitor diagnostic-settings show` |
| Authentication error | Wrong connection string or missing role assignment | Check Secret contents or workload identity setup |
| Events from wrong cluster | Shared Event Hub without `clusterIdentity` | Set `clusterIdentity` to the AKS resource ID |
| High `cloud_lag_seconds` | Consumer group falling behind | Check consumer group lag in Azure portal |

## Related

- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and design
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — Full `spec.cloud` field reference
- [Helm Values](../configuration/helm-values.md) — `cloudAuditLog` configuration
- [Metrics Reference](../reference/metrics.md) — Cloud metrics
