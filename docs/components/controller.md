# Controller

The controller is the central coordinator of Audicia. It implements the
Kubernetes Operator pattern using `controller-runtime`, managing pipeline
lifecycle, event processing, periodic flushing, and report persistence.

**Package:** `internal/controller/`

---

## Where It Sits in the Pipeline

The controller is not a single stage — it **orchestrates** the entire pipeline.
It owns the event loop, starts and stops pipelines when `AudiciaSource`
resources change, and writes `AudiciaPolicyReport` CRDs back to the Kubernetes
API.

```
Audit Log → Ingestor → Filter → Normalizer → Aggregator → Strategy → Compliance → Report
                                    ↑ **Controller** manages the full pipeline ↑
```

---

## Reconciliation Loop

The controller watches `AudiciaSource` custom resources. When one is created,
updated, or deleted, the `Reconcile` function is called:

### Create / Update

1. Check if a pipeline goroutine is already running for this source.
2. Compare `spec.generation` — if unchanged, the reconcile is a no-op (prevents
   reconcile storms).
3. If the spec changed (generation bump), stop the old pipeline and start a new
   one.
4. Set the `Ready` condition to `PipelineStarting`, then `PipelineRunning`.

### Delete

When an `AudiciaSource` is deleted, the pipeline goroutine is cancelled.
`AudiciaPolicyReport` resources with owner references to the source are garbage
collected by Kubernetes.

---

## Event Loop

Each pipeline goroutine runs an event loop that multiplexes three concerns in a
single `select` statement:

1. **Event processing:** Reads audit events from the ingestor channel. For each
   event, runs the `Filter → Normalize Subject → Normalize Event → Aggregate`
   pipeline.
2. **Periodic flush:** On a configurable interval (default: 30 seconds), flushes
   all accumulated data — generates manifests via the
   [Strategy Engine](strategy-engine.md), evaluates compliance via the
   [Compliance Engine](compliance-engine.md), and writes reports to the API.
3. **Graceful shutdown:** On context cancellation, performs a final flush before
   the goroutine exits.

---

## Report Persistence

### Flush Cycle

On each flush, the controller iterates over all subjects that have accumulated
rules:

1. **Generate manifests** — calls the [Strategy Engine](strategy-engine.md) with
   the subject's aggregated rules.
2. **Resolve effective RBAC** — calls the
   [Compliance Engine](compliance-engine.md) resolver.
3. **Evaluate compliance** — diffs observed vs. effective rules to produce a
   score.
4. **Create or update** the `AudiciaPolicyReport` CRD with all status fields.
5. **Update checkpoint** — persists the processing position in
   `AudiciaSource.status`.

### Conflict Handling

Both report and checkpoint updates use `retry.RetryOnConflict` with
`DefaultRetry` to handle concurrent updates (e.g., from multiple reconcile loops
or during leader election transitions).

### Owner References

`AudiciaPolicyReport` resources in the same namespace as the source get an owner
reference pointing to the `AudiciaSource`. This enables automatic garbage
collection when the source is deleted.

---

## Rule Compaction

The controller runs a two-phase compaction during each flush:

1. **Retention compaction:** Drops rules with `lastSeen` older than
   `retentionDays` (default: 30 days).
2. **Count compaction:** If the rule count still exceeds `maxRulesPerReport`
   (default: 200), drops the oldest rules by `lastSeen` until under the limit.

Compacted rules are logged at `INFO` level before removal for audit trail
purposes.

---

## Concurrency and Leader Election

| Setting                 | Default                 | Description                                                    |
| ----------------------- | ----------------------- | -------------------------------------------------------------- |
| `CONCURRENT_RECONCILES` | `1`                     | Number of parallel reconcile loops.                            |
| Leader election         | Enabled                 | Only one replica processes at a time. Uses a `Lease` resource. |
| Leader election ID      | `audicia-operator-lock` | Name of the Lease resource.                                    |

With leader election enabled, you can run multiple replicas for availability —
only the leader actively processes events. On leader failover, the new leader
resumes from the last checkpoint.

---

## Core Functions

| Function               | Purpose                                                                                                                                 |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `Reconcile`            | Kubernetes controller entry point. Tracks CRD generation to prevent anti-thrashing and starts or stops pipelines when the spec changes. |
| `processEvent`         | Hot path for every audit event. Runs the filter → normalize subject → normalize rule → aggregate pipeline.                              |
| `compactRules`         | Two-phase retention: first drops rules older than `retentionDays`, then truncates by count down to `maxRulesPerReport`.                 |
| `flushSubjectReport`   | Write path. Generates manifests via the strategy package, then creates or updates the `AudiciaPolicyReport` CRD.                        |
| `populateReportStatus` | Invokes `EffectiveRules` and `diff.Evaluate` to compute the compliance score, then sets all status fields on the report.                |
| `eventLoop`            | Multiplexes event processing, periodic flush cycles, and graceful shutdown into a single select loop.                                   |

---

## Related

- [Architecture](../concepts/architecture.md) — System overview and
  reconciliation sequence diagram
- [Pipeline](../concepts/pipeline.md) — Stage-by-stage processing overview
- [Ingestor](ingestor.md) — Provides the event stream
- [Strategy Engine](strategy-engine.md) — Generates manifests during flush
- [Compliance Engine](compliance-engine.md) — Evaluates RBAC drift during flush
- [Helm Values](../configuration/helm-values.md) — Operator runtime
  configuration
