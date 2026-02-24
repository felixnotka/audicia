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
  processing. Each provider has its own implementation (`EventHubSource` for Azure, `CloudWatchSource` for AWS,
  `PubSubSource` for GCP).
- **EnvelopeParser** — Unwraps the cloud-provider-specific JSON envelope and extracts audit events. Azure wraps
  events in `records[].properties.log`, AWS delivers raw audit JSON in CloudWatch log events, and GCP wraps events
  in Cloud Logging `LogEntry` objects with `protoPayload` containing the audit data.

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

Checkpoint behavior varies by provider:

- **Azure Event Hub**: Optionally supports Blob Storage-backed checkpoint persistence for distributed consumption.
  When `storageAccountURL` is configured, the Event Hub processor handles partition ownership and checkpointing
  automatically.
- **AWS CloudWatch**: Pull-based — the `startTime` parameter resumes from the last processed event timestamp.
  Implements the `CheckpointRestorer` interface to restore `startTime` before connecting.
- **GCP Pub/Sub**: Push-based — Pub/Sub manages delivery state. Individual messages are acknowledged after processing;
  unacknowledged messages are redelivered automatically.

## Build Tags

Cloud provider adapters are compiled conditionally using Go build tags. The default binary includes no cloud SDKs:

| Build Tag | Adapter            | SDK Dependencies                                                 |
|-----------|--------------------|------------------------------------------------------------------|
| `azure`   | Azure Event Hub    | `azeventhubs/v2`, `azidentity`, `azblob`                        |
| `aws`     | AWS CloudWatch     | `aws-sdk-go-v2/service/cloudwatchlogs`, `aws-sdk-go-v2/config`  |
| `gcp`     | GCP Pub/Sub        | `cloud.google.com/go/pubsub`                                    |

Build with all cloud adapters:

```bash
go build -tags azure,aws,gcp ./cmd/audicia/
```

Or with Docker:

```bash
docker build --build-arg GO_BUILD_TAGS=azure,aws,gcp -t audicia:cloud .
```

You can also build with a single provider tag if you only need one adapter.

## Supported Providers

| Provider         | Status    | Auth Mechanism                | Guide                                           |
|------------------|-----------|-------------------------------|------------------------------------------------|
| Azure Event Hub  | Supported | Azure Workload Identity       | [AKS Setup](../guides/aks-setup.md)            |
| AWS CloudWatch   | Supported | IRSA (IAM Roles for SA)       | [EKS Setup](../guides/eks-setup.md)            |
| GCP Pub/Sub      | Supported | Workload Identity Federation  | [GKE Setup](../guides/gke-setup.md)            |

All providers use managed identity for authentication — no static credentials or connection strings are stored in
CRD resources.

## Related

- [AKS Setup Guide](../guides/aks-setup.md) — End-to-end Azure Event Hub configuration
- [EKS Setup Guide](../guides/eks-setup.md) — End-to-end AWS CloudWatch configuration
- [GKE Setup Guide](../guides/gke-setup.md) — End-to-end GCP Pub/Sub configuration
- [Ingestor Component](../components/ingestor.md) — Ingestion mode details
- [Pipeline](pipeline.md) — Stage-by-stage processing overview
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — `spec.cloud` field reference
- [Security Model](security-model.md) — Cloud trust boundaries
