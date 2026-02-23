# Documentation Audit — Scores & Findings

Audited: 29 public-facing docs (32 total, 3 internal excluded from scoring).

Last updated after fix pass.

---

## 1. Readability — 8 / 10

**Strengths:**
- Consistent structure across all component pages (Purpose → Pipeline position → Key behaviours → Core functions → Related links).
- Good use of tables for quick-scan reference (Helm values, CRD fields, metrics, features).
- Conceptual docs (architecture, pipeline, compliance-scoring) are well-written with clear explanations and diagrams.
- Getting Started guides follow a logical progression: Introduction → Installation → Quick Start (File) → Quick Start (Webhook).
- Code blocks are appropriately used for commands and YAML examples.

**Weaknesses:**
- `guides/webhook-setup.md` is 817 lines — by far the longest file. Could benefit from splitting or adding a table of contents.
- The Mermaid diagrams in `architecture.md` may not render with `@deno/gfm` — verify.
- The ASCII trust boundary diagram in `security-model.md` is functional but hard to parse on narrow screens.

---

## 2. Duplication — 8 / 10 (was 6)

**Improved:** After the fix pass, compliance scoring content now has a single source of truth
(`concepts/compliance-scoring.md`). Architecture, compliance-engine, and features all cross-link to it instead of
repeating the formula, thresholds, and sensitive resource list.

**Remaining overlap (acceptable):**
- Filter chain behaviour is described at different detail levels in filter.md, filter-recipes.md, pipeline.md, and features.md — this is intentional (component vs guide vs overview vs reference).
- Webhook setup steps overlap between quick-start-webhook.md and webhook-setup.md — the quick start is a subset of the full guide, by design.

---

## 3. Correctness & Validity — 9 / 10 (was 7)

**Fixed:**
- All 12 broken external links resolved (SECURITY.md → security-model.md, README.md → internal docs links, examples/ → new examples page)
- API group migration table corrected (only `extensions` → `apps`, not 3 migrations)
- NamespaceStrict scope mode documentation corrected (can produce ClusterRoles when only cluster-scoped rules exist)
- Compliance score edge cases documented (zero-denominator behaviour)
- Demo walkthrough kind container name fixed (`audicia-dev-control-plane`)
- Image tag default corrected (empty string → appVersion, not `latest`)
- Metric pipeline position clarified (increments after filter + normalizer)
- Checkpoint storage clarified (CRD status fields in etcd, not a separate file)

**Remaining items to verify:**
- Kubernetes 1.35 version claim — confirmed correct by user.
- Mermaid diagram rendering — needs visual verification in browser.

---

## 4. Maintainability — 8 / 10 (was 7)

**Improved:**
- Compliance scoring has a single source of truth, reducing the 4-way update burden to 1.
- All examples are now on a dedicated page (`reference/examples.md`) — updates to examples only need changes in one place.
- All internal doc links resolve correctly; no more `../../` links escaping the docs directory.

**Remaining considerations:**
- `webhook-setup.md` at 817 lines is hard to maintain as a single file.
- No automated link checking in the build pipeline (could be added as a CI step).

---

## Summary

| Category | Before | After | Notes |
|----------|--------|-------|-------|
| Readability | 8/10 | **8/10** | Unchanged — structural improvements from the restructure hold |
| Duplication | 6/10 | **8/10** | Compliance scoring deduplicated; features.md trimmed |
| Correctness | 7/10 | **9/10** | 12 broken links fixed; 6 factual corrections; edge cases documented |
| Maintainability | 7/10 | **8/10** | Single source of truth for scoring; examples centralized |
| **Overall** | 7/10 | **8.25/10** | Significant improvement across all dimensions |
