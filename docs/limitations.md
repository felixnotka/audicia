# Known Limitations

Current limitations of Audicia, organized by area.

---

## RBAC Resolution

| Limitation | Impact |
|------------|--------|
| **Aggregated ClusterRoles** | The RBAC resolver does NOT follow label-selector-based aggregation. Permissions from aggregated roles may appear as "uncovered" in compliance reports. |
| **Group membership** | Audit events carry the username, not the group. Group-based compliance requires matching group bindings by name — group-to-binding attribution is ambiguous. |

---

## Policy Generation

| Limitation | Impact |
|------------|--------|
| **ResourceNames** | The `resourceNames: Explicit` option is defined in the CRD but not yet wired in the strategy engine output. Only `Omit` (default) is functional. |
| **Group subject extraction** | Audit events carry the username in `event.User.Username`, not the group. Group-based policy reports require a different input mechanism. |

---

## Compliance Scoring

| Limitation | Impact |
|------------|--------|
| **Wildcard counting** | `resources: ["*"]` counts as 1 excess rule, not N individual resources. A single broad grant shows as 1 unused rule even if it covers hundreds of resources. |
| **Non-resource URL matching** | Exact match only (no glob or prefix patterns). `/metrics` and `/metrics/cadvisor` are treated as distinct URLs. |

---

## Platform

| Limitation | Impact |
|------------|--------|
| **Managed Kubernetes** | EKS, GKE, and AKS don't expose apiserver flags or audit log files. Audicia would need cloud-specific log ingestors (CloudWatch, Cloud Logging, Azure Monitor) — not yet implemented. |
| **TLS cert rotation** | `ListenAndServeTLS` loads certificates at startup. For rotation without pod restart, `tls.Config.GetCertificate` would be needed (not yet implemented). |
| **Inode detection** | Log rotation detection uses `syscall.Stat_t` on Linux. On non-Linux platforms, inode detection is disabled — rotation falls back to file-not-found handling. |
