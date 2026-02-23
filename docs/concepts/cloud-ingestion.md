# Cloud Ingestion

## Why Cloud Ingestion?

Managed Kubernetes platforms (AKS, EKS, GKE) do not expose kube-apiserver flags or audit log files on disk. Instead,
they export audit events through cloud-native pipelines:

| Platform | Pipeline              | Envelope Format                                |
|----------|-----------------------|------------------------------------------------|
| **AKS**  | Azure Event Hub       | Azure Diagnostic Settings JSON (`records[]`)   |
| **EKS**  | CloudWatch Logs       | CloudWatch log event JSON                      |
| **GKE**  | Cloud Pub/Sub         | Cloud Logging JSON payload                     |

Audicia's cloud ingestion mode connects to these pipelines and extracts standard `audit.k8s.io/v1.Event` structs
from the provider-specific envelope format — feeding them into the same pipeline as file and webhook sources.

## Architecture

```
Cloud Pipeline → MessageSource → EnvelopeParser → CloudIngestor → Filter → ... → Report
```

Cloud ingestion introduces two abstractions:

- **MessageSource** — Connects to the cloud message bus, receives batches of messages, and acknowledges them after
  processing. Each provider has its own implementation (e.g., `EventHubSource` for Azure).
- **EnvelopeParser** — Unwraps the cloud-provider-specific JSON envelope and extracts audit events. For example,
  Azure wraps events in `{ "records": [{ "category": "kube-audit", "properties": { "log": "<audit JSON>" } }] }`.

The `CloudIngestor` orchestrates these two interfaces: connect → receive batch → parse envelopes → validate cluster
identity → emit events → acknowledge → update checkpoint.

## Cluster Identity Validation

When multiple clusters share a single cloud pipeline (e.g., one Event Hub for several AKS clusters), Audicia validates
that received events belong to the expected cluster. The `clusterIdentity` field in the CRD is matched against event
annotations and request URIs.

If the identity cannot be verified (e.g., missing annotations), the event is allowed by default — this is
defense-in-depth, not a hard gate.

## Checkpoint and Recovery

Cloud checkpoints track per-partition sequence numbers (e.g., Event Hub partition offsets). On restart, the ingestor
resumes from the last acknowledged position, stored in `AudiciaSource.status.cloudCheckpoint.partitionOffsets`.

Optionally, Azure Event Hub supports Blob Storage-backed checkpoint persistence for distributed consumption. When
`storageAccountURL` is configured, the Event Hub processor handles partition ownership and checkpointing automatically.

## Build Tags

Cloud provider adapters are compiled conditionally using Go build tags. The default binary includes no cloud SDKs:

| Build Tag | Adapter            | SDK Dependencies                           |
|-----------|--------------------|-------------------------------------------|
| `azure`   | Azure Event Hub    | `azeventhubs/v2`, `azidentity`, `azblob`  |
| _(none)_  | EKS (planned)      | —                                          |
| _(none)_  | GKE (planned)      | —                                          |

Build the Azure-enabled binary:

```bash
go build -tags azure ./cmd/audicia/
```

Or with Docker:

```bash
docker build --build-arg GO_BUILD_TAGS=azure -t audicia:azure .
```

## Supported Providers

| Provider         | Status        | Guide                                           |
|------------------|---------------|------------------------------------------------|
| Azure Event Hub  | Supported     | [AKS Setup](../guides/aks-setup.md)            |
| AWS CloudWatch   | Planned       | —                                               |
| GCP Pub/Sub      | Planned       | —                                               |

## Related

- [AKS Setup Guide](../guides/aks-setup.md) — End-to-end Azure Event Hub configuration
- [Ingestor Component](../components/ingestor.md) — Ingestion mode details
- [Pipeline](pipeline.md) — Stage-by-stage processing overview
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — `spec.cloud` field reference
- [Security Model](security-model.md) — Cloud trust boundaries
