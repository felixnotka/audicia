# Introduction

## What is Audicia?

Audicia is a Kubernetes Operator that generates least-privilege RBAC policies from audit logs. It watches what your
workloads actually do — which API calls they make, which resources they access — and produces the minimal Roles and
RoleBindings to satisfy exactly that access.

```
403 Forbidden → Audit Event → Audicia → Role + RoleBinding → 200 OK
```

## The Problem

Every Kubernetes cluster has over-permissioned service accounts. Teams bind `cluster-admin` because writing correct
RBAC is too hard:

- The combinatorial surface area is enormous: API groups × resources × subresources × verbs × namespaces.
- Getting it wrong means either blocked workloads (too strict) or security vulnerabilities (too loose).
- Manual RBAC drifts out of sync as workloads evolve.

Audicia fixes this by observing actual API access patterns and generating the minimal policy that satisfies them.

## Key Concepts

### AudiciaSource

An `AudiciaSource` is a custom resource that tells Audicia where to find audit events. It supports two ingestion
modes:

- **File-based** (`K8sAuditLog`): Tails a Kubernetes audit log file on disk with checkpoint/resume.
- **Webhook** (`Webhook`): Receives real-time audit events via HTTPS from the kube-apiserver's audit webhook backend.

### AudiciaPolicyReport

An `AudiciaPolicyReport` is the output — one per subject (ServiceAccount, User, or Group). It contains:

- **Observed rules**: What API calls the subject actually made, with timestamps and counts.
- **Suggested policy**: Ready-to-apply YAML manifests (Role, ClusterRole, RoleBinding, ClusterRoleBinding).
- **Compliance score**: How well the subject's current RBAC matches its actual usage (Green/Yellow/Red).

### Compliance Scoring

Audicia resolves the subject's effective RBAC permissions (all bindings and roles) and compares them against observed
usage:

| Score    | Severity | Meaning                                  |
|----------|----------|------------------------------------------|
| >= 80%   | Green    | Tight permissions, little excess          |
| >= 50%   | Yellow   | Moderate overprivilege                    |
| < 50%    | Red      | Significant overprivilege                 |

It also flags sensitive excess — unused grants on high-risk resources like secrets, nodes, and webhookconfigurations.

## Supported Platforms

| Platform             | File Mode    | Webhook Mode |
|----------------------|--------------|--------------|
| kubeadm (bare metal) | Full support | Full support |
| k3s / RKE2           | Full support | Full support |
| EKS / GKE / AKS     | Not supported | Not supported |

Managed Kubernetes (EKS, GKE, AKS) does not expose apiserver flags or audit log files. Cloud-specific log ingestors
are planned for the future.

## What's Next

- [Installation](installation.md) — Prerequisites and Helm install
- [Quick Start: File Ingestion](quick-start-file.md) — Tail an audit log and generate your first report
- [Quick Start: Webhook Ingestion](quick-start-webhook.md) — Real-time audit events via HTTPS webhook
