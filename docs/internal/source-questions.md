# Source Code Questions — Answered

All 10 questions have been answered and the documentation has been updated accordingly.

---

## 1. Kubernetes version compatibility ✅ CONFIRMED CORRECT
K8s 1.27-1.35 is accurate. Helm chart specifies `kubeVersion: ">=1.27.0-0"`, go.mod targets k8s libs v0.31.0.

**Action:** No change needed.

---

## 2. Filter OR-semantics ✅ CONFIRMED CORRECT (surprising but accurate)
It IS OR. Two independent `if` statements in `pkg/filter/filter.go` — either match triggers the rule.

**Action:** No change needed. The docs already describe this correctly in `components/filter.md`.

---

## 3. Deduplication key ✅ CONFIRMED CORRECT
5-tuple `(APIGroup, Resource, Verb, NonResourceURL, Namespace)`. `resourceName` is intentionally excluded.

**Action:** No change needed.

---

## 4. API group migration list ✅ DOCS CORRECTED
Only ONE migration exists: `extensions` → `apps`. The docs overstated 3 migrations.

**Action:** Fixed in `normalizer.md`, `architecture.md`, `features.md`.

---

## 5. NamespaceStrict with cluster-scoped resources ✅ DOCS CORRECTED
Cluster-scoped rules are merged into namespace Roles when namespaced rules exist. A ClusterRole IS generated when
the subject has ONLY cluster-scoped activity.

**Action:** Fixed in `strategy-engine.md`, `features.md`, `architecture.md`.

---

## 6. Compliance score when denominator is 0 ✅ DOCS CORRECTED
- 0 effective + 0 observed → Score 100, Green
- 0 effective + observed rules → nil (cannot evaluate)
- No division by zero.

**Action:** Added edge case tables to `compliance-scoring.md` and `compliance-engine.md`.

---

## 7. Checkpoint format and corruption ✅ DOCS CLARIFIED
Stored in `AudiciaSource.status` fields (etcd-backed): `fileOffset` (int64), `inode` (uint64), `lastTimestamp`
(RFC3339). Not a file. Corruption = API server unavailability, handled by `retry.RetryOnConflict`.
`lastTimestamp` parse failure is silently ignored.

**Action:** Added storage clarification to `features.md` §8.

---

## 8. kind container name bug ✅ DOCS FIXED
`kind-control-plane` was wrong — should be `audicia-dev-control-plane` when using `--name audicia-dev`.

**Action:** Fixed in `demo-walkthrough.md`.

---

## 9. Image tag default ✅ DOCS FIXED
Empty string defaults to `Chart.AppVersion`, not `latest`.

**Action:** Fixed in `helm-values.md`.

---

## 10. Metric pipeline position ✅ DOCS CLARIFIED
`audicia_events_processed_total` increments after filter + normalizer, before aggregator.
`result=accepted` means the event passed through and is policy-relevant.

**Action:** Added pipeline position note to `metrics.md`.
