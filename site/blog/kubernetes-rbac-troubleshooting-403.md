---
title: "Kubernetes RBAC Troubleshooting: Common 403 Errors and How to Fix Them"
seo_title: "Kubernetes RBAC Troubleshooting: Common 403 Errors and How to Fix Them"
published_at: 2026-04-21T08:00:00.000Z
snippet: "Troubleshoot Kubernetes RBAC 403 Forbidden errors: missing bindings, wrong API groups, missing subresources, namespace scope mistakes, and how to fix each one."
description: "Troubleshoot Kubernetes 403 Forbidden errors: missing bindings, wrong API groups, missing subresources, and namespace scope mistakes. Practical fix for each."
---

## The 403 Error

Every Kubernetes engineer has seen it:

```
Error from server (Forbidden): pods is forbidden: User
"system:serviceaccount:my-team:backend" cannot list resource "pods"
in API group "" in the namespace "my-team"
```

The error message is informative — it tells you exactly which subject, verb,
resource, API group, and namespace failed. But knowing _what_ failed is
different from knowing _why_ it failed and how to fix it correctly.

## Cause 1: No Binding Exists

**The error:** The subject has no RoleBinding or ClusterRoleBinding at all.

**How to check:**

```bash
# Check for RoleBindings in the namespace
kubectl get rolebindings -n my-team -o json | \
  jq -r '.items[] |
    select(.subjects[]? |
      .kind == "ServiceAccount" and
      .name == "backend" and
      .namespace == "my-team"
    ) | .metadata.name'

# Check for ClusterRoleBindings
kubectl get clusterrolebindings -o json | \
  jq -r '.items[] |
    select(.subjects[]? |
      .kind == "ServiceAccount" and
      .name == "backend" and
      .namespace == "my-team"
    ) | .metadata.name'
```

**The fix:** Create a RoleBinding referencing a Role that grants the required
permissions.

**Common mistake:** Creating a Role but forgetting the RoleBinding. A Role alone
does nothing — it must be bound to a subject.

## Cause 2: Wrong API Group

**The error:** The Role specifies the wrong API group for the resource.

Example: trying to access Deployments with `apiGroups: [""]` instead of
`apiGroups: ["apps"]`.

**How to check:**

```bash
kubectl api-resources | grep deployments
```

```
deployments   deploy   apps/v1   true   Deployment
```

The `apps` in `apps/v1` is the API group.

**The fix:** Update the Role's `apiGroups` to match the resource's actual API
group:

```yaml
rules:
  - apiGroups: ["apps"] # Not ""
    resources: ["deployments"]
    verbs: ["get", "list", "watch"]
```

**Common mistake:** Using `""` (the core API group) for resources that live in
other API groups. Core resources (pods, services, configmaps, secrets) use `""`.
Everything else has a specific API group.

## Cause 3: Missing Subresource

**The error:** The subject needs access to a subresource (like `pods/exec` or
`pods/log`) but the Role only grants access to the parent resource.

**How to check:**

```bash
kubectl auth can-i create pods/exec \
  --as=system:serviceaccount:my-team:backend \
  -n my-team
```

**The fix:** Add a separate rule for the subresource:

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
```

**Common mistake:** Assuming `get pods` grants access to `pods/exec` and
`pods/log`. These are separate RBAC resources that need their own rules.

## Cause 4: Namespace Scope Mismatch

**The error:** The Role exists but in the wrong namespace, or the binding uses a
ClusterRole when a RoleBinding was needed.

**How to check:**

```bash
# List the subject's effective permissions in a specific namespace
kubectl auth can-i --list \
  --as=system:serviceaccount:my-team:backend \
  -n my-team
```

**The fix:** Ensure the RoleBinding is in the correct namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: backend-pods
  namespace: my-team # Must match the target namespace
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: my-team
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```

**Common mistake:** Creating a RoleBinding in namespace `default` when the
workload runs in `my-team`.

## Cause 5: Missing Verb

**The error:** The Role grants some verbs but not the one the workload needs.

Example: the Role grants `get` and `list` on pods, but the workload also needs
`watch` for informer-based controllers.

**How to check:**

```bash
kubectl auth can-i watch pods \
  --as=system:serviceaccount:my-team:backend \
  -n my-team
```

**The fix:** Add the missing verb:

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"] # Added "watch"
```

**Common mistake:** Omitting `watch` when a controller uses informers. Most Go
controllers require `get`, `list`, and `watch` together.

## Cause 6: ServiceAccount Namespace Mismatch in Binding

**The error:** The RoleBinding references a ServiceAccount with the wrong
namespace.

```yaml
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: default # Wrong — should be my-team
```

**How to check:** Inspect the binding's subjects:

```bash
kubectl get rolebinding backend-pods -n my-team -o yaml
```

**The fix:** Update the subject's namespace to match where the ServiceAccount
actually lives.

## Preventing 403 Errors Systematically

The root cause of most 403 errors is that RBAC policies are written by hand,
based on incomplete knowledge of what a workload needs. Each of the causes above
represents a specific way that hand-written RBAC can be wrong.

RBAC generators eliminate these errors by producing policies from observed audit
log data. The generated policy includes the exact API groups, subresources,
verbs, and namespace scoping the workload requires — because it was observed
doing it.

For a walkthrough of this approach, see
[The 403 Cycle: Why RBAC Breaks in Practice](/blog/kubernetes-rbac-broken-in-practice).

## Further Reading

- **[Kubernetes RBAC Explained](/blog/kubernetes-rbac-explained)** — a practical
  guide to Roles, Bindings, and how they work
- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  generating correct policies that prevent 403 errors
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and automate RBAC generation
