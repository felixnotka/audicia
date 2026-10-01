# SonarQube rule decisions

`sonar-project.properties` points here for the reasoning behind every rule it
suppresses. A suppression is a claim that the rule is wrong for this code, not
that the finding is inconvenient, so each one below says why.

Coverage exclusions are listed at the end; they answer a different question -
what the coverage figure is allowed to describe.

## Suppressed rules

### `go:S100` - function names should match a naming convention

Scope: `**/*_test.go`

Go test names use underscores to separate the unit, the case and the
expectation: `TestResolve_MissingASN_FallsBackToBot`. That is the convention the
standard library and the wider ecosystem follow, and the rule's camel-case
expectation would make the names harder to read, not easier.

### `go:S1192` - string literals should not be duplicated

Scope: `**/*_test.go`

A table-driven test repeats the same literal across cases on purpose. Hoisting
them into constants moves the value away from the case that uses it, and reading
the case then requires a second lookup. Duplication is the cheaper trade here.

### `kubernetes:S6897` - persistent storage should be declared

Scope: `deploy/helm/templates/**`

The operator keeps no state of its own. It reads audit logs from a host path and
writes its findings to the Kubernetes API as custom resources, so there is
nothing for a PersistentVolumeClaim to hold.

### `kubernetes:S6596` - image tags should be pinned

Scope: `deploy/helm/templates/**`

The templates do not carry a tag at all; Helm renders one from `appVersion` or
from an explicit value at install time. The rule reads the unrendered template
and reports a value that never reaches a cluster.

### `docker:S8431` - use either the version tag or the digest

Scope: `**/Dockerfile`

Base images carry both on purpose:

    FROM denoland/deno:2.9.5@sha256:b429777c3dcff34a6488f365a1537db...

The digest is what pins the image - it is what the daemon resolves and what
makes the build reproducible. The tag is what makes the line readable and what
Renovate matches on to propose an update. The `docker:pinDigests` preset in
`renovate.json` maintains the pair together.

Dropping the digest loses the pin. Dropping the tag keeps the pin but leaves a
line nobody can read at a glance, and takes away the handle Renovate uses. The
rule assumes the tag is a redundant second source of truth; here it is
documentation attached to a pin.

## Coverage exclusions

Excluded from coverage rather than from analysis, so the code is still checked
for defects - only the coverage percentage ignores it:

- `site/**` - the site has no Go tests, and including it would report the whole
  front end as uncovered Go code.
- `deploy/helm/**`, `operator/build/**`, `operator/hack/**` - packaging and
  tooling, not runtime behaviour.
- `operator/pkg/apis/**`, `operator/pkg/metrics/**`, `operator/pkg/operator/**` -
  type declarations and wiring with no branching to cover.
- `operator/pkg/ingestor/cloud/{aws,azure,gcp}/**` and `registry.go` - the cloud
  adapters sit behind build tags and need real SDK credentials to exercise.

Duplication analysis additionally skips `**/*_test.go`, `site/**` and generated
`zz_generated.*.go` files.
