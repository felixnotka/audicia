# Architecture

## System Overview

Audicia is a Kubernetes Operator built on `controller-runtime`. It watches `AudiciaSource` custom resources, ingests
audit log streams, and produces `AudiciaPolicyReport` CRDs containing evidence-based RBAC policies.

## Data Flow

Every audit event flows through a six-stage pipeline:

```
Audit Log → Ingestor → Filter → Normalizer → Aggregator → Strategy Engine → Compliance Engine → Report
```

1. **[Ingestor](../components/ingestor.md)** — Reads audit events from a file on disk or an HTTPS webhook endpoint. Outputs raw `audit.k8s.io/v1.Event` structs.
2. **[Filter](../components/filter.md)** — Drops events that shouldn't generate policy recommendations (system users, noisy namespaces). Configurable allow/deny chain.
3. **[Normalizer](../components/normalizer.md)** — Converts raw events into canonical RBAC rules: parses subject identity, concatenates subresources, migrates API groups (`extensions` → `apps`).
4. **[Aggregator](../components/aggregator.md)** — Deduplicates rules per subject. Same rule observed twice increments the count and updates `lastSeen`.
5. **[Strategy Engine](../components/strategy-engine.md)** — Applies policy knobs (scope mode, verb merging, wildcards) to shape the final RBAC output.
6. **[Compliance Engine](../components/compliance-engine.md)** — Resolves effective RBAC permissions and diffs against observed usage to produce a compliance score.

The output is an `AudiciaPolicyReport` CRD per subject, containing observed rules, suggested RBAC manifests, and compliance data.

## Event Ingestion Scope

Audicia processes all audit events that pass through its filter chain, regardless of HTTP status code:

- **Allowed requests (2xx)** — The primary source of truth for right-sizing permissions.
- **Denied requests (403)** — Shows what access is needed but missing.
- **Other errors (4xx, 5xx)** — Generally excluded from policy generation.

### Required Audit Level

Audicia requires `Metadata` audit level at minimum. The key fields used from each event:

- `user.username`, `user.groups` — Subject identity
- `verb` — The API verb (get, list, create, etc.)
- `objectRef.resource`, `objectRef.subresource`, `objectRef.namespace`, `objectRef.apiGroup` — Target resource
- `requestURI` — For non-resource URL detection
- `responseStatus.code` — To distinguish allowed vs. denied
- `auditID` — For webhook deduplication

`RequestResponse` level works but generates significantly more data that Audicia does not use.

## Object Size Limits and Retention

Kubernetes etcd has a default object size limit of 1.5MB. Audicia enforces limits to prevent etcd pressure:

| Limit                    | Default    | Configurable                      |
|--------------------------|------------|-----------------------------------|
| **Max rules per report** | 200        | `spec.limits.maxRulesPerReport`   |
| **Retention window**     | 30 days    | `spec.limits.retentionDays`       |
| **Min update interval**  | 30 seconds | `spec.checkpoint.intervalSeconds` |
| **Max batch size**       | 500 events | `spec.checkpoint.batchSize`       |

When a report exceeds `maxRulesPerReport`, the oldest rules (by `lastSeen`) are dropped first. Compacted rules are logged before removal.

**Scaling guidance:** A typical microservice generates 5-20 unique rules. A namespace with 50 service accounts produces ~50 reports, each typically 5-50KB.

## Security

Audicia follows the principle of least privilege in its own design:

- **Read-only** access to audit log sources and RBAC objects. No secrets access, no impersonation.
- **No auto-apply.** Audicia generates policy recommendations. Humans or GitOps pipelines apply them.
- **Minimal footprint.** Single Deployment, no external dependencies (no database, no message queue).
- **Webhook security.** TLS-only, mTLS recommended, rate limited, NetworkPolicy restricted.

See the [Security Model](security-model.md) for the full threat model.
