---
title: "Using Audicia with GitOps: ArgoCD and Flux Integration"
seo_title: "Using Audicia with GitOps: ArgoCD and Flux Integration"
published_at: 2026-04-14T08:00:00.000Z
snippet: "How to integrate Audicia's generated RBAC policies into ArgoCD and Flux GitOps workflows — from CRD output to committed YAML to auto-synced Roles."
description: "Integrate Audicia-generated RBAC policies into ArgoCD and Flux GitOps workflows. Export CRD output to committed YAML and auto-sync least-privilege Roles."
---

## Why GitOps for RBAC

GitOps treats a Git repository as the source of truth for cluster state. Every
change is committed, reviewed, and auditable. This is exactly how RBAC changes
should be managed:

- **Peer review** — RBAC changes go through pull requests before being applied
- **Audit trail** — every permission change is a Git commit with a timestamp and
  author
- **Rollback** — reverting an RBAC change is a Git revert
- **Consistency** — the same RBAC policies deploy to every environment

Audicia produces RBAC policies as CRD output. GitOps turns that output into
reviewed, versioned, deployable manifests.

## The Workflow

```
Audit Logs → Audicia → AudiciaPolicyReport CRD → Export → Git Commit → PR Review → ArgoCD/Flux Sync
```

1. Audicia continuously processes audit events and produces policy reports
2. You export the suggested policies from the CRD
3. The exported YAML is committed to your GitOps repository
4. A pull request is created for review
5. Once merged, ArgoCD or Flux syncs the new Roles and RoleBindings to the
   cluster

## Exporting Suggested Policies

Each `AudiciaPolicyReport` contains ready-to-apply RBAC manifests in
`status.suggestedPolicy.manifests`. Export them with kubectl:

```bash
# Export a single subject's policy
kubectl get apreport report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  > policies/my-team/backend-rbac.yaml

# Export all policies in a namespace
for report in $(kubectl get apreport -n my-team -o name); do
  name=$(echo $report | cut -d/ -f2)
  kubectl get apreport $name -n my-team \
    -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
    > policies/my-team/${name}.yaml
done
```

The exported YAML is complete and apply-ready — it includes Role, RoleBinding,
and optionally ClusterRole and ClusterRoleBinding resources.

## ArgoCD Integration

### Repository Structure

A common layout for RBAC policies in an ArgoCD-managed repository:

```
├── apps/
│   └── my-app/
│       ├── deployment.yaml
│       └── service.yaml
├── rbac/
│   └── my-team/
│       ├── backend-rbac.yaml
│       └── worker-rbac.yaml
└── argocd/
    └── rbac-app.yaml
```

### ArgoCD Application

Create an ArgoCD Application that syncs the RBAC directory:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: rbac-policies
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/k8s-manifests
    path: rbac
    targetRevision: main
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: false
      selfHeal: true
```

Setting `prune: false` prevents ArgoCD from deleting Roles that are removed from
Git. This is a safety measure — you should explicitly manage RBAC deletions
rather than letting sync handle them automatically.

### The PR Workflow

```bash
# Export updated policies
kubectl get apreport report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  > rbac/my-team/backend-rbac.yaml

# Create a branch and PR
git checkout -b rbac/tighten-backend
git add rbac/my-team/backend-rbac.yaml
git commit -m "rbac: tighten backend permissions (audicia suggestion)"
git push origin rbac/tighten-backend
# Create PR for review
```

The PR shows exactly what RBAC changes are proposed. Reviewers can compare the
suggested Role against the current one and approve or request changes.

## Flux Integration

### Repository Structure

Flux uses Kustomization resources to manage paths within a repository:

```
├── clusters/
│   └── production/
│       └── kustomization.yaml
├── rbac/
│   └── my-team/
│       ├── kustomization.yaml
│       ├── backend-rbac.yaml
│       └── worker-rbac.yaml
```

### Flux Kustomization

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: rbac-policies
  namespace: flux-system
spec:
  interval: 5m
  path: ./rbac
  prune: false
  sourceRef:
    kind: GitRepository
    name: k8s-manifests
```

Like ArgoCD, set `prune: false` to prevent automatic deletion of Roles when they
are removed from Git.

### Flux PR Workflow

The export and PR workflow is identical to ArgoCD. Flux monitors the repository
and syncs changes after merge.

## Automating the Export

For continuous automation, create a CronJob or CI pipeline that periodically
exports policies:

```bash
#!/bin/bash
# export-rbac.sh — run periodically or on AudiciaPolicyReport changes

NAMESPACES="my-team staging production"

for ns in $NAMESPACES; do
  mkdir -p rbac/$ns
  for report in $(kubectl get apreport -n $ns -o name); do
    name=$(echo $report | cut -d/ -f2)
    kubectl get apreport $name -n $ns \
      -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
      > rbac/$ns/${name}.yaml
  done
done

# Check for changes
if git diff --quiet rbac/; then
  echo "No RBAC changes detected"
  exit 0
fi

# Commit and push
git add rbac/
git commit -m "rbac: update suggested policies (automated export)"
git push origin main
```

For production use, this script should create a pull request rather than pushing
directly to main, ensuring that all RBAC changes go through review.

## Safety Considerations

### Never Auto-Apply Without Review

Audicia generates suggestions. The GitOps workflow adds a review step before
those suggestions become live RBAC. This is intentional — automated privilege
changes without review are a security risk.

### Incremental Rollout

When tightening permissions for many service accounts, roll out changes
incrementally:

1. Start with non-production namespaces
2. Monitor for 403 errors after applying tighter policies
3. Move to production after validating in staging

### Keep the Old Bindings Until Verified

Do not delete the old ClusterRoleBinding in the same PR that adds the new Role.
Apply the new Role first, verify the workload works, then remove the old binding
in a separate PR.

## Further Reading

- **[Kubernetes RBAC for Multi-Tenant Clusters](/blog/kubernetes-rbac-multi-tenant)**
  — per-namespace policy generation for teams
- **[Generating RBAC from Audit Logs](/blog/generate-rbac-from-audit-logs)** —
  the full before/after walkthrough
- **[Getting Started Guide](/docs/getting-started/introduction)** — install
  Audicia and start generating exportable policies
