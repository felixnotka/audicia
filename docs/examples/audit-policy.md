# Audit Policy

Kubernetes audit policy for use with Audicia. Logs all API requests at Metadata
level while skipping high-volume, low-value endpoints.

**See also:** [Audit Policy Guide](../guides/audit-policy.md)

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Skip health endpoints (high volume, no RBAC value)
  - level: None
    nonResourceURLs:
      - /healthz*
      - /livez*
      - /readyz*

  # Skip apiserver-to-apiserver traffic
  - level: None
    users:
      - system:apiserver

  # Log everything else at Metadata level
  - level: Metadata
    omitStages:
      - RequestReceived
```

## Customization

- **Reduce volume further:** Add `level: None` rules for noisy resources like
  `events`. See
  [Production Tuning](../guides/audit-policy.md#production-tuning).
- **Namespace scoping:** Restrict logging to specific namespaces for targeted
  rollouts. See
  [Namespace-Scoped Policies](../guides/audit-policy.md#namespace-scoped-policies).
- **`omitStages: [RequestReceived]`** halves the event count per API call while
  keeping the `ResponseComplete` event with the status code.
