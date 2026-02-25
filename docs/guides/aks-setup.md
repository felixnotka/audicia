# AKS Setup (Event Hub)

This guide walks through configuring Audicia to ingest audit logs from an Azure Kubernetes Service (AKS) cluster via
Azure Event Hub using Workload Identity.

## Prerequisites

- An AKS cluster
- An Azure Event Hub namespace and instance
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

## Step 2: Set Up Workload Identity

Audicia authenticates to Azure Event Hub using Workload Identity. Create a managed identity, grant it access, and
federate it with the Kubernetes ServiceAccount.

**1. Create a managed identity:**

```bash
az identity create \
  --name audicia-operator \
  --resource-group <RG> \
  --location <LOCATION>
```

Note the `clientId` from the output — this is the `<MANAGED_IDENTITY_CLIENT_ID>` used below.

**2. Grant it the Event Hubs Data Receiver role:**

```bash
az role assignment create \
  --assignee <MANAGED_IDENTITY_CLIENT_ID> \
  --role "Azure Event Hubs Data Receiver" \
  --scope /subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.EventHub/namespaces/<NAMESPACE>
```

**3. Federate the identity with the Kubernetes ServiceAccount:**

```bash
AKS_OIDC_ISSUER=$(az aks show --name <CLUSTER> --resource-group <RG> --query "oidcIssuerProfile.issuerUrl" -o tsv)

az identity federated-credential create \
  --name audicia-federated \
  --identity-name audicia-operator \
  --resource-group <RG> \
  --issuer "$AKS_OIDC_ISSUER" \
  --subject system:serviceaccount:audicia-system:audicia-operator \
  --audiences api://AzureADTokenExchange
```

> **Note:** The `--subject` must match the namespace and ServiceAccount name used by the Helm chart
> (`audicia-system:audicia-operator` by default).

## Step 3: Install with Helm

```bash
helm repo add audicia https://charts.audicia.io

helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set image.tag=<VERSION>-azure \
  --set cloudAuditLog.enabled=true \
  --set cloudAuditLog.provider=AzureEventHub \
  --set cloudAuditLog.clusterIdentity="/subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.ContainerService/managedClusters/<CLUSTER>" \
  --set cloudAuditLog.azure.eventHubNamespace="<NAMESPACE>.servicebus.windows.net" \
  --set cloudAuditLog.azure.eventHubName="<EVENT_HUB_NAME>" \
  --set serviceAccount.annotations."azure\.workload\.identity/client-id"="<MANAGED_IDENTITY_CLIENT_ID>"
```

The Helm chart automatically adds the `azure.workload.identity/use: "true"` pod label when the Azure provider is
configured, which causes the Workload Identity webhook to inject `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, and the
federated token volume into the pod.

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
| Authentication error | Missing role assignment or unfederated identity | Verify `az role assignment list` and federated credential |
| Multiple identity error | Pod has multiple identities, missing WI label | Ensure `azure.workload.identity/use: "true"` pod label is set |
| Events from wrong cluster | Shared Event Hub without `clusterIdentity` | Set `clusterIdentity` to the AKS resource ID |
| High `cloud_lag_seconds` | Consumer group falling behind | Check consumer group lag in Azure portal |

## Related

- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and design
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — Full `spec.cloud` field reference
- [Helm Values](../configuration/helm-values.md) — `cloudAuditLog` configuration
- [Metrics Reference](../reference/metrics.md) — Cloud metrics
