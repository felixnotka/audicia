# Upgrading to 0.5.0

## What Changed

Audicia 0.5.0 splits the single `AudiciaPolicyReport` CRD into two purpose-built
resources:

| Old (0.4.x)                              | New (0.5.0)                                                   |
| ---------------------------------------- | ------------------------------------------------------------- |
| `AudiciaPolicyReport` (short: `apr`)     | `AudiciaReport` (short: `ar`) – compliance and observed rules |
| `status.suggestedPolicy.manifests` field | `AudiciaPolicy` (short: `ap`) – suggested RBAC manifests      |

**Why:** Compliance scoring and RBAC suggestions have different lifecycles. A
compliance report updates every flush cycle, while a suggested policy should go
through an approval workflow before being applied. Separating them enables the
new `Pending → Approved → Applied → Outdated` state machine on policies without
cluttering the compliance report.

## What You Lose

Upgrading requires deleting the old CRD, which deletes all existing
`AudiciaPolicyReport` resources:

- **Observed rules** – The operator re-accumulates rules from the next flush
  cycle. For file mode, it resumes from its checkpoint position. For webhook and
  cloud mode, only new events are captured.
- **Historical counts** – `firstSeen` timestamps and `count` values reset. The
  aggregator state is in-memory and lost on operator restart.
- **Compliance scores** – Regenerated once enough rules accumulate for the diff
  engine to evaluate.

The underlying audit log data is not affected. If you use file mode, the
operator picks up from its last checkpoint and re-accumulates rules within one
flush cycle.

## Prerequisites

- Helm 3
- `kubectl` access to the cluster
- The new chart version (0.5.0+) available in your Helm repo

## Step-by-Step

### 1. Apply the new CRDs

Helm does not upgrade CRDs on `helm upgrade`. You must apply them manually
before upgrading the operator:

```bash
# Download the new CRD manifests from the chart
helm pull audicia/audicia-operator --version 0.5.0 --untar

# Apply the new CRDs
kubectl apply -f audicia-operator/crds/audicia.io_audiciareports.yaml
kubectl apply -f audicia-operator/crds/audicia.io_audiciapolicies.yaml
```

Verify:

```bash
kubectl get crd audiciareports.audicia.io audiciapolicies.audicia.io
```

### 2. Upgrade the operator

```bash
helm upgrade audicia audicia/audicia-operator \
  -n audicia-system \
  -f your-values.yaml
```

The new operator starts and begins writing `AudiciaReport` and `AudiciaPolicy`
resources. It no longer manages `AudiciaPolicyReport`.

### 3. Delete the old CRD

Once you confirm reports and policies are being created:

```bash
# Verify new resources exist
kubectl get audiciareports --all-namespaces
kubectl get audiciapolicies --all-namespaces

# Delete the old CRD (this removes all AudiciaPolicyReport resources)
kubectl delete crd audiciapolicyreports.audicia.io
```

### 4. Update your tooling

Update any scripts, dashboards, or automation that reference the old resource:

| Before (0.4.x)                                      | After (0.5.0)                                           |
| --------------------------------------------------- | ------------------------------------------------------- |
| `kubectl get audiciapolicyreports`                  | `kubectl get audiciareports` (or `kubectl get ar`)      |
| `kubectl get apr`                                   | `kubectl get ar`                                        |
| `-o jsonpath='{.status.suggestedPolicy.manifests}'` | `kubectl get audiciapolicies` (or `kubectl get ap`)     |
| `audiciapolicyreports` in RBAC rules                | `audiciareports` + `audiciapolicies`                    |
| `audicia_reports_updated_total` metric              | Still exists, plus new `audicia_policies_updated_total` |

### 5. Update RBAC for report readers

If you have custom RBAC rules granting access to `audiciapolicyreports`, update
them:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: audicia-report-reader
rules:
  - apiGroups: ["audicia.io"]
    resources: ["audiciareports"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["audicia.io"]
    resources: ["audiciapolicies"]
    verbs: ["get", "list", "watch"]
```

## Extracting Policy Manifests (New Workflow)

In 0.4.x, manifests were embedded in the report's status. In 0.5.0, they live in
a separate `AudiciaPolicy` resource:

```bash
# Old (0.4.x)
kubectl get apr report-backend -n my-team \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}'

# New (0.5.0)
kubectl get apolicy policy-backend -n my-team \
  -o jsonpath='{range .spec.manifests[*]}{@}{"\n---\n"}{end}'
```

Each `AudiciaPolicy` has a `status.state` field tracking its lifecycle:

| State      | Meaning                                               |
| ---------- | ----------------------------------------------------- |
| `Pending`  | Newly generated, awaiting review                      |
| `Approved` | Reviewed and approved (set manually or via tooling)   |
| `Applied`  | The manifests have been applied to the cluster        |
| `Outdated` | The operator detected manifest changes since approval |

## Rollback

If you need to revert to 0.4.x:

```bash
helm rollback audicia -n audicia-system
kubectl delete crd audiciareports.audicia.io audiciapolicies.audicia.io
```

The old `AudiciaPolicyReport` CRD will be reinstalled by the next `helm install`
of the 0.4.x chart. Reports regenerate from the next flush.
