# Quick Start: GKE Cloud Ingestion

This tutorial walks you through setting up Audicia to ingest audit logs from a
GKE cluster via Cloud Logging and Pub/Sub. Unlike file or webhook mode, cloud
ingestion works without control plane access — GKE sends audit logs to Cloud
Logging, a log router sink forwards them to Pub/Sub, and Audicia consumes them.

## Prerequisites

- A GKE cluster
- Helm 3
- `gcloud` CLI authenticated with sufficient permissions

## Step 1: Create a Pub/Sub Topic and Log Router Sink

Create a Pub/Sub topic for Audicia to consume, then route GKE audit logs to it:

```bash
# Create the Pub/Sub topic
gcloud pubsub topics create audicia-audit-logs \
  --project <PROJECT_ID>

# Create a Cloud Logging sink that routes GKE audit events to the topic
gcloud logging sinks create audicia-audit-sink \
  "pubsub.googleapis.com/projects/<PROJECT_ID>/topics/audicia-audit-logs" \
  --project <PROJECT_ID> \
  --log-filter='resource.type="k8s_cluster" AND
    logName:("cloudaudit.googleapis.com%2Factivity" OR
             "cloudaudit.googleapis.com%2Fdata_access") AND
    resource.labels.cluster_name="<CLUSTER_NAME>"'
```

Grant the sink's writer identity publish access:

```bash
WRITER_IDENTITY=$(gcloud logging sinks describe audicia-audit-sink \
  --project <PROJECT_ID> \
  --format='value(writerIdentity)')

gcloud pubsub topics add-iam-policy-binding audicia-audit-logs \
  --project <PROJECT_ID> \
  --member="${WRITER_IDENTITY}" \
  --role="roles/pubsub.publisher"
```

Create a subscription for Audicia to pull from:

```bash
gcloud pubsub subscriptions create audicia-audit-sub \
  --topic audicia-audit-logs \
  --project <PROJECT_ID> \
  --ack-deadline=60 \
  --message-retention-duration=7d
```

## Step 2: Set Up Workload Identity Federation

Create a GCP Service Account, grant it Pub/Sub access, and bind it to the
Kubernetes ServiceAccount:

```bash
# Create GCP Service Account
gcloud iam service-accounts create audicia-operator \
  --display-name "Audicia Operator" \
  --project <PROJECT_ID>

# Grant Pub/Sub Subscriber permission
gcloud pubsub subscriptions add-iam-policy-binding audicia-audit-sub \
  --project <PROJECT_ID> \
  --member="serviceAccount:audicia-operator@<PROJECT_ID>.iam.gserviceaccount.com" \
  --role="roles/pubsub.subscriber"

# Bind Kubernetes SA to GCP SA
gcloud iam service-accounts add-iam-policy-binding \
  audicia-operator@<PROJECT_ID>.iam.gserviceaccount.com \
  --project <PROJECT_ID> \
  --member="serviceAccount:<PROJECT_ID>.svc.id.goog[audicia-system/audicia-operator]" \
  --role="roles/iam.workloadIdentityUser"
```

> **Note:** The Kubernetes namespace and ServiceAccount name
> (`audicia-system:audicia-operator`) must match the Helm chart defaults.

## Step 3: Install Audicia

Create a `values-gke.yaml` file with your cluster-specific configuration:

```yaml
# values-gke.yaml
cloudAuditLog:
  enabled: true
  provider: GCPPubSub
  gcp:
    projectID: "<PROJECT_ID>"
    subscriptionID: "audicia-audit-sub"

image:
  tag: "<VERSION>-gcp"

serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: "audicia-operator@<PROJECT_ID>.iam.gserviceaccount.com"
```

Install with Helm:

```bash
helm repo add audicia https://charts.audicia.io

helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  --version <VERSION> \
  -f values-gke.yaml
```

> **Tip:** Pin `--version` to a specific chart version for reproducible
> deployments. Check in `values-gke.yaml` alongside your other infrastructure
> config.

The `iam.gke.io/gcp-service-account` ServiceAccount annotation enables
[Workload Identity Federation](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity).
GKE projects a federated token into the pod, which the GCP client libraries
discover automatically via Application Default Credentials (ADC).

## Step 4: Create an AudiciaSource

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: gke-cloud-audit
  namespace: audicia-system
spec:
  sourceType: CloudAuditLog
  cloud:
    provider: GCPPubSub
    clusterIdentity: "projects/<PROJECT_ID>/locations/<LOCATION>/clusters/<CLUSTER_NAME>"
    gcp:
      projectID: "<PROJECT_ID>"
      subscriptionID: "audicia-audit-sub"
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
kubectl apply -f gke-cloud-audit.yaml
```

## Step 5: Verify Events Flow

Check that the operator is ingesting events:

```bash
# Check AudiciaSource status
kubectl get audiciasource gke-cloud-audit -n audicia-system

# Check operator logs
kubectl logs -l app.kubernetes.io/name=audicia -n audicia-system --tail=20
```

You should see `audicia_cloud_messages_received_total` incrementing. After a
flush cycle, policy reports start appearing:

```bash
kubectl get audiciapolicyreports --all-namespaces
```

## What's Next

- [GKE Setup Guide](../guides/gke-setup.md) — Full guide with log router
  details, production hardening, and troubleshooting
- [NetworkPolicy Example](../examples/network-policy.md) — Restrict Audicia
  network access
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and
  design
- [Filter Recipes](../guides/filter-recipes.md) — Common filter configurations
  for production
- [Compliance Scoring](../concepts/compliance-scoring.md) — How RBAC drift
  detection works
