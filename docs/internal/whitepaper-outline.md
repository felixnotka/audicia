# Whitepaper Outline: Continuous Least-Privilege RBAC for Kubernetes

**Working title:** *Continuous Least-Privilege RBAC for Kubernetes: From Audit Logs to Automated Policy Generation*

**Target audience:** Platform engineers, security architects, DevOps leads, CISOs evaluating Kubernetes security
posture.

**Format:** ~50 pages, heavy on diagrams and graphics (~20 figures, roughly one visual every 2–3 pages).

**Distribution strategy:** Front-load educational content (Parts I and II) so it works as a standalone Kubernetes
security reference. Readers share it for the ecosystem overview; Audicia is positioned as the natural solution by the
time they reach Part III.

---

## Part I — The Problem (pages 1–12)

### 1. Executive Summary (1–2 pages)

One-page summary for decision-makers. The core thesis: manual RBAC doesn't scale, static scanning can't see runtime
behavior, and the Kubernetes operator pattern enables a new category of continuous policy generation.

### 2. The State of Kubernetes Access Control (3–4 pages)

- The adoption curve: K8s in production is no longer optional
- RBAC as the primary authorization mechanism — how it works at the API server level
- **Diagram:** API request flow through authentication → authorization (RBAC) → admission → execution
- The combinatorial surface area: API groups × resources × subresources × verbs × namespaces
- **Diagram:** Explosion visual showing how a single microservice with 3 resources creates dozens of rule combinations

### 3. Why RBAC Fails in Practice (3–4 pages)

- The "403 → cluster-admin" cycle (with a realistic timeline diagram)
- Real-world incidents: cryptojacking via over-permissioned SAs, lateral movement via namespace-crossing bindings
- **Diagram:** Attack path visualization — compromised pod → excessive RBAC → secret access → cluster takeover
- The compliance gap: SOC 2, ISO 27001, PCI DSS, FedRAMP all require least-privilege evidence that teams can't produce
- **Table:** Compliance framework mapping — which controls require least-privilege, what evidence is needed

### 4. The Current Tooling Landscape (3–4 pages)

- Three categories: scanners, enforcers, generators
- **Diagram:** 2×2 quadrant — static vs. runtime analysis × scanning vs. generation
- Tool-by-tool analysis: audit2rbac, Trivy, OPA/Gatekeeper, KubeAudit, KubeLinter
- **Table:** Feature comparison matrix
- The gap: no tool does continuous, operator-native, stateful policy generation

---

## Part II — The Operator Pattern (pages 13–22)

### 5. The Kubernetes Operator Pattern (3–4 pages)

- What an operator is: extending the Kubernetes control plane with domain-specific automation
- The reconciliation loop: desired state → observe → diff → act
- **Diagram:** Generic operator reconciliation loop with custom resource → controller → managed resources
- Why operators matter for security: self-healing, declarative, auditable
- controller-runtime as the foundation — how it provides leader election, caching, event handling

### 6. Custom Resource Definitions as API Contracts (2–3 pages)

- CRDs as the extension mechanism: turning domain objects into first-class Kubernetes citizens
- The lifecycle: define schema → register with API server → reconcile with controller
- **Diagram:** CRD registration flow and how kubectl, client-go, and informers interact with custom resources
- Why CRD output matters: GitOps compatibility, `kubectl get`, RBAC on the CRD itself, owner references for garbage
  collection

### 7. Audit Logging as a Security Data Source (3–4 pages)

- What Kubernetes audit logging captures: every authenticated API request
- Audit levels: None, Metadata, Request, RequestResponse — what each provides
- **Diagram:** Audit event lifecycle — API request → audit backend → log file / webhook
- The audit policy: how to configure what gets logged without drowning in data
- **Table:** Key fields in an audit event and what each tells you (user, verb, objectRef, responseStatus, auditID)
- Why audit logs are underutilized: most teams enable them for compliance and never read them

---

## Part III — How Audicia Works (pages 23–38)

### 8. Architecture Overview (3–4 pages)

- The data pipeline model: Ingest → Filter → Normalize → Aggregate → Strategy → Report
- **Diagram:** Full pipeline architecture with annotations per stage
- Two CRDs: AudiciaSource (input configuration) and AudiciaPolicyReport (output)
- Design principles: never auto-apply, CRD-native, minimal operator permissions, open core

### 9. Ingestion: File and Webhook Sources (2–3 pages)

- File ingestion: tailing the audit log with checkpoint/resume
- Checkpoint mechanics: file offset + inode tracking for restart resilience
- **Diagram:** File ingestion state machine — open → tail → checkpoint → rotation detection → re-open
- **Webhook ingestion (implemented):** real-time events via the Kubernetes audit webhook backend
- mTLS support: client certificate verification with configurable CA bundle
- Deduplication by auditID, rate limiting, backpressure handling

### 10. The Normalization Pipeline (3–4 pages)

- Subject normalization: parsing ServiceAccount, User, and Group identities from the username string
- **Diagram:** Subject identity parsing tree — `system:serviceaccount:ns:name` → Kind=ServiceAccount, Namespace=ns,
  Name=name
- Event normalization: API group migration (extensions/v1beta1 → apps/v1), subresource concatenation (pods + exec →
  pods/exec)
- **Table:** Common API group migrations and why they matter
- Non-resource URL detection: /metrics, /healthz, /api paths

### 11. Noise Filtering (2–3 pages)

- The signal-to-noise problem: system components generate 90%+ of audit events
- Configurable allow/deny filter chains with first-match-wins semantics
- System user filtering: automatically excluding kube-scheduler, kube-controller-manager, etc. while preserving service
  accounts
- **Diagram:** Filter chain evaluation flow — event enters → match first rule → allow/deny → next rule → default allow
- Practical filter recipes: system-noise-only, namespace-allowlist, single-team focus

### 12. Rule Aggregation and Compaction (2–3 pages)

- From raw events to deduplicated rules: grouping by (subject, apiGroup, resource, namespace, verb)
- Tracking metadata: firstSeen, lastSeen, count for each rule
- Compaction under limits: maxRulesPerReport, retentionDays — keeping the most recently active rules
- **Diagram:** Aggregation pipeline — 10,000 raw events → 500 unique rules → 50 compacted rules per report

### 13. The Policy Strategy Engine (3–4 pages)

- Configurable knobs that control the shape of generated RBAC
- **scopeMode:** NamespaceStrict (per-namespace Roles) vs. ClusterScopeAllowed (ClusterRoles for Users/Groups)
- **Diagram:** Side-by-side comparison of NamespaceStrict vs. ClusterScopeAllowed output for the same input
- **verbMerge:** Smart (merge same-resource rules) vs. Exact (one rule per verb)
- **wildcards:** Forbidden (never emit `*`) vs. Safe (collapse when all 8 verbs observed)
- Per-namespace Role generation for ServiceAccounts accessing cross-namespace resources
- **Diagram:** ServiceAccount in namespace A accessing resources in namespaces B and C → Role in B + Role in C
- PolicyRule deduplication: how ObservedRules (with namespace) become PolicyRules (without) without duplicates

### 14. RBAC Compliance Scoring (3–4 pages)

- **This is implemented.** The diff engine resolves effective RBAC permissions for each subject and compares them
  against
  observed usage
- Score formula: usedEffective / totalEffective * 100
- Green (≥80%) / Yellow (≥50%) / Red (<50%) severity thresholds
- Sensitive excess detection: secrets, nodes, webhookconfigurations, CRDs, tokenreviews
- **Diagram:** Effective permissions vs. observed usage Venn diagram — used, excess, uncovered
- **Table:** Example compliance report for a real-world ServiceAccount showing score breakdown

### 15. The Output: AudiciaPolicyReport (2–3 pages)

- Report structure: status.observedRules + status.suggestedPolicy.manifests + status.compliance + conditions
- Each report is a timestamped, diffable evidence artifact with compliance scoring
- **Code listing:** Example AudiciaPolicyReport YAML showing observedRules, generated Role+RoleBinding, and compliance
- Integration points: kubectl, GitOps (ArgoCD/Flux), future dashboard and PR automation
- **Diagram:** Review-and-apply workflow — Report generated → human reviews → kubectl apply / GitOps merge → RBAC active

---

## Part IV — Operations and Compliance (pages 41–48)

### 16. Deployment and Operations (2–3 pages)

- Helm chart installation: single command, sensible defaults
- Security context: running as non-root, read-only access to audit logs
- **Table:** Helm values reference — key configuration options
- Observability: Prometheus metrics (events processed, reports updated, pipeline latency, checkpoint lag)
- Leader election for HA deployments

### 17. Compliance as a Continuous Process (3–4 pages)

- The traditional audit cycle: quarterly evidence scramble with screenshots and spreadsheets
- How AudiciaPolicyReports serve as continuous compliance evidence
- **Table:** Mapping Audicia outputs to compliance controls — SOC 2 CC6.1, ISO 27001 A.8.3, PCI DSS Req 7, NIST AC-6
- **Diagram:** Traditional compliance cycle (quarterly panic) vs. Audicia continuous compliance (always audit-ready)
- Evidence chain: audit log → AudiciaSource → AudiciaPolicyReport → applied Role — every step is timestamped and
  in-cluster

### 18. Security Model (1–2 pages)

- Audicia's own RBAC footprint: what permissions does the operator need?
- Principle: the tool that generates least-privilege should itself be least-privileged
- No Secrets access, no Impersonation, no cluster-admin
- **Table:** Audicia's own ClusterRole — the minimal set of permissions it requires
- Supply chain: container image signing, SBOM, vulnerability scanning

---

## Part V — Vision (pages 49–52)

### 19. Roadmap: From Policy Generator to Access Control Plane (2–3 pages)

- **Completed:** Full pipeline, webhook ingestion, mTLS, compliance scoring, drift detection
- Next: Dashboard UI (Hubble pattern), GitOps PR automation, anomaly alerting
- Future: Multi-cluster federation, compliance report exports, NetworkPolicy generation
- **Diagram:** Evolution timeline — Generate → Detect (done) → Govern → Platform

### 20. Getting Started (1 page)

- Quick start commands
- Links to documentation, GitHub repo, community channels
- Call to action: install, try on a dev cluster, contribute

---

## Appendices

- **A.** Kubernetes Audit Policy reference configuration
- **B.** Full AudiciaSource CRD spec reference
- **C.** Full AudiciaPolicyReport CRD spec reference
- **D.** Glossary of terms

---

## Diagram / Graphic Summary

| #  | Section | Diagram                                                          | Type                 |
|----|---------|------------------------------------------------------------------|----------------------|
| 1  | §2      | API request flow: authn → authz (RBAC) → admission → execution   | Flow diagram         |
| 2  | §2      | RBAC combinatorial explosion: 3 resources × verbs × subresources | Infographic          |
| 3  | §3      | The 403 → cluster-admin cycle timeline                           | Timeline             |
| 4  | §3      | Attack path: compromised pod → excessive RBAC → cluster takeover | Attack tree          |
| 5  | §4      | Tooling quadrant: static/runtime × scanning/generation           | 2×2 matrix           |
| 6  | §5      | Operator reconciliation loop                                     | Loop diagram         |
| 7  | §6      | CRD registration and informer interaction flow                   | Sequence diagram     |
| 8  | §7      | Audit event lifecycle: API request → backend → log/webhook       | Flow diagram         |
| 9  | §8      | Full Audicia pipeline architecture                               | Architecture diagram |
| 10 | §9      | File ingestion state machine                                     | State machine        |
| 11 | §10     | Subject identity parsing tree                                    | Decision tree        |
| 12 | §11     | Filter chain evaluation flow                                     | Flowchart            |
| 13 | §12     | Aggregation funnel: 10K events → 500 rules → 50 compacted        | Funnel visual        |
| 14 | §13     | NamespaceStrict vs. ClusterScopeAllowed side-by-side             | Comparison           |
| 15 | §13     | Cross-namespace SA Role generation                               | Architecture diagram |
| 16 | §14     | Review-and-apply workflow                                        | Workflow diagram     |
| 17 | §16     | Quarterly panic vs. continuous compliance                        | Before/after         |
| 18 | §16     | Evidence chain: audit log → source → report → Role               | Chain diagram        |
| 19 | §18     | Roadmap evolution timeline                                       | Timeline             |
| 20 | §19     | Getting started flow                                             | Step diagram         |
