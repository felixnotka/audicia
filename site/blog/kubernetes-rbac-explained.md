---
title: "Kubernetes RBAC Explained: Roles, ClusterRoles, Bindings, and How They Work"
seo_title: "Kubernetes RBAC Explained: Roles, ClusterRoles, Bindings, and How They Work"
published_at: 2026-03-17T08:00:00.000Z
snippet: "A practical guide to Kubernetes RBAC: how Roles, ClusterRoles, RoleBindings, and ClusterRoleBindings work together to control access."
description: "Understand Kubernetes RBAC: Roles vs ClusterRoles, RoleBindings vs ClusterRoleBindings, subjects, verbs, and resources. Practical YAML examples included."
---

## What Is Kubernetes RBAC?

Role-Based Access Control (RBAC) is how Kubernetes decides who can do what. When
any request reaches the API server — a `kubectl` command, a controller
reconciliation, an admission webhook — RBAC determines whether to allow or deny
it.

RBAC answers three questions for every request:

1. **Who** is making the request? (the subject)
2. **What** are they trying to do? (the verb)
3. **Which resource** are they targeting? (the API group, resource, and
   namespace)

## The Four RBAC Resources

Kubernetes RBAC uses four resource types that work in pairs:

| Resource             | Scope     | Purpose                                       |
| -------------------- | --------- | --------------------------------------------- |
| `Role`               | Namespace | Defines permissions within a single namespace |
| `ClusterRole`        | Cluster   | Defines permissions across the entire cluster |
| `RoleBinding`        | Namespace | Grants a Role or ClusterRole to subjects      |
| `ClusterRoleBinding` | Cluster   | Grants a ClusterRole to subjects cluster-wide |

**Roles** define _what_ is allowed. **Bindings** define _who_ gets that access.

## Subjects: Who Gets Access

A subject is the identity making the API request. Kubernetes recognizes three
subject types:

### ServiceAccount

The most common subject in production clusters. Every pod runs as a
ServiceAccount. If no ServiceAccount is specified, the pod uses the `default`
ServiceAccount in its namespace.

```yaml
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: my-team
```

ServiceAccounts are namespaced — the name `backend` in namespace `my-team` is a
different identity from `backend` in namespace `other-team`.

### User

Typically used for human operators authenticating via certificates, OIDC tokens,
or other identity providers. Users are not Kubernetes resources — they exist
only as identities in the authentication layer.

```yaml
subjects:
  - kind: User
    name: jane@example.com
    apiGroup: rbac.authorization.k8s.io
```

### Group

A set of Users or ServiceAccounts. Every ServiceAccount automatically belongs to
the group `system:serviceaccounts` and `system:serviceaccounts:<namespace>`.

```yaml
subjects:
  - kind: Group
    name: system:serviceaccounts:my-team
    apiGroup: rbac.authorization.k8s.io
```

## Roles: Defining Permissions

A Role is a list of rules. Each rule specifies:

- **apiGroups** — which API groups the rule applies to (`""` is the core group)
- **resources** — which resource types (`pods`, `deployments`, `configmaps`)
- **verbs** — which operations (`get`, `list`, `watch`, `create`, `update`,
  `patch`, `delete`, `deletecollection`)

### Namespace-Scoped Role

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: my-team
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
```

This Role allows reading pods, but only in the `my-team` namespace.

### ClusterRole

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: node-reader
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
```

ClusterRoles are not namespaced. They can be used for cluster-scoped resources
(like nodes) or bound to specific namespaces via RoleBindings.

## Bindings: Connecting Subjects to Roles

### RoleBinding

A RoleBinding grants the permissions defined in a Role (or ClusterRole) to
subjects within a specific namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: backend-pod-reader
  namespace: my-team
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: my-team
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```

### ClusterRoleBinding

A ClusterRoleBinding grants permissions cluster-wide:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: backend-node-reader
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: my-team
roleRef:
  kind: ClusterRole
  name: node-reader
  apiGroup: rbac.authorization.k8s.io
```

### ClusterRole + RoleBinding (Reusable Roles)

A common pattern is defining a ClusterRole once and binding it to different
namespaces with RoleBindings:

```yaml
# Define the ClusterRole once
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: configmap-editor
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "update"]
---
# Bind it in namespace A
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: backend-configmaps
  namespace: team-a
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: team-a
roleRef:
  kind: ClusterRole
  name: configmap-editor
  apiGroup: rbac.authorization.k8s.io
```

The ClusterRole defines the rules, but the RoleBinding scopes them to `team-a`.
The same ClusterRole can be reused across many namespaces.

## Subresources

Some resources have subresources that require separate RBAC rules. Common
examples:

| Resource/Subresource    | What It Does                          |
| ----------------------- | ------------------------------------- |
| `pods/exec`             | Execute commands in a running pod     |
| `pods/log`              | Read pod logs                         |
| `deployments/scale`     | Scale a deployment up or down         |
| `serviceaccounts/token` | Request a bound service account token |
| `pods/status`           | Read or update pod status             |

Subresources are specified in the `resources` field with a slash:

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
```

Having `get pods` does NOT grant access to `pods/exec` or `pods/log`. Each
subresource needs its own rule.

## API Groups

Kubernetes organizes resources into API groups:

| API Group                   | Resources                                  |
| --------------------------- | ------------------------------------------ |
| `""` (core)                 | pods, services, configmaps, secrets, nodes |
| `apps`                      | deployments, statefulsets, daemonsets      |
| `batch`                     | jobs, cronjobs                             |
| `rbac.authorization.k8s.io` | roles, rolebindings, clusterroles          |
| `networking.k8s.io`         | networkpolicies, ingresses                 |
| `policy`                    | poddisruptionbudgets                       |

When writing RBAC rules, the `apiGroups` field must match the resource's API
group. Getting this wrong is one of the most common causes of unexpected 403
errors.

## The Verb Model

Kubernetes defines eight standard verbs:

| Verb               | HTTP Method | Description                       |
| ------------------ | ----------- | --------------------------------- |
| `get`              | GET         | Read a single resource by name    |
| `list`             | GET         | List all resources in a namespace |
| `watch`            | GET         | Stream changes to resources       |
| `create`           | POST        | Create a new resource             |
| `update`           | PUT         | Replace an entire resource        |
| `patch`            | PATCH       | Partially modify a resource       |
| `delete`           | DELETE      | Delete a single resource          |
| `deletecollection` | DELETE      | Delete all resources of a type    |

Read access typically requires `get`, `list`, and `watch` together. Write access
typically requires `create`, `update`, and `patch`.

## Common Mistakes

### Granting cluster-admin to Workloads

`cluster-admin` grants every verb on every resource in every namespace. It is
intended for cluster operators, not workloads. If a service account needs broad
access, define a custom ClusterRole with only the required permissions.

### Forgetting Namespace Scope

A Role in namespace `A` does not grant access in namespace `B`. If a workload
needs access across namespaces, use a ClusterRole with namespace-scoped
RoleBindings — not a ClusterRoleBinding.

### Missing Subresources

Granting `get pods` does not grant `pods/exec` or `pods/log`. These are separate
subresources that need their own rules. This is a frequent source of 403 errors.

### Stale Permissions

RBAC is typically written once and never updated. As workloads evolve — new
controllers, new API calls, removed features — the RBAC policy drifts out of
sync. This creates both excess permissions (security risk) and missing
permissions (availability risk).

## Automating RBAC

Writing correct RBAC by hand is difficult because of the combinatorial
complexity. For a practical approach to generating correct policies from
observed behavior, see
[The 403 Cycle: Why RBAC Breaks in Practice](/blog/kubernetes-rbac-broken-in-practice).

For a hands-on walkthrough of generating least-privilege Roles from audit log
data, see
[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs).

## Further Reading

- **[How to Audit Kubernetes RBAC](/blog/kubernetes-rbac-audit)** — finding who
  has access to what with kubectl and Audicia
- **[Kubernetes RBAC Best Practices](/blog/kubernetes-rbac-best-practices)** —
  opinionated production guide
- **[Getting Started with Audicia](/docs/getting-started/introduction)** —
  install Audicia and automate RBAC generation
