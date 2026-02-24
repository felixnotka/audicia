# Quick Start: AKS Cloud Ingestion

This tutorial walks you through setting up Audicia to ingest audit logs from an Azure Kubernetes Service (AKS) cluster
via Azure Event Hub. Unlike file or webhook mode, cloud ingestion works without control plane access — AKS streams
audit logs to Event Hub, and Audicia consumes them.

## Prerequisites

- An Azure Event Hub namespace and instance receiving the diagnostic logs
- The Audicia operator image built with the `azure` build tag
- Helm 3

## Step 1: Enable AKS Diagnostic Settings

Configure your AKS cluster to send audit logs to the Event Hub:

```bash
az monitor diagnostic-settings create \
  --resource /subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.ContainerService/managedClusters/<CLUSTER> \
  --name aks-audit-to-eventhub \
  --event-hub <EVENT_HUB_NAME> \
  --event-hub-rule /subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.EventHub/namespaces/<NAMESPACE>/authorizationRules/RootManageSharedAccessKey \
  --logs '[{"category": "kube-audit-admin", "enabled": true}]'
```

> **Tip:** Use `kube-audit-admin` to reduce volume (excludes read-only events) or `kube-audit` for complete coverage.

## Step 2: Set Up Workload Identity

Create a managed identity, grant it access to the Event Hub, and federate it with the Kubernetes ServiceAccount:

```bash
# Create managed identity
az identity create \
  --name audicia-operator \
  --resource-group <RG> \
  --location <LOCATION>
```

Note the `clientId` from the output — you will need it below.

```bash
# Grant Event Hub read access
az role assignment create \
  --assignee <MANAGED_IDENTITY_CLIENT_ID> \
  --role "Azure Event Hubs Data Receiver" \
  --scope /subscriptions/<SUB>/resourceGroups/<RG>/providers/Microsoft.EventHub/namespaces/<NAMESPACE>

# Federate with the Kubernetes ServiceAccount
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

## Step 3: Install Audicia

```bash
helm repo add audicia https://charts.audicia.io

helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set cloudAuditLog.enabled=true \
  --set cloudAuditLog.provider=AzureEventHub \
  --set cloudAuditLog.azure.eventHubNamespace="<NAMESPACE>.servicebus.windows.net" \
  --set cloudAuditLog.azure.eventHubName="<EVENT_HUB_NAME>" \
  --set serviceAccount.annotations."azure\.workload\.identity/client-id"="<MANAGED_IDENTITY_CLIENT_ID>"
```

The Helm chart automatically adds the `azure.workload.identity/use: "true"` pod label when the Azure provider is
configured, which causes the Workload Identity webhook to inject the required credentials into the pod.

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

```bash
kubectl apply -f aks-cloud-audit.yaml
```

## Step 5: Verify Events Flow

Check that the operator is ingesting events:

```bash
# Check AudiciaSource status
kubectl get audiciasource aks-cloud-audit -n audicia-system

# Check operator logs
kubectl logs -l app.kubernetes.io/name=audicia -n audicia-system --tail=20
```

You should see `audicia_cloud_messages_received_total` incrementing. After a flush cycle, policy reports start
appearing:

```bash
kubectl get audiciapolicyreports --all-namespaces
```

## What's Next

- [AKS Setup Guide](../guides/aks-setup.md) — Full guide with blob checkpoints and troubleshooting
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and design
- [Filter Recipes](../guides/filter-recipes.md) — Common filter configurations for production
- [Compliance Scoring](../concepts/compliance-scoring.md) — How RBAC drift detection works
