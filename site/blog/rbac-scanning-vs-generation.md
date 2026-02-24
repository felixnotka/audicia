---
title: "The Difference Between RBAC Scanning and RBAC Generation (And Why It Matters)"
seo_title: "RBAC Scanning vs RBAC Generation: Why the Difference Matters"
published_at: 2026-05-05T08:00:00.000Z
snippet: "Scanning finds problems in existing RBAC. Generation creates correct RBAC from observed data. Understanding the difference is the first step to a complete security stack."
description: "RBAC scanning finds problems in existing policies. RBAC generation creates correct policies from audit log data. Why you need both for complete Kubernetes security."
---

## Two Different Problems

Kubernetes RBAC tools are often discussed as if they solve the same problem.
They don't. There are two fundamentally different problems:

1. **"Is this RBAC policy bad?"** — scanning
2. **"What should this RBAC policy be?"** — generation

A scanner can tell you that a `cluster-admin` binding exists where it shouldn't.
But it cannot tell you what the correct binding should be. A generator can
produce the minimal policy a workload needs. But it cannot check your existing
manifests for known anti-patterns.

These are complementary, not interchangeable.

## What Scanners Do

RBAC scanners analyze static Kubernetes manifests — YAML files, Helm charts,
Kustomize overlays — and flag security issues.

**Input:** Static YAML definitions (what you intend to deploy)

**Output:** A list of findings (warnings, violations, recommendations)

**Examples:** Trivy, KubeAudit, KubeLinter, Polaris

**What they check:**

- ClusterRoleBindings referencing `cluster-admin` in non-system namespaces
- Roles with wildcard (`*`) verbs or resources
- Bindings to service accounts with `secrets` access
- Missing NetworkPolicies alongside overprivileged RBAC
- CIS Benchmark compliance for RBAC configurations

**What they cannot check:**

- Whether a workload actually uses the permissions it is granted
- What permissions a workload needs based on runtime behavior
- How granted permissions compare to observed usage over time

Scanning answers: _"Is this YAML bad?"_

## What Generators Do

RBAC generators observe runtime API access patterns — from Kubernetes audit logs
— and produce the minimal RBAC policy that satisfies those patterns.

**Input:** Kubernetes audit log events (what workloads actually do)

**Output:** Ready-to-apply Role and ClusterRole manifests

**Examples:** Audicia, audit2rbac

**What they produce:**

- Roles with only the verbs and resources actually observed
- Per-namespace scoping based on actual namespace access
- Subresource rules based on actual subresource usage (like `pods/exec`)
- API group assignments based on actual API group usage

**What they cannot do:**

- Check for known anti-patterns in existing YAML
- Detect misconfigurations that do not involve RBAC
- Run in CI without audit log data

Generation answers: _"What should this RBAC be?"_

## The Gap Between Them

Consider this scenario:

A service account has a ClusterRoleBinding to `cluster-admin`. A scanner flags
it — correctly — as overprivileged.

Now what?

The scanner cannot tell you what to replace `cluster-admin` with. It can say
"this is bad" but not "this is what good looks like." The security team knows
they need to tighten the binding, but they do not know what the minimal policy
should be.

This is where a generator fills the gap. It reads the audit logs, observes what
the service account actually does, and produces the exact Role that satisfies
its behavior. The security team replaces `cluster-admin` with a generated policy
that matches reality.

## Why You Need Both

A complete RBAC security stack uses scanning and generation together:

### Without a Scanner

You generate correct policies from audit logs, but you do not catch:

- Manually created overprivileged bindings that bypass the generated policies
- RBAC misconfigurations introduced by Helm charts or third-party operators
- Known anti-patterns that a human reviewer might miss

### Without a Generator

You scan for problems, but you cannot fix them efficiently:

- Remediating flagged findings requires hand-writing RBAC YAML
- Nobody knows what the correct policy should be without audit log data
- Fixes are guesswork that often create new 403 errors

### With Both

```
Audit Logs → Generator → Correct Policies
Correct Policies → Scanner → Validated Manifests
Scanner → CI Pipeline → Prevention
Generator → Operator → Continuous Refinement
```

Generators produce correct policies. Scanners validate them. The combination
closes the loop.

## Adding Enforcement

A third category — enforcers like OPA/Gatekeeper and Kyverno — adds runtime
guardrails:

| Category  | Question                        | When                 |
| --------- | ------------------------------- | -------------------- |
| Scanner   | Is this YAML bad?               | Build time / CI      |
| Generator | What should this RBAC be?       | Runtime / continuous |
| Enforcer  | Should this request be allowed? | Admission time       |

Enforcers prevent overprivileged bindings from being created in the first place.
But they still need policies to enforce — which generators provide.

For a detailed comparison of tools across all three categories, see
[Kubernetes RBAC Tools Compared](/blog/kubernetes-rbac-tools-compared).

## Practical Integration

The ideal workflow:

1. **Generate** least-privilege policies from audit log data (Audicia)
2. **Scan** those policies and all other manifests in CI (Trivy, KubeAudit)
3. **Enforce** that new bindings cannot bypass generated policies
   (OPA/Gatekeeper)
4. **Measure** compliance continuously (Audicia's compliance scoring)

No single tool covers all four steps. Understanding which step each tool
addresses is the foundation for a complete RBAC security stack.

## Further Reading

- **[Kubernetes RBAC Tools Compared](/blog/kubernetes-rbac-tools-compared)** —
  detailed comparison across scanners, enforcers, and generators
- **[audit2rbac vs Audicia](/blog/audit2rbac-vs-audicia)** — comparing two RBAC
  generators
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start generating correct RBAC policies
