# Security Model

Audicia is a security tool that processes sensitive data (audit logs) and
produces security-critical output (RBAC policy recommendations). This document
describes the trust boundaries, threat model, and security design principles.

## Trust Boundaries

Audicia operates within three trust zones:

**Trusted zone:** The kube-apiserver writes audit events to a log file or PVC.
Audicia reads these events and produces AudiciaPolicyReport CRDs. The audit log,
the operator, and the output reports are all within the cluster's control plane
trust boundary.

**Semi-trusted zone:** In webhook mode, the kube-apiserver sends audit events
over HTTPS. This network path must be authenticated (TLS + mTLS recommended) and
restricted (NetworkPolicy) to prevent spoofing.

**Cloud zone:** In cloud mode, audit events flow through a cloud-managed
pipeline (e.g., Azure Event Hub). The trust boundary extends to the cloud IAM
layer — access to the message bus is controlled by cloud credentials or workload
identity. Audicia validates cluster identity to prevent cross-cluster event
leakage.

## Threat Model

### Audit Log Poisoning (File Source)

**Attack:** An attacker injects fabricated events into the audit log file to
trick Audicia into generating overly permissive policies.

**Trust assumption:** The audit log is written exclusively by the
kube-apiserver. If an attacker has write access, they already have node-level
compromise — at which point RBAC is moot.

**Mitigations:** Read-only mount from Audicia's perspective. Host-level file
integrity monitoring. Schema validation drops malformed entries.

### Webhook Spoofing / Poisoning

**Attack:** An attacker sends fabricated audit events to the webhook endpoint.

**This is the highest-risk attack surface.** Unlike file ingestion (where the
trust boundary is the node), webhook ingestion exposes a network endpoint.

**Mitigations:**

- **TLS required.** Plaintext HTTP is not supported.
- **mTLS recommended.** Only the kube-apiserver's client certificate is
  accepted.
- **NetworkPolicy.** Restrict ingress to the kube-apiserver's Pod CIDR or node
  IPs.
- **Rate limiting.** Default 100 req/s, configurable.
- **Request size limit.** Default 1MB, configurable.

### Overly Permissive Policy Suggestions

**Attack:** Through log poisoning or normalization edge cases, Audicia generates
policies that grant more access than intended.

**Mitigations:**

- **`wildcards: Forbidden` by default.** Never generates `*` unless explicitly
  opted in.
- **No `cluster-admin` generation.** Hardcoded safety rail.
- **Standard verb allowlist only.** Custom or unexpected verbs are dropped.
- **`scopeMode: NamespaceStrict` by default.** ClusterRoles are not generated
  unless enabled.
- **No auto-apply.** A human or reviewed GitOps pipeline must act on
  suggestions.

### Report Data Leakage

**Attack:** An unauthorized user reads `AudiciaPolicyReport` resources, gaining
visibility into access patterns.

**Mitigations:** RBAC should restrict report reads to cluster administrators. In
multi-tenant clusters, use namespace-scoped reports with per-namespace RBAC.

### Cloud Log Tampering

**Attack:** An attacker with cloud IAM access injects fabricated events into the
message bus (e.g., Event Hub) to influence generated RBAC policies.

**Trust assumption:** The cloud audit pipeline is managed by the cloud provider
and protected by IAM policies. Write access to the message bus implies
cloud-level compromise.

**Mitigations:**

- **IAM least-privilege.** Grant only `Data Receiver` (read) roles to the
  Audicia identity.
- **Cluster identity validation.** `clusterIdentity` in the CRD filters events
  from other clusters sharing the same bus.
- **Envelope schema validation.** Malformed messages are skipped and logged.
- **Same pipeline safety guardrails** apply: no auto-apply, no `cluster-admin`,
  wildcards forbidden by default.

### Denial of Service via Webhook

**Attack:** An attacker floods the webhook endpoint.

**Mitigations:** Rate limiting, request size limits, NetworkPolicy, backpressure
(429 when saturated).

## Security Design Principles

### Minimal Operator Permissions

| Permission                     | Scope      | Reason                                       |
| ------------------------------ | ---------- | -------------------------------------------- |
| get/list/watch `AudiciaSource` | Namespaced | Read input configuration                     |
| CRUD `AudiciaPolicyReport`     | Namespaced | Write output reports                         |
| update `AudiciaSource/status`  | Namespaced | Persist checkpoint state                     |
| get/list/watch RBAC objects    | Cluster    | Resolve effective permissions for compliance |
| create/patch `events`          | Namespaced | Emit Kubernetes events                       |
| CRUD `leases`                  | Namespaced | Leader election                              |

The operator does **not** request: secrets access, impersonate permissions,
write access to Roles/RoleBindings, or cluster-admin.

### No Auto-Apply

Audicia never applies generated policies automatically. This is a deliberate
design choice — automated privilege escalation is a security anti-pattern.

### Report Sensitivity

`AudiciaPolicyReport` resources contain sensitive access pattern data. Restrict
reads to cluster administrators:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: audicia-report-reader
rules:
  - apiGroups: ["audicia.io"]
    resources: ["audiciapolicyreports"]
    verbs: ["get", "list", "watch"]
```

For multi-tenant clusters, use namespace-scoped RoleBindings so each tenant sees
only their own reports.

## Vulnerability Reporting

To report security vulnerabilities, please email info@audicia.io. See the
project's
[SECURITY.md](https://github.com/felixnotka/audicia/blob/main/SECURITY.md) on
GitHub for the full vulnerability reporting process and disclosure timeline.
