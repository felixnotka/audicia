---
title: "Kubernetes Audit Policy Design: What to Log Without Drowning in Data"
seo_title: "Kubernetes Audit Policy Design: What to Log Without Drowning in Data"
published_at: 2026-04-03T08:00:00.000Z
snippet: "How to design a Kubernetes audit policy that captures everything needed for RBAC analysis while filtering the noise that inflates log volume by 5–10×."
description: "Design a Kubernetes audit policy that captures RBAC-relevant data while filtering noise. Covers audit levels, omitStages, namespace scoping, and volume tuning."
---

## The Design Problem

The default approach to Kubernetes audit logging is simple: log everything at
`Metadata` level. This works — but it generates enormous volume. A production
cluster with 50 nodes can produce 1–10 GB of audit logs per day, and much of
that data is noise that adds no value for RBAC analysis, security investigation,
or compliance evidence.

Good audit policy design captures the data you need while filtering the data you
don't. The goal is a policy that is comprehensive enough for security and RBAC
analysis while lean enough to run sustainably in production.

## Audit Policy Basics

A Kubernetes audit policy is an ordered list of rules. Each rule specifies:

- **level** — what to record (`None`, `Metadata`, `Request`, `RequestResponse`)
- **matching criteria** — which events the rule applies to (users, resources,
  namespaces, verbs, nonResourceURLs)
- **omitStages** — which request stages to skip

Rules are evaluated in order. The first matching rule determines the audit level
for that event.

## Start With the Right Level

| Level             | What Is Recorded                     | Daily Volume (50 nodes) |
| ----------------- | ------------------------------------ | ----------------------- |
| `None`            | Nothing                              | 0                       |
| `Metadata`        | Who, what, when, where, status code  | 1–10 GB                 |
| `Request`         | Metadata + full request body         | 3–30 GB                 |
| `RequestResponse` | Metadata + request + response bodies | 10–100 GB               |

**`Metadata` is the right default for most clusters.** It captures everything
needed for RBAC generation (subject, verb, resource, namespace, subresource, API
group, status code) without the storage cost of request and response bodies.

`RequestResponse` is useful for forensic investigation (seeing exactly what was
sent or returned), but it dramatically increases volume and may capture
sensitive data like Secret values in response bodies.

## The High-Noise Sources

Three categories of events dominate audit log volume without contributing to
RBAC analysis:

### Health and Readiness Endpoints

The kubelet, kube-proxy, and various controllers poll `/healthz`, `/livez`, and
`/readyz` constantly. These are non-resource URL requests with no RBAC value:

```yaml
- level: None
  nonResourceURLs:
    - /healthz*
    - /livez*
    - /readyz*
```

**Volume reduction:** ~10–15% of total events.

### API Server Self-Traffic

The kube-apiserver makes internal calls to itself (leader election, discovery,
etc.). These appear as `system:apiserver` in audit logs:

```yaml
- level: None
  users:
    - system:apiserver
```

**Volume reduction:** ~25–30% of total events.

### Event Objects

Kubernetes `Event` resources (`events.v1`) are read-heavy and high-volume.
Controllers and kubectl constantly list and watch events. These reads have no
RBAC value for workload policy generation:

```yaml
- level: None
  verbs: ["get", "list", "watch"]
  resources:
    - group: ""
      resources: ["events"]
```

**Volume reduction:** ~5–10% of total events, depending on cluster activity.

### The omitStages Directive

Every API call generates two audit events: `RequestReceived` (when the request
arrives) and `ResponseComplete` (when the response is sent). The
`RequestReceived` stage contains no status code and no information that the
`ResponseComplete` stage doesn't already include.

```yaml
omitStages:
  - RequestReceived
```

**Volume reduction:** 50% — this single directive halves the number of events
per API call.

## The Recommended Policy

Combining all the filters above:

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  - level: None
    nonResourceURLs: ["/healthz*", "/livez*", "/readyz*"]

  - level: None
    users: ["system:apiserver"]

  - level: None
    verbs: ["get", "list", "watch"]
    resources:
      - group: ""
        resources: ["events"]

  - level: Metadata
    omitStages: ["RequestReceived"]
```

This policy reduces volume by 40–60% compared to logging everything at
`Metadata` level, while retaining all the data needed for RBAC generation,
security investigation, and compliance evidence.

## Advanced Patterns

### Namespace-Scoped Logging

If you only need RBAC analysis for specific namespaces:

```yaml
rules:
  - level: None
    nonResourceURLs: ["/healthz*", "/livez*", "/readyz*"]
  - level: None
    users: ["system:apiserver"]

  - level: Metadata
    namespaces: ["production", "staging"]
    omitStages: ["RequestReceived"]

  - level: None
```

The final `level: None` rule acts as a catch-all, dropping everything not in the
target namespaces.

### Mixed Levels for Forensics

Log sensitive resources at `Request` level while keeping everything else at
`Metadata`:

```yaml
rules:
  - level: None
    nonResourceURLs: ["/healthz*", "/livez*", "/readyz*"]
  - level: None
    users: ["system:apiserver"]

  - level: Request
    resources:
      - group: ""
        resources: ["secrets"]
      - group: "rbac.authorization.k8s.io"
        resources: [
          "roles",
          "rolebindings",
          "clusterroles",
          "clusterrolebindings",
        ]
    omitStages: ["RequestReceived"]

  - level: Metadata
    omitStages: ["RequestReceived"]
```

This captures request bodies for secrets and RBAC changes (useful for forensic
investigation of who created what binding) while keeping the rest at `Metadata`.

### Excluding Noisy Controllers

If a specific controller generates excessive audit volume:

```yaml
rules:
  - level: None
    users: ["system:serviceaccount:kube-system:replicaset-controller"]
    verbs: ["get", "list", "watch"]
```

Be careful with controller exclusions — they prevent RBAC analysis for that
controller. Only exclude controllers whose RBAC you do not need to manage.

## Audit Policy vs. Audicia Filters

Both the audit policy and Audicia's filter chain control what gets processed,
but at different layers:

| Layer              | Controls                   | When                     |
| ------------------ | -------------------------- | ------------------------ |
| **Audit policy**   | What the apiserver records | At event generation time |
| **Audicia filter** | What Audicia processes     | At event ingestion time  |

Use the audit policy to reduce log volume at the source (fewer bytes written to
disk or sent over the network). Use Audicia's filters for fine-grained control
over which subjects and namespaces generate reports.

The two layers are complementary. An event that is dropped by the audit policy
is never written and never reaches Audicia. An event that passes the audit
policy but is dropped by Audicia's filter is written to the log but not included
in policy reports.

## Further Reading

- **[How to Enable Kubernetes Audit Logging](/blog/kubernetes-audit-logging-guide)**
  — platform-specific setup for kubeadm, kind, EKS, GKE, AKS
- **[Filter Recipes](/blog/kubernetes-audit-log-filter-recipes)** — Audicia
  filter chain patterns for common scenarios
- **[Audit Policy Guide](/docs/guides/audit-policy)** — Audicia's documentation
  on configuring the audit policy
