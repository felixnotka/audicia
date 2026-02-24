---
title: "Why Your Kubernetes Security Tool Shouldn't Need cluster-admin"
seo_title: "Why Your Kubernetes Security Tool Shouldn't Need cluster-admin"
published_at: 2026-05-01T08:00:00.000Z
snippet: "Security tools that require cluster-admin create the same overprivilege problem they claim to solve. Here's why minimal permissions matter for security tooling."
description: "Security tools that require cluster-admin create the overprivilege problem they claim to solve. Why minimal operator permissions matter and how Audicia achieves it."
---

## The Irony

Many Kubernetes security tools require `cluster-admin` to install. The very
tools that promise to improve your security posture start by requesting the
broadest possible permissions.

This creates a paradox: the tool that is supposed to find overprivileged service
accounts is itself overprivileged.

## What cluster-admin Actually Grants

`cluster-admin` is a built-in ClusterRole that grants every verb on every
resource in every namespace. This includes:

- **Secrets** in all namespaces — including cloud credentials, TLS keys, and API
  tokens
- **RBAC modification** — the ability to create new ClusterRoleBindings,
  effectively granting itself or others any additional permissions
- **Node access** — direct manipulation of cluster nodes
- **CRD management** — ability to create, modify, or delete any custom resource
  definition
- **Impersonation** — the ability to act as any user, group, or service account

A compromised security tool with `cluster-admin` gives an attacker complete
control of the cluster. The blast radius is total.

## Why Security Tools Request It

Three common reasons:

### Convenience

`cluster-admin` works for everything. It eliminates RBAC troubleshooting during
installation. The tool author does not need to document which specific
permissions are required because everything is already granted.

### Discovery

Some tools discover what resources exist in the cluster at runtime. They scan
CRDs, list resources across all API groups, or watch objects in namespaces they
do not know about at install time. Broad access simplifies this.

### Lazy Trust Model

Some tools assume they are trusted because they are security tools. The logic
is: "this tool needs to see everything to protect everything, so it needs
cluster-admin." This conflates visibility with privilege.

## The Better Approach

A security tool should request only the permissions it needs to function. Not
more, not less.

For an RBAC analysis tool like Audicia, the required permissions are narrow:

| Permission                   | Scope      | Why                                          |
| ---------------------------- | ---------- | -------------------------------------------- |
| get/list/watch AudiciaSource | Namespaced | Read input configuration                     |
| CRUD AudiciaPolicyReport     | Namespaced | Write output reports                         |
| update AudiciaSource/status  | Namespaced | Persist checkpoint state                     |
| get/list/watch RBAC objects  | Cluster    | Resolve effective permissions for compliance |
| create/patch events          | Namespaced | Emit Kubernetes events                       |
| CRUD leases                  | Namespaced | Leader election                              |

Notice what is **not** in this list:

- **No secrets access** — the operator never reads secrets
- **No impersonate permissions** — the operator never impersonates other users
- **No write access to Roles or RoleBindings** — the operator generates
  suggestions but never applies them
- **No node access** — the operator does not interact with nodes
- **No CRD management** — the operator reads its own CRDs but does not create or
  modify them at runtime

## The No-Auto-Apply Principle

Audicia never applies generated policies automatically. This is a deliberate
security design choice.

A security tool that both generates and applies RBAC policies is a privilege
escalation vector. If the tool is compromised, an attacker can use it to create
arbitrary RBAC bindings — effectively granting themselves any permissions they
want.

By separating generation (Audicia) from application (human review or GitOps
pipeline), the tool cannot be weaponized for privilege escalation.

## Evaluating Security Tool Permissions

When evaluating any Kubernetes security tool, check its RBAC requirements:

1. **Read the Helm chart** — look at the ClusterRole or Role defined in the
   chart templates
2. **Check for `cluster-admin`** — any reference to `cluster-admin` in a
   ClusterRoleBinding is a red flag
3. **Look for wildcard resources** — `resources: ["*"]` means the tool requests
   access to everything
4. **Check for secrets access** — does the tool need `get secrets`? Why?
5. **Check for RBAC write access** — can the tool create Roles or Bindings? Does
   it need to?

## The Trust Boundary

A security tool operates within your cluster's trust boundary. Its permissions
determine the blast radius if the tool itself is compromised:

| Tool Permission Level   | Blast Radius of Compromise                     |
| ----------------------- | ---------------------------------------------- |
| `cluster-admin`         | Total cluster compromise                       |
| Broad ClusterRole       | Access to many resource types across cluster   |
| Namespaced Role         | Limited to specific resources in one namespace |
| Minimal namespaced RBAC | Limited to specific resources, no escalation   |

The goal is to keep security tools at the bottom of this table — minimal
permissions, minimal blast radius.

## Further Reading

- **[Kubernetes RBAC Tools Compared](/blog/kubernetes-rbac-tools-compared)** —
  comparing scanners, enforcers, and generators
- **[The Difference Between Scanning and Generation](/blog/rbac-scanning-vs-generation)**
  — why different tools need different permissions
- **[Security Model Documentation](/docs/concepts/security-model)** — Audicia's
  full threat model and security design
