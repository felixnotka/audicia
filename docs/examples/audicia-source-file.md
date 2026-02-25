# AudiciaSource: File Mode

AudiciaSource for file-based audit log ingestion. Reads from a hostPath volume
on a control plane node.

**See also:** [Quick Start: File Mode](../getting-started/quick-start-file.md) |
[Ingestor](../components/ingestor.md)

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: prod-audit-logs
  namespace: audicia-system
spec:
  sourceType: K8sAuditLog
  location:
    path: /var/log/kubernetes/audit/audit.log
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Deny
      userPattern: "^system:apiserver$"
```

## Customization

- **`location.path`** — Must match the `--audit-log-path` flag on the
  kube-apiserver.
- **Filters** — Adjust to match your environment. See
  [Filter Recipes](../guides/filter-recipes.md).
- **Helm requirement:** `auditLog.enabled=true` and control plane scheduling
  (nodeSelector + tolerations).
