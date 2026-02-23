# Strategy Engine

The strategy engine transforms aggregated observed rules into least-privilege RBAC manifests. It applies
user-configured policy knobs to control the shape, scope, and verbosity of the generated output.

**Package:** `pkg/strategy/`

---

## Where It Sits in the Pipeline

```
Audit Log → Ingestor → Filter → Normalizer → Aggregator → **Strategy** → Compliance → Report
```

**Input:** Deduplicated, sorted rule sets per subject from the [Aggregator](aggregator.md).
**Output:** Rendered Kubernetes RBAC YAML manifests (Role, ClusterRole, RoleBinding, ClusterRoleBinding).

---

## Policy Strategy Knobs

The strategy engine is configured via `AudiciaSource.spec.policyStrategy`:

```yaml
spec:
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
    resourceNames: Omit
```

### Scope Mode

Controls whether the engine generates namespace-scoped Roles or cluster-wide ClusterRoles.

| Mode                        | Behavior                                                                                                                            |
|-----------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `NamespaceStrict` (default) | Generates per-namespace `Role` + `RoleBinding` pairs. When a subject has both namespaced and cluster-scoped rules, the cluster-scoped rules are merged into each namespace's Role. When a subject has **only** cluster-scoped rules (no namespaced activity), a `ClusterRole` + `ClusterRoleBinding` is generated. |
| `ClusterScopeAllowed`       | Emits a single `ClusterRole` + `ClusterRoleBinding` covering everything.                                                            |

### Verb Merge

Controls whether rules for the same resource are consolidated.

| Mode              | Behavior                                                                                                       |
|-------------------|----------------------------------------------------------------------------------------------------------------|
| `Smart` (default) | Collapses rules with the same `(apiGroup, resource, namespace)` into one rule with merged verb lists.          |
| `Exact`           | Keeps one rule per observed verb. No merging.                                                                  |

For example, with `Smart` mode, separate observations of `get`, `list`, and `watch` on `pods` become a single rule
with `verbs: [get, list, watch]`.

### Wildcard Mode

Controls whether the engine generates `*` wildcard verbs.

| Mode                  | Behavior                                                         |
|-----------------------|------------------------------------------------------------------|
| `Forbidden` (default) | Never generates `*` verbs.                                       |
| `Safe`                | Replaces complete verb sets (all 8 standard verbs) with `["*"]`. |

### Resource Names

Controls whether generated rules include `resourceNames` constraints.

| Mode             | Behavior                                                                                        |
|------------------|-------------------------------------------------------------------------------------------------|
| `Omit` (default) | Does not include `resourceNames` in generated rules.                                            |
| `Explicit`       | Includes observed resource names in rules (defined but not yet wired in strategy output).       |

---

## Manifest Generation

The engine generates complete, `kubectl apply`-ready YAML depending on the subject type and scope mode:

### ServiceAccount Subjects

ServiceAccounts get per-namespace `Role` + `RoleBinding` pairs in each target namespace they access. Cluster-scoped
rules (non-resource URLs like `/metrics`, `/healthz`) are emitted as a separate `ClusterRole` + `ClusterRoleBinding`.

If a ServiceAccount accesses resources in namespaces X and Y, it gets separate Role + RoleBinding in each namespace.

### User/Group Subjects

Depends on scope mode:
- **NamespaceStrict:** Generates per-namespace Roles, similar to ServiceAccounts.
- **ClusterScopeAllowed:** Generates a single ClusterRole + ClusterRoleBinding.

### Output Properties

| Property                     | Details                                                                                                      |
|------------------------------|--------------------------------------------------------------------------------------------------------------|
| **Standard verbs only**      | Only the 8 standard Kubernetes API verbs are emitted. Non-standard verbs are silently dropped.               |
| **PolicyRule deduplication** | Duplicate PolicyRules (after dropping namespace) are deduplicated within a single Role.                      |
| **Name sanitization**        | Subject names are sanitized for Kubernetes object names (max 50 chars, lowercase, special chars replaced).   |
| **Rendered YAML**            | Output is complete, `kubectl apply`-ready YAML.                                                              |

---

## Safety Guardrails

Regardless of configuration, the strategy engine enforces hardcoded safety limits:

- **Never generates `cluster-admin`** equivalent bindings.
- **Standard verb allowlist only.** Only emits: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`,
  `deletecollection`. Non-standard verbs from audit events are silently dropped.
- **`wildcards: Safe` requires evidence.** All 8 standard verbs must be observed for a resource before emitting `*`.
  This is a resource-level check, not cluster-level.

---

## Core Functions

| Function               | Purpose                                                                                                                                                                                 |
|------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `GenerateManifests`    | Top-level orchestrator. Runs the full pipeline: `filterVerbs` → `mergeVerbs` → `applyWildcards`, then branches on subject kind and scope mode to emit Roles and Bindings.              |
| `mergeVerbs`           | Collapses rules that differ only by verb into single rules with merged verb lists, reducing manifest verbosity.                                                                         |
| `applyWildcards`       | Replaces a full verb list with `["*"]` when all 8 standard verbs have been observed. Only applies to resource rules, never to non-resource URLs.                                       |
| `filterVerbs`          | Strips non-standard verbs from observed rules and removes any rules left with no valid verbs remaining.                                                                                 |
| `generatePerNamespace` | ServiceAccount code path. Groups rules by namespace and attributes cluster-scoped resource rules to the ServiceAccount's home namespace.                                               |
| `groupByNamespace`     | Partitions a flat rule list by namespace. Rules with an empty namespace field are assigned to the provided home namespace.                                                              |
| `renderRole`           | Converts `ObservedRules` into Kubernetes `PolicyRules` with cross-namespace deduplication, then marshals the result to YAML.                                                           |

---

## Related

- [Aggregator](aggregator.md) — Provides the deduplicated rule sets
- [Compliance Engine](compliance-engine.md) — Evaluates generated policies against effective RBAC
- [Pipeline](../concepts/pipeline.md) — Stage-by-stage processing overview
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — `spec.policyStrategy` field reference
