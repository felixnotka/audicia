---
title: "Kubernetes RBAC for Multi-Tenant Clusters: Per-Namespace Policy Generation"
seo_title: "Kubernetes RBAC for Multi-Tenant Clusters: Per-Namespace Policy Generation"
published_at: 2026-04-10T08:00:00.000Z
snippet: "How to generate per-namespace RBAC policies in multi-tenant Kubernetes clusters — isolating teams while maintaining least-privilege across all tenants."
description: "Generate per-namespace RBAC policies for multi-tenant Kubernetes clusters. Isolate teams with namespace-scoped Roles while maintaining least-privilege access."
---

## The Multi-Tenant RBAC Challenge

Multi-tenant Kubernetes clusters share infrastructure across teams. Each team
operates in its own namespaces, runs its own workloads, and should have isolated
permissions.

The RBAC challenge in multi-tenant clusters is scope: a service account in
`team-a` should never have access to resources in `team-b`. But cluster-wide
bindings, shared ClusterRoles, and inherited permissions make isolation
difficult to achieve and harder to verify.

## Why Namespace Scoping Matters

In a multi-tenant cluster, the blast radius of a compromised service account
depends on its RBAC scope:

- **Namespace-scoped** (Role + RoleBinding): the service account can only access
  resources in its own namespace. Compromise is contained.
- **Cluster-scoped** (ClusterRole + ClusterRoleBinding): the service account can
  access resources in every namespace. Compromise affects all tenants.

The default approach in many clusters is to use ClusterRoleBindings for
convenience — binding a broad ClusterRole to a service account so it works
across namespaces. This is the opposite of multi-tenant isolation.

## Generating Per-Namespace Policies

Audicia's strategy engine supports namespace-strict policy generation via the
`scopeMode` knob:

```yaml
spec:
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
```

With `NamespaceStrict` (the default), Audicia generates:

- A separate **Role + RoleBinding** pair for each namespace where the subject
  was observed
- No ClusterRoles or ClusterRoleBindings for namespace-scoped resources
- ClusterRole + ClusterRoleBinding only for genuinely cluster-scoped activity
  (like reading nodes)

### Example: Service Account Accessing Two Namespaces

If `backend` in `team-a` reads pods in `team-a` and reads configmaps in a shared
`infra` namespace, Audicia generates two separate Roles:

```yaml
# Role in team-a namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: suggested-backend-role
  namespace: team-a
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
# Role in infra namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: suggested-backend-role
  namespace: infra
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
```

Each Role is scoped to its namespace. The service account gets exactly the
permissions it needs in each namespace and nothing more.

## Filtering by Tenant

In a multi-tenant cluster, you may want Audicia to generate reports for only
specific teams. The filter chain supports namespace-based allowlisting:

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Allow
      namespacePattern: "^team-a-.*"
    - action: Deny
      userPattern: ".*"
```

This configuration only processes events from namespaces matching `team-a-*`.
Events from other tenants are dropped before they enter the pipeline.

### Per-Tenant AudiciaSource

For fine-grained control, create a separate `AudiciaSource` per tenant:

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: team-a-audit
  namespace: team-a
spec:
  sourceType: K8sAuditLog
  location:
    path: /var/log/kube-audit.log
  filters:
    - action: Allow
      namespacePattern: "^team-a$"
    - action: Deny
      userPattern: ".*"
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
```

Each `AudiciaSource` produces its own set of `AudiciaPolicyReport` CRDs, scoped
to its configured namespace filter.

## Cross-Namespace Access Patterns

Multi-tenant clusters often have legitimate cross-namespace access:

- A monitoring agent in `monitoring` that reads pods across all namespaces
- A CI/CD pipeline in `ci` that creates deployments in multiple team namespaces
- A shared service in `infra` that reads configmaps in several namespaces

With `NamespaceStrict`, Audicia generates per-namespace Roles for each namespace
the subject accesses. The monitoring agent gets a Role in each namespace it
reads from — not a single ClusterRole that grants access to everything.

This is the correct trade-off for multi-tenant isolation: more Roles, but each
one is scoped and auditable.

## Compliance Per Tenant

Each `AudiciaPolicyReport` includes a compliance score. In a multi-tenant
cluster, you can view compliance per namespace:

```bash
kubectl get apreport -n team-a -o wide
```

```
NAME              SUBJECT    KIND             COMPLIANCE   SCORE   SENSITIVE   AGE
report-backend    backend    ServiceAccount   Green        88      false       2d
report-worker     worker     ServiceAccount   Yellow       62      false       2d
```

This gives each tenant visibility into their own RBAC health without exposing
data from other tenants. Use namespace-scoped RBAC on the reports themselves to
enforce tenant isolation of compliance data.

## Further Reading

- **[Audicia Strategy Knobs Explained](/blog/audicia-strategy-knobs-explained)**
  — deep dive into scopeMode, verbMerge, and wildcards
- **[Using Audicia with GitOps](/blog/audicia-gitops-argocd-flux)** — managing
  per-namespace policies in ArgoCD and Flux
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia in your multi-tenant cluster
