# Feature Reference

Index of all features supported by Audicia. Each entry links to the relevant
deep-dive documentation.

---

## Ingestion

- **File-based ingestion** — Tail a Kubernetes audit log file with
  checkpoint/resume and log rotation detection.
  [Ingestor](../components/ingestor.md)
- **Webhook ingestion** — Receive real-time audit events via HTTPS with TLS,
  mTLS, rate limiting, and deduplication. [Ingestor](../components/ingestor.md)
  | [Webhook Setup](../guides/webhook-setup.md)
- **Cloud ingestion** — Connect to cloud message buses (Azure Event Hub, AWS
  CloudWatch, GCP Pub/Sub) for managed Kubernetes audit logs.
  [Ingestor](../components/ingestor.md) |
  [Cloud Ingestion](../concepts/cloud-ingestion.md) |
  [AKS Setup](../guides/aks-setup.md) | [EKS Setup](../guides/eks-setup.md) |
  [GKE Setup](../guides/gke-setup.md)
- **Multi-mode** — Run file, webhook, and cloud ingestion simultaneously. Each
  AudiciaSource gets its own pipeline.

## Processing

- **Event filtering** — Ordered allow/deny chain with user and namespace
  patterns. First match wins. [Filter](../components/filter.md) |
  [Filter Recipes](../guides/filter-recipes.md)
- **Subject normalization** — Parses `system:serviceaccount:<ns>:<name>` into
  structured identities. [Normalizer](../components/normalizer.md)
- **Event normalization** — Subresource concatenation (`pods/exec`), API group
  migration (`extensions` → `apps`), non-resource URL handling.
  [Normalizer](../components/normalizer.md)
- **Rule aggregation** — Deduplication per subject with
  `firstSeen`/`lastSeen`/`count` tracking.
  [Aggregator](../components/aggregator.md)

## Policy Generation

- **Scope modes** — `NamespaceStrict` (per-namespace Roles) or
  `ClusterScopeAllowed` (single ClusterRole).
  [Strategy Engine](../components/strategy-engine.md)
- **Verb merging** — `Smart` collapses same-resource rules into merged verb
  lists; `Exact` keeps one rule per verb.
  [Strategy Engine](../components/strategy-engine.md)
- **Wildcard control** — `Forbidden` (never emit `*`) or `Safe` (allow when all
  8 standard verbs observed).
  [Strategy Engine](../components/strategy-engine.md)
- **Rendered output** — Complete, kubectl-ready YAML (Role, ClusterRole,
  RoleBinding, ClusterRoleBinding).
  [AudiciaPolicyReport CRD](crd-audiciapolicyreport.md)

## Compliance

- **RBAC drift detection** — Resolves effective permissions and diffs against
  observed usage. [Compliance Scoring](../concepts/compliance-scoring.md)
- **Compliance scoring** — `usedEffective / totalEffective × 100` with
  Green/Yellow/Red severity.
  [Compliance Scoring](../concepts/compliance-scoring.md)
- **Sensitive excess detection** — Flags unused grants on secrets, nodes,
  webhooks, CRDs, and other high-risk resources.
  [Compliance Engine](../components/compliance-engine.md)

## Operations

- **Checkpoint and persistence** — Periodic flush to `AudiciaSource.status`
  (etcd-backed) with conflict retry. [Controller](../components/controller.md)
- **Retention and limits** — Configurable max rules per report and retention
  window. [Helm Values](../configuration/helm-values.md)
- **Prometheus metrics** — 13 operator metrics covering events processed,
  filtered, rules generated, pipeline latency, and cloud ingestion.
  [Metrics](metrics.md)
- **Health probes** — Liveness and readiness endpoints for production
  monitoring. [Helm Values](../configuration/helm-values.md#health-probes)
- **Helm chart** — Single-command install from `charts.audicia.io`.
  [Helm Values](../configuration/helm-values.md)

## CRDs

- **`AudiciaSource`** — Input configuration defining where and how to ingest
  audit events. Short names: `as`, `asrc`. [Reference](crd-audiciasource.md)
- **`AudiciaPolicyReport`** — Output reports with observed rules, suggested
  policy, and compliance score. Short names: `apr`, `apreport`.
  [Reference](crd-audiciapolicyreport.md)

## Subject Types

| Kind             | Source                                                 |
| ---------------- | ------------------------------------------------------ |
| `ServiceAccount` | Parsed from `system:serviceaccount:<ns>:<name>`        |
| `User`           | Any non-system username (e.g., `admin@example.com`)    |
| `Group`          | Defined in CRD but not yet extracted from audit events |

## Platform Compatibility

| Platform             | File Mode     | Webhook Mode  | Cloud Mode   |
| -------------------- | ------------- | ------------- | ------------ |
| kubeadm (bare metal) | Full support  | Full support  | N/A          |
| k3s / RKE2           | Full support  | Full support  | N/A          |
| AKS                  | Not supported | Not supported | Full support |
| EKS                  | Not supported | Not supported | Full support |
| GKE                  | Not supported | Not supported | Full support |

Inode-based log rotation detection uses `syscall.Stat_t` on Linux. On non-Linux
platforms, inode detection is disabled — rotation falls back to file-not-found
handling.

For known limitations, see [Limitations](../limitations.md).
