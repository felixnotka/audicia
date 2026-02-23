# Changelog

All notable changes to Audicia are documented here.

The project uses automatic patch-level versioning: `version.json` defines Major.Minor, and CI auto-increments the patch on each release to `main`.

---

## 0.1.2

### Added
- `webhook.hostPort` Helm value — exposes the webhook directly on the host, bypassing ClusterIP routing issues with Cilium and other kube-proxy-free CNIs
- `webhook.service.nodePort` Helm value — optional NodePort service type for the webhook
- Troubleshooting section for ClusterIP unreachable from host namespace

### Fixed
- Remaining incorrect Helm chart name references in webhook and mTLS guides
- Documented audit log file permissions (root-owned) and two workarounds
- Documented kube-apiserver restart procedure for kubeadm clusters

### Changed
- All webhook docs now cover both ClusterIP and hostPort modes (installation, quick start, webhook setup, mTLS setup, helm values reference, troubleshooting)

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
