# Filter

The filter removes audit events that should not generate policy recommendations.
It runs an ordered allow/deny chain where the first matching rule wins.

**Package:** `pkg/filter/`

---

## Where It Sits in the Pipeline

```
Audit Log → Ingestor → **Filter** → Normalizer → Aggregator → Strategy → Compliance → Report
```

**Input:** Raw `audit.k8s.io/v1.Event` structs from the ingestor. **Output:**
Events that pass the filter chain — forwarded to the normalizer.

Events that match a `Deny` rule or are system users (when `ignoreSystemUsers` is
enabled) are dropped silently. Dropped events are counted in the
`audicia_events_filtered_total` Prometheus metric.

---

## How Filtering Works

### Filter Chain Evaluation

The filter evaluates rules in order. **First match wins.** If no rule matches,
the event is **allowed** by default (default-allow policy).

Each rule can match on:

| Field              | Match Type | Target                      |
| ------------------ | ---------- | --------------------------- |
| `userPattern`      | Regex      | `event.User.Username`       |
| `namespacePattern` | Regex      | `event.ObjectRef.Namespace` |

Rules use OR-semantics — if a rule specifies both `userPattern` and
`namespacePattern`, either match triggers the rule.

### System User Filtering

`spec.ignoreSystemUsers` (default: `true`) provides a built-in filter that drops
all `system:*` users **except** `system:serviceaccount:*` (service accounts are
always included, since they represent workload identity).

This runs independently of the filter chain and is applied first.

---

## Configuration

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*" # Node heartbeat noise
    - action: Deny
      userPattern: "^system:kube-.*" # Control plane internals
    - action: Allow
      namespacePattern: "^my-team-.*" # Only care about our namespaces
```

See the [Filter Recipes](../guides/filter-recipes.md) guide for common
configurations including system noise reduction, namespace allowlisting, and
production-hardened setups.

---

## Core Functions

| Function | Purpose                                                                                                                                                             |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Allow`  | Evaluates the ordered allow/deny filter chain with OR-semantics on user and namespace patterns. First match wins; default is allow. Returns `true` if event passes. |

---

## Related

- [Filter Recipes](../guides/filter-recipes.md) — Common filter configurations
- [Pipeline](../concepts/pipeline.md) — Stage-by-stage processing overview
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — `spec.filters` field
  reference
