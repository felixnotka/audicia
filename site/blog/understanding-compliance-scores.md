---
title: "Understanding Compliance Scores: Red, Yellow, Green"
published_at: 2026-02-22T08:00:00.000Z
snippet: "Audicia scores every service account's RBAC permissions. Here's how the scoring works and what to do about a Red rating."
---

## The Question Auditors Ask

"Can you prove your service accounts follow least privilege?"

Most teams can't answer this without a manual spreadsheet exercise. Audicia
answers it automatically with a compliance score for every subject in your
cluster.

## How Scoring Works

The compliance score is a single number from 0 to 100:

```
score = usedEffective / totalEffective * 100
```

- **usedEffective** — the number of distinct permission rules the subject
  actually exercised (observed in audit logs)
- **totalEffective** — the total number of permission rules granted to the
  subject via Roles and ClusterRoles

A score of 100 means the subject uses every permission it has. A score of 10
means it uses only 10% — the other 90% is excess privilege.

## The Severity Bands

| Score  | Severity | Meaning                                                                   |
| ------ | -------- | ------------------------------------------------------------------------- |
| 76–100 | Green    | Tight permissions. The subject uses most of what it's granted.            |
| 34–75  | Yellow   | Moderate excess. Some cleanup is warranted.                               |
| 0–33   | Red      | Significant overprivilege. The subject has far more access than it needs. |

## Sensitive Excess Detection

Not all excess permissions are equal. Having an unused `list pods` verb is
different from having an unused `create secrets` verb.

Audicia flags specific excess permissions as **sensitive**:

- Secrets access (`get`, `list`, `watch` on secrets)
- Node operations (direct node access is rarely needed by workloads)
- Webhook configurations (mutating/validating webhook access)
- CRD management (creating or modifying custom resource definitions)

When sensitive excess is detected, the report flags it explicitly. This lets
teams prioritize: fix the dangerous excess first, clean up the rest later.

## What To Do About a Red Score

A Red score doesn't mean something is broken — it means there's an opportunity
to tighten permissions. The typical workflow:

1. **Read the report** — `kubectl get apreport <name> -o yaml` shows the full
   suggested policy
2. **Review the diff** — compare the suggested policy against the current
   Roles/Bindings
3. **Apply in a non-production environment first** — verify that the workload
   still functions correctly with tighter permissions
4. **Apply in production** — once validated, apply the suggested Roles and
   Bindings

Audicia never applies policies automatically. The operator generates
recommendations; humans (or reviewed CI pipelines) make the decision.

## Continuous Scoring

Because Audicia runs continuously as an operator, scores update as workload
behavior changes. A deployment that gains new API calls will see its
`usedEffective` count grow, improving its score naturally. A deployment that
loses access to APIs it was using will see its score change too.

This makes compliance scoring a living metric rather than a point-in-time
snapshot.

## See It In Action

```bash
kubectl get apreport --all-namespaces -o wide
```

```
NAMESPACE   NAME             SUBJECT   KIND             COMPLIANCE   SCORE   USED   TOTAL   SENSITIVE   AGE
my-team     report-backend   backend   ServiceAccount   Red          25      2      8       true        15m
my-team     report-worker    worker    ServiceAccount   Green        88      7      8       false       15m
```

The [quick start guide](/docs/getting-started/quick-start-file) takes about five
minutes to set up, and you'll see scores for every service account in your
cluster.
