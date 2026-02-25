# Aggregator

The aggregator deduplicates and merges observed rules per subject. Audit events
arriving over time produce many duplicate rule observations — the aggregator
collapses them into a compact rule set with metadata tracking.

**Package:** `pkg/aggregator/`

---

## Where It Sits in the Pipeline

```
Audit Log → Ingestor → Filter → Normalizer → **Aggregator** → Strategy → Compliance → Report
```

**Input:** Normalized `(Subject, CanonicalRule)` pairs from the normalizer.
**Output:** A deduplicated rule set per subject, with `firstSeen`, `lastSeen`,
and `count` metadata.

---

## How Aggregation Works

Each unique subject (ServiceAccount, User, or Group) gets its own aggregator
instance. Within each instance, rules are deduplicated by a composite key:

**Deduplication key:** `(APIGroup, Resource, Verb, NonResourceURL, Namespace)`

When a rule with the same key arrives:

- The `count` is incremented
- The `lastSeen` timestamp is updated
- The `firstSeen` timestamp is preserved

When a new key arrives:

- A new rule entry is created with `count=1` and `firstSeen=lastSeen=now`

### Idempotency

The aggregator is designed for at-least-once processing. Reprocessing the same
event after a restart produces the same rule set — counts may over-count
slightly, but the rule set itself remains correct. This is an important property
because the [Ingestor](ingestor.md) may deliver some duplicate events after a
restart (especially in webhook mode).

### Deterministic Output

Rules are sorted by namespace, API group, resource, then verb before being
passed to the [Strategy Engine](strategy-engine.md). This ensures that the same
input always produces the same output, making reports stable and diff-friendly.

---

## Rule Retention and Limits

The aggregator enforces configurable limits to prevent unbounded growth:

| Limit                | Default | CRD Field                       | Behavior                                                      |
| -------------------- | ------- | ------------------------------- | ------------------------------------------------------------- |
| **Retention window** | 30 days | `spec.limits.retentionDays`     | Rules not seen within this window are dropped during flush.   |
| **Max rules**        | 200     | `spec.limits.maxRulesPerReport` | Oldest rules (by `lastSeen`) are dropped first when exceeded. |

**Compaction behavior:** When a report exceeds `maxRulesPerReport`, rules are
prioritized by `lastSeen` (most recent kept). Compacted rules are logged at
`INFO` level with their full details before removal, providing an audit trail.

**Scaling guidance:**

- A typical microservice generates 5-20 unique rules. 200 rules covers even
  complex workloads.
- A namespace with 50 service accounts produces ~50 reports (one per subject).
  Each report is typically 5-50KB.

---

## Core Functions

| Function | Purpose                                                                                                                                                                |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Add`    | Inserts or merges an observed rule, keyed on the tuple `(APIGroup, Resource, Verb, NonResourceURL, Namespace)`. Increments count and updates `lastSeen` on duplicates. |

---

## Related

- [Pipeline](../concepts/pipeline.md) — Stage-by-stage processing overview
- [Strategy Engine](strategy-engine.md) — How aggregated rules become RBAC
  manifests
- [Controller](controller.md) — Manages flush cycles and compaction
