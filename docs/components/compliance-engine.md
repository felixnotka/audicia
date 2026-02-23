# Compliance Engine

The compliance engine compares observed usage (what a subject actually does) against effective RBAC permissions (what
it's allowed to do) to produce a compliance score and identify overprivilege. It consists of two sub-components: the
**RBAC Resolver** and the **Diff Engine**.

**Packages:** `pkg/rbac/` (resolver), `pkg/diff/` (diff engine)

---

## Where It Sits in the Pipeline

```
Audit Log → Ingestor → Filter → Normalizer → Aggregator → Strategy → **Compliance** → Report
```

**Input:** Observed rules from the [Aggregator](aggregator.md) and the subject identity.
**Output:** A `ComplianceReport` containing score, severity, used/excess/uncovered counts, and sensitive excess flags.

---

## RBAC Resolver

The resolver queries the Kubernetes API to determine what a subject is **actually** allowed to do, independent of what
Audicia has observed.

Given a subject (ServiceAccount, User, or Group), the resolver:

1. **Lists all ClusterRoleBindings** — filters by subject match — resolves each referenced ClusterRole into PolicyRules
   with cluster-wide scope (Namespace="").
2. **Lists all RoleBindings** — filters by subject match — resolves each referenced Role or ClusterRole into PolicyRules
   scoped to the RoleBinding's namespace.
3. **Returns `[]ScopedRule`** — a flat list of `rbacv1.PolicyRule` entries, each annotated with the namespace they apply
   in.

### Design Decisions

| Decision                                              | Rationale                                                                          |
|-------------------------------------------------------|------------------------------------------------------------------------------------|
| Uses the caching client (not direct API reader)       | RBAC types change infrequently; informer cache avoids API server load              |
| Skips deleted/missing roles silently                  | Graceful degradation — a missing role doesn't block the entire evaluation          |
| Does not resolve aggregated ClusterRoles              | Label-selector aggregation is complex and uncommon; documented as known limitation |
| Subject matching: SA by name+namespace, User by name  | Follows Kubernetes RBAC binding semantics                                          |

### RBAC Informer Cache

On startup, the operator registers informers for `ClusterRole`, `ClusterRoleBinding`, `Role`, and `RoleBinding` so
the cache is warm for compliance evaluation. This means RBAC queries are fast, local reads from the cache rather than
API server calls.

---

## Diff Engine

The diff engine is a pure function: `Evaluate(observed []ObservedRule, effective []ScopedRule) *ComplianceReport`.

**Score:** `usedEffective / totalEffective × 100` — classifies each effective rule as **used** (exercised by an
observed action), **excess** (never observed), or flags observed actions with no effective rule as **uncovered**.
Excess grants on sensitive resources (secrets, nodes, webhook configurations, CRDs, etc.) are flagged in
`sensitiveExcess`.

See [Compliance Scoring](../concepts/compliance-scoring.md) for the full formula, severity thresholds, matching
rules, and the complete sensitive resource list.

---

## Edge Cases

- **No RBAC + no observations:** Score 100, Green (nothing to do).
- **No RBAC + observations exist:** Compliance is `nil` — the score cannot be evaluated. Report still gets observed rules and suggested policy.
- **RBAC exists + no observations:** Score 0, Red — all grants are excess.

## Graceful Degradation

If the RBAC resolver fails (e.g., the operator doesn't have RBAC read permissions, or the API server is unreachable),
compliance is `nil` — the report still gets observed rules and suggested policy. The operator logs the error and
continues normally.

---

## Core Functions

### Resolver (`pkg/rbac/`)

| Function                       | Purpose                                                                                                       |
|--------------------------------|---------------------------------------------------------------------------------------------------------------|
| `EffectiveRules`               | Combines rules from both `ClusterRoleBindings` and `RoleBindings` for a given subject into a single set.     |
| `matchesSubject`               | Three-way identity matching across ServiceAccount, User, and Group subject types.                             |
| `rulesFromClusterRoleBindings` | Lists all ClusterRoleBindings, filters by subject match, resolves each to its backing ClusterRole rules.      |
| `rulesFromRoleBindings`        | Lists all RoleBindings in a namespace, filters by subject match, resolves each to its backing Role rules.     |

### Diff Engine (`pkg/diff/`)

| Function               | Purpose                                                                                                                                  |
|------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `Evaluate`             | Entry point for compliance scoring. Computes `score = usedEffective / totalEffective` and classifies severity.                          |
| `matchesResourceRule`  | Namespace-aware RBAC matching with `ResourceNames` exclusion. Handles wildcard expansion for API groups, resources, and verbs.           |
| `sliceCovers`          | Wildcard-aware set containment check. A `"*"` entry short-circuits to `true`.                                                           |
| `markUsed`             | Tags **all** effective rules that cover an observed action, not just the first match. Required for accurate excess detection.            |
| `classifyEffective`    | Partitions effective rules into used vs. excess buckets and flags sensitive excess grants.                                               |
| `isCovered`            | Dispatch function that routes to `matchesResourceRule` or `matchesNonResourceURL` based on rule type.                                   |

---

## Related

- [Compliance Scoring](../concepts/compliance-scoring.md) — Conceptual overview of how scoring works
- [Strategy Engine](strategy-engine.md) — Generates the RBAC manifests that get evaluated
- [Controller](controller.md) — Orchestrates the compliance evaluation cycle
- [AudiciaPolicyReport CRD](../reference/crd-audiciapolicyreport.md) — `status.compliance` field reference
