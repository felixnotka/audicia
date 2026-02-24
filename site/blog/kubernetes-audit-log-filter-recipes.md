---
title: "Filter Recipes: Cutting 90% of Kubernetes Audit Log Noise"
seo_title: "Filter Recipes: Cutting 90% of Kubernetes Audit Log Noise"
published_at: 2026-04-24T08:00:00.000Z
snippet: "Practical filter recipes for Audicia: block system noise, allowlist specific namespaces, exclude monitoring agents, and focus on the events that matter."
description: "Practical Audicia filter recipes for Kubernetes audit logs: block system noise, allowlist namespaces, exclude monitoring agents, and reduce processing volume."
---

## Two Layers of Filtering

Kubernetes audit logs are noisy. System controllers, node heartbeats, health
checks, and internal API server traffic generate thousands of events per minute
that have no value for RBAC analysis.

Audicia provides two filtering layers:

1. **Kubernetes audit policy** — controls what the apiserver records (at the
   source)
2. **Audicia filter chain** — controls which recorded events Audicia processes
   into policy reports

The audit policy reduces log volume on disk. Audicia's filter chain reduces what
enters the RBAC generation pipeline. Both layers work together.

For audit policy design, see
[Kubernetes Audit Policy Design](/blog/kubernetes-audit-policy-design).

This post focuses on Audicia's filter chain recipes.

## How the Filter Chain Works

Audicia evaluates filter rules top-to-bottom. **First match wins.** If no rule
matches, the event is **allowed** by default.

Each rule has:

- **`action`** — `Allow` or `Deny`
- **`userPattern`** — regex matched against the event's `user.username`
- **`namespacePattern`** — regex matched against the event's
  `objectRef.namespace`

Additionally, `ignoreSystemUsers: true` (the default) automatically drops all
`system:*` users except service accounts (`system:serviceaccount:*`).

## Recipe 1: Block System Noise

The simplest production filter. Blocks noisy system components while allowing
everything else:

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Deny
      userPattern: "^system:apiserver$"
```

**What flows through:** All service accounts, all human users, all namespaces.

**Volume reduction:** 50–70% of events are dropped.

**Best for:** Development clusters, initial exploration.

## Recipe 2: Namespace Allowlist

Only process events from specific namespaces:

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Allow
      namespacePattern: "^(production|staging)-.*"
    - action: Deny
      userPattern: ".*"
```

**What flows through:** Only events in namespaces matching `production-*` or
`staging-*`.

**Volume reduction:** 80–95% depending on how many namespaces exist.

**Best for:** Multi-tenant clusters where you want reports for specific teams.

The key is the final `Deny` rule with `userPattern: ".*"` — this catches
everything that didn't match the `Allow` rule and drops it.

## Recipe 3: Single-Namespace Focus

Generate reports for only one team's namespace:

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Allow
      namespacePattern: "^my-team$"
    - action: Deny
      userPattern: ".*"
```

**What flows through:** Only events in the `my-team` namespace.

**Best for:** Pilot deployments, per-team rollout.

## Recipe 4: Exclude Noisy Service Accounts

If a specific service account generates too much noise (monitoring agents, log
collectors):

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:serviceaccount:monitoring:prometheus$"
    - action: Deny
      userPattern: "^system:serviceaccount:logging:fluentbit$"
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
```

**Rule:** Put the most specific rules first, then broader system filters.

**Best for:** Clusters where monitoring or logging agents generate
disproportionate audit volume.

## Recipe 5: Production Hardened

Block system noise and exclude system namespaces:

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Deny
      userPattern: "^system:apiserver$"
    - action: Deny
      namespacePattern: "^kube-"
```

**What flows through:** All non-system users in non-system namespaces.

**Best for:** Production clusters where you want comprehensive coverage without
system namespace noise.

## Recipe 6: Allowlist + Exclude

Combine namespace allowlisting with specific exclusions:

```yaml
spec:
  ignoreSystemUsers: true
  filters:
    # Exclude specific noisy accounts first
    - action: Deny
      userPattern: "^system:serviceaccount:production:metrics-collector$"
    # Allow only production namespaces
    - action: Allow
      namespacePattern: "^production$"
    - action: Allow
      namespacePattern: "^production-.*"
    # Drop everything else
    - action: Deny
      userPattern: ".*"
```

**Best for:** Production-focused analysis with specific exclusions.

## Debugging Filters

If reports are not appearing for expected subjects:

### Check Operator Logs

```bash
kubectl logs -n audicia-system \
  -l app.kubernetes.io/name=audicia-operator | grep filtered
```

The operator logs the number of events filtered per cycle.

### Temporarily Remove All Filters

```yaml
spec:
  ignoreSystemUsers: false
  filters: []
```

This disables all filtering. If reports appear, the issue is in the filter
chain. Add rules back one at a time to identify which one is too broad.

### Verify the Username Format

Kubernetes audit events use the full username string. For ServiceAccounts, the
format is:

```
system:serviceaccount:<namespace>:<name>
```

Make sure your regex patterns match this format exactly:

```yaml
# Correct: match the full SA username
- action: Deny
  userPattern: "^system:serviceaccount:monitoring:prometheus$"

# Wrong: this won't match a SA username
- action: Deny
  userPattern: "^prometheus$"
```

## Filter Chain vs. Audit Policy

| Layer           | Controls                | When                |
| --------------- | ----------------------- | ------------------- |
| Audit policy    | What the apiserver logs | At event generation |
| Audicia filters | What Audicia processes  | At event ingestion  |

Use the audit policy for coarse volume reduction at the source. Use Audicia's
filters for fine-grained control without losing the underlying audit trail.

## Further Reading

- **[Kubernetes Audit Policy Design](/blog/kubernetes-audit-policy-design)** —
  designing the apiserver audit policy
- **[How to Enable Audit Logging](/blog/kubernetes-audit-logging-guide)** —
  platform-specific setup instructions
- **[Filter Recipes Guide](/docs/guides/filter-recipes)** — the full filter
  recipes documentation
