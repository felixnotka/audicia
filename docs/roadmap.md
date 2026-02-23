# Roadmap

## Vision

Audicia is the best tool for generating least-privilege RBAC policies from Kubernetes audit logs. Focus on doing one
thing extremely well: observe what your workloads do, generate correct policies, and show you where you're
overprivileged.

---

## Completed

Everything below is implemented, tested, and running in production.

| Feature                        | Description                                                                              |
|--------------------------------|------------------------------------------------------------------------------------------|
| **CRD Definitions**            | `AudiciaSource` and `AudiciaPolicyReport` v1alpha1 with full spec/status separation      |
| **File Ingestion**             | Tail K8s audit log files with checkpoint/resume, inode-based rotation detection          |
| **Webhook Ingestion**          | HTTPS receiver with TLS, mTLS, rate limiting, deduplication, backpressure                |
| **Subject Normalizer**         | Parse ServiceAccount, User, and Group identities from audit events                       |
| **Event Normalizer**           | Subresource concatenation, API group migration, non-resource URL handling                |
| **Noise Filtering**            | Configurable allow/deny chains for users and namespaces (first-match-wins)               |
| **Rule Aggregation**           | Deduplication with firstSeen/lastSeen/count tracking, deterministic output               |
| **Policy Strategy Engine**     | scopeMode, verbMerge, wildcards, resourceNames knobs                                     |
| **Rendered Output**            | Generate Role/ClusterRole/RoleBinding/ClusterRoleBinding YAML in AudiciaPolicyReport     |
| **RBAC Compliance Scoring**    | Resolve effective permissions, diff against observed usage, Green/Yellow/Red scoring     |
| **Sensitive Excess Detection** | Flag unused grants on secrets, nodes, webhooks, CRDs, tokenreviews                       |
| **Helm Chart**                 | Single-command install with file/webhook modes, ServiceMonitor, full RBAC                |
| **Prometheus Metrics**         | 13 operator metrics (events processed, filtered, rules generated, pipeline latency, cloud ingestion) |
| **mTLS Webhook Security**      | Client certificate verification with configurable CA bundle                              |
| **Cloud Ingestion (AKS)**      | Azure Event Hub adapter for AKS audit logs with cluster identity validation              |
| **Cloud Metrics**              | 5 dedicated cloud ingestion metrics (messages received, acked, errors, lag, parse errors) |

---

## In Progress

| Feature                   | Description                                                              | Status  |
|---------------------------|--------------------------------------------------------------------------|---------|
| **Documentation Website** | Deno + Fresh docs site (kubernetes.io-style) with getting started guides | Active  |
| **Helm Registry**         | Publish chart to a public Helm registry for `helm repo add` installation | Planned |
| **CI Hardening**          | GitHub Actions: lint, unit tests, e2e tests on PRs, vulnerability scans  | Planned |
| **Supply Chain Security** | Cosign image signing, SBOM publishing, govulncheck in CI                 | Planned |

---

## Next

| Feature                | Description                                                                    |
|------------------------|--------------------------------------------------------------------------------|
| **Cloud Ingestion (EKS, GKE)** | AWS CloudWatch and GCP Pub/Sub adapters for EKS and GKE audit logs           |
| **Dashboard UI**       | Web interface for browsing reports, compliance posture, and policy suggestions |
| **GitOps Integration** | AudiciaPolicyReport triggers a PR to your policy repo with suggested changes   |
| **Historical Diffing** | Structured diffs on observedRules between report versions                      |
| **Kubernetes Events** | Emit Kubernetes Events on AudiciaSource and AudiciaPolicyReport for key state transitions (IngestionStarted, ReportUpdated, IngestionError, etc.) |
| **Extended Conditions** | Add Degraded, Error, Processing, and Stale conditions to AudiciaSource and AudiciaPolicyReport CRDs |

---

## Ideas (No Timeline)

Things that might make sense eventually, but only after the core is rock-solid:

- **Multi-cluster aggregation** — Unified view across multiple clusters
- **Compliance report exports** — PDF/CSV for SOC 2, ISO 27001, PCI DSS audits
- **Anomaly alerting** — Detect permission spikes via Prometheus/Alertmanager

---

## Design Principles

1. **Never auto-apply.** Audicia generates recommendations. Humans (or reviewed GitOps pipelines) apply them.
2. **CRD-native.** Every output is a Kubernetes resource. No side-channel files, no external databases required.
3. **Minimal operator permissions.** Audicia itself should be a model of least-privilege.
4. **Do one thing well.** Focus on audit log → RBAC policy generation. Don't try to be a platform.
