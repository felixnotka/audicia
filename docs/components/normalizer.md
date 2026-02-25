# Normalizer

The normalizer converts raw audit event data into Audicia's canonical internal
representation. It handles two concerns: **subject normalization** (who made the
request) and **event normalization** (what they did).

**Package:** `pkg/normalizer/`

---

## Where It Sits in the Pipeline

```
Audit Log → Ingestor → Filter → **Normalizer** → Aggregator → Strategy → Compliance → Report
```

**Input:** Filtered `audit.k8s.io/v1.Event` structs. **Output:** Structured
`(Subject, CanonicalRule)` pairs ready for aggregation.

---

## Subject Normalization

Converts raw username strings into structured identity objects:

| Raw Input                               | Normalized Output                                      |
| --------------------------------------- | ------------------------------------------------------ |
| `system:serviceaccount:my-team:backend` | `Kind=ServiceAccount, Name=backend, Namespace=my-team` |
| `alice@example.com`                     | `Kind=User, Name=alice@example.com`                    |
| `system:kube-controller-manager`        | Filtered out (system user, configurable)               |

**ServiceAccount parsing:** The normalizer splits the
`system:serviceaccount:<namespace>:<name>` format to extract the namespace and
name, creating a properly scoped identity that drives per-namespace Role
generation downstream.

**Groups:** Group metadata is captured from the audit event but not used for
binding generation by default. Group-to-binding attribution is ambiguous — a
single request may carry multiple groups, and it's unclear which group should
receive the binding.

---

## Event Normalization

Converts raw audit event fields into canonical RBAC rule components. This is the
core mapping contract that ensures generated policies are syntactically correct
Kubernetes RBAC.

| Input                                | Output                          | Rule                                                |
| ------------------------------------ | ------------------------------- | --------------------------------------------------- |
| `resource=pods, subresource=exec`    | `resources: ["pods/exec"]`      | Subresource concatenation (mandatory for RBAC)      |
| `requestURI=/metrics, objectRef=nil` | `nonResourceURLs: ["/metrics"]` | Non-resource URL detection (emitted as ClusterRole) |
| `apiGroup=extensions/v1beta1`        | `apiGroups: ["apps"]`           | API group migration to stable equivalents           |
| `resourceName=my-pod`                | _(omitted by default)_          | Configurable via `policyStrategy.resourceNames`     |

### API Group Migration

The normalizer maintains a migration table that maps deprecated API groups to
their stable equivalents:

- `extensions` → `apps` (covers Deployments, ReplicaSets, DaemonSets using the
  legacy `extensions/v1beta1` group)

All other API groups — including `networking.k8s.io`, `policy`,
`rbac.authorization.k8s.io`, etc. — pass through verbatim. If your cluster still
emits events with other deprecated API groups, the generated policies will
reference those groups as-is.

### Edge Cases

- **CRDs with unusual group names** are passed through verbatim — no migration
  is applied.
- **Aggregated API servers** are also passed through verbatim.
- **Non-standard verbs** are preserved by the normalizer (filtering happens
  later in the [Strategy Engine](strategy-engine.md)).

---

## Core Functions

| Function           | Purpose                                                                                                                                                             |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `NormalizeEvent`   | Converts raw audit fields into a `CanonicalRule`. Handles non-resource URLs, API group migration (e.g., `extensions` → `apps`), and subresource path concatenation. |
| `NormalizeSubject` | Parses `system:serviceaccount:<ns>:<name>` strings, classifies subject kind (ServiceAccount, User, Group), and gates system user filtering.                         |

---

## Related

- [Architecture](../concepts/architecture.md) — System overview and data flow
- [Pipeline](../concepts/pipeline.md) — Stage-by-stage processing overview
- [Strategy Engine](strategy-engine.md) — How normalized rules become RBAC
  manifests
