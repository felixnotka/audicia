# Quick Start: File-Based Ingestion

This tutorial walks you through setting up Audicia to tail a Kubernetes audit
log file, observe API access patterns, and generate your first RBAC policy
report.

## Prerequisites

- Audit logging enabled on your cluster (see
  [Audit Policy Guide](../guides/audit-policy.md))
- `kubectl` configured
- Helm 3

## Step 1: Install Audicia

Create a `values-file.yaml` file with your cluster-specific configuration:

```yaml
# values-file.yaml
auditLog:
  enabled: true
  hostPath: /var/log/kubernetes/audit/audit.log

nodeSelector:
  node-role.kubernetes.io/control-plane: ""

tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
```

Install with Helm:

```bash
helm repo add audicia https://charts.audicia.io

helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-file.yaml
```

> **Permission denied?** Audit logs are typically owned by root. If the operator
> cannot read the log, add the following to your `values-file.yaml` to run as
> root:
>
> ```yaml
> podSecurityContext:
>   runAsUser: 0
>   runAsNonRoot: false
> ```
>
> Alternatively, relax file permissions on the host with
> `chmod 644 /var/log/kubernetes/audit/audit.log`.

## Step 2: Create an AudiciaSource

Save the following manifest as `audicia-source-file.yaml` (see the
[File Mode Example](../examples/audicia-source-file.md) for customization
options):

```yaml
# audicia-source-file.yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: dev-cluster-audit
spec:
  sourceType: K8sAuditLog
  location:
    path: /var/log/kubernetes/audit/audit.log
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
  limits:
    maxRulesPerReport: 200
    retentionDays: 30
```

```bash
kubectl apply -f audicia-source-file.yaml
```

Verify the source started:

```bash
kubectl describe audiciasource dev-cluster-audit
```

Look for the `Ready` condition in the status.

## Step 3: Generate Some API Traffic

If you don't already have workloads generating audit events, create some:

```bash
# Create a namespace and service account
kubectl create namespace demo
kubectl create serviceaccount demo-app -n demo

# Make some API calls as the service account
kubectl get pods -n demo
kubectl get configmaps -n demo
kubectl get services -n demo
```

Wait 30-60 seconds for the flush cycle to process the events.

## Step 4: View the Policy Report

```bash
kubectl get audiciareports --all-namespaces
```

You should see reports for each subject that generated audit events:

```
NAMESPACE   NAME              SUBJECT    KIND             COMPLIANCE   SCORE   AGE
demo        report-demo-app   demo-app   ServiceAccount                          60s
```

Inspect a report:

```bash
kubectl get audiciareport report-demo-app -n demo -o yaml
```

The report's `status` contains the observed rules and compliance data:

```yaml
status:
  observedRules:
    - apiGroups: [""]
      resources: ["pods"]
      verbs: ["list"]
      firstSeen: "2026-02-20T10:00:00Z"
      lastSeen: "2026-02-20T10:00:00Z"
      count: 1
```

> **Note:** Suggested RBAC manifests are no longer part of the report. They live
> on the companion `AudiciaPolicy` resource. See the next step to retrieve them.

## Step 5: Apply the Suggested Policy

Suggested RBAC manifests live on the `AudiciaPolicy` resource. Review and apply
them:

```bash
# Dry-run first
kubectl get apolicy policy-demo-app -n demo \
  -o jsonpath='{range .spec.manifests[*]}{@}{"\n---\n"}{end}' \
  | kubectl apply --dry-run=client -f -

# Apply for real
kubectl get apolicy policy-demo-app -n demo \
  -o jsonpath='{range .spec.manifests[*]}{@}{"\n---\n"}{end}' \
  | kubectl apply -f -
```

> **Tip:** For GitOps workflows, pipe the output to a file in your policy repo:
>
> ```bash
> kubectl get apolicy policy-demo-app -n demo \
>   -o jsonpath='{range .spec.manifests[*]}{@}{"\n---\n"}{end}' \
>   > policies/demo/demo-app-rbac.yaml
> ```

## Step 6: Verify Compliance

After applying the policy and the next flush cycle:

```bash
kubectl get audiciareports -n demo
```

If the applied RBAC matches usage, you'll see a Green compliance score.

## What's Next

- [Filter Recipes](../guides/filter-recipes.md) – Common filter configurations
  for production
- [Compliance Scoring](../concepts/compliance-scoring.md) – How RBAC drift
  detection works
- [Feature Reference](../reference/features.md) – Full configuration options
