# Audit Policy Configuration

This guide explains how to configure Kubernetes audit logging for optimal
Audicia usage.

## Why Audit Logging?

Audicia needs audit events to observe API access patterns. Without audit
logging, the kube-apiserver doesn't record who accessed what.

## Recommended Audit Level

Audicia requires **`Metadata` level** at minimum. This provides the fields
Audicia needs without the overhead of full request/response bodies:

| Field                   | Purpose                                |
| ----------------------- | -------------------------------------- |
| `user.username`         | Subject identity                       |
| `user.groups`           | Group metadata                         |
| `verb`                  | The API verb (get, list, create, etc.) |
| `objectRef.resource`    | Target resource                        |
| `objectRef.subresource` | Target subresource (e.g., exec, log)   |
| `objectRef.namespace`   | Target namespace                       |
| `objectRef.apiGroup`    | API group                              |
| `requestURI`            | For non-resource URL detection         |
| `responseStatus.code`   | To distinguish allowed vs. denied      |
| `auditID`               | For webhook deduplication              |

`RequestResponse` level works but generates significantly more data
(request/response bodies) that Audicia does not use.

## The Example Audit Policy

Audicia includes a ready-to-use audit policy (see
[Audit Policy Example](../examples/audit-policy.md)):

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Skip health/readiness endpoints (high volume, no RBAC value)
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

### Why These Rules?

- **Health endpoints** generate enormous volume and are never useful for RBAC
  analysis.
- **system:apiserver** is internal plumbing — filtering it at the audit level
  reduces log volume by ~30%.
- **`omitStages: [RequestReceived]`** skips the initial "request received"
  stage, halving the number of events per API call while keeping the
  "ResponseComplete" event with the status code.

## Installing the Audit Policy

### File-Based Ingestion

Add to your kube-apiserver:

```yaml
- --audit-policy-file=/etc/kubernetes/audit-policy.yaml
- --audit-log-path=/var/log/kube-audit.log
```

### Webhook Ingestion

Add to your kube-apiserver:

```yaml
- --audit-policy-file=/etc/kubernetes/audit-policy.yaml
- --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

### Both Simultaneously

You can use both backends at the same time:

```yaml
- --audit-policy-file=/etc/kubernetes/audit-policy.yaml
- --audit-log-path=/var/log/kube-audit.log
- --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

## Production Tuning

### Reduce Volume for High-Traffic Clusters

Add specific `None` rules for high-volume, low-value endpoints:

```yaml
rules:
  - level: None
    nonResourceURLs: ["/healthz*", "/livez*", "/readyz*"]

  - level: None
    users: ["system:apiserver"]

  # Skip high-volume read-only requests to well-known resources
  - level: None
    verbs: ["get", "list", "watch"]
    resources:
      - group: ""
        resources: ["events"] # Events generate massive audit volume

  - level: Metadata
    omitStages: [RequestReceived]
```

### Namespace-Scoped Policies

If you only care about specific namespaces, use the audit policy to reduce
volume:

```yaml
rules:
  - level: None
    nonResourceURLs: ["/healthz*", "/livez*", "/readyz*"]
  - level: None
    users: ["system:apiserver"]

  # Only log events in target namespaces
  - level: Metadata
    namespaces: ["production", "staging"]
    omitStages: [RequestReceived]

  # Skip everything else
  - level: None
```

> **Note:** This is complementary to Audicia's filter chain. The audit policy
> controls what the apiserver logs; Audicia's filters control what gets
> processed into reports. Use both for maximum efficiency.

## Log Rotation

For file-based ingestion, configure log rotation to prevent disk exhaustion:

```yaml
- --audit-log-maxsize=100 # Max size in MB per file
- --audit-log-maxbackup=3 # Number of old files to retain
- --audit-log-maxage=7 # Max days to retain old files
```

Audicia handles rotation automatically via inode tracking (Linux) — when it
detects the file was rotated, it resets the offset and starts reading the new
file.

## Verify It's Working

```bash
# Check the audit log exists and has events
head -5 /var/log/kube-audit.log

# Count events per minute (rough volume estimate)
wc -l /var/log/kube-audit.log

# Check a specific field Audicia needs
cat /var/log/kube-audit.log | jq -r '.verb' | sort | uniq -c | sort -rn
```
