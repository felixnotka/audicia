# AudiciaSource: Webhook Mode

AudiciaSource for webhook-based real-time audit event ingestion from the
kube-apiserver.

**See also:**
[Quick Start: Webhook Mode](../getting-started/quick-start-webhook.md) |
[Webhook Setup Guide](../guides/webhook-setup.md)

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: realtime-audit
  namespace: audicia-system
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
    rateLimitPerSecond: 100
    maxRequestBodyBytes: 1048576 # 1MB

  policyStrategy:
    scopeMode: ClusterScopeAllowed
    verbMerge: Exact
    wildcards: Forbidden

  filters:
    - action: Deny
      userPattern: "^system:anonymous$"
    - action: Deny
      userPattern: "^system:kube-proxy$"
    - action: Deny
      userPattern: "^system:kube-scheduler$"
    - action: Deny
      userPattern: "^system:kube-controller-manager$"
    # Allow everything else
    - action: Allow
      userPattern: ".*"
```

## Customization

- **`tlsSecretName`** — Must reference a `kubernetes.io/tls` Secret in the same
  namespace.
- **mTLS** — Add `clientCASecretName` for production hardening. See
  [Hardened Example](audicia-source-hardened.md).
- **Rate limiting** — Increase `rateLimitPerSecond` for high-traffic clusters.
