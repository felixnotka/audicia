# Contributing to Audicia

Thank you for your interest in contributing to Audicia. This document explains how to get involved.

## Ways to Contribute

- **Bug reports** — Open an issue with steps to reproduce.
- **Feature requests** — Open an issue describing the use case, not just the desired feature.
- **Code contributions** — Fix bugs, add features, improve tests.
- **Documentation** — Fix typos, improve explanations, add examples.
- **Testing** — Run Audicia against different Kubernetes versions and audit log formats. Report what works and what
  doesn't.

## Development Setup

### Prerequisites

**Operator:**
- Go 1.22+
- Docker (for building container images)
- `kind` or `minikube` (for local Kubernetes cluster)
- `kubectl`
- `helm`
- `controller-gen` (for CRD generation: `go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest`)
- `golangci-lint` (for linting: see [install guide](https://golangci-lint.run/welcome/install/))

**Site:**
- Deno 2.5.6+

### Project Structure

```
audicia/
├── operator/                 # Go operator (module, Dockerfile, Makefile, tests)
│   ├── cmd/audicia/          # Operator entrypoint (main.go)
│   ├── pkg/
│   │   ├── apis/audicia.io/  # CRD type definitions
│   │   ├── operator/         # Manager setup, config
│   │   ├── controller/       # Reconciler(s)
│   │   ├── ingestor/         # Audit log readers (file, webhook)
│   │   ├── normalizer/       # Subject + event normalization
│   │   ├── filter/           # Allow/deny filter chain
│   │   ├── aggregator/       # Rule deduplication
│   │   ├── strategy/         # Policy generation engine
│   │   ├── rbac/             # RBAC resolver (effective permissions)
│   │   ├── diff/             # Compliance diff engine
│   │   └── metrics/          # Prometheus metrics
│   ├── build/                # Dockerfile
│   ├── go.mod, go.sum        # Go dependencies
│   ├── Makefile              # Build, test, deploy targets
│   └── .golangci.yml         # Linter config
├── site/                     # Website (Deno/Fresh, serves audicia.io + charts.audicia.io)
│   ├── routes/               # File-system routing (landing, docs, blog)
│   ├── lib/                  # Docs loader, charts middleware, logging
│   ├── components/           # Preact components
│   └── blog/                 # Blog posts (markdown)
├── docs/                     # Documentation (single source of truth for operator + site)
│   └── examples/             # Example manifests (markdown with inline YAML)
├── deploy/helm/              # Helm chart
└── marketing/                # Brand assets, website copy
```

### Building the Operator

```bash
git clone https://github.com/felixnotka/audicia.git
cd audicia/operator
make build
```

### Running Operator Tests

```bash
cd operator
make test          # Unit tests
make test-e2e      # End-to-end tests (requires a running cluster)
make lint          # Linting (golangci-lint)
```

### Running Locally

```bash
cd operator

# Start a local cluster with audit logging enabled
make kind-cluster

# Install CRDs
make install-crds

# Run the operator locally (outside the cluster)
make run
```

### Running the Site Locally

```bash
cd site
deno task dev      # Serves at http://localhost:8000
```

## Pull Request Process

1. **Fork and branch.** Create a feature branch from `main`.
2. **Keep it focused.** One PR per feature or fix. Small PRs get reviewed faster.
3. **Write tests.** New features need unit tests. Bug fixes need regression tests.
4. **Run checks locally.** `make test && make lint` must pass before submitting.
5. **Write a clear description.** Explain *what* changed and *why*. Link to the issue if one exists.
6. **Write a clear commit message.** Follow the format below.

### Commit Messages

```
component: short description of change

Longer explanation of why this change is needed, what it does,
and any important design decisions.
```

Examples of good component prefixes: `ingestor:`, `normalizer:`, `aggregator:`, `helm:`, `docs:`, `ci:`, `site:`, `controller:`.

## Testing

### Running Tests

```bash
cd operator
go test ./pkg/...          # All unit tests
go test ./pkg/rbac/...     # RBAC resolver tests
go test ./pkg/diff/...     # Compliance diff engine tests
go test ./pkg/strategy/... # Policy strategy tests
```

### Test Patterns

- **Fake client builder:** Tests that interact with the Kubernetes API use `fake.NewClientBuilder()` from
  `sigs.k8s.io/controller-runtime/pkg/client/fake`. This provides an in-memory client with real scheme registration.
- **Table-driven tests:** Most test files use Go table-driven tests for comprehensive coverage of edge cases.
- **Pure function testing:** The diff engine (`operator/pkg/diff/`) is a pure function with no I/O — tests are fast and
  deterministic.

### CI vs Local

The project CI runs `go test ./pkg/...` and `golangci-lint` on every PR from the `operator/` directory. If you don't
have a local Go toolchain, you can rely on CI for test validation — just ensure your changes are consistent with the
existing test patterns.

## Code Review

All submissions require review before merging. Reviewers will look for:

- Correctness — Does it handle edge cases?
- Tests — Are new paths covered?
- Simplicity — Is there a simpler way to achieve this?
- Consistency — Does it follow existing patterns in the codebase?

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). Be respectful,
constructive, and inclusive.

## License

By contributing to Audicia, you agree that your contributions will be licensed under the Apache License 2.0.

## Questions?

- Open a GitHub Discussion for general questions.
- For security issues, see [SECURITY.md](SECURITY.md).
