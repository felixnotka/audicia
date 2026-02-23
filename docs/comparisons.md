# Competitive Comparison

## The Landscape

Kubernetes RBAC security tools fall into three categories: **scanners** (find problems), **enforcers** (block
violations), and **generators** (create correct policies). Audicia is the only tool that operates as a **continuous,
operator-native policy generator** from runtime audit data.

## Feature Comparison

| Capability                               | **Audicia** | **audit2rbac** |  **Trivy**  | **OPA/Gatekeeper** | **KubeAudit** |
|------------------------------------------|:-----------:|:--------------:|:-----------:|:------------------:|:-------------:|
| Generates RBAC from runtime behavior     |     Yes     |      Yes       |     No      |         No         |      No       |
| Kubernetes Operator (controller-runtime) |     Yes     |    No (CLI)    | No (CLI/CI) |        Yes         |   No (CLI)    |
| Stateful processing (checkpoint/resume)  |     Yes     |       No       |     N/A     |        N/A         |      N/A      |
| Incremental diffing                      |     Yes     |       No       |     No      |         No         |      No       |
| CRD output (GitOps-native)               |     Yes     | No (raw YAML)  |     No      |        Yes         |      No       |
| Subject normalization                    |     Yes     |    Partial     |     N/A     |        N/A         |      N/A      |
| Subresource handling (pods/exec)         |     Yes     |    Partial     |     N/A     |        N/A         |      N/A      |
| API group migration                      |     Yes     |       No       |     No      |        N/A         |      No       |
| Configurable policy strategy             |     Yes     |       No       |     N/A     |        N/A         |      N/A      |
| Webhook ingestion                        |     Yes     |       No       |     N/A     |        N/A         |      N/A      |
| Compliance evidence artifacts            |     Yes     |       No       |   Partial   |         No         |      No       |
| Static YAML scanning                     |     No      |       No       |     Yes     |         No         |      Yes      |
| Runtime policy enforcement               |     No      |       No       |     No      |        Yes         |      No       |

## Tool-by-Tool Analysis

### vs. audit2rbac

**What it is:** A CLI tool by @liggitt (Kubernetes core maintainer) that reads an audit log file and outputs RBAC
objects.

**Where it falls short for production use:**

- **No state.** Every run re-reads the entire audit log from the beginning. A 10GB audit log means 10GB of I/O per run.
- **No CRDs.** Output is raw YAML files dumped to disk. No Kubernetes-native way to track, version, or reconcile the
  output.
- **No diffing.** Cannot answer "what changed since last Tuesday?" — it regenerates everything from scratch.
- **No normalization.** Does not migrate deprecated API groups or handle edge cases in subject identity.
- **No strategy control.** No knobs for verb merging, wildcard policies, or scope control.

**Audicia's position:** audit2rbac is a useful script for one-time analysis. Audicia is the platform that runs
continuously in your cluster.

### vs. Trivy

**What it is:** An all-in-one security scanner (container images, IaC, SBOM, Kubernetes manifests).

**Why it's not competitive:** Trivy analyzes YAML at rest. It can detect `cluster-admin` bindings or missing security
contexts in your manifests. But it cannot observe what a workload *actually does* at runtime. A workload that only calls
`GET /api/v1/configmaps` will look identical to one that calls every API endpoint — until you look at the audit log.

**Audicia's position:** Trivy tells you what's wrong with your current policies. Audicia tells you what the correct
policies should be. Use both.

### vs. OPA / Gatekeeper

**What it is:** A policy engine that evaluates admission requests against Rego policies.

**Why it's complementary, not competitive:** OPA answers "should this request be allowed?" Audicia answers "what
requests have been made, and what RBAC should allow them?" You still need to *write* the OPA policies or RBAC rules.
Audicia writes them for you.

**Audicia's position:** Audicia generates policy. OPA enforces policy. The ideal stack uses both: Audicia generates
least-privilege Roles, and Gatekeeper ensures no one manually creates over-permissioned bindings.

### vs. KubeAudit

**What it is:** A CLI/CI tool that audits Kubernetes manifests for security best practices.

**Why it's different:** KubeAudit checks for known anti-patterns in static YAML (running as root, missing network
policies, etc.). It does not process audit logs or generate RBAC policies.

**Audicia's position:** Different problem domain. KubeAudit is a linter for manifests. Audicia is a runtime analyzer for
access patterns.

## Positioning Summary

Audicia occupies the **runtime analysis + policy generation** space. It analyzes live API access patterns from audit logs and generates correct RBAC policies. Other tools focus on different quadrants: Trivy and KubeAudit scan static YAML for known anti-patterns, OPA/Gatekeeper enforces policies at admission time, and audit2rbac is a one-shot CLI tool without state management or operator semantics.

No other tool operates as a Kubernetes-native controller with continuous processing, normalization, compliance scoring, and GitOps-ready CRD output.
