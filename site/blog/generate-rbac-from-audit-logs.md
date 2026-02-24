---
title: "Generating Least-Privilege RBAC from Kubernetes Audit Logs"
seo_title: "Generate Least-Privilege RBAC from Kubernetes Audit Logs"
published_at: 2026-03-03T08:00:00.000Z
snippet: "A full before-and-after walkthrough: start with cluster-admin, install Audicia, and generate the minimal RBAC policy your workloads actually need."
description: "Generate least-privilege Kubernetes RBAC policies from audit logs with Audicia. Full walkthrough from cluster-admin to minimal Roles in under five minutes."
---

## The Overprivilege Problem

Every Kubernetes cluster has at least one service account bound to
`cluster-admin`. Someone needed to unblock a 403, the binding was created in a
hurry, and nobody ever tightened it. That service account now has full access to
every resource in every namespace — secrets, nodes, CRDs, everything.

The correct policy exists somewhere in the audit logs. Every API call your
workload makes is recorded: the verb, the resource, the namespace, the
subresource. The data is there. The question is how to extract it.

Audicia is a Kubernetes RBAC generator that does exactly this. It reads your
audit logs, extracts the permissions each subject actually uses, and produces
ready-to-apply Roles and ClusterRoles with nothing extra.

This post walks through the full process: from an overprivileged service account
to a least-privilege Role generated from real cluster behavior.

## Before: The cluster-admin Binding

Here is a service account with far more access than it needs:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: backend-cluster-admin
subjects:
  - kind: ServiceAccount
    name: backend
    namespace: my-team
roleRef:
  kind: ClusterRole
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io
```

This grants `backend` full access to every resource in every namespace. In
practice, the service account only needs to read pods and configmaps in its own
namespace. But nobody knows that without analyzing the audit logs.

## Step 1: Install Audicia

Audicia runs as a Kubernetes Operator. Install it with Helm:

```bash
helm install audicia oci://ghcr.io/felixnotka/audicia/charts/audicia-operator \
  -n audicia-system --create-namespace \
  --set auditLog.enabled=true \
  --set auditLog.hostPath=/var/log/kube-audit.log
```

This deploys the operator and configures it to tail the audit log file from the
control plane node.

## Step 2: Create an AudiciaSource

Tell Audicia where your audit log lives and how to shape the generated policies:

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: production-audit
spec:
  sourceType: K8sAuditLog
  location:
    path: /var/log/kube-audit.log
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Deny
      userPattern: "^system:apiserver$"
  checkpoint:
    intervalSeconds: 30
    batchSize: 500
```

Key settings:

- **`scopeMode: NamespaceStrict`** — generates per-namespace Roles, not
  ClusterRoles
- **`verbMerge: Smart`** — collapses rules with the same resource into a single
  rule with merged verbs
- **`wildcards: Forbidden`** — never generates wildcard `*` verbs
- **`ignoreSystemUsers: true`** — filters out `system:masters` and other
  built-in subjects

```bash
kubectl apply -f audicia-source.yaml
```

## Step 3: Wait for Audit Events

Audicia processes events in batches (default: 500 events or every 30 seconds).
As your cluster generates API traffic, the pipeline runs:

**Ingest → Filter → Normalize → Aggregate → Strategy → Report**

Each stage has a specific job:

1. **Ingest** reads raw JSON lines from the audit log with checkpoint/resume
2. **Filter** drops system traffic, health checks, and configurable patterns
3. **Normalize** parses `system:serviceaccount:my-team:backend` into a
   structured identity and migrates deprecated API groups like
   `extensions/v1beta1` to `apps/v1`
4. **Aggregate** deduplicates thousands of events into a compact rule set per
   subject — 10,000 `GET pods` calls become one aggregated rule
5. **Strategy** converts aggregated rules into RBAC manifests using the policy
   knobs from your AudiciaSource
6. **Report** writes everything to an `AudiciaPolicyReport` CRD

## Step 4: Read the Policy Report

After a flush cycle, Audicia produces a report for every observed subject:

```bash
kubectl get audiciapolicyreports -n my-team
```

```
NAMESPACE   NAME             SUBJECT   KIND             COMPLIANCE   SCORE   AGE
my-team     report-backend   backend   ServiceAccount   Red          25      5m
```

A score of 25 means the service account uses only 25% of its granted
permissions. The other 75% is excess privilege — flagged Red.

## Step 5: Extract the Suggested Policy

The report contains the minimal RBAC manifests your service account actually
needs:

```bash
kubectl get audiciapolicyreport report-backend -n my-team \
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
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
```

Compare this to `cluster-admin`. Instead of full access to everything, the
generated Role grants exactly three rules covering the resources and verbs the
service account actually used.

## After: The Least-Privilege Role

Apply the suggested policy (dry-run first):

```bash
# Dry-run to review what will be created
kubectl get audiciapolicyreport report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  | kubectl apply --dry-run=client -f -

# Apply for real
kubectl get audiciapolicyreport report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  | kubectl apply -f -
```

Then remove the overprivileged binding:

```bash
kubectl delete clusterrolebinding backend-cluster-admin
```

After the next flush cycle, the compliance score updates:

```bash
kubectl get audiciapolicyreports -n my-team
```

```
NAMESPACE   NAME             SUBJECT   KIND             COMPLIANCE   SCORE   AGE
my-team     report-backend   backend   ServiceAccount   Green        92      15m
```

Green. The service account now has tight permissions matching its actual
behavior.

## Why Audit Logs Are the Right Data Source

Static analysis tools can scan your YAML and flag overprivileged bindings, but
they cannot tell you what a workload actually does at runtime. Audit logs are
the only data source that records real API access patterns:

- Which subjects called which endpoints
- Which verbs were used on which resources
- Which namespaces were accessed
- Which subresources were invoked (like `pods/exec` or `pods/log`)

Audicia normalizes this data automatically — handling subresource concatenation,
deprecated API group migration (`extensions/v1beta1` → `apps/v1`), and identity
parsing for service accounts, users, and groups.

## GitOps Workflow

For teams using ArgoCD or Flux, the suggested manifests integrate directly:

```bash
kubectl get audiciapolicyreport report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  > policies/my-team/backend-rbac.yaml

git add policies/my-team/backend-rbac.yaml
git commit -m "rbac: tighten backend permissions (Audicia suggestion)"
git push
```

ArgoCD picks up the new Role on the next sync. No manual YAML authoring
required.

## Continuous Refinement

Because Audicia runs continuously as an operator — not a one-shot script — the
reports update as workload behavior changes. If the backend service starts
accessing a new resource, the next report includes that permission. If it stops
using a resource, the compliance score reflects the new excess.

This makes RBAC generation a living process rather than a quarterly exercise.
For a deep dive into how Audicia detects permission drift over time, see
[Kubernetes RBAC Drift Detection](/blog/kubernetes-rbac-drift-detection).

## What's Next

- **[How to Enable Kubernetes Audit Logging](/blog/kubernetes-audit-logging-guide)**
  — step-by-step instructions for kubeadm, kind, EKS, GKE, and AKS
- **[audit2rbac vs Audicia](/blog/audit2rbac-vs-audicia)** — how Audicia
  compares to the original audit-log-based RBAC generator
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and generate your first policy reports in under five minutes
