# Ingestor

The ingestor is the entry point of Audicia's pipeline. It abstracts the audit log source into a unified event stream,
outputting raw `audit.k8s.io/v1.Event` structs regardless of source type.

**Package:** `pkg/ingestor/`

---

## Where It Sits in the Pipeline

```
Audit Log → **Ingestor** → Filter → Normalizer → Aggregator → Strategy → Compliance → Report
```

**Input:** Raw audit events from a file on disk or an HTTPS webhook endpoint.
**Output:** Parsed `audit.k8s.io/v1.Event` structs on an internal event channel.

The ingestor knows nothing about RBAC. Its only job is to reliably deliver audit events to the rest of the pipeline.

---

## Ingestion Modes

Each `AudiciaSource` CR specifies one of two ingestion modes. Both can run simultaneously — each source gets its own
pipeline goroutine.

### File-Based Ingestion (`K8sAuditLog`)

Tails a Kubernetes audit log file on disk, reading JSON-encoded audit events line by line.

| Behavior                     | Details                                                                                                |
|------------------------------|--------------------------------------------------------------------------------------------------------|
| **Continuous tailing**       | Polls the file every 1s for new data after exhausting current content.                                 |
| **Checkpoint / resume**      | Tracks byte offset in `AudiciaSource.status.fileOffset`. Resumes from last position after pod restart. |
| **Log rotation detection**   | Compares inode numbers (Linux only via `syscall.Stat_t`). Resets offset to 0 when inode changes.       |
| **Configurable batch size**  | `spec.checkpoint.batchSize` (default 500). Controls the channel buffer size.                           |
| **Malformed line tolerance** | Skips lines that don't parse as valid `audit.k8s.io/v1.Event` JSON.                                   |

**CRD configuration:**

```yaml
spec:
  sourceType: K8sAuditLog
  location:
    path: /var/log/kubernetes/audit/audit.log
```

**Helm requirement:** `auditLog.enabled=true`, `auditLog.hostPath=<path>`. The pod needs control plane scheduling
(nodeSelector, tolerations) and typically `runAsUser: 0` for hostPath read access.

### Webhook Ingestion (`Webhook`)

Receives real-time audit events via an HTTPS endpoint. The kube-apiserver pushes events using
`--audit-webhook-config-file`.

| Behavior                      | Details                                                                                              |
|-------------------------------|------------------------------------------------------------------------------------------------------|
| **HTTPS server**              | TLS certificate and key loaded from a mounted Kubernetes Secret at `/etc/audicia/webhook-tls/`.      |
| **mTLS (optional)**           | When `clientCASecretName` is set, requires and verifies client certificates against the CA bundle.    |
| **Rate limiting**             | Token-bucket rate limiter. `spec.webhook.rateLimitPerSecond` (default 100). Returns HTTP 429.        |
| **Request body size limit**   | `spec.webhook.maxRequestBodyBytes` (default 1MB). Returns HTTP 413 when exceeded.                   |
| **Audit event deduplication** | LRU cache (10,000 entries) keyed by `auditID`. Prevents duplicate processing on retries.            |
| **Backpressure**              | Returns HTTP 429 when the internal event channel (500 buffer) is full.                               |
| **Graceful shutdown**         | 5-second graceful shutdown on context cancellation.                                                  |
| **POST-only enforcement**     | Rejects non-POST requests with HTTP 405.                                                             |

**CRD configuration:**

```yaml
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
    clientCASecretName: ""        # optional, enables mTLS
    rateLimitPerSecond: 100
    maxRequestBodyBytes: 1048576
```

**Helm requirement:** `webhook.enabled=true`, `webhook.tlsSecretName=<secret>`. Does NOT need control plane
scheduling — runs on any node.

---

## Core Functions

| Function             | Purpose                                                                                                                        |
|----------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `readFile`           | File mode entry point. Detects log rotation via inode comparison and resumes from the last checkpoint offset.                   |
| `pollForData`        | Tail-follow loop with a 1-second tick interval. Re-checks the inode on each poll cycle to detect rotation during idle periods. |
| `handleAuditRequest` | Webhook mode handler. Enforces POST method, rate limiting, body size limits, JSON parsing, deduplication, and backpressure.    |
| `seen`               | Bounded FIFO deduplication cache. Prevents duplicate processing when the same audit event is delivered more than once.         |
| `allow`              | Token-bucket rate limiter. Returns `false` (HTTP 429) when the per-second request threshold is exceeded.                       |

---

## Related

- [Architecture](../concepts/architecture.md) — System overview and data flow
- [Pipeline](../concepts/pipeline.md) — Stage-by-stage processing overview
- [Webhook Setup Guide](../guides/webhook-setup.md) — Full webhook configuration walkthrough
- [mTLS Setup Guide](../guides/mtls-setup.md) — Client certificate verification
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — Full field reference
