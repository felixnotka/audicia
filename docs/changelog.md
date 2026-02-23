# Changelog

All notable changes to Audicia are documented here.

The project uses automatic patch-level versioning: `version.json` defines Major.Minor, and CI auto-increments the patch on each release to `main`.

---

## 0.2.0

### Added
- **Cloud audit log ingestion** — New `CloudAuditLog` source type for managed Kubernetes platforms that export audit logs through cloud-native pipelines
- **Azure Event Hub adapter** — Full adapter for AKS audit logs via Azure Event Hub with Diagnostic Settings envelope parsing, partition-based checkpointing, and workload identity support
- **`spec.cloud` CRD fields** — `CloudConfig`, `AzureEventHubConfig`, `AWSCloudWatchConfig` (placeholder), `GCPPubSubConfig` (placeholder) types added to AudiciaSource
- **`status.cloudCheckpoint`** — Per-partition sequence number tracking for cloud source recovery
- **Cluster identity validation** — Defense-in-depth filter for shared Event Hub scenarios, matching events against `clusterIdentity`
- **`cloudAuditLog` Helm values** — Full configuration section for cloud provider, credentials, and Azure-specific settings
- **Cloud credential volume mount** — Conditional `cloud-credentials` Secret volume in the Deployment template
- **5 cloud Prometheus metrics** — `cloud_messages_received_total`, `cloud_messages_acked_total`, `cloud_receive_errors_total`, `cloud_lag_seconds`, `cloud_envelope_parse_errors_total`
- **Go build tags** — `azure` build tag for conditional Azure SDK compilation; default binary remains cloud-free
- **`build-azure` Make target** — Build and Docker targets for the Azure-enabled binary
- **Cloud Ingestion concept page** — Architecture overview of MessageSource/EnvelopeParser abstractions and provider registry
- **AKS Setup guide** — End-to-end walkthrough for Azure Event Hub configuration with connection string and Workload Identity paths
- **AKS Quick Start** — Streamlined getting-started guide for AKS cloud ingestion via Workload Identity
- **Cloud AKS example** — AudiciaSource YAML example for AKS Event Hub ingestion
- **Multi-arch Docker images** — CI now builds `linux/amd64` and `linux/arm64` images for ARM-based AKS node pools
- **Azure build tag in CI** — Lint, test, and Docker build pipelines include `-tags azure` so the Azure adapter is compiled, tested, and shipped

### Changed
- Platform compatibility table updated across docs: AKS now shows "Full support" for Cloud Mode
- Managed Kubernetes limitation updated: AKS addressed via cloud ingestion, EKS/GKE planned
- Dockerfile uses `TARGETARCH` from Buildx instead of hardcoded `GOARCH=amd64`
- AKS guide now includes full Workload Identity setup steps (managed identity, role assignment, federated credential)
- Helm install commands in AKS docs include `helm repo add` and separate variants for connection string vs Workload Identity

---

## 0.1.2

### Added
- `webhook.hostPort` Helm value — exposes the webhook directly on the host, bypassing ClusterIP routing issues with Cilium and other kube-proxy-free CNIs
- `webhook.service.nodePort` Helm value — optional NodePort service type for the webhook
- Dedicated [Kube-Proxy-Free Guide](guides/kube-proxy-free.md) covering hostPort setup, NodePort, and ClusterIP diagnostics
- [RBAC Policy Generation](concepts/rbac-generation.md) concept page — explains what gets generated, the observation-to-RBAC pipeline, safety guardrails, and how to use the output

### Fixed
- Remaining incorrect Helm chart name references in webhook and mTLS guides
- Documented audit log file permissions (root-owned) and two workarounds
- Documented kube-apiserver restart procedure for kubeadm clusters

### Changed
- Consolidated all kube-proxy-free / hostPort content into a single dedicated guide instead of duplicating across 7 files
- Main webhook docs (setup, quick start, mTLS, installation, helm values) now focus on the standard ClusterIP path with callout links to the kube-proxy-free guide

---

## 0.1.1

### Fixed
- Helm chart `image.tag` no longer pins a stale image digest; defaults to chart `appVersion`
- Fixed incorrect Helm chart name (`audicia` → `audicia-operator`) across installation docs
- Updated site favicon and privacy policy

---

## 0.1.0

**Released:** 2026-02-23

### Added
- Initial release of the Audicia Kubernetes operator
- File-based audit log ingestion with checkpoint/resume via inode tracking
- Production-ready webhook ingestion mode with TLS and mTLS client certificate verification
- AudiciaSource and AudiciaPolicyReport CRDs (`audicia.io/v1alpha1`)
- Subject normalizer (ServiceAccount identity extraction from audit events)
- Event normalizer (API group migration, subresource handling)
- Configurable noise filter with allow/deny chains
- Rule aggregator with firstSeen/lastSeen/count tracking
- Policy strategy engine with scopeMode, verbMerge, and wildcards knobs
- Per-namespace Role generation for cross-namespace ServiceAccounts
- Rendered Role, ClusterRole, RoleBinding, and ClusterRoleBinding output
- RBAC resolver and compliance diff engine for comparing observed vs. granted permissions
- Compliance scoring with Green/Yellow/Red severity bands
- Sensitive excess detection for secrets, nodes, webhooks, and CRDs
- Helm chart for single-command installation
- CI pipeline with automated versioning, linting, testing, Docker build, and GitHub Releases
- Documentation website with getting-started guides, component deep-dives, examples, and API reference
- Helm chart repository
- `--version` flag on the operator binary
