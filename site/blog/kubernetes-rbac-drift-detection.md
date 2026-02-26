---
title: "Kubernetes RBAC Drift Detection: Finding Unused Permissions"
seo_title: "Kubernetes RBAC Drift Detection: Finding Unused Permissions"
published_at: 2026-03-28T08:00:00.000Z
snippet: "RBAC drift is the gap between what a workload is allowed to do and what it actually does. Here's how to detect it and reduce excess privilege."
description: "Detect Kubernetes RBAC drift: find unused permissions, measure overprivilege with compliance scores, and reduce excess access with audit-log-based analysis."
---

## What Is RBAC Drift?

RBAC drift is the growing gap between what a subject is _allowed_ to do and what
it _actually_ does.

It happens naturally:

- A service account was granted `create deployments` during development, but the
  deployment logic was moved to a different controller
- A CI pipeline had `get secrets` for a feature that was removed six months ago
- An operator was granted broad access during initial setup and never tightened

Each of these creates excess permissions — grants that exist in RBAC but are
never exercised at runtime. Over time, the gap widens. This is drift.

## Why Drift Matters

### Security Risk

Every excess permission is attack surface. If a service account with unused
`get secrets` cluster-wide is compromised, the attacker can read every secret in
every namespace — even though the workload never needed that access.

### Compliance Violations

Compliance frameworks like SOC 2, ISO 27001, and PCI DSS require least-privilege
access controls. Drift means your RBAC no longer reflects actual usage, which
auditors will flag.

### Blast Radius

The more excess permissions a subject has, the larger the blast radius of a
compromise. A service account that only needs `get pods` in one namespace but
has `cluster-admin` gives an attacker full cluster access.

## Detecting Drift

Drift detection requires two data sources:

1. **Granted permissions** — what RBAC currently allows (from Roles, Bindings)
2. **Observed usage** — what the subject actually does (from audit logs)

The difference between granted and observed is the drift.

### The Resolution Process

For a given subject (ServiceAccount, User, or Group):

1. **Resolve effective permissions** — query all ClusterRoleBindings and
   RoleBindings that reference the subject, resolve each to its PolicyRules
2. **Collect observed actions** — extract API calls from audit logs: verb,
   resource, API group, namespace, subresource
3. **Compare** — for each effective rule, check whether any observed action
   exercised it

Rules that were exercised are **used**. Rules that were never exercised are
**excess** — this is the drift.

### Manual Detection

You can approximate drift detection manually by combining `kubectl auth can-i`
with audit log analysis:

```bash
# List all effective permissions
kubectl auth can-i --list \
  --as=system:serviceaccount:my-team:backend \
  -n my-team

# Compare against actual audit log usage
jq 'select(.user.username == "system:serviceaccount:my-team:backend")
  | {verb, resource: .objectRef.resource}' \
  /var/log/kubernetes/audit/audit.log | sort -u
```

Compare the two outputs. Any permission in the first list that does not appear
in the second list is excess.

This works for one-time analysis but does not scale across dozens of service
accounts and does not handle edge cases like subresource matching, wildcard
expansion, or namespace scoping.

### Automated Detection with Audicia

Audicia automates the entire process. Its compliance engine resolves effective
permissions, diffs them against observed usage, and produces a structured
report:

```bash
kubectl get apreport report-backend -n my-team -o wide
```

```
NAMESPACE   NAME             SUBJECT   KIND             COMPLIANCE   SCORE   AGE   NEEDED   EXCESS   UNGRANTED   SENSITIVE   AUDIT EVENTS
my-team     report-backend   backend   ServiceAccount   Red          25      1h    2        6        0           true        1500
```

This tells you:

- The service account has **2 needed permission rules** and **6 excess grants**
- It exercised only a fraction of its permissions in the observation period
- **6 rules are excess** — this is the drift
- **Sensitive excess is present** — unused grants include high-risk resources

### Reading the Drift Details

The full report shows exactly which permissions are used and which are excess:

```yaml
status:
  compliance:
    score: 25
    severity: Red
    usedCount: 2
    excessCount: 6
    uncoveredCount: 0
    sensitiveExcess:
      - secrets
    lastEvaluatedTime: "2026-03-28T12:00:00Z"
```

The `sensitiveExcess` field lists the specific high-risk resources where the
subject has unused access. In this case, the service account has `secrets`
access that it never uses — a priority remediation target.

## Acting on Drift

### Step 1: Review the Suggested Policy

Each report includes the minimal RBAC the subject needs:

```bash
kubectl get apreport report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}'
```

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: suggested-backend-role
  namespace: my-team
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
```

Compare this against the current Roles. The difference is the drift — every rule
in the current Role that is not in the suggested Role is excess.

### Step 2: Test in Non-Production

Apply the suggested policy in a staging environment first:

```bash
kubectl apply --dry-run=client -f suggested-backend-role.yaml
kubectl apply -f suggested-backend-role.yaml -n staging
```

Monitor the workload for 403 errors. If the suggested policy is missing a
permission the workload needs intermittently (like a rarely-triggered code
path), extend the observation period before applying to production.

### Step 3: Apply and Remove the Overprivileged Binding

```bash
# Apply the tight policy
kubectl apply -f suggested-backend-role.yaml -n my-team

# Remove the overprivileged binding
kubectl delete clusterrolebinding backend-cluster-admin
```

### Step 4: Monitor the Score

After applying the tighter policy, the compliance score updates on the next
evaluation cycle:

```
NAMESPACE   NAME             SUBJECT   KIND             COMPLIANCE   SCORE   AGE
my-team     report-backend   backend   ServiceAccount   Green        92      2h
```

Green means the drift is resolved — the subject's permissions now closely match
its actual behavior.

## Continuous Drift Detection

Drift is not a one-time problem. Workloads change, new API calls are added, old
code paths are removed. A policy that was tight last month may be drifting
today.

Because Audicia runs continuously as a Kubernetes Operator, it detects drift as
it develops. The compliance score updates with each evaluation cycle. If a
previously Green service account starts drifting (its score drops because it
stopped using some permissions), the report reflects the change.

This makes drift detection a continuous process rather than a quarterly audit
exercise.

## Further Reading

- **[How to Audit Kubernetes RBAC](/blog/kubernetes-rbac-audit)** — the manual
  kubectl audit process
- **[Kubernetes RBAC Compliance Evidence](/blog/kubernetes-rbac-compliance-evidence)**
  — mapping drift detection results to SOC 2, ISO 27001, PCI DSS
- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  full before/after walkthrough
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start detecting drift
