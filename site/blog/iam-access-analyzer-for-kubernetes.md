---
title: "IAM Access Analyzer for Kubernetes: How to Right-Size RBAC Permissions"
seo_title: "IAM Access Analyzer for Kubernetes: How to Right-Size RBAC Permissions"
published_at: 2026-04-17T08:00:00.000Z
snippet: "AWS IAM Access Analyzer right-sizes IAM policies based on observed usage. Here's how to do the same thing for Kubernetes RBAC with audit-log-based analysis."
description: "Right-size Kubernetes RBAC like AWS IAM Access Analyzer. Compare granted permissions against observed audit log usage to eliminate excess privileges."
---

## The AWS Pattern

AWS IAM Access Analyzer solves a problem that every cloud platform has: users
create overly broad IAM policies because getting the exact permissions right is
too hard. Access Analyzer examines CloudTrail logs to determine which IAM
actions were actually used, then generates a tightened policy that covers only
observed behavior.

The result: IAM policies that match reality instead of guesswork.

Kubernetes has the same problem — but until recently, no equivalent tool.

## The Kubernetes Equivalent

The pattern is identical:

| AWS                  | Kubernetes               |
| -------------------- | ------------------------ |
| IAM Policies         | Roles and ClusterRoles   |
| CloudTrail Logs      | Kubernetes Audit Logs    |
| IAM Access Analyzer  | RBAC Generator (Audicia) |
| Tightened IAM Policy | Suggested Role YAML      |

In both cases, the approach is:

1. **Observe** what the identity actually does (from access logs)
2. **Compare** observed actions against granted permissions
3. **Generate** a tightened policy that covers only what was observed
4. **Apply** the tighter policy, removing excess privileges

## Why Kubernetes Needs This

Kubernetes RBAC has the same combinatorial complexity as IAM:

- **API groups** — `""`, `apps`, `batch`, `rbac.authorization.k8s.io`, etc.
- **Resources** — pods, deployments, secrets, configmaps, and dozens more
- **Subresources** — `pods/exec`, `pods/log`, `deployments/scale`
- **Verbs** — get, list, watch, create, update, patch, delete, deletecollection
- **Namespaces** — each combination can apply per-namespace or cluster-wide

A single workload might need 3 verbs on 4 resources across 2 namespaces. But the
service account is typically bound to a broad Role (or `cluster-admin`) because
nobody took the time to figure out the exact permissions needed.

## How Right-Sizing Works

### Step 1: Collect Observed Usage

Kubernetes audit logs record every API call with the fields needed for RBAC
analysis:

```json
{
  "user": { "username": "system:serviceaccount:my-team:backend" },
  "verb": "get",
  "objectRef": {
    "resource": "pods",
    "namespace": "my-team",
    "apiGroup": ""
  },
  "responseStatus": { "code": 200 }
}
```

Over time, this data builds a complete picture of what each service account
actually does.

### Step 2: Resolve Granted Permissions

Query the existing Roles and Bindings for the subject to determine what it is
currently allowed to do. This includes:

- All ClusterRoleBindings referencing the subject
- All RoleBindings in relevant namespaces
- Resolution of each binding to its backing Role or ClusterRole

### Step 3: Diff

Compare granted against observed. For each granted permission rule:

- **Used** — at least one audit event exercised this permission
- **Excess** — no audit event exercised this permission

The excess rules are the overprivilege. The ratio of used to total gives the
compliance score.

### Step 4: Generate the Tightened Policy

From the observed actions, generate the minimal Role that covers all observed
behavior:

```yaml
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
    verbs: ["get"]
```

This is the Kubernetes equivalent of IAM Access Analyzer's tightened policy.

## What Makes This Hard

### Subresources

Kubernetes subresources (`pods/exec`, `pods/log`, `deployments/scale`) are
separate RBAC resources that need their own rules. Audit logs record the
subresource separately from the parent resource. A proper analyzer must
concatenate them correctly — `get pods/log` is different from `get pods`.

### API Group Migration

Kubernetes deprecated `extensions/v1beta1` years ago. But old audit logs and
some controllers still reference the deprecated group. An analyzer that
generates RBAC from these events must normalize `extensions/v1beta1` to
`apps/v1` to produce valid modern policies.

### Namespace Scoping

A service account may access resources in multiple namespaces. IAM Access
Analyzer can generate a single tightened policy. For Kubernetes, the equivalent
is per-namespace Roles — each scoped to the namespace where the access was
observed.

### Observation Period

IAM Access Analyzer uses CloudTrail data over a configurable window. For
Kubernetes, the observation period determines completeness. A workload with a
monthly batch job needs at least 30 days of audit data before the generated
policy is complete.

## Sensitive Excess Detection

Like IAM Access Analyzer flagging high-risk IAM actions, Audicia flags excess
RBAC permissions on sensitive resources:

- **Secrets** — unused access to secrets
- **Nodes** — unused direct node access
- **Webhook configurations** — unused mutating or validating webhook access
- **RBAC resources** — unused ability to modify roles and bindings

These are flagged in the `sensitiveExcess` field of each compliance report,
allowing teams to prioritize the most dangerous excess first.

## Further Reading

- **[Kubernetes RBAC Drift Detection](/blog/kubernetes-rbac-drift-detection)** —
  detecting and acting on permission drift
- **[Kubernetes RBAC Compliance Evidence](/blog/kubernetes-rbac-compliance-evidence)**
  — mapping right-sizing results to SOC 2, ISO 27001, PCI DSS
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start right-sizing RBAC
