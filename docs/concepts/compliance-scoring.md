# Compliance Scoring

Audicia's compliance scoring compares what a subject actually does (observed
usage from audit logs) against what it's allowed to do (effective RBAC
permissions). This reveals overprivileged subjects and drives least-privilege
remediation.

## How It Works

### Step 1: Resolve Effective Permissions

For each subject (ServiceAccount, User, or Group), Audicia:

1. Lists all **ClusterRoleBindings** — filters by subject match — resolves each
   referenced ClusterRole into PolicyRules with cluster-wide scope.
2. Lists all **RoleBindings** — filters by subject match — resolves each
   referenced Role or ClusterRole into PolicyRules scoped to the RoleBinding's
   namespace.
3. Returns a flat list of `ScopedRule` entries (PolicyRule + Namespace).

This uses the controller-runtime informer cache, so it's fast and doesn't hit
the API server directly.

### Step 2: Diff Against Observed Usage

The diff engine is a pure function:
`Evaluate(observed, effective) → ComplianceReport`.

For each effective rule, it checks: **was this permission exercised by any
observed action?**

- **Used**: The effective rule was exercised at least once.
- **Excess**: The effective rule was never observed in use — this is
  overprivilege.
- **Uncovered**: An observed action that isn't covered by any effective rule
  (may indicate aggregated ClusterRoles or other mechanisms the resolver doesn't
  handle).

### Step 3: Calculate Score

```
Score = usedEffective / totalEffective × 100
```

Both numerator and denominator use the same unit (effective rules) to avoid
inflation when a single broad rule covers many observed actions.

## Severity Thresholds

| Score  | Severity | Meaning                                                                     |
| ------ | -------- | --------------------------------------------------------------------------- |
| >= 80% | Green    | Tight permissions — most granted access is actually used                    |
| >= 50% | Yellow   | Moderate overprivilege — review excess grants                               |
| < 50%  | Red      | Significant overprivilege — the subject uses less than half its permissions |

## Matching Rules

### Namespace Scoping

- Cluster-wide effective rules (from ClusterRoleBindings) cover observed rules
  in **any** namespace.
- Namespace-scoped effective rules (from RoleBindings) only cover observed rules
  in **their own** namespace.

### Wildcard Handling

- `*` in verbs, resources, or apiGroups matches everything.
- A single effective rule with `resources: ["*"]` covers all observed resource
  types.

### ResourceNames

Effective rules constrained by `resourceNames` are treated as **NOT** covering
general observed actions. This is conservative — audit events don't capture
which specific resource instance was accessed, so we can't confirm the match.

### Non-Resource URLs

Non-resource URLs (e.g., `/metrics`, `/healthz`) are matched separately from
resource rules.

## Sensitive Excess Detection

Excess grants on high-risk resources are flagged in `sensitiveExcess`. These
resources are:

- `secrets`
- `nodes`
- `clusterroles`, `clusterrolebindings`, `roles`, `rolebindings`
- `mutatingwebhookconfigurations`, `validatingwebhookconfigurations`
- `certificatesigningrequests`
- `tokenreviews`, `subjectaccessreviews`, `selfsubjectaccessreviews`,
  `selfsubjectrulesreviews`
- `persistentvolumes`, `storageclasses`
- `customresourcedefinitions`
- `serviceaccounts/token`

## Example

A ServiceAccount `backend` in namespace `my-team` has a broad Role granting
access to pods, configmaps, secrets, services, and deployments. But audit logs
show it only accesses pods and configmaps.

```
$ kubectl get apreport -n my-team -o wide
NAME              SUBJECT    KIND             COMPLIANCE   SCORE   AGE   NEEDED   EXCESS   UNGRANTED   SENSITIVE   AUDIT EVENTS
report-backend    backend    ServiceAccount   Red          40      1h    2        3        0           true        150
```

```yaml
status:
  compliance:
    score: 40
    severity: Red
    usedCount: 2
    excessCount: 3
    uncoveredCount: 0
    sensitiveExcess:
      - secrets
    lastEvaluatedTime: "2026-02-20T12:00:00Z"
```

This tells you: the SA uses only 2 of its 5 granted permissions, and has unused
access to secrets.

## Edge Cases

| Scenario                      | Effective rules | Observed rules | Result                                                                                            |
| ----------------------------- | --------------- | -------------- | ------------------------------------------------------------------------------------------------- |
| No RBAC + no observations     | 0               | 0              | Score 100, Green — nothing to do                                                                  |
| No RBAC + observations exist  | 0               | > 0            | Compliance is `nil` — cannot be evaluated. Report still gets observed rules and suggested policy. |
| RBAC exists + no observations | > 0             | 0              | Score 0, Red — all grants are excess                                                              |

## Graceful Degradation

If the RBAC resolver fails (e.g., the operator doesn't have RBAC read
permissions, or the API server is unreachable), compliance is `nil` — the report
still gets observed rules and suggested policy. The operator logs the error and
continues normally.

## Known Limitations

See [Limitations](../limitations.md) for current compliance scoring limitations.
