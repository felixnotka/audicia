---
title: "What Can You Do with Kubernetes Audit Logs? 5 Use Cases Beyond Storage"
seo_title: "What Can You Do with Kubernetes Audit Logs? 5 Use Cases Beyond Storage"
published_at: 2026-04-07T08:00:00.000Z
snippet: "Kubernetes audit logs are more than a compliance checkbox. Here are five practical use cases: RBAC generation, security investigation, anomaly detection, drift detection, and compliance evidence."
description: "Five practical use cases for Kubernetes audit logs: RBAC generation, security investigation, anomaly detection, drift detection, and compliance evidence."
---

## More Than a Checkbox

Most teams enable Kubernetes audit logging because a compliance framework
requires it. The logs are written to a file or shipped to a SIEM, and nobody
looks at them until something goes wrong.

This is a waste. Audit logs are the only data source that records every API
request to your cluster — who made it, what resource was targeted, which verb
was used, and whether it succeeded. This data enables five practical use cases
that go beyond storage.

## Use Case 1: RBAC Policy Generation

**The problem:** Writing correct RBAC by hand is impractical. The combinatorial
surface (API groups × resources × subresources × verbs × namespaces) is too
large, and getting it wrong either blocks workloads or creates security
vulnerabilities.

**How audit logs help:** Every API call your workload makes is recorded with the
exact verb, resource, API group, namespace, and subresource. This is precisely
the information needed to construct a minimal RBAC policy.

**In practice:** An RBAC generator reads the audit log, extracts the permissions
each subject uses, and produces ready-to-apply Roles and ClusterRoles. Instead
of guessing what a service account needs, you observe what it actually does.

```
Audit Log → Extract per-subject API calls → Generate Role YAML
```

For a full walkthrough, see
[Generating Least-Privilege RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs).

## Use Case 2: Security Investigation

**The problem:** When a security incident occurs — a compromised pod, an
unauthorized deletion, a suspicious RBAC change — the first question is always
"who did this and when?"

**How audit logs help:** Each audit event records:

- **Who:** `user.username` and `user.groups`
- **What:** `verb`, `objectRef.resource`, `objectRef.subresource`
- **Where:** `objectRef.namespace`
- **When:** `requestReceivedTimestamp`
- **Result:** `responseStatus.code`

**In practice:** Investigating a deleted namespace:

```bash
jq 'select(.verb == "delete" and
           .objectRef.resource == "namespaces" and
           .objectRef.name == "production")
  | {user: .user.username, time: .requestReceivedTimestamp,
     status: .responseStatus.code}' \
  /var/log/kubernetes/audit/audit.log
```

This returns the exact user, timestamp, and status code for the deletion. No
guesswork, no asking around.

## Use Case 3: Anomaly Detection

**The problem:** Compromised service accounts often exhibit unusual access
patterns — reading secrets they have never read before, accessing namespaces
they normally do not touch, or using verbs they have never used.

**How audit logs help:** Audit logs establish a behavioral baseline for each
subject. By comparing current behavior against historical patterns, you can
detect anomalies:

- A service account that normally reads pods in `my-team` suddenly reads secrets
  in `kube-system`
- A CI pipeline account that normally creates deployments starts listing
  clusterrolebindings
- A human user who normally works in `staging` starts making changes in
  `production`

**In practice:** Build per-subject access profiles from audit data. Alert when a
subject accesses a resource or namespace it has never accessed before. The
Audicia pipeline naturally produces these profiles as observed rule sets — a
foundation for anomaly detection.

## Use Case 4: Drift Detection

**The problem:** RBAC permissions are set once and rarely updated. As workloads
evolve, the gap between what is granted and what is used grows. This is
permission drift — excess privileges that create unnecessary attack surface.

**How audit logs help:** By comparing granted RBAC (from Roles and Bindings)
against observed API access (from audit logs), you can quantify drift for every
subject:

```
Drift = Granted Permissions − Used Permissions
```

**In practice:** A service account has 8 effective permission rules. Audit logs
show it uses only 2 of them. The other 6 are drift — excess privileges that
should be removed. If any of the excess permissions include sensitive resources
like secrets, the remediation is urgent.

For a detailed guide on detecting and acting on drift, see
[Kubernetes RBAC Drift Detection](/blog/kubernetes-rbac-drift-detection).

## Use Case 5: Compliance Evidence

**The problem:** SOC 2, ISO 27001, PCI DSS, and NIST all require evidence that
access controls follow least privilege. Most teams produce this evidence
manually — screenshots, spreadsheets, point-in-time snapshots that are stale by
the time they reach an auditor.

**How audit logs help:** Audit logs provide continuous, machine-readable
evidence of actual access patterns. Combined with RBAC state, they demonstrate:

- Which subjects accessed which resources (observed usage)
- Which permissions are granted vs. which are used (compliance score)
- When the last review occurred (evaluation timestamps)

**In practice:** A compliance report generated from audit log data provides
auditors with a quantitative answer to "do your service accounts follow least
privilege?" — not a screenshot, but a scored evaluation for every subject in the
cluster.

See
[Kubernetes RBAC Compliance Evidence](/blog/kubernetes-rbac-compliance-evidence)
for mapping these outputs to specific compliance controls.

## What You Need to Get Started

All five use cases require one prerequisite: **audit logging must be enabled.**

If your cluster does not have audit logging configured, none of this data
exists. The first step is always to turn it on.

For step-by-step instructions on enabling audit logging across kubeadm, kind,
EKS, GKE, and AKS, see
[How to Enable Kubernetes Audit Logging](/blog/kubernetes-audit-logging-guide).

## Further Reading

- **[How to Enable Audit Logging](/blog/kubernetes-audit-logging-guide)** —
  step-by-step platform guide
- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  full before/after walkthrough
- **[Pipeline Architecture](/docs/concepts/pipeline)** — how Audicia processes
  audit events through six stages
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start using your audit logs
