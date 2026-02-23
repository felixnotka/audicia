# The Audicia Pipeline

Audicia processes audit events through a staged pipeline. Each stage is decoupled, testable, and has a single
responsibility.

```
Audit Log → Ingestor → Filter → Normalizer → Aggregator → Strategy → Diff Engine → Report
```

## 1. Ingestion

**Package:** `pkg/ingestor/` | **Deep-dive:** [Ingestor Component](../components/ingestor.md)

The ingestor abstracts the audit log source into a unified event stream.

| Source               | Mechanism                      | State Tracking                                 |
|----------------------|--------------------------------|------------------------------------------------|
| File (`K8sAuditLog`) | Tail with fsnotify, 1s polling | inode + fileOffset + lastTimestamp             |
| Webhook              | HTTPS POST receiver            | auditID-based LRU dedup cache (10,000 entries) |

Both sources output raw `audit.k8s.io/v1.Event` structs. The ingestor knows nothing about RBAC.

**File ingestion** supports checkpoint/resume: on restart, it resumes from the last saved byte offset. Inode tracking
(Linux-only) detects log rotation and resets the offset.

**Webhook ingestion** is stateless — it handles deduplication via an in-memory LRU cache keyed by `auditID`. After
restart, some duplicates may occur; the aggregator handles idempotent merging.

## 2. Filtering

**Package:** `pkg/filter/` | **Deep-dive:** [Filter Component](../components/filter.md)

An ordered allow/deny chain. **First match wins.** If no rule matches, the event is allowed by default.

```yaml
filters:
  - action: Deny
    userPattern: "^system:node:.*"
  - action: Deny
    userPattern: "^system:kube-.*"
  - action: Allow
    namespacePattern: "^my-team-.*"
```

Additionally, `ignoreSystemUsers: true` (default) drops all `system:*` users except service accounts.

See [Filter Recipes](../guides/filter-recipes.md) for common configurations.

## 3. Subject Normalization

**Package:** `pkg/normalizer/` | **Deep-dive:** [Normalizer Component](../components/normalizer.md)

Converts raw username strings into structured identity objects:

| Raw Input                               | Normalized Output                                    |
|-----------------------------------------|------------------------------------------------------|
| `system:serviceaccount:my-team:backend` | Kind=ServiceAccount, Name=backend, Namespace=my-team |
| `alice@example.com`                     | Kind=User, Name=alice@example.com                    |

Groups are captured as metadata but not used for binding generation by default.

## 4. Event Normalization

**Package:** `pkg/normalizer/` | **Deep-dive:** [Normalizer Component](../components/normalizer.md)

Converts raw audit event fields into canonical RBAC rule components:

| Input                                | Output                          | Rule                |
|--------------------------------------|---------------------------------|---------------------|
| `resource=pods, subresource=exec`    | `resources: ["pods/exec"]`      | Concatenation       |
| `requestURI=/metrics, objectRef=nil` | `nonResourceURLs: ["/metrics"]` | Non-resource URL    |
| `apiGroup=extensions/v1beta1`        | `apiGroups: ["apps"]`           | API group migration |

## 5. Aggregation

**Package:** `pkg/aggregator/` | **Deep-dive:** [Aggregator Component](../components/aggregator.md)

Deduplicates and merges observed rules per subject:

- **Deduplication key:** `(apiGroup, resource, verb, nonResourceURL, namespace)`
- **Metadata:** firstSeen, lastSeen, count
- **Idempotent:** Reprocessing the same event after restart produces the same rule set

## 6. Policy Strategy

**Package:** `pkg/strategy/` | **Deep-dive:** [Strategy Engine Component](../components/strategy-engine.md)

Applies user-configured knobs to shape the final RBAC output:

| Knob        | Options                              | Default         | Effect                                  |
|-------------|--------------------------------------|-----------------|-----------------------------------------|
| `scopeMode` | NamespaceStrict, ClusterScopeAllowed | NamespaceStrict | Controls Role vs ClusterRole generation |
| `verbMerge` | Smart, Exact                         | Smart           | Merges same-resource rules by verb      |
| `wildcards` | Forbidden, Safe                      | Forbidden       | Controls wildcard verb generation       |

Safety guardrails: never generates `cluster-admin`, only emits standard K8s verbs, `Safe` wildcards require all 8
verbs observed.

See [RBAC Policy Generation](rbac-generation.md) for details on the generated output and how to use it.

## 7. RBAC Resolver + Diff Engine

**Packages:** `pkg/rbac/`, `pkg/diff/` | **Deep-dive:** [Compliance Engine Component](../components/compliance-engine.md)

The resolver queries all ClusterRoleBindings and RoleBindings in the cluster to determine the subject's effective
permissions. The diff engine compares observed usage against effective permissions to produce a compliance score.

See [Compliance Scoring](compliance-scoring.md) for details.

## 8. Report Output

The final output is an `AudiciaPolicyReport` CRD containing:

- `status.observedRules` — Structured data for machine consumption
- `status.suggestedPolicy.manifests` — Ready-to-apply RBAC YAML
- `status.compliance` — Score, severity, excess/uncovered counts

See the [AudiciaPolicyReport CRD](../reference/crd-audiciapolicyreport.md) for the full field reference.

## Processing Model

Events are processed in batches (default: 500). The pipeline flushes reports and checkpoints on a configurable
interval (default: 30 seconds). On graceful shutdown, a final flush is performed.

Each `AudiciaSource` gets its own pipeline goroutine with generation tracking to prevent reconcile storms.
See the [Controller Component](../components/controller.md) for details on the event loop and reconciliation.
