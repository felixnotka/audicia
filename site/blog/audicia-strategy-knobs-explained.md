---
title: "Audicia Strategy Knobs: Choosing the Right RBAC Policy Shape"
seo_title: "Audicia Strategy Knobs: Choosing the Right RBAC Policy Shape"
published_at: 2026-04-28T08:00:00.000Z
snippet: "How to configure Audicia's strategy knobs — scopeMode, verbMerge, wildcards — to control the shape and scope of generated RBAC policies."
description: "Configure Audicia's strategy knobs to control generated RBAC policy shape: scopeMode for namespace vs cluster, verbMerge for rule consolidation, and wildcards."
---

## Why Configuration Matters

Different clusters have different RBAC requirements. A single-tenant dev cluster
might benefit from ClusterRoles for simplicity. A multi-tenant production
cluster needs strict namespace-scoped Roles for isolation. A compliance-focused
environment should never generate wildcard permissions.

Audicia's strategy engine provides three knobs that control the shape, scope,
and verbosity of generated RBAC manifests. Each knob has sensible defaults, but
understanding the options lets you match the output to your environment.

## The Three Knobs

```yaml
spec:
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
```

## Scope Mode

Controls whether Audicia generates namespace-scoped Roles or cluster-wide
ClusterRoles.

### NamespaceStrict (Default)

```yaml
scopeMode: NamespaceStrict
```

Generates a separate **Role + RoleBinding** pair for each namespace where the
subject was observed. Cluster-scoped rules (non-resource URLs like `/metrics`)
are emitted as a separate ClusterRole + ClusterRoleBinding.

**When to use:** Multi-tenant clusters, production environments, any cluster
where namespace isolation matters.

**Example output:** A service account observed in two namespaces gets two Roles:

```yaml
# Role in team-a
kind: Role
metadata:
  namespace: team-a
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
# Role in shared-infra
kind: Role
metadata:
  namespace: shared-infra
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
```

### ClusterScopeAllowed

```yaml
scopeMode: ClusterScopeAllowed
```

Generates a single **ClusterRole + ClusterRoleBinding** covering all observed
access.

**When to use:** Single-tenant clusters, dev environments, or workloads that
legitimately need cluster-wide access (like monitoring agents that read pods in
all namespaces).

**Example output:**

```yaml
kind: ClusterRole
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
```

**Trade-off:** Simpler output, but the ClusterRoleBinding grants access to every
namespace — including ones the workload has never touched.

## Verb Merge

Controls whether rules for the same resource are consolidated into a single rule
with merged verbs.

### Smart (Default)

```yaml
verbMerge: Smart
```

Rules with the same API group, resource, and namespace are merged into a single
rule with a combined verb list.

**Before merging (3 separate rules):**

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["watch"]
```

**After merging (1 rule):**

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
```

**When to use:** Always, unless you need one-rule-per-verb for audit trail
purposes.

### Exact

```yaml
verbMerge: Exact
```

Keeps one rule per observed verb. No merging.

**When to use:** When you need an explicit mapping between each observed action
and the corresponding RBAC rule. Useful for detailed auditing or when you want
to see exactly which verb was observed independently.

**Trade-off:** More verbose output — a service account that reads pods generates
three rules instead of one.

## Wildcards

Controls whether the strategy engine generates `*` wildcard verbs.

### Forbidden (Default)

```yaml
wildcards: Forbidden
```

Never generates `*` in verb lists. Every verb is listed explicitly.

**When to use:** Compliance-focused environments, production clusters, any
context where wildcard permissions are prohibited by policy.

### Safe

```yaml
wildcards: Safe
```

Replaces a complete verb list with `["*"]` when all 8 standard Kubernetes verbs
have been observed for a resource:

```yaml
# If all 8 verbs were observed:
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["*"]
```

The 8 standard verbs are: `get`, `list`, `watch`, `create`, `update`, `patch`,
`delete`, `deletecollection`. All 8 must be observed before `*` is emitted.

**When to use:** Only in environments where wildcard permissions are acceptable
and you want compact output for workloads with full access to specific
resources.

**Safety guardrail:** Even in `Safe` mode, Audicia never generates `*` for
resources or API groups — only for verbs on a specific resource where all verbs
were observed.

## Safety Guardrails

Regardless of configuration, Audicia enforces hardcoded safety limits:

- **Never generates `cluster-admin` equivalent bindings** — no `*` on resources
  or API groups
- **Standard verb allowlist only** — custom or unexpected verbs from audit
  events are silently dropped
- **`wildcards: Safe` requires evidence** — all 8 standard verbs must be
  observed before emitting `*`
- **Name sanitization** — generated resource names are capped at 50 characters,
  lowercased, and special characters are replaced

## Choosing the Right Configuration

| Cluster Type             | scopeMode           | verbMerge | wildcards |
| ------------------------ | ------------------- | --------- | --------- |
| Multi-tenant production  | NamespaceStrict     | Smart     | Forbidden |
| Single-tenant production | NamespaceStrict     | Smart     | Forbidden |
| Development              | ClusterScopeAllowed | Smart     | Safe      |
| Compliance-critical      | NamespaceStrict     | Smart     | Forbidden |
| Per-verb audit trail     | NamespaceStrict     | Exact     | Forbidden |

For most clusters, the defaults (`NamespaceStrict`, `Smart`, `Forbidden`) are
the right choice.

## Further Reading

- **[Kubernetes RBAC for Multi-Tenant Clusters](/blog/kubernetes-rbac-multi-tenant)**
  — per-namespace policy generation with NamespaceStrict
- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  full walkthrough using the default knobs
- **[Strategy Engine Reference](/docs/components/strategy-engine)** — technical
  documentation for the strategy engine
