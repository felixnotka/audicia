# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Audicia, **please report it responsibly.**

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, email: **security@audicia.io**

You will receive an acknowledgment within 48 hours and a detailed response within 7 days indicating next steps.

## What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Scope

The following are in scope for security reports:

- The Audicia Operator binary and container image
- CRD validation and admission logic
- Audit log parsing and ingestion (injection, path traversal, etc.)
- RBAC policy generation correctness (generating overly permissive policies)
- Webhook receiver security (authentication, DoS, spoofing)
- Helm chart default configurations
- Dependencies with known CVEs

## Threat Model

Audicia is a security tool that processes sensitive data (audit logs) and produces security-critical output (RBAC policy
recommendations). The following threat model defines the trust boundaries and known attack surfaces.

### Trust Boundaries

```
┌─────────────────────────────────────────────────────────┐
│                  Trusted Zone                            │
│                                                         │
│  ┌──────────────┐    ┌──────────────┐    ┌───────────┐  │
│  │ kube-apiserver│───►│  Audit Log   │───►│  Audicia   │  │
│  │ (writes logs) │    │  (file/PVC)  │    │  Operator  │  │
│  └──────────────┘    └──────────────┘    └─────┬─────┘  │
│                                                │        │
│                                    ┌───────────▼──────┐ │
│                                    │AudiciaPolicyReport│ │
│                                    │     (CRDs)       │ │
│                                    └──────────────────┘ │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│               Semi-Trusted Zone                          │
│                                                         │
│  ┌──────────────┐                                       │
│  │ Webhook Input │  (kube-apiserver webhook backend)     │
│  │ (HTTP POST)   │  Must be authenticated + restricted   │
│  └──────────────┘                                       │
└─────────────────────────────────────────────────────────┘
```

### Threat: Audit Log Poisoning (File Source)

**Attack:** An attacker with write access to the audit log file injects fabricated events to trick Audicia into
generating overly permissive policies.

**Trust assumption:** The audit log file is written exclusively by the kube-apiserver. Only the kube-apiserver process
and the host root user have write access. If an attacker has write access to the audit log, they already have node-level
or kube-apiserver-level compromise — at which point RBAC is moot.

**Mitigations:**

- Audit log files should be on a read-only mount from Audicia's perspective (Audicia only needs read access).
- Host-level file integrity monitoring (e.g., AIDE, osquery) can detect unauthorized modifications.
- Audicia validates that events conform to the `audit.k8s.io/v1` schema and drops malformed entries.

### Threat: Webhook Spoofing / Poisoning

**Attack:** An attacker sends fabricated audit events to Audicia's webhook endpoint, injecting events that generate
dangerous policy recommendations.

**This is the highest-risk attack surface.** Unlike file ingestion (where the trust boundary is the node), webhook
ingestion exposes a network endpoint.

**Mitigations (secure-by-default):**

- **TLS required.** The webhook receiver only listens on HTTPS. Plaintext HTTP is not supported.
- **mTLS recommended.** Configure the webhook with a client CA bundle. Only the kube-apiserver's client certificate is
  accepted. See [Hardened Webhook Example](docs/examples/audicia-source-hardened.md).
- **NetworkPolicy.** Restrict ingress to the webhook Service to only the kube-apiserver Pod CIDR or node IPs.
- **Rate limiting.** The webhook receiver enforces a configurable request rate limit (default: 100 req/s) and maximum
  request body size (default: 1MB).
- **Backpressure.** If the processing pipeline is saturated, the webhook returns `429 Too Many Requests`. The
  kube-apiserver audit webhook backend handles retries natively.

### Threat: Overly Permissive Policy Suggestions

**Attack:** Through log poisoning, misconfiguration, or edge cases in normalization, Audicia generates policies that
grant more access than intended.

**This is the most impactful failure mode for a security tool.** A suggestion of `cluster-admin` based on fabricated
events would undermine the tool's purpose.

**Mitigations (safety guardrails):**

- **`wildcards: Forbidden` by default.** Audicia never generates `*` verbs or resources unless the operator explicitly
  opts in via `wildcards: Safe`.
- **No `cluster-admin` generation.** Audicia will never suggest a ClusterRoleBinding to `cluster-admin`, regardless of
  observed events. This is a hardcoded safety rail.
- **Verb allowlist.** Only standard Kubernetes verbs are emitted: `get`, `list`, `watch`, `create`, `update`, `patch`,
  `delete`, `deletecollection`. Custom or unexpected verbs are logged and dropped.
- **`scopeMode: NamespaceStrict` by default.** ClusterRoles are not generated unless explicitly enabled. Cluster-scoped
  actions observed in NamespaceStrict mode are reported as gaps/warnings, not as ClusterRole suggestions.
- **No Auto-Apply.** Even if a dangerous policy is suggested, it is never applied automatically. A human or reviewed
  GitOps pipeline must act on it.

### Threat: Report Data Leakage

**Attack:** An unauthorized user reads `AudiciaPolicyReport` resources, gaining visibility into who accesses what across
the cluster.

**Mitigations:**

- Reports contain sensitive access patterns. RBAC should restrict reads to cluster administrators only.
- See the [Sensitive Data in Reports](#sensitive-data-in-reports) section for a ready-to-use ClusterRole example.
- In multi-tenant clusters, consider namespace-scoped reports with per-namespace RBAC rather than cluster-wide access.

### Threat: Denial of Service via Webhook

**Attack:** An attacker floods the webhook endpoint to degrade Audicia's processing or the kube-apiserver's audit
delivery.

**Mitigations:**

- Rate limiting on the receiver (configurable, default 100 req/s).
- Maximum request body size (default 1MB).
- NetworkPolicy restricting ingress sources.
- The operator processes events in batches with configurable concurrency. Excess events are queued, not dropped, up to a
  configurable buffer limit.

## Security Design Principles

Audicia is a security tool. We hold ourselves to a higher standard.

### Minimal Operator Permissions

The Audicia operator requests only the permissions it needs:

| Permission                                                          | Scope                 | Reason                   |
|---------------------------------------------------------------------|-----------------------|--------------------------|
| `get`, `list`, `watch` on `AudiciaSource`                           | Namespaced or Cluster | Read input configuration |
| `get`, `list`, `watch`, `create`, `update` on `AudiciaPolicyReport` | Namespaced or Cluster | Write output reports     |
| `update` on `AudiciaSource/status`                                  | Namespaced or Cluster | Persist checkpoint state |
| Read access to audit log volume                                     | Host path or PVC      | Ingest audit events      |

The operator does **not** request:

- `secrets` access
- `impersonate` permissions
- Write access to any core Kubernetes resources (Roles, RoleBindings, etc.)
- Cluster-admin or wildcard permissions

### No Auto-Apply

Audicia **never** applies generated policies automatically. It produces `AudiciaPolicyReport` CRDs. A human or a
reviewed GitOps pipeline must apply the suggested manifests. This is a deliberate design choice — automated privilege
escalation is a security anti-pattern.

### Sensitive Data in Reports

`AudiciaPolicyReport` resources contain information about who accessed what in your cluster. This is sensitive.

**Recommendation:** Configure RBAC so only cluster administrators can read `AudiciaPolicyReport` resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: audicia-report-reader
rules:
  - apiGroups: [ "audicia.io" ]
    resources: [ "audiciapolicyreports" ]
    verbs: [ "get", "list", "watch" ]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: audicia-report-reader-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: audicia-report-reader
subjects:
  - kind: Group
    name: cluster-admins
    apiGroup: rbac.authorization.k8s.io
```

**Multi-tenant guidance:** If namespace administrators should see reports for their own namespaces, use namespace-scoped
RoleBindings instead of a ClusterRoleBinding. Each tenant gets visibility into their own reports only.

### Supply Chain Security

**Current:**

- Container images are built via `docker build` and pushed to Docker Hub (`felixnotka/audicia-operator`).
- Go dependencies are managed via `go.mod` with pinned versions.

**Planned (not yet implemented):**

- Container image signing with [cosign](https://github.com/sigstore/cosign) (keyless/OIDC).
- Helm chart publishing to a verified registry.
- Automated CVE scanning with `govulncheck` and Trivy in CI.
- SBOM (Software Bill of Materials) generation and publication with each release.

See [RELEASING.md](RELEASING.md) for the current release process.

### Disclosure Timeline

| Event                        | Timeline                                           |
|------------------------------|----------------------------------------------------|
| Report received              | T+0                                                |
| Acknowledgment               | T+48h                                              |
| Assessment and triage        | T+7d                                               |
| Fix developed and tested     | T+30d (target)                                     |
| CVE assigned (if applicable) | T+30d                                              |
| Public disclosure            | T+90d or when fix is released (whichever is first) |

We follow coordinated disclosure. We will not publicly disclose a vulnerability before a fix is available unless 90 days
have elapsed.
