---
title: "How to Audit Kubernetes RBAC: Finding Who Has Access to What"
seo_title: "How to Audit Kubernetes RBAC: Finding Who Has Access to What"
published_at: 2026-03-21T08:00:00.000Z
snippet: "Step-by-step guide to auditing Kubernetes RBAC with kubectl: find cluster-admin bindings, check effective permissions, and identify unused grants."
description: "Audit Kubernetes RBAC step by step with kubectl. Find cluster-admin bindings, check effective permissions, and identify overprivileged service accounts."
---

## Why Audit RBAC?

Kubernetes RBAC controls who can do what in your cluster. But RBAC
configurations drift over time — bindings accumulate, permissions expand, and
nobody removes access that is no longer needed.

A periodic RBAC audit answers three questions:

1. **Who has cluster-admin?** — identify the most dangerous bindings first
2. **What can each service account actually do?** — map effective permissions
3. **Which permissions are actually used?** — find excess privilege

The first two can be answered with `kubectl`. The third requires audit log data.

## Step 1: Find cluster-admin Bindings

The most critical finding in any RBAC audit is non-system subjects bound to
`cluster-admin`. This grants full access to everything:

```bash
kubectl get clusterrolebindings -o json | \
  jq -r '.items[] |
    select(.roleRef.name == "cluster-admin") |
    .metadata.name + " → " +
    (.subjects[]? | .kind + ":" + .namespace + "/" + .name)'
```

Expected output for a healthy cluster:

```
system:masters → Group:/system:masters
```

Any ServiceAccount that appears in this list is a remediation priority. Service
accounts should never need `cluster-admin`.

## Step 2: Check a Subject's Effective Permissions

`kubectl auth can-i` answers whether a specific subject can perform a specific
action:

```bash
# Can the backend SA in my-team get pods?
kubectl auth can-i get pods \
  --as=system:serviceaccount:my-team:backend \
  -n my-team
```

To see all permissions for a subject, use `--list`:

```bash
kubectl auth can-i --list \
  --as=system:serviceaccount:my-team:backend \
  -n my-team
```

This outputs a table of every resource and verb the subject is allowed to access
in that namespace. It also includes non-resource URLs like `/healthz` and
`/metrics`.

### Checking Cluster-Wide Permissions

For cluster-scoped permissions (nodes, namespaces, ClusterRoles), omit the
namespace flag:

```bash
kubectl auth can-i --list \
  --as=system:serviceaccount:my-team:backend
```

## Step 3: List All Bindings for a Subject

To find every RoleBinding and ClusterRoleBinding that references a specific
service account:

```bash
# ClusterRoleBindings
kubectl get clusterrolebindings -o json | \
  jq -r '.items[] |
    select(.subjects[]? |
      .kind == "ServiceAccount" and
      .name == "backend" and
      .namespace == "my-team"
    ) | .metadata.name + " → " + .roleRef.name'

# RoleBindings (in a specific namespace)
kubectl get rolebindings -n my-team -o json | \
  jq -r '.items[] |
    select(.subjects[]? |
      .kind == "ServiceAccount" and
      .name == "backend"
    ) | .metadata.name + " → " + .roleRef.name'
```

## Step 4: Inspect the Roles

Once you know which Roles and ClusterRoles a subject is bound to, inspect them:

```bash
kubectl describe role pod-reader -n my-team
kubectl describe clusterrole node-reader
```

Or get the full YAML:

```bash
kubectl get role pod-reader -n my-team -o yaml
kubectl get clusterrole node-reader -o yaml
```

Look for:

- **Wildcard verbs** (`["*"]`) — grants every operation
- **Wildcard resources** (`["*"]`) — grants access to every resource type
- **Secrets access** — often unnecessary and high-risk
- **RBAC access** (`roles`, `rolebindings`, `clusterroles`) — allows privilege
  escalation
- **Node access** — rarely needed by workloads

## Step 5: Finding Unused Permissions

Steps 1–4 tell you what is _granted_. They do not tell you what is _used_.

A service account may have `get`, `list`, `watch`, `create`, `update`, `patch`,
and `delete` on pods — but if it only ever calls `get` and `list`, the other
five verbs are excess privilege.

Finding unused permissions requires comparing granted RBAC against actual API
access patterns. These patterns are recorded in Kubernetes audit logs.

### Manual Approach

If you have audit logging enabled, you can search the audit log for a specific
service account:

```bash
# Find all API calls made by the backend SA
jq 'select(.user.username == "system:serviceaccount:my-team:backend")
  | {verb, resource: .objectRef.resource,
     namespace: .objectRef.namespace,
     subresource: .objectRef.subresource}' \
  /var/log/kube-audit.log | sort | uniq -c | sort -rn
```

This works for one-time analysis, but it has limitations:

- You must re-process the entire log file each time
- No state across log rotations
- No handling of subresource concatenation or API group migration
- No compliance score or structured output

### Automated Approach with Audicia

Audicia automates the entire audit process. It continuously processes audit
events, resolves each subject's effective RBAC permissions, and produces a
compliance report comparing granted vs. observed:

```bash
kubectl get apreport --all-namespaces -o wide
```

```
NAMESPACE   NAME             SUBJECT   KIND             COMPLIANCE   SCORE   AGE   NEEDED   EXCESS   UNGRANTED   SENSITIVE   AUDIT EVENTS
my-team     report-backend   backend   ServiceAccount   Red          25      15m   2        6        0           true        1500
my-team     report-worker    worker    ServiceAccount   Green        88      15m   7        1        0           false       3200
```

A score of 25 means the service account uses only 25% of its granted
permissions. The `SENSITIVE` column flags when unused permissions include
high-risk resources like secrets or webhook configurations.

Each report contains the complete suggested policy — the minimal Roles and
RoleBindings the subject actually needs:

```bash
kubectl get apreport report-backend -n my-team \
  -o jsonpath='{.status.suggestedPolicy.manifests[0]}'
```

## Building a Regular Audit Process

### Quarterly Manual Audit

At minimum, run Steps 1–4 quarterly and document the results. Focus on:

1. Any new `cluster-admin` bindings since the last audit
2. ServiceAccounts in non-system namespaces with cluster-wide bindings
3. Roles with wildcard verbs or resources
4. Unused bindings (service accounts that no longer exist)

### Continuous Automated Audit

For continuous visibility, deploy Audicia as an operator. It runs the entire
audit loop automatically — ingesting audit events, resolving effective RBAC,
computing compliance scores, and producing structured reports that can be
exported to compliance dashboards or GitOps repositories.

## Further Reading

- **[Kubernetes RBAC Drift Detection](/blog/kubernetes-rbac-drift-detection)** —
  how to detect and act on permission drift over time
- **[Kubernetes RBAC Compliance Evidence](/blog/kubernetes-rbac-compliance-evidence)**
  — mapping audit results to SOC 2, ISO 27001, and PCI DSS
- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  full before/after walkthrough
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and automate RBAC auditing
