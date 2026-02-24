---
title: "Introducing Audicia: Stop Writing RBAC by Hand"
seo_title: "Stop Writing RBAC by Hand — Introducing Audicia"
published_at: 2026-02-20T08:00:00.000Z
snippet: "Audicia is a Kubernetes Operator that watches your audit logs and generates least-privilege RBAC policies. Here's why I built it and how it works."
description: "Audicia is a Kubernetes Operator that generates least-privilege RBAC policies from audit logs. Open source, Apache 2.0, never auto-applies."
---

## The Problem

Every Kubernetes cluster has an RBAC problem. Service accounts accumulate
permissions over time. Someone binds `cluster-admin` to unblock a 403, and
nobody reverts it. Auditors ask for least-privilege evidence, and the team
scrambles to produce spreadsheets that are stale before the audit is over.

If you've been there, you know the pain. Audicia exists because I got tired of
it.

## What Audicia Does

Audicia is a Kubernetes Operator that:

1. **Ingests audit logs** — either by tailing files on the control plane or
   receiving real-time events via webhook
2. **Normalizes and filters events** — parsing ServiceAccount identities,
   handling subresources, migrating deprecated API groups, and dropping system
   noise
3. **Generates policy reports** — an `AudiciaPolicyReport` CRD per subject,
   containing observed rules, ready-to-apply RBAC YAML, and a compliance score

The compliance score compares what each subject _actually uses_ against what
it's _allowed to use_. Red means significant overprivilege. Green means tight
permissions. Learn more about how this works in
[Understanding Compliance Scores](/blog/understanding-compliance-scores).

## How It Works

```bash
# Install
helm install audicia ./deploy/helm -n audicia-system --create-namespace

# Point at your audit log
kubectl apply -f audicia-source.yaml

# Check reports
kubectl get apreport --all-namespaces
```

That's it. Three commands to start generating least-privilege RBAC policies from
real cluster behavior.

## What Makes Audicia Different

- **Continuous** — runs as an operator, not a one-shot CLI tool
- **Stateful** — checkpoint/resume means no data loss on restarts
- **CRD-native** — output is a Kubernetes resource, ready for GitOps
- **Compliance scoring** — built-in Red/Yellow/Green scoring with sensitive
  excess detection
- **Never auto-applies** — Audicia generates recommendations, humans or reviewed
  pipelines apply them

## Open Source

Audicia is Apache 2.0 licensed. No paid tier, no enterprise edition, no feature
gating. The full operator, both ingestion modes, compliance scoring, and the
complete Helm chart ship free.

I believe security tools should be transparent and auditable. You can read every
line of code.

## Get Started

Check out the [getting started guide](/docs/getting-started/introduction) to
install Audicia and generate your first policy reports. If you want to
understand the internals, read
[how the pipeline works](/blog/how-audicia-processes-audit-logs).

View the source on [GitHub](https://github.com/felixnotka/audicia).
