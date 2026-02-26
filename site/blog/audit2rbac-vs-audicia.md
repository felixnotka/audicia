---
title: "audit2rbac vs Audicia: CLI Script vs Kubernetes Operator"
seo_title: "audit2rbac vs Audicia: CLI Script vs Kubernetes Operator"
published_at: 2026-03-07T08:00:00.000Z
snippet: "A side-by-side comparison of audit2rbac and Audicia — two tools that generate RBAC from audit logs, with very different architectures."
description: "Compare audit2rbac and Audicia for Kubernetes RBAC generation. Side-by-side feature comparison: CLI vs Operator, stateless vs stateful, raw YAML vs CRDs."
---

## Two Approaches to the Same Problem

Both audit2rbac and Audicia solve the same fundamental problem: generating
Kubernetes RBAC policies from audit log data instead of writing them by hand.
Both read audit events, extract which subjects accessed which resources, and
produce Role/ClusterRole manifests.

The difference is in architecture. audit2rbac is a CLI tool designed for one-
time analysis. Audicia is a Kubernetes Operator designed for continuous
operation. This architectural difference cascades into every other capability.

## Respecting audit2rbac

audit2rbac was created by Tim Allclair (@liggitt), a Kubernetes SIG-Auth co-
lead. It is simple, effective, and well-established. If you need a quick, one-
time analysis of an audit log file, audit2rbac does the job with minimal setup.

Audicia exists because many teams need more than one-time analysis. They need
continuous RBAC generation that runs as part of the cluster infrastructure, with
state management, compliance scoring, and GitOps integration.

## Side-by-Side Comparison

| Capability                 | audit2rbac                       | Audicia                                      |
| -------------------------- | -------------------------------- | -------------------------------------------- |
| **Architecture**           | CLI tool                         | Kubernetes Operator                          |
| **Execution model**        | One-shot (run, get output, done) | Continuous (runs in-cluster)                 |
| **State across restarts**  | None (re-reads entire log)       | Checkpoint/resume via CRD status             |
| **Output format**          | Raw YAML to stdout               | `AudiciaPolicyReport` CRD                    |
| **Ingestion modes**        | File only                        | File, webhook, cloud (AKS)                   |
| **Subject normalization**  | Partial                          | Full (SA, User, Group parsing)               |
| **Subresource handling**   | Partial                          | Full concatenation (`pods/exec`)             |
| **API group migration**    | No                               | Yes (`extensions` → `apps`)                  |
| **Strategy configuration** | No                               | Yes (scopeMode, verbMerge, wildcards)        |
| **Filter chain**           | No                               | Ordered allow/deny rules                     |
| **Compliance scoring**     | No                               | 0–100 score with severity bands              |
| **Drift detection**        | No                               | Observed vs. granted diff                    |
| **Webhook ingestion**      | No                               | TLS/mTLS server with rate limiting           |
| **Cloud ingestion**        | No                               | Azure Event Hub (AKS)                        |
| **Deduplication**          | No                               | LRU cache (webhook), aggregation (all modes) |
| **CRD integration**        | No                               | Input and output are CRDs                    |
| **Helm chart**             | No                               | Full chart with RBAC, scheduling, TLS        |

## The Gaps That Matter

### No State Across Restarts

audit2rbac re-reads the entire audit log file from the beginning on every run.
For a large audit log (hundreds of megabytes), this means re-processing millions
of events each time. There is no checkpoint mechanism.

Audicia tracks its read position in the `AudiciaSource` CRD status field. On
restart, it resumes from where it left off. On log rotation (detected via inode
change on Linux), it automatically resets and begins reading the new file.

### No CRD Output

audit2rbac writes raw YAML to stdout. You pipe it to a file, review it, and
apply it. There is no structured Kubernetes resource representing the output.

Audicia writes `AudiciaPolicyReport` CRDs. These are first-class Kubernetes
resources that you can query with `kubectl`, watch with controllers, and
integrate into GitOps pipelines. The report includes the suggested manifests,
compliance metadata, observed rule counts, and timestamps.

```bash
# audit2rbac: pipe to file
audit2rbac -f audit.log --serviceaccount my-team:backend > backend-role.yaml

# Audicia: query the CRD
kubectl get audiciapolicyreport report-backend -n my-team -o yaml
```

### No Compliance Scoring

audit2rbac generates policies. It does not tell you how those policies compare
to what is currently granted. There is no concept of "this service account uses
25% of its permissions."

Audicia's diff engine compares observed permissions (from audit logs) against
granted permissions (from existing Roles and Bindings). The result is a
compliance score from 0 to 100 with Red/Yellow/Green severity bands.

### No API Group Migration

Kubernetes deprecated the `extensions/v1beta1` API group years ago. Workloads
migrated to `apps/v1`, but old audit logs (and some controllers) still reference
the deprecated group. audit2rbac passes API groups through verbatim, which can
produce policies referencing deprecated groups.

Audicia's normalizer automatically migrates `extensions/v1beta1` to `apps/v1`,
ensuring generated policies use current API versions.

### No Strategy Configuration

audit2rbac produces one output shape: one Role per namespace with all observed
verbs listed individually. There are no knobs to control the output.

Audicia provides configurable strategy knobs:

- **scopeMode** — `NamespaceStrict` for per-namespace Roles, or
  `ClusterScopeAllowed` for a single ClusterRole
- **verbMerge** — `Smart` collapses rules with the same resource into one rule
  with merged verbs, `Exact` keeps them separate
- **wildcards** — `Forbidden` never generates `*`, `Safe` allows `*` only when
  all 8 standard verbs are observed

### No Filtering

audit2rbac processes every event in the file. If system components generate
noise, that noise becomes part of the generated policy.

Audicia has an ordered allow/deny filter chain. The default configuration
filters `system:node:*`, `system:kube-*`, and `system:apiserver`. Custom
patterns can be added per `AudiciaSource`.

### No Webhook Ingestion

audit2rbac reads files. If your cluster uses the webhook audit backend instead
of file-based logging, audit2rbac cannot ingest events directly.

Audicia supports a webhook ingestion mode with a TLS (or mTLS) HTTPS server. The
kube-apiserver pushes events to Audicia in real time. This includes rate
limiting (token bucket, default 100/sec), request body limits, and LRU
deduplication.

## When to Use Each Tool

**Use audit2rbac when:**

- You need a quick, one-time analysis of an audit log file
- You are debugging a specific 403 error and want to see what permissions are
  needed
- You do not need continuous monitoring or compliance scoring
- Your audit log file is small enough to re-process from scratch each time

**Use Audicia when:**

- You need continuous RBAC generation running in your cluster
- You want compliance scoring and drift detection
- You use GitOps and need CRD-based output
- Your audit logs are large and you need checkpoint/resume
- You need webhook or cloud-based ingestion
- You want configurable policy strategy (scope, verb merging, wildcards)
- You need to produce compliance evidence for SOC 2, ISO 27001, or similar
  frameworks

## Getting Started

Audicia installs in under five minutes:

```bash
helm install audicia oci://ghcr.io/felixnotka/audicia/charts/audicia-operator \
  -n audicia-system --create-namespace \
  --set auditLog.enabled=true \
  --set auditLog.hostPath=/var/log/kubernetes/audit/audit.log
```

See the [getting started guide](/docs/getting-started/introduction) for the full
walkthrough.

## Further Reading

- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  full before/after walkthrough
- **[Kubernetes RBAC Tools Compared](/blog/kubernetes-rbac-tools-compared)** —
  where generators fit alongside scanners and enforcers
- **[Pipeline Architecture](/docs/concepts/pipeline)** — how Audicia processes
  events through six stages
