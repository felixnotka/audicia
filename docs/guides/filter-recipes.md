# Filter Recipes

Audicia's filter chain controls which audit events are processed into policy reports. Filters are evaluated as an
ordered allow/deny chain — **first match wins**. If no rule matches, the event is **allowed** by default.

## How Filters Work

Each filter rule has:

- **`action`**: `Allow` or `Deny`
- **`userPattern`**: Regex matched against `event.User.Username` (optional)
- **`namespacePattern`**: Regex matched against `event.ObjectRef.Namespace` (optional)

Rules are evaluated top-to-bottom. The first matching rule determines the outcome.

Additionally, `ignoreSystemUsers: true` (the default) automatically drops all `system:*` users except service
accounts (`system:serviceaccount:*`).

## Recipe: Block System Noise Only

The simplest production filter. Blocks noisy system components while allowing everything else:

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

**Best for:** Development clusters, initial exploration, understanding what Audicia sees.

## Recipe: Namespace Allowlist

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

**What flows through:** Only events in namespaces matching `production-*` or `staging-*`.

**Best for:** Multi-tenant clusters where you want reports for specific teams.

## Recipe: Single-Team Focus

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

**Best for:** Per-team rollout, pilot deployments.

## Recipe: Production Hardened

The filter chain used in the hardened webhook example
([Hardened Example](../examples/audicia-source-hardened.md)):

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

**What flows through:** All non-system users in non-system namespaces. Service accounts in application namespaces
are included.

**Best for:** Production clusters where you want comprehensive coverage without system noise.

## Recipe: Exclude Specific Service Accounts

If a particular service account generates too much noise (e.g., a monitoring agent):

```yaml
spec:
  filters:
    - action: Deny
      userPattern: "^system:serviceaccount:monitoring:prometheus$"
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
```

**Tip:** Put the most specific rules first, then the broader system filters.

## Filter vs. Audit Policy

Both the Kubernetes audit policy and Audicia's filters control what gets processed, but at different layers:

| Layer               | Controls                | When                     |
|---------------------|-------------------------|--------------------------|
| **Audit policy**    | What the apiserver logs | At event generation time |
| **Audicia filters** | What Audicia processes  | At event ingestion time  |

Use the audit policy to reduce log volume at the source. Use Audicia's filters for fine-grained control over which
subjects and namespaces generate reports.

## Debugging Filters

If reports aren't appearing for expected subjects:

1. **Check operator logs** for filtered event counts:
   ```bash
   kubectl logs -n audicia-system -l app.kubernetes.io/name=audicia-operator | grep filtered
   ```

2. **Temporarily remove all filters** to confirm events flow:
   ```yaml
   spec:
     ignoreSystemUsers: false
     filters: []
   ```

3. **Add filters back one at a time** to identify which rule is too broad.
