# Changelog

All notable changes to Audicia are documented here.

The project uses automatic patch-level versioning: `version.json` defines
Major.Minor, and CI auto-increments the patch on each release to `main`.

---

## 0.3.3

### Added

- **`excessRules` and `uncoveredRules` in ComplianceReport** — the CRD status
  now includes the full rule lists for excess and uncovered permissions, making
  reports self-contained. Previously only counts were reported, requiring manual
  diffs to identify specific unused or ungranted rules
- **"How mTLS Works" section** in the Webhook Setup Guide — clear conceptual
  explanation of the three-step mTLS handshake, moved from the standalone guide
- **"Verify mTLS Is Working" section** in the Webhook Setup Guide — includes
  curl test for unauthorized client rejection

### Changed

- **Getting-started guides use values files** — all installation guides now show
  named `values-*.yaml` files (`values-file.yaml`, `values-webhook.yaml`,
  `values-webhook-mtls.yaml`, `values-dual.yaml`) instead of long `--set` chains
- **File-based `kubectl apply`** — quick-start guides use
  `kubectl apply -f <file>.yaml` instead of heredoc (`<<EOF`) patterns
- **Self-contained quick starts** — file and webhook quick-start guides now
  include their own Helm install steps instead of deferring to the installation
  page
- **mTLS documentation consolidated** — `webhook-setup.md` is now the single
  source of truth for all webhook TLS and mTLS configuration; `mtls-setup.md`
  replaced with a redirect page preserving existing bookmarks
- **Cross-references updated** — 7 links across 5 files now point to the correct
  `webhook-setup.md` anchors instead of `mtls-setup.md`

### Fixed

- SonarQube quality gate failure on `zz_generated.deepcopy.go` — excluded
  generated deepcopy files (`**/zz_generated.*.go`) from duplication analysis

---

## 0.3.2

### Fixed

- E2E file-mode tests failing after audit log path migration: the kube-apiserver
  creates audit log files with `0600` permissions (owner-only). The E2E helm
  install now sets `runAsUser=0` to match production file-mode requirements,
  where root access is needed to read the audit log
- Deployment template hostPath volume type changed from `File` (requires
  pre-existing file) to unset (no pre-existence check), improving compatibility
  with clusters where the audit log is created by the kube-apiserver at startup

---

## 0.3.1

### Added

- **`hostNetwork` Helm value** — enables host network namespace for the operator
  pod, bypassing CNI service routing issues on control plane nodes. Required for
  file-mode deployments on Cilium and other kube-proxy-free clusters where pods
  cannot reach the Kubernetes API server ClusterIP (`10.96.0.1:443`). See the
  updated [Kube-Proxy-Free Guide](guides/kube-proxy-free.md)
- **`dnsPolicy` Helm value** — configurable DNS policy; automatically set to
  `ClusterFirstWithHostNet` when `hostNetwork` is enabled
- **Startup retry with exponential backoff** — the operator now retries startup
  up to 5 times (2s, 4s, 8s, 16s, 32s, capped at 60s) instead of crashing
  immediately on transient API server connectivity failures. Configurable via
  `STARTUP_MAX_RETRIES` environment variable
- Kube-Proxy-Free Guide updated with a dedicated
  [File Mode section](guides/kube-proxy-free.md#file-mode-hostnetwork) covering
  the `hostNetwork` workaround

### Changed

- **Default audit log path** — standardized to
  `/var/log/kubernetes/audit/audit.log` across Helm defaults, docs, examples,
  and kind configs, matching the CNCF recommended path (previously
  `/var/log/kube-audit.log`)
- **Leader election disabled by default** — single-replica deployments (the
  default) no longer require leader election, removing an unnecessary API
  dependency at startup. Enable it explicitly with
  `operator.leaderElection.enabled=true` when running multiple replicas
- File mode installation examples across all docs now include
  `--set
  hostNetwork=true` for kube-proxy-free clusters

### Fixed

- Operator startup failure on Cilium kube-proxy-free clusters in file mode: pods
  on control plane nodes could not reach the Kubernetes service ClusterIP
  through the CNI datapath, causing `dial tcp 10.96.0.1:443: i/o timeout` during
  RBAC cache informer initialization
- `staticcheck SA1019` — replaced deprecated `result.Requeue` with
  `result.RequeueAfter` in controller reconcile tests

---

## 0.3.0

### Added

- **SonarQube quality gate enforcement** — PRs that fail the SonarQube quality
  gate can no longer be merged; the `sonarqube-quality-gate-action` step blocks
  the pipeline
- **Nightly CI workflow** — Scheduled build (02:00 UTC daily) runs tests,
  coverage, and SonarQube analysis independently of the main pipeline; also
  supports manual dispatch
- **E2E tests in main pipeline** — End-to-end tests now run as part of the
  standard lint-and-test workflow on every PR, not just nightly
- **Per-cloud-provider Docker images** — CI builds separate images with `azure`,
  `aws`, and `gcp` build tags alongside the default cloud-free image
- **README badges** — Pipeline status, nightly status, and license badges
- **Controller test coverage** — Unit tests for `flushCloudCheckpoint`,
  `eventLoop`, and additional uncovered controller paths
- **EKS and GKE documentation** — Quick start guides, setup guides, and example
  manifests for AWS CloudWatch Logs and GCP Pub/Sub ingestion

### Fixed

- 19 SonarQube code issues across operator and site: reduced cognitive
  complexity in GCP parser and docs search index builder, replaced deprecated
  patterns (`.match()` → `RegExp.exec()`, `.replace()` → `.replaceAll()`), added
  `Readonly` props, switched to `TypeError`, used `String.raw` template tags,
  stable React keys, and PascalCase component naming
- 3 additional SonarQube issues from post-scan feedback: removed unnecessary
  non-null assertions, fixed interactive role on non-interactive element
- Controller `staticcheck QF1008` — removed redundant embedded field selector
- E2E race condition and lint errors in controller tests
- Duplicate `.footer` CSS selector merged into one block

### Changed

- SonarQube coverage and duplication exclusions tuned to reduce false positives
  on test files, site code, and cloud provider adapters
- Docs navigation updated with EKS/GKE cloud examples

---

## 0.2.1

### Added

- **AWS CloudWatch adapter** — Adapter for EKS audit logs via CloudWatch Logs
  with workload identity support
- **GCP Pub/Sub adapter** — Adapter for GKE audit logs via Cloud Pub/Sub with
  Cloud Logging LogEntry parsing and raw K8s event auto-detection
- **SEO foundation** — Meta tags, sitemap, RSS feed, 404 page, and internal link
  structure for the documentation site
- **Blog content** — 20 SEO blog posts covering Kubernetes RBAC, audit logging,
  and security automation topics

### Fixed

- GCP parse lint error — removed always-nil error return
- GCP parse type error and missing cloud adapter dependencies

---

## 0.2.0

### Added

- **Cloud audit log ingestion** — New `CloudAuditLog` source type for managed
  Kubernetes platforms that export audit logs through cloud-native pipelines
- **Azure Event Hub adapter** — Full adapter for AKS audit logs via Azure Event
  Hub with Diagnostic Settings envelope parsing, partition-based checkpointing,
  and workload identity support
- **`spec.cloud` CRD fields** — `CloudConfig`, `AzureEventHubConfig`,
  `AWSCloudWatchConfig` (placeholder), `GCPPubSubConfig` (placeholder) types
  added to AudiciaSource
- **`status.cloudCheckpoint`** — Per-partition sequence number tracking for
  cloud source recovery
- **Cluster identity validation** — Defense-in-depth filter for shared Event Hub
  scenarios, matching events against `clusterIdentity`
- **`cloudAuditLog` Helm values** — Full configuration section for cloud
  provider and Azure-specific settings
- **Azure Workload Identity pod label** — Helm chart auto-adds
  `azure.workload.identity/use: "true"` pod label for AzureEventHub provider
- **5 cloud Prometheus metrics** — `cloud_messages_received_total`,
  `cloud_messages_acked_total`, `cloud_receive_errors_total`,
  `cloud_lag_seconds`, `cloud_envelope_parse_errors_total`
- **Go build tags** — `azure` build tag for conditional Azure SDK compilation;
  default binary remains cloud-free
- **`build-azure` Make target** — Build and Docker targets for the Azure-enabled
  binary
- **Cloud Ingestion concept page** — Architecture overview of
  MessageSource/EnvelopeParser abstractions and provider registry
- **AKS Setup guide** — End-to-end walkthrough for Azure Event Hub configuration
  with Workload Identity
- **AKS Quick Start** — Streamlined getting-started guide for AKS cloud
  ingestion via Workload Identity
- **Cloud AKS example** — AudiciaSource YAML example for AKS Event Hub ingestion
- **Multi-arch Docker images** — CI now builds `linux/amd64` and `linux/arm64`
  images for ARM-based AKS node pools
- **Azure build tag in CI** — Lint, test, and Docker build pipelines include
  `-tags azure` so the Azure adapter is compiled, tested, and shipped

### Changed

- Platform compatibility table updated across docs: AKS now shows "Full support"
  for Cloud Mode
- Managed Kubernetes limitation updated: AKS addressed via cloud ingestion,
  EKS/GKE planned
- Dockerfile uses `TARGETARCH` from Buildx instead of hardcoded `GOARCH=amd64`
- AKS guide now includes full Workload Identity setup steps (managed identity,
  role assignment, federated credential)
- Helm install commands in AKS docs include `helm repo add` and Workload
  Identity ServiceAccount annotation
- Removed connection string authentication — Azure Event Hub now uses Workload
  Identity exclusively
- Removed `credentialSecretName` from CRD, Helm values, and deployment template

---

## 0.1.2

### Added

- `webhook.hostPort` Helm value — exposes the webhook directly on the host,
  bypassing ClusterIP routing issues with Cilium and other kube-proxy-free CNIs
- `webhook.service.nodePort` Helm value — optional NodePort service type for the
  webhook
- Dedicated [Kube-Proxy-Free Guide](guides/kube-proxy-free.md) covering hostPort
  setup, NodePort, and ClusterIP diagnostics
- [RBAC Policy Generation](concepts/rbac-generation.md) concept page — explains
  what gets generated, the observation-to-RBAC pipeline, safety guardrails, and
  how to use the output

### Fixed

- Remaining incorrect Helm chart name references in webhook and mTLS guides
- Documented audit log file permissions (root-owned) and two workarounds
- Documented kube-apiserver restart procedure for kubeadm clusters

### Changed

- Consolidated all kube-proxy-free / hostPort content into a single dedicated
  guide instead of duplicating across 7 files
- Main webhook docs (setup, quick start, mTLS, installation, helm values) now
  focus on the standard ClusterIP path with callout links to the kube-proxy-free
  guide

---

## 0.1.1

### Fixed

- Helm chart `image.tag` no longer pins a stale image digest; defaults to chart
  `appVersion`
- Fixed incorrect Helm chart name (`audicia` → `audicia-operator`) across
  installation docs
- Updated site favicon and privacy policy

---

## 0.1.0

**Released:** 2026-02-23

### Added

- Initial release of the Audicia Kubernetes operator
- File-based audit log ingestion with checkpoint/resume via inode tracking
- Production-ready webhook ingestion mode with TLS and mTLS client certificate
  verification
- AudiciaSource and AudiciaPolicyReport CRDs (`audicia.io/v1alpha1`)
- Subject normalizer (ServiceAccount identity extraction from audit events)
- Event normalizer (API group migration, subresource handling)
- Configurable noise filter with allow/deny chains
- Rule aggregator with firstSeen/lastSeen/count tracking
- Policy strategy engine with scopeMode, verbMerge, and wildcards knobs
- Per-namespace Role generation for cross-namespace ServiceAccounts
- Rendered Role, ClusterRole, RoleBinding, and ClusterRoleBinding output
- RBAC resolver and compliance diff engine for comparing observed vs. granted
  permissions
- Compliance scoring with Green/Yellow/Red severity bands
- Sensitive excess detection for secrets, nodes, webhooks, and CRDs
- Helm chart for single-command installation
- CI pipeline with automated versioning, linting, testing, Docker build, and
  GitHub Releases
- Documentation website with getting-started guides, component deep-dives,
  examples, and API reference
- Helm chart repository
- `--version` flag on the operator binary
