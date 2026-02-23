# RBAC Policy Generation

Audicia's core output is a set of least-privilege RBAC manifests derived from real API usage.
The generated policy represents what a subject *actually needs*, not what someone guessed at
provisioning time. This page explains what gets generated, how, and what you can do with it.

## What Gets Generated

For each subject (ServiceAccount, User, or Group) that appears in the audit log, Audicia
produces ready-to-apply Kubernetes RBAC manifests:

| Generated Resource     | When                                                       |
|------------------------|------------------------------------------------------------|
| `Role`                 | Subject accessed namespaced resources                      |
| `RoleBinding`          | Paired with the Role, scoped to the target namespace       |
| `ClusterRole`          | Subject accessed cluster-scoped resources or non-resource URLs |
| `ClusterRoleBinding`   | Paired with the ClusterRole                                |

These manifests live in `status.suggestedPolicy.manifests` on the `AudiciaPolicyReport` CR
and can be extracted with:

```bash
kubectl get apreport <NAME> -n <NAMESPACE> \
  -o jsonpath='{.status.suggestedPolicy.manifests}' | jq -r '.[]'
```

The output is complete, `kubectl apply`-ready YAML.

## From Audit Events to RBAC

The generation pipeline works in three stages:

```
Observed Rules → Strategy Engine → RBAC Manifests
```

### 1. Observation

The aggregator collects deduplicated rules per subject. Each rule captures:

- **apiGroup** — e.g. `""` (core), `apps`, `rbac.authorization.k8s.io`
- **resource** — e.g. `pods`, `deployments`, `pods/exec`
- **verb** — e.g. `get`, `list`, `create`
- **namespace** — where the access occurred
- **count** — how many times this access pattern was observed
- **firstSeen / lastSeen** — time range of activity

### 2. Strategy

The [strategy engine](../components/strategy-engine.md) applies configurable knobs to
shape the RBAC output:

| Knob        | Effect                                                                     |
|-------------|----------------------------------------------------------------------------|
| `scopeMode` | `NamespaceStrict` generates per-namespace Roles; `ClusterScopeAllowed` generates a single ClusterRole |
| `verbMerge` | `Smart` collapses `get`+`list`+`watch` on the same resource into one rule; `Exact` keeps them separate |
| `wildcards` | `Forbidden` never uses `*`; `Safe` replaces all-8-verbs with `["*"]`       |

### 3. Rendering

The engine produces complete manifests with:

- **Proper namespace scoping** — A ServiceAccount accessing resources in namespaces X and
  Y gets separate Role + RoleBinding pairs in each namespace
- **Cluster-scoped separation** — Non-resource URLs (`/metrics`, `/healthz`) and
  cluster-scoped resources get their own ClusterRole + ClusterRoleBinding
- **Name sanitization** — Generated names are Kubernetes-safe (lowercase, max 50 chars)
- **Standard verbs only** — Only the 8 standard API verbs are emitted; non-standard verbs
  from audit events are silently dropped

## Safety Guardrails

Regardless of configuration, the strategy engine enforces hard limits:

- **Never generates `cluster-admin`** equivalent bindings
- **Standard verb allowlist only** — `get`, `list`, `watch`, `create`, `update`, `patch`,
  `delete`, `deletecollection`
- **`wildcards: Safe` requires evidence** — All 8 standard verbs must be observed for a
  resource before emitting `*`

## Example

A ServiceAccount `backend` in namespace `my-team` accesses pods (get/list/watch),
configmaps (get/watch), and pods/exec (create). The generated policy:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: suggested-backend-role
  namespace: my-team
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: suggested-backend-binding
  namespace: my-team
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: suggested-backend-role
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: my-team
```

This is the minimal RBAC the SA needs based on observed behavior. Compare it against the
SA's current permissions via [compliance scoring](compliance-scoring.md) to see what's
excessive.

## Using the Generated Policy

### Review and Apply

The suggested policy is a starting point, not a blind apply. Recommended workflow:

1. **Collect** — Let Audicia observe for a representative period (at least one full
   business cycle — a day, a week, or a deployment cycle)
2. **Review** — Inspect the generated manifests for completeness. Are all expected
   access patterns captured? Check the `observedRules` for gaps.
3. **Compare** — Use the compliance score to identify overprivilege in the current RBAC
4. **Apply** — Replace the existing broad Role/ClusterRole with the generated least-privilege version
5. **Monitor** — Watch for `403 Forbidden` errors after tightening permissions. If
   something was missed, Audicia will capture it in the next report cycle.

### Observation Period

The quality of the generated policy depends on the observation window. Short windows may
miss infrequent operations:

| Activity Type          | Typical Frequency        | Minimum Observation |
|------------------------|--------------------------|---------------------|
| Read operations        | Continuous               | 1 day               |
| Deployments            | Weekly                   | 1 week              |
| CronJob-triggered      | Varies (daily, weekly)   | 1 full cycle        |
| Emergency operations   | Rare                     | 1+ months           |

Use `retentionDays` on the AudiciaSource to control how long rules are kept before being
evicted.

### Cross-Namespace Access

ServiceAccounts that access resources in multiple namespaces get separate Roles in each
namespace. This follows the principle of least privilege — the binding in namespace X
doesn't grant access to namespace Y.

### What About Groups?

Group identities are captured from the audit log but are not used for binding generation
by default. The generated bindings reference the subject (ServiceAccount or User) directly.

## Relationship to Compliance Scoring

RBAC generation and [compliance scoring](compliance-scoring.md) are complementary:

| Aspect               | RBAC Generation                        | Compliance Scoring                            |
|----------------------|----------------------------------------|-----------------------------------------------|
| **Question**         | "What RBAC does this subject need?"    | "How much of its current RBAC does it use?"   |
| **Input**            | Observed audit events                  | Observed events + existing RBAC bindings      |
| **Output**           | Ready-to-apply Role/ClusterRole YAML   | Score (0-100), severity, excess/uncovered     |
| **Action**           | Replace existing overprivileged RBAC   | Identify subjects to investigate              |

Together they form a closed loop: compliance scoring identifies the problem (overprivilege),
and RBAC generation provides the solution (least-privilege policy).

## Related

- [Strategy Engine](../components/strategy-engine.md) — Deep-dive on scopeMode, verbMerge, wildcards
- [Compliance Scoring](compliance-scoring.md) — How compliance scores are calculated
- [AudiciaPolicyReport CRD](../reference/crd-audiciapolicyreport.md) — Full field reference
- [Policy Report Example](../examples/policy-report.md) — Example generated report
