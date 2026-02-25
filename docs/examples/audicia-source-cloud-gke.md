# AudiciaSource: Cloud (GKE)

An AudiciaSource configured for cloud-based ingestion from a GKE cluster via Pub/Sub using Workload Identity Federation.

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
    clusterIdentity: "projects/my-project/locations/us-central1-a/clusters/my-cluster"
    gcp:
      projectID: "my-project"
      subscriptionID: "audicia-audit-sub"
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
| `cloud.provider` | `GCPPubSub` | GCP Pub/Sub adapter |
| `cloud.clusterIdentity` | GKE cluster resource name | Used to filter events if multiple clusters share a Pub/Sub topic |
| `cloud.gcp.projectID` | GCP project ID | The project containing the Pub/Sub subscription |
| `cloud.gcp.subscriptionID` | Subscription name | Must exist and have messages routed from Cloud Logging |

## Authentication

Authentication uses Workload Identity Federation. The ServiceAccount must be annotated with the GCP
Service Account email:

```bash
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set image.tag=<VERSION>-gcp \
  --set cloudAuditLog.enabled=true \
  --set cloudAuditLog.provider=GCPPubSub \
  --set cloudAuditLog.gcp.projectID="my-project" \
  --set cloudAuditLog.gcp.subscriptionID="audicia-audit-sub" \
  --set serviceAccount.annotations."iam\.gke\.io/gcp-service-account"="audicia-operator@my-project.iam.gserviceaccount.com"
```

The GCP Service Account needs the following role on the Pub/Sub subscription:

- `roles/pubsub.subscriber`

See the [GKE Setup Guide](../guides/gke-setup.md) for full Workload Identity Federation setup including
GCP Service Account creation, IAM binding, and Cloud Logging sink configuration.

## Related

- [GKE Setup Guide](../guides/gke-setup.md) — Step-by-step walkthrough
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture overview
- [AudiciaSource CRD Reference](../reference/crd-audiciasource.md) — Full field reference
