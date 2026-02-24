---
title: "Kubernetes RBAC Compliance: Producing Evidence for SOC 2 and ISO 27001"
seo_title: "Kubernetes RBAC Compliance: Producing Evidence for SOC 2 and ISO 27001"
published_at: 2026-03-24T08:00:00.000Z
snippet: "How to produce Kubernetes RBAC compliance evidence for SOC 2, ISO 27001, PCI DSS, and NIST — and why point-in-time screenshots are not enough."
description: "Produce Kubernetes RBAC compliance evidence for SOC 2, ISO 27001, PCI DSS, and NIST. Map AudiciaPolicyReport CRDs directly to audit control requirements."
---

## What Auditors Actually Want

Compliance auditors do not ask for Kubernetes YAML. They ask questions like:

- _"Can you demonstrate that service accounts follow least-privilege?"_
- _"How do you know which subjects have access to sensitive resources?"_
- _"When was the last time you reviewed RBAC permissions?"_
- _"What is your process for detecting and remediating overprivilege?"_

These questions require evidence — not intentions, not policies, but data
showing that access controls match actual usage.

## The Evidence Gap

Most teams produce RBAC compliance evidence manually:

1. An engineer runs `kubectl get clusterrolebindings` and screenshots the output
2. Someone builds a spreadsheet mapping service accounts to their roles
3. The spreadsheet is reviewed once, submitted to the auditor, and immediately
   stale

This approach has three problems:

- **Point-in-time only.** The evidence is valid for the moment it was captured.
  By the next day, new bindings may exist.
- **No usage data.** The spreadsheet shows what is _granted_, not what is
  _used_. An auditor cannot tell whether a service account with `get secrets`
  actually reads secrets.
- **Manual and error-prone.** Gathering evidence across dozens of namespaces and
  hundreds of service accounts takes days and is never complete.

## Mapping Controls to Kubernetes RBAC

### SOC 2 — CC6.1 (Logical Access)

SOC 2 CC6.1 requires that logical access to information assets is restricted
based on the principle of least privilege. For Kubernetes, this means:

- Service accounts should have only the permissions they need
- Excess permissions should be identified and remediated
- Evidence should show the gap between granted and used access

**Audicia mapping:** Each `AudiciaPolicyReport` contains a compliance score
(0–100) comparing observed usage against granted permissions. A score of 90
means the subject uses 90% of its grants — tight permissions. A score of 25
means 75% is excess privilege.

### ISO 27001 — A.8.3 (Access Restriction)

ISO 27001 A.8.3 requires that access to information and application system
functions is restricted in accordance with the access control policy. Evidence
must show:

- Access is limited to what is required for the job function
- Periodic reviews identify excessive access
- Remediation actions are documented

**Audicia mapping:** The compliance score provides the periodic review
automatically. The suggested policy in each report documents the remediation
action. The `lastEvaluatedTime` timestamp proves when the review occurred.

### PCI DSS — Requirement 7 (Restrict Access)

PCI DSS Requirement 7 mandates restricting access to cardholder data to
personnel whose job requires it. In Kubernetes, this extends to service accounts
accessing secrets, configmaps, or any resource that may contain sensitive data.

**Audicia mapping:** The `sensitiveExcess` field flags unused grants on
high-risk resources (secrets, nodes, webhook configurations, CRDs). If a service
account has `get secrets` but never uses it, this appears in the report.

### NIST SP 800-53 — AC-6 (Least Privilege)

NIST AC-6 requires that the information system enforces the most restrictive set
of rights/privileges or accesses needed by users for the performance of
specified tasks. Sub-controls include:

- AC-6(1): Authorize access for security functions
- AC-6(3): Authorize network access
- AC-6(5): Privileged accounts

**Audicia mapping:** The compliance report for each subject shows the exact
permissions used vs. granted, providing direct evidence for AC-6 compliance. The
suggested policy represents the most restrictive set of privileges needed.

## From Manual to Continuous

### Before: Quarterly Audit Sprint

```
Week 1: Engineer runs kubectl commands, builds spreadsheet
Week 2: Security team reviews, identifies issues
Week 3: Engineering team writes new RBAC YAML
Week 4: Changes deployed, spreadsheet updated
```

This cycle takes a month and produces evidence that is stale by the time it
reaches the auditor.

### After: Always Audit-Ready

With Audicia running continuously as an operator, compliance evidence is always
current:

```bash
# Compliance summary for all subjects
kubectl get apreport --all-namespaces -o wide
```

```
NAMESPACE    NAME               SUBJECT      KIND             COMPLIANCE   SCORE   SENSITIVE   AGE
production   report-backend     backend      ServiceAccount   Green        92      false       2d
production   report-worker      worker       ServiceAccount   Yellow       65      false       2d
production   report-scheduler   scheduler    ServiceAccount   Red          30      true        2d
staging      report-backend     backend      ServiceAccount   Green        88      false       2d
```

Each report includes:

- **Compliance score and severity** — quantitative measure of least-privilege
- **Used vs. excess counts** — how many granted permissions are exercised
- **Sensitive excess flags** — unused access to high-risk resources
- **Suggested policy** — the minimal RBAC that satisfies observed usage
- **Last evaluated timestamp** — when the comparison was last computed

### Exporting Evidence

For auditors who need documents rather than `kubectl` output:

```bash
# Export all reports as YAML
kubectl get apreport --all-namespaces -o yaml > rbac-compliance-evidence.yaml

# Export suggested policies for a specific namespace
kubectl get apreport -n production \
  -o jsonpath='{range .items[*]}{.metadata.name}: {.status.compliance.score}% ({.status.compliance.severity})\n{end}'
```

For GitOps workflows, suggested policies can be committed directly to a
repository, creating a versioned audit trail:

```bash
kubectl get apreport report-backend -n production \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  > policies/production/backend-rbac.yaml

git add policies/
git commit -m "rbac: update compliance evidence (automated)"
```

## Sensitive Excess: The High-Priority Findings

Not all excess permissions carry equal risk. Audicia flags unused grants on
sensitive resources:

- **Secrets** — access to secrets that are never read
- **Nodes** — direct node access (rarely needed by workloads)
- **Webhook configurations** — mutating or validating admission webhooks
- **RBAC resources** — ability to modify roles and bindings (privilege
  escalation)
- **CRDs** — ability to modify custom resource definitions
- **Service account tokens** — ability to request tokens for other accounts

When a report shows `sensitiveExcess: true`, that subject should be prioritized
for remediation regardless of its overall compliance score.

## Further Reading

- **[Kubernetes RBAC Drift Detection](/blog/kubernetes-rbac-drift-detection)** —
  detecting permission changes over time
- **[How to Audit Kubernetes RBAC](/blog/kubernetes-rbac-audit)** — the manual
  audit process with kubectl
- **[IAM Access Analyzer for Kubernetes](/blog/iam-access-analyzer-for-kubernetes)**
  — right-sizing RBAC permissions like AWS IAM Access Analyzer
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start producing compliance evidence
