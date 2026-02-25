# GKE Setup (Pub/Sub)

This guide walks through configuring Audicia to ingest audit logs from a GKE cluster via
Cloud Logging and Pub/Sub using Workload Identity Federation.

## Prerequisites

- A GKE cluster
- Helm 3
- `gcloud` CLI authenticated with sufficient permissions

## Step 1: Verify GKE Audit Logs in Cloud Logging

GKE automatically sends API server audit logs to Cloud Logging. Verify that audit events exist:

```bash
gcloud logging read \
  'resource.type="k8s_cluster" AND
   logName:"cloudaudit.googleapis.com%2Factivity" AND
   resource.labels.cluster_name="<CLUSTER_NAME>"' \
  --project <PROJECT_ID> \
  --limit 5 \
  --format json
```

You should see entries with `protoPayload.serviceName: "k8s.io"` and method names like
`io.k8s.core.v1.pods.list`.

## Step 2: Create a Pub/Sub Topic and Log Router Sink

Create a Pub/Sub topic for Audicia to consume, then route GKE audit logs to it:

```bash
# Create the Pub/Sub topic.
gcloud pubsub topics create audicia-audit-logs \
  --project <PROJECT_ID>

# Create a Cloud Logging sink that routes GKE audit events to the topic.
gcloud logging sinks create audicia-audit-sink \
  "pubsub.googleapis.com/projects/<PROJECT_ID>/topics/audicia-audit-logs" \
  --project <PROJECT_ID> \
  --log-filter='resource.type="k8s_cluster" AND
    logName:("cloudaudit.googleapis.com%2Factivity" OR
             "cloudaudit.googleapis.com%2Fdata_access") AND
    resource.labels.cluster_name="<CLUSTER_NAME>"'
```

The sink creates a writer service account automatically. Grant it publish access:

```bash
# Get the sink's writer identity.
WRITER_IDENTITY=$(gcloud logging sinks describe audicia-audit-sink \
  --project <PROJECT_ID> \
  --format='value(writerIdentity)')

# Grant Pub/Sub Publisher role to the sink.
gcloud pubsub topics add-iam-policy-binding audicia-audit-logs \
  --project <PROJECT_ID> \
  --member="${WRITER_IDENTITY}" \
  --role="roles/pubsub.publisher"
```

## Step 3: Create a Pub/Sub Subscription

Create a subscription for Audicia to pull messages from:

```bash
gcloud pubsub subscriptions create audicia-audit-sub \
  --topic audicia-audit-logs \
  --project <PROJECT_ID> \
  --ack-deadline=60 \
  --message-retention-duration=7d
```

The 60-second ack deadline gives Audicia time to process batches. The 7-day retention
ensures events are not lost during operator downtime.

## Step 4: Set Up Workload Identity Federation

Workload Identity Federation lets the Audicia ServiceAccount assume a GCP IAM identity
without static credentials.

**1. Create a GCP Service Account:**

```bash
gcloud iam service-accounts create audicia-operator \
  --display-name "Audicia Operator" \
  --project <PROJECT_ID>
```

**2. Grant Pub/Sub Subscriber permission:**

```bash
gcloud pubsub subscriptions add-iam-policy-binding audicia-audit-sub \
  --project <PROJECT_ID> \
  --member="serviceAccount:audicia-operator@<PROJECT_ID>.iam.gserviceaccount.com" \
  --role="roles/pubsub.subscriber"
```

**3. Bind the Kubernetes ServiceAccount to the GCP Service Account:**

```bash
gcloud iam service-accounts add-iam-policy-binding \
  audicia-operator@<PROJECT_ID>.iam.gserviceaccount.com \
  --project <PROJECT_ID> \
  --member="serviceAccount:<PROJECT_ID>.svc.id.goog[audicia-system/audicia-operator]" \
  --role="roles/iam.workloadIdentityUser"
```

> **Note:** The Kubernetes namespace and ServiceAccount name (`audicia-system:audicia-operator`)
> must match the Helm chart defaults.

## Step 5: Install with Helm

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

> **Tip:** Pin `--version` to a specific chart version for reproducible deployments.
> Check in `values-gke.yaml` alongside your other infrastructure config.

The `iam.gke.io/gcp-service-account` ServiceAccount annotation enables
[Workload Identity Federation](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity).
GKE projects a federated token into the pod, which the GCP client libraries discover automatically
via Application Default Credentials (ADC).

## Step 6: Create an AudiciaSource

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

## Step 7: Verify

Check that the operator is ingesting events:

```bash
# Check AudiciaSource status
kubectl get audiciasource gke-cloud-audit -n audicia-system -o yaml

# Check operator logs
kubectl logs -l app.kubernetes.io/name=audicia -n audicia-system

# Check metrics
kubectl port-forward svc/audicia-metrics 8080:8080 -n audicia-system
curl localhost:8080/metrics | grep audicia_cloud
```

You should see `audicia_cloud_messages_received_total` incrementing and `AudiciaPolicyReport` resources being created.

## Production Hardening

The steps above get Audicia running. For production environments, consider the following additional
measures.

### Data Access Logs

GKE Admin Activity logs are always on, but Data Access logs (which capture read operations like `list`
and `get`) must be enabled separately. Audicia benefits from both log types for complete RBAC coverage:

```bash
# Enable Data Access audit logs for GKE in your project's IAM audit config
gcloud projects get-iam-policy <PROJECT_ID> --format=json > policy.json
# Edit policy.json to add auditConfigs for container.googleapis.com
gcloud projects set-iam-policy <PROJECT_ID> policy.json
```

### Pub/Sub Encryption and Retention

By default, Pub/Sub encrypts messages with Google-managed keys. For regulated environments,
use a customer-managed encryption key (CMEK):

```bash
gcloud pubsub topics update audicia-audit-logs \
  --project <PROJECT_ID> \
  --topic-encryption-key="projects/<PROJECT_ID>/locations/<LOCATION>/keyRings/<RING>/cryptoKeys/<KEY>"
```

Review the subscription message retention (default 7 days in this guide) against your compliance
requirements.

### Pod Security

Add a `securityContext` to the Helm values to run the operator as non-root:

```yaml
# values-gke.yaml (add to existing file)
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65534
  fsGroup: 65534

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

### Network Policy

Restrict the operator's network access. See the [NetworkPolicy example](../examples/network-policy.md)
for a ready-to-use manifest that limits egress to the Kubernetes API server and Pub/Sub endpoints.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| No messages received | Log Router sink not routing events | Verify sink filter with `gcloud logging sinks describe` |
| No messages received | Subscription has no messages | Check `gcloud pubsub subscriptions pull --auto-ack` for messages |
| `PermissionDenied` on Pub/Sub | Missing subscriber permission | Verify `roles/pubsub.subscriber` on the subscription |
| `PermissionDenied` on topic | Sink writer identity lacks publish access | Grant `roles/pubsub.publisher` to the sink writer identity |
| Authentication error | Workload Identity not configured | Check SA annotation and GKE WIF binding |
| `could not find default credentials` | WIF token not projected | Verify GKE Workload Identity is enabled on the cluster and node pool |
| Events from wrong cluster | Sink filter too broad | Add `resource.labels.cluster_name` to the sink filter |
| 0 audit events parsed | Non-K8s audit entries | Verify sink filter includes `protoPayload.serviceName="k8s.io"` |
| High `cloud_lag_seconds` | Large backlog | Increase `checkpoint.batchSize`, check subscription backlog with `gcloud pubsub subscriptions describe` |

## Related

- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and design
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — Full `spec.cloud` field reference
- [Helm Values](../configuration/helm-values.md) — `cloudAuditLog` configuration
- [Metrics Reference](../reference/metrics.md) — Cloud metrics
