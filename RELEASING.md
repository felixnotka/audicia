# Releasing

## Versioning Policy

Audicia follows [Semantic Versioning](https://semver.org/) for the operator binary and Helm chart.

CRD API versions follow Kubernetes conventions:

| API Version | Stability    | Compatibility Guarantee                                                                     |
|-------------|--------------|---------------------------------------------------------------------------------------------|
| `v1alpha1`  | Experimental | Breaking changes may occur between minor releases. Migration guides will be provided.       |
| `v1beta1`   | Pre-stable   | Breaking changes only between major releases. Deprecation notices with 2-release lead time. |
| `v1`        | Stable       | No breaking changes within a major version.                                                 |

**Current API version: `v1alpha1`**

During `v1alpha1`, we aim for stability but reserve the right to make breaking CRD schema changes. When this happens:

1. The changelog will document the breaking change.
2. A migration guide will be provided.
3. Where possible, a conversion webhook or migration script will be included.

## Release Process

### Pre-release Checks

- [ ] All CI gates pass (lint, unit tests, e2e tests, vulnerability scans)
- [ ] CHANGELOG.md is updated with all changes since last release
- [ ] CRD schema changes are documented (if any)
- [ ] Helm chart version is bumped
- [ ] Container image builds and scans clean

### Release Pipeline

Releases are fully automated via GitHub Actions. Pushing to `main` triggers:

1. **Version computation** — `version.json` (Major.Minor) + auto-incremented patch from git tags
2. **Change detection** — Only operator/Helm changes trigger operator builds
3. **Lint and test** — Go linting (`golangci-lint`) and unit tests
4. **Docker build and push** — Multi-platform image with SBOM and provenance attestation
5. **Helm chart packaging** — Bundled as a GitHub Release artifact
6. **Git tag and GitHub Release** — Created automatically with generated release notes
7. **Site build and push** — Downloads all chart releases, regenerates `charts.audicia.io` index

Pushes to `develop` produce dev-tagged images (`X.Y.Z-dev.N`) without tags or releases.

### Planned (Not Yet Implemented)

- Cosign image signing (keyless/OIDC)

## Changelog

All notable changes are documented in [`docs/changelog.md`](docs/changelog.md).

Format follows [Keep a Changelog](https://keepachangelog.com/). Each release entry includes:

- **Added** — New features
- **Changed** — Changes in existing functionality
- **Deprecated** — Features that will be removed
- **Removed** — Removed features
- **Fixed** — Bug fixes
- **Security** — Vulnerability fixes
- **Breaking** — Changes that require user action (CRD migrations, config changes)

## Supported Kubernetes Versions

| Audicia Version | Kubernetes Versions | Audit Event Versions |
|-----------------|---------------------|----------------------|
| v0.1.x          | 1.27 – 1.35         | `audit.k8s.io/v1`    |

We test against a range of Kubernetes versions. Older versions may work but are not guaranteed.

## Helm Chart Versioning

The Helm chart version tracks independently from the operator version:

- **Chart version** (`Chart.yaml` → `version`): Bumped when chart templates, values, or defaults change.
- **App version** (`Chart.yaml` → `appVersion`): Tracks the operator container image tag.

Users pin to a chart version, which determines the operator version:

```bash
helm install audicia oci://charts.audicia.io/audicia-operator --version 0.4.0
```
