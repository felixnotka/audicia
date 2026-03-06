# AudiciaPolicyReport CRD (Deprecated)

> **Deprecated in 0.5.0.** This CRD has been split into
> [AudiciaReport](crd-audiciareport.md) (compliance scoring, observed rules) and
> [AudiciaPolicy](crd-audiciapolicy.md) (suggested RBAC manifests with approval
> workflow). See the [Upgrading to 0.5.0](../guides/upgrading-to-0.5.md) guide
> for migration steps.

---

**API Group:** `audicia.io/v1alpha1` **Scope:** Namespaced **Short names:**
`apr`, `apreport`

## Overview

`AudiciaPolicyReport` was the single output CRD used from 0.1.0 through 0.4.x.
It combined compliance scoring, observed RBAC rules, and suggested policy
manifests into one resource. In 0.5.0, these concerns were separated:

| Old field                          | New location                                        |
| ---------------------------------- | --------------------------------------------------- |
| `status.observedRules`             | `AudiciaReport` `.status.observedRules`             |
| `status.compliance`                | `AudiciaReport` `.status.compliance`                |
| `status.eventsProcessed`           | `AudiciaReport` `.status.eventsProcessed`           |
| `status.suggestedPolicy.manifests` | `AudiciaPolicy` `.spec.manifests`                   |
| (n/a)                              | `AudiciaPolicy` `.status.state` (approval workflow) |

## Example (0.4.x)

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaPolicyReport
metadata:
  name: report-sa-backend
  namespace: my-team
spec:
  subject:
    kind: ServiceAccount
    name: backend
    namespace: my-team
status:
  eventsProcessed: 24458
  lastProcessedTime: "2026-02-14T12:05:00Z"
  observedRules:
    - apiGroups: [""]
      resources: ["pods"]
      verbs: ["get", "list", "watch"]
      count: 14320
    - apiGroups: [""]
      resources: ["configmaps"]
      verbs: ["get", "watch"]
      count: 9841
  suggestedPolicy:
    manifests:
      - |
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
              resources: ["configmaps"]
              verbs: ["get", "watch"]
  compliance:
    score: 50
    severity: Yellow
    usedCount: 4
    excessCount: 4
    hasSensitiveExcess: true
    sensitiveExcess:
      - secrets
```

## spec

| Field               | Type   | Required | Description                              |
| ------------------- | ------ | -------- | ---------------------------------------- |
| `subject.kind`      | string | Yes      | `ServiceAccount`, `User`, or `Group`     |
| `subject.name`      | string | Yes      | Name of the subject                      |
| `subject.namespace` | string | No       | Namespace (relevant for ServiceAccounts) |

## status

| Field                           | Type           | Description                                          |
| ------------------------------- | -------------- | ---------------------------------------------------- |
| `observedRules[]`               | ObservedRule[] | Structured list of observed RBAC rules               |
| `suggestedPolicy.manifests`     | string[]       | Rendered YAML (Role, ClusterRole, Binding) manifests |
| `compliance.score`              | int32          | Compliance score (0–100)                             |
| `compliance.severity`           | string         | `Green` (>= 80), `Yellow` (>= 50), `Red` (< 50)      |
| `compliance.usedCount`          | int32          | Effective rules that were observed in use            |
| `compliance.excessCount`        | int32          | Effective rules never observed (overprivilege)       |
| `compliance.uncoveredCount`     | int32          | Observed actions not covered by any effective rule   |
| `compliance.hasSensitiveExcess` | bool           | True when excess grants include sensitive resources  |
| `compliance.sensitiveExcess`    | string[]       | Sensitive resources with unused grants               |
| `eventsProcessed`               | int64          | Total audit events processed for this report         |
| `lastProcessedTime`             | date-time      | Timestamp of the most recent processed event         |
| `conditions[]`                  | Condition[]    | Standard Kubernetes conditions (`Ready`)             |

## Migration

```bash
# 1. Apply the new CRDs
kubectl apply -f audicia-operator/crds/audicia.io_audiciareports.yaml
kubectl apply -f audicia-operator/crds/audicia.io_audiciapolicies.yaml

# 2. Upgrade the operator
helm upgrade audicia audicia/audicia-operator -n audicia-system -f your-values.yaml

# 3. Delete the old CRD
kubectl delete crd audiciapolicyreports.audicia.io
```

See [Upgrading to 0.5.0](../guides/upgrading-to-0.5.md) for the full guide.
