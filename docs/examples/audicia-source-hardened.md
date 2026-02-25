# AudiciaSource: Production Hardened

Production-hardened AudiciaSource with mTLS, rate limiting, checkpointing, and
retention limits.

**See also:** [Webhook Setup Guide](../guides/webhook-setup.md) |
[mTLS Setup](../guides/mtls-setup.md) |
[Filter Recipes](../guides/filter-recipes.md)

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: realtime-audit-hardened
  namespace: audicia-system
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
    clientCASecretName: kube-apiserver-client-ca
    rateLimitPerSecond: 100
    maxRequestBodyBytes: 1048576 # 1MB

  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden

  checkpoint:
    intervalSeconds: 30
    batchSize: 500

  limits:
    maxRulesPerReport: 200
    retentionDays: 30

  filters:
    - action: Deny
      userPattern: "^system:anonymous$"
    - action: Deny
      userPattern: "^system:kube-proxy$"
    - action: Deny
      userPattern: "^system:kube-scheduler$"
    - action: Deny
      userPattern: "^system:kube-controller-manager$"
    - action: Allow
      userPattern: ".*"
```

## Key Differences from Basic Webhook

- **`clientCASecretName`** — Enables mTLS. Only the kube-apiserver (presenting a
  valid client certificate) can send events.
- **`checkpoint`** — Persists processing state every 30 seconds for resume after
  restart.
- **`limits`** — Caps report size at 200 rules and drops rules not seen in 30
  days.
- **`NamespaceStrict`** — Generates per-namespace Roles instead of ClusterRoles.
