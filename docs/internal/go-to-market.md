# Go-to-Market Strategy

## Model: Open Source First

Audicia is **free and open source** (Apache 2.0). The priority is building a tool that people actually want to use.
Monetisation comes later, if at all — the focus right now is adoption and feedback.

### What Ships Free

Everything. There is no paid tier today.

- Kubernetes Operator (controller-runtime)
- `AudiciaSource` and `AudiciaPolicyReport` CRDs
- File and Webhook audit log ingestion (TLS + mTLS)
- Subject normalization and event normalization
- Policy strategy knobs (scopeMode, verbMerge, wildcards)
- RBAC compliance scoring (resolver + diff engine)
- Noise filtering (allow/deny chains)
- Helm chart

### Future Monetisation (Ideas, Not Plans)

If Audicia gains real traction, these are the kinds of features that could justify a paid tier someday:

- **Dashboard UI** — Visual policy browser and approval workflows
- **Compliance Exports** — PDF/CSV reports for SOC 2, ISO 27001 evidence
- **Multi-Cluster Aggregation** — Unified view across clusters
- **Priority Support** — SLA-backed support

None of these are being built right now. They're listed here so the architecture doesn't accidentally make them
impossible.

---

## Distribution

### Primary: Helm Chart

```bash
helm install audicia ./deploy/helm -n audicia-system --create-namespace
```

One command. Sensible defaults. Works out of the box with standard kube-apiserver audit log paths.

Once the project is stable enough, publish to a Helm repository:

```bash
helm repo add audicia https://charts.audicia.io
helm install audicia audicia/audicia-operator
```

### Secondary: OperatorHub.io

List on OperatorHub once the CRD API is stable (v1beta1+). Good for discovery by teams using OLM.

---

## Adoption Strategy

### Stage 1: Build Something Useful (Now)

Make the core loop work reliably: audit log → policy report → apply → verify.

- Get real-world testing on production clusters (kubeadm, EKS, GKE, etc.)
- Fix edge cases in normalization, filtering, and RBAC resolution
- Write clear documentation so someone can install and understand Audicia in 15 minutes
- Dogfood it — run Audicia on its own cluster

### Stage 2: Tell People About It

Once the tool is solid and the docs are good:

- Blog post: "Stop Writing RBAC by Hand"
- Post to r/kubernetes, CNCF Slack #security, Hacker News
- Record a short demo video showing the Red→Green compliance loop
- Submit a KubeCon lightning talk CFP

**Metric:** GitHub stars, issues from real users, Helm installs.

### Stage 3: Grow Based on Feedback

Let adoption guide what to build next. If people ask for multi-cluster support, build it. If nobody cares about
compliance exports, don't.

---

## Why Audicia Has a Shot

1. **Real Problem.** RBAC is painful. Everyone hand-writes it, nobody audits it, and it drifts silently. Audicia
   automates the part that nobody wants to do.
2. **Operator-Native.** CRD-based means it integrates with GitOps, monitoring, and existing K8s workflows natively.
   CLI tools can't compete on integration depth.
3. **Open Source.** Security teams don't deploy black-box binaries. Being open source builds the trust needed to run
   inside production clusters.
4. **Normalization Engine.** The subject/event normalization and RBAC resolution logic is non-trivial to replicate.
   Each edge case took real debugging to handle correctly.
