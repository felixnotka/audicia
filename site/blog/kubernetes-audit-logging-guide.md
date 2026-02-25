---
title: "How to Enable Kubernetes Audit Logging: Complete Guide (2026)"
seo_title: "How to Enable Kubernetes Audit Logging: Complete Guide (2026)"
published_at: 2026-03-10T08:00:00.000Z
snippet: "Step-by-step instructions to enable Kubernetes audit logging on kubeadm, kind, EKS, GKE, and AKS — with a recommended audit policy YAML."
description: "Enable Kubernetes audit logging step by step on kubeadm, kind, EKS, GKE, and AKS. Includes a recommended audit policy YAML and production tuning tips."
---

## Why Enable Audit Logging

The Kubernetes API server processes every request to your cluster — every
`kubectl get pods`, every controller reconciliation, every admission webhook
call. By default, none of this is recorded.

Audit logging changes that. When enabled, the kube-apiserver writes a structured
JSON record for every API request: who made it, what resource was accessed,
which verb was used, and whether it succeeded or failed.

This data is essential for:

- **Security investigation** — who deleted that namespace?
- **Compliance evidence** — proving which subjects accessed which resources
- **RBAC generation** — extracting the permissions workloads actually need
- **Anomaly detection** — spotting unusual access patterns

Without audit logging, you are flying blind.

## Audit Levels

Kubernetes supports four audit levels, from least to most verbose:

| Level             | What is recorded                                                                |
| ----------------- | ------------------------------------------------------------------------------- |
| `None`            | Nothing. The event is skipped entirely.                                         |
| `Metadata`        | Request metadata only: user, verb, resource, namespace, timestamp, status code. |
| `Request`         | Metadata plus the full request body.                                            |
| `RequestResponse` | Metadata plus the full request and response bodies.                             |

**For most use cases, `Metadata` is sufficient.** It captures everything needed
for RBAC generation, compliance evidence, and security investigation — without
the storage cost of full request/response bodies.

`RequestResponse` is useful for forensics (seeing exactly what was sent or
returned), but it dramatically increases log volume and can contain sensitive
data like Secret values in response bodies.

## Audit Backends

The kube-apiserver supports two audit backends:

### Log File Backend

Events are written as JSON lines to a file on the control plane node. Simple to
set up, works with any Kubernetes distribution that exposes apiserver flags.

```yaml
# kube-apiserver flags
- --audit-policy-file=/etc/kubernetes/audit-policy.yaml
- --audit-log-path=/var/log/kube-audit.log
- --audit-log-maxsize=100 # Max size in MB per file
- --audit-log-maxbackup=3 # Number of old files to retain
- --audit-log-maxage=7 # Max days to retain old files
```

### Webhook Backend

Events are sent as HTTP POST requests to an external endpoint. Useful when you
cannot (or do not want to) access files on the control plane node.

```yaml
# kube-apiserver flags
- --audit-policy-file=/etc/kubernetes/audit-policy.yaml
- --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

Both backends can be enabled simultaneously. You can also use `omitStages` to
skip the `RequestReceived` stage, which halves the number of events per API call
while still capturing the final `ResponseComplete` stage with the status code.

## The Recommended Audit Policy

Here is an audit policy that balances coverage with volume. It captures
everything needed for RBAC generation and security monitoring while filtering
the noisiest endpoints:

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Skip health/readiness endpoints (high volume, no RBAC value)
  - level: None
    nonResourceURLs:
      - /healthz*
      - /livez*
      - /readyz*

  # Skip apiserver-to-apiserver traffic
  - level: None
    users:
      - system:apiserver

  # Skip high-volume event objects
  - level: None
    verbs: ["get", "list", "watch"]
    resources:
      - group: ""
        resources: ["events"]

  # Log everything else at Metadata level
  - level: Metadata
    omitStages:
      - RequestReceived
```

**Why these rules:**

- **Health endpoints** generate enormous volume and are never useful for RBAC
  analysis
- **`system:apiserver`** traffic is internal to the API server and not relevant
  to workload permissions — filtering it reduces volume by roughly 30%
- **Event objects** (`events.v1`) are read-only and high-volume; excluding them
  avoids inflating generated policies with noise
- **`omitStages: [RequestReceived]`** skips the initial stage of each API call,
  halving the number of events while keeping the `ResponseComplete` stage that
  includes the status code

## Platform-Specific Instructions

### kubeadm

Edit the kube-apiserver static pod manifest on the control plane node:

```bash
sudo vi /etc/kubernetes/manifests/kube-apiserver.yaml
```

Add the audit flags to the command section:

```yaml
spec:
  containers:
    - command:
        - kube-apiserver
        # ... existing flags ...
        - --audit-policy-file=/etc/kubernetes/audit-policy.yaml
        - --audit-log-path=/var/log/kube-audit.log
        - --audit-log-maxsize=100
        - --audit-log-maxbackup=3
        - --audit-log-maxage=7
```

Add volume mounts for the policy file and log directory:

```yaml
  volumeMounts:
    - name: audit-policy
      mountPath: /etc/kubernetes/audit-policy.yaml
      readOnly: true
    - name: audit-log
      mountPath: /var/log/kube-audit.log
volumes:
  - name: audit-policy
    hostPath:
      path: /etc/kubernetes/audit-policy.yaml
      type: File
  - name: audit-log
    hostPath:
      path: /var/log/kube-audit.log
      type: FileOrCreate
```

Save the file. The kubelet detects the change and restarts the apiserver
automatically. Verify with:

```bash
ls -la /var/log/kube-audit.log
```

### kind (for local development)

kind requires a cluster configuration file to enable audit logging:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
          kind: ClusterConfiguration
          apiServer:
            extraArgs:
              audit-policy-file: /etc/kubernetes/audit-policy.yaml
              audit-log-path: /var/log/kube-audit.log
    extraMounts:
      - hostPath: ./audit-policy.yaml
        containerPath: /etc/kubernetes/audit-policy.yaml
        readOnly: true
```

Create the cluster:

```bash
kind create cluster --config kind-audit-config.yaml
```

The audit log is available inside the kind container at
`/var/log/kube-audit.log`. Audicia's Helm chart can mount it via `hostPath`.

### Amazon EKS

EKS does not expose apiserver flags directly. Instead, it streams audit logs to
CloudWatch Logs.

**Enable control plane logging** in the EKS console or via CLI:

```bash
aws eks update-cluster-config \
  --name my-cluster \
  --logging '{"clusterLogging":[{"types":["audit"],"enabled":true}]}'
```

Audit events are written to the CloudWatch log group
`/aws/eks/my-cluster/cluster`. You can process them with a Lambda function or
stream them to an external system.

**For Audicia:** Audicia natively supports EKS via the CloudWatch Logs adapter.
Configure an `AudiciaSource` with `provider: AWSCloudWatch` and point it at your
cluster's log group. Authentication uses IRSA (IAM Roles for Service Accounts).
See the [EKS setup guide](/docs/guides/eks-setup) for details.

### Google GKE

GKE enables admin activity audit logging by default. For data access logs (which
include read operations needed for complete RBAC analysis):

1. Open the Google Cloud Console
2. Navigate to **IAM & Admin → Audit Logs**
3. Find the **Kubernetes Engine API** service
4. Enable **Data Read** and **Data Write** log types

Audit events are available in Cloud Logging. Export them with a log sink:

```bash
gcloud logging sinks create gke-audit-sink \
  storage.googleapis.com/my-audit-bucket \
  --log-filter='resource.type="k8s_cluster" logName:"cloudaudit.googleapis.com"'
```

**For Audicia:** Audicia natively supports GKE via the Cloud Pub/Sub adapter.
Route audit logs to a Pub/Sub topic using a Cloud Logging sink, then configure
an `AudiciaSource` with `provider: GCPPubSub`. Authentication uses Workload
Identity Federation. See the [GKE setup guide](/docs/guides/gke-setup) for
details.

### Azure AKS

AKS provides audit logs through Azure Diagnostic Settings. Audicia has native
support for AKS via Azure Event Hub ingestion.

**Enable diagnostic settings:**

1. In the Azure portal, navigate to your AKS cluster
2. Go to **Monitoring → Diagnostic settings**
3. Add a diagnostic setting with **kube-audit** or **kube-audit-admin** category
4. Stream to an Event Hub

**Configure Audicia for AKS:**

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: aks-audit
spec:
  sourceType: CloudAuditLog
  cloud:
    provider: AzureEventHub
    clusterIdentity: "/subscriptions/.../managedClusters/my-cluster"
    azure:
      eventHubNamespace: "myns.servicebus.windows.net"
      eventHubName: "aks-audit-logs"
      consumerGroup: "$Default"
```

See the [AKS setup guide](/docs/getting-started/quick-start-aks) for the full
walkthrough.

## Production Tuning

### Namespace-Scoped Logging

If you only care about specific namespaces, scope the audit policy:

```yaml
rules:
  # Only log events in target namespaces
  - level: Metadata
    namespaces: ["production", "staging"]
    omitStages: ["RequestReceived"]

  # Skip everything else
  - level: None
```

This dramatically reduces volume in clusters with many namespaces.

### Log Rotation

For file-based logging, configure rotation to prevent disk exhaustion:

```yaml
- --audit-log-maxsize=100 # Rotate at 100 MB
- --audit-log-maxbackup=3 # Keep 3 old files
- --audit-log-maxage=7 # Delete files older than 7 days
```

Audicia handles log rotation automatically via inode tracking. When the
kube-apiserver rotates the log file (creating a new inode), Audicia detects the
change and resets its read offset to the beginning of the new file.

### Volume Estimation

A rough guide for audit log volume at `Metadata` level with
`omitStages:
[RequestReceived]`:

| Cluster size                         | Estimated daily volume |
| ------------------------------------ | ---------------------- |
| Dev (5 nodes, low traffic)           | 50–200 MB              |
| Staging (10 nodes, moderate traffic) | 200 MB–1 GB            |
| Production (50+ nodes, high traffic) | 1–10 GB                |

The recommended audit policy above reduces volume by 40–60% compared to logging
everything at `Metadata` level.

## Now That You Have Audit Logs

With audit logging enabled, your cluster is recording the data needed to
generate correct RBAC policies. The next step is to turn that data into action.

**[Generate RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** — see
how Audicia converts audit events into least-privilege Roles in a full before/
after walkthrough.

## Further Reading

- **[Kubernetes Audit Policy Design](/blog/kubernetes-audit-policy-design)** —
  advanced audit policy patterns for reducing noise
- **[What Can You Do with Kubernetes Audit Logs?](/blog/kubernetes-audit-log-use-cases)**
  — five use cases beyond storage
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start generating RBAC policies
