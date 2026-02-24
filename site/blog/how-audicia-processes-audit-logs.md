---
title: "How Audicia Turns Audit Logs into RBAC Policies"
seo_title: "How Audicia's Pipeline Turns 10,000 Events into 50 Rules"
published_at: 2026-02-21T08:00:00.000Z
snippet: "A look inside Audicia's six-stage pipeline: from raw kube-apiserver audit events to ready-to-apply Roles and ClusterRoles."
description: "How Audicia processes 10,000 raw Kubernetes audit events into 50 compacted RBAC rules per subject through a six-stage pipeline."
---

## From Noise to Signal

A busy Kubernetes cluster can produce thousands of audit events per minute. Most
of them are noise — system components polling endpoints, health checks, watch
streams. Audicia's job is to extract the signal: which subjects actually need
which permissions.

This post walks through the six pipeline stages that make that happen.

## The Pipeline

Every audit event flows through the same path:

**Ingest → Filter → Normalize → Aggregate → Strategy → Report**

Each stage is a separate Go package with a clear contract. Events flow forward;
nothing loops back.

### 1. Ingest

Audicia supports two ingestion modes:

- **File mode** — tails the kube-apiserver audit log file directly from a
  hostPath mount. Includes checkpoint/resume via inode tracking so restarts
  don't lose progress.
- **Webhook mode** — runs a TLS (or mTLS) server that receives audit events
  pushed by the kube-apiserver's webhook backend. Includes rate limiting and
  deduplication.

Both modes produce the same internal event struct. Everything downstream is
agnostic to the source.

### 2. Filter

The filter chain drops events that would produce misleading policies:

- **System subjects** — `system:masters`, kube-system service accounts, and the
  apiserver itself
- **Read-only discovery** — every authenticated user can `GET /api` and `/apis`;
  including these would inflate every policy
- **Configurable rules** — users define additional allow/deny patterns in the
  `AudiciaSource` spec

Filtering happens early to keep memory and CPU low. A well-tuned filter drops
80–90% of raw events.

### 3. Normalize

Raw audit events are messy. The normalizer cleans them up:

- **Subject extraction** — parses `user.username` to extract ServiceAccount
  namespace and name from the `system:serviceaccount:ns:name` format
- **API group migration** — rewrites deprecated groups (`extensions/v1beta1` →
  `apps/v1`) so policies use current API versions
- **Subresource handling** — splits `pods/exec` into the correct resource +
  subresource pair

### 4. Aggregate

The aggregator collapses thousands of normalized events into a compact rule set
per subject:

- Groups by (subject, namespace, apiGroup, resource, subresource)
- Merges verbs into sets
- Tracks `firstSeen`, `lastSeen`, and `count` for each rule

A subject that called `GET /api/v1/namespaces/default/pods` ten thousand times
produces one aggregated rule, not ten thousand.

### 5. Strategy

The strategy engine converts aggregated rules into actual RBAC manifests.
Configuration knobs include:

- **scopeMode** — `Namespace` for Roles, `Cluster` for ClusterRoles, or `Auto`
  to decide per-rule
- **verbMerge** — whether to collapse verbs like `get`, `list`, `watch` into `*`
- **wildcards** — whether to use `*` for resources when a subject touches all
  resources in a group

### 6. Report

The final output is an `AudiciaPolicyReport` — a CRD containing:

- The suggested Role/ClusterRole/Binding YAML, ready to `kubectl apply`
- A [compliance score](/blog/understanding-compliance-scores) comparing observed
  permissions vs. granted permissions
- Metadata like firstSeen/lastSeen timestamps and event counts

## Why a Pipeline?

Each stage has a single responsibility and can be tested in isolation. If you
want to add a new filter rule, you touch `pkg/filter/` and nothing else. If the
normalization logic needs a new API group mapping, that's isolated to
`pkg/normalizer/`.

This also means the pipeline is easy to reason about when debugging: you can log
the output of each stage and see exactly where an event was dropped,
transformed, or aggregated.

## Try It

The [quick start guide](/docs/getting-started/quick-start-file) gets you from
zero to policy reports in under five minutes. For a deeper look at the
architecture, see the [architecture docs](/docs/concepts/architecture).
