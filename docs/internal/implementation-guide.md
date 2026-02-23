# Building a Kubernetes Operator from Scratch

How to go from an empty directory to a production-grade Kubernetes operator. This guide uses Audicia as the running
example, but the process applies to any operator you'll ever build. It's written for you — not contributors, not
investors. It's the playbook you wish someone had given you.

---

## The Big Picture

Building an operator has 10 stages. Most people jump straight to code and get stuck at stage 6 when nothing fits
together. Don't do that.

```
 1. Define the Problem           — What does the operator do? What does it NOT do?
 2. Design the CRDs              — Your API. Gets locked in early. Think hard here.
 3. Set Up the Repository        — Go module, directory structure, tooling.
 4. Implement the Types          — Go structs with kubebuilder markers.
 5. Build the Data Pipeline      — Your business logic, without Kubernetes.
 6. Wire the Controller          — Connect pipeline to the reconcile loop.
 7. Test Locally                 — kind cluster, end-to-end proof.
 8. Package for Distribution     — Helm chart, container image, RBAC.
 9. Harden                       — Tests, edge cases, observability.
10. Ship                         — CI/CD, release process, docs.
```

Stages 1-2 are design. 3-4 are scaffolding. 5-7 are the core build. 8-10 are productionization. Most of the real
thinking happens in 1-2 and 5-6. Everything else is mechanical.

---

## Stage 1: Define the Problem (1-2 days)

Before you write any code, answer these questions in a document. If you can't answer them clearly, you're not ready to
build.

### What to write down

**The problem statement (2-3 paragraphs):**

- What is the human pain you're solving?
- Who feels that pain? (persona: "a platform engineer at a company with 50+ microservices")
- What do they do today, and why is it bad?

**Goals (bullet list, 4-6 items):**

- What the operator WILL do. Be specific. Not "manages RBAC" but "generates least-privilege Role and RoleBinding
  manifests from Kubernetes audit log events."

**Non-goals (bullet list, 3-5 items):**

- What the operator will NOT do. This is more important than the goals. It's where you draw the boundary. Every operator
  has scope creep pressure — non-goals are your defense.
- The most important non-goal for security-sensitive operators: "Does not auto-apply generated resources."
  Human-in-the-loop is a feature, not a limitation.

**The core loop (one sentence):**

- Describe the fundamental value cycle in one sentence. For Audicia: "Observe denied API requests in audit logs,
  generate the minimal RBAC policy that would have allowed them, output as a CRD for human review."
- If you can't say it in one sentence, your operator does too much.

**Differentiation:**

- What exists today that partially solves this? (tools, scripts, manual processes)
- Why is an operator the right form factor? (needs to be continuous, needs cluster state, needs to watch something)

### Output

A `PROBLEM_STATEMENT.md` or `idea.md` file. Keep it under 2 pages. If it's longer, you're over-designing.

### Audicia example

Audicia's core loop: `403 Forbidden → Audit Event → Audicia → Role + RoleBinding → 200 OK`

Key non-goals that shaped every subsequent decision:

- Does not apply policies (no privilege escalation risk)
- Does not require cluster-admin (reads audit logs, writes CRDs)
- Does not replace enforcement tools (OPA/Gatekeeper handle enforcement)

---

## Stage 2: Design the CRDs (2-3 days)

This is the most important stage. Your CRDs are your API. Once users depend on them, they're extremely hard to change.
Spend more time here than feels comfortable.

### The Kubernetes API conventions you must follow

Read these before designing anything:

- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [CRD Versioning](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/)

The critical rules:

**1. Spec vs Status**

- `.spec` = what the user wants (desired state, configuration, identity)
- `.status` = what the operator observed (current state, generated output, conditions)
- Users write spec. The operator writes status. Never the other way around.
- If you put operator-generated data in spec, you've created a fight between the user and the operator over who owns the
  field.

**2. One CRD per concern**

- Don't put input config and output data in the same CRD unless they're truly about the same object.
- Audicia uses two CRDs: `AudiciaSource` (input: where to read, how to generate) and `AudiciaPolicyReport` (output: what
  was observed, what policy to suggest).

**3. Name your CRDs well**

- Include your project name to avoid collisions: `AudiciaSource`, not `Source`
- Use singular PascalCase: `AudiciaSource`, not `AudiciaSources`
- Pick a unique API group: `audicia.io`, not `audicia.k8s.io` (the k8s.io suffix is reserved)

**4. Think about the status subresource**

- Always use `+kubebuilder:subresource:status`. This lets the operator update status independently from spec, and
  prevents users from accidentally overwriting operator data.
- Status updates go through a separate API endpoint (`/status`), so RBAC is more granular.

### How to design your CRDs

**Step 1: Write example YAML first, not Go code.**

Before any Go types, write the YAML manifests that a user would create and the YAML the operator would produce. Put them
in `docs/examples/`. This forces you to think about the user experience:

```yaml
# What the user creates:
apiVersion: yourproject.io/v1alpha1
kind: YourInput
metadata:
  name: my-config
spec:
# configuration fields...

# What the operator produces (in .status):
status:
  observedThings: [ ... ]
  generatedOutput: "..."
  conditions:
    - type: Ready
      status: "True"
```

**Step 2: Identify every knob.**

List every configuration option the user might need. Group them:

- Required fields (the minimum to be useful)
- Optional fields with sensible defaults
- Advanced fields most users will never touch

Use `+kubebuilder:default=` markers for optional fields. Use `+kubebuilder:validation:Required` for required ones. Use
enums (`+kubebuilder:validation:Enum=`) wherever there's a fixed set of choices.

**Step 3: Design conditions.**

Every CRD should have a `conditions` field in status. Use standard condition types:

- `Ready` — the resource is fully operational
- `Progressing` — the operator is working on it
- `Degraded` — partially working but not fully healthy

Conditions have `Type`, `Status` (True/False/Unknown), `Reason` (machine-readable), `Message` (human-readable), and
`ObservedGeneration` (which version of spec this condition reflects).

**Step 4: Version your API as v1alpha1.**

Always start with `v1alpha1`. This signals to users that the API may change. You can promote to `v1beta1` and then `v1`
as it stabilizes. Don't start at `v1` — you will want to change things.

### Pitfalls

- **Don't put secrets in your CRD spec.** Reference Kubernetes Secrets by name instead:
  `tlsSecretName: "my-tls-secret"`, not `tlsCert: "base64data..."`.
- **Don't store unbounded lists in status.** If your operator could generate thousands of items, add limits. Kubernetes
  etcd has a ~1.5MB per-object limit. Design a `maxItems` field.
- **Don't use arbitrary maps.** `map[string]string` is hard to validate and document. Use typed structs.
- **Do add print columns.** `+kubebuilder:printcolumn` markers make `kubectl get` output useful instead of just showing
  name and age.
- **Do add short names.** `+kubebuilder:resource:shortName={as,asrc}` so users can type `kubectl get as` instead of
  `kubectl get audiciasources`.

### Output

- Example YAML files in `docs/examples/`
- A mental model of your CRDs (you'll write the Go types in stage 4)
- An `ARCHITECTURE.md` describing the data flow

---

## Stage 3: Set Up the Repository (half day)

### Prerequisites on your machine

```bash
# Go 1.22+
# https://go.dev/dl/

# controller-gen — generates DeepCopy and CRD YAML from Go type markers
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# golangci-lint — Go linter
# https://golangci-lint.run/welcome/install/

# kind — local Kubernetes clusters in Docker
# https://kind.sigs.k8s.io/docs/user/quick-start/#installation

# Helm 3 — Kubernetes package manager
# https://helm.sh/docs/intro/install/

# kubectl
# https://kubernetes.io/docs/tasks/tools/

# Docker (or Podman)
# https://docs.docker.com/get-docker/
```

### Initialize the repo

```bash
mkdir myoperator && cd myoperator
git init

# Go module — use your real GitHub org
go mod init github.com/yourorg/myoperator

# Create the directory structure
mkdir -p cmd/myoperator
mkdir -p pkg/apis/yourproject.io/v1alpha1
mkdir -p pkg/operator
mkdir -p pkg/controller/yourresource
mkdir -p build
mkdir -p deploy/helm
mkdir -p hack
mkdir -p tests/e2e
mkdir -p examples
```

### Directory layout convention

Follow the pattern used by established operators (trivy-operator, cert-manager, etc.):

```
myoperator/
├── cmd/myoperator/          # Entrypoint. Thin main.go.
│   └── main.go
├── pkg/                     # All Go packages (not internal/)
│   ├── apis/project.io/     # CRD type definitions
│   │   └── v1alpha1/
│   ├── operator/            # Manager setup, config
│   ├── controller/          # Reconcilers (one per CRD)
│   └── ...                  # Your business logic packages
├── deploy/helm/             # Helm chart
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── crds/                # Generated CRD YAMLs
│   └── templates/
├── build/
│   └── Dockerfile           # Container image
├── hack/
│   └── boilerplate.go.txt   # License header for generated files
├── docs/examples/           # Example CRs for users (markdown with inline YAML)
├── tests/e2e/               # Integration/e2e tests
├── Makefile                 # Build automation
├── .golangci.yml            # Linter config
└── .gitignore
```

**Why `pkg/` and not `internal/`?**

`internal/` prevents other Go modules from importing your packages. Operators in the ecosystem (trivy-operator, kyverno)
use `pkg/` because:

- It allows building plugins or extensions that import your types
- Users can write tools that import your API types
- It doesn't hurt you — if you don't want people importing something, just don't document it

### Key files to create

**`.gitignore`:**

```gitignore
bin/
dist/
vendor/
coverage.txt
*.exe
*.dll
*.so
*.dylib
*.test
.idea/
.vscode/
.DS_Store
Thumbs.db
```

**`hack/boilerplate.go.txt`** (used by controller-gen for generated file headers):

```
/*
Copyright <YEAR> <Your name or org>.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/
```

**`Makefile`** (start minimal, expand as needed):

```makefile
BINARY_NAME := myoperator
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
IMG ?= ghcr.io/yourorg/myoperator:$(VERSION)

.PHONY: build
build: fmt vet
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/myoperator/

.PHONY: run
run: fmt vet
	go run -ldflags "$(LDFLAGS)" ./cmd/myoperator/

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

.PHONY: generate
generate:
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/..."
	controller-gen crd paths="./pkg/apis/..." output:crd:artifacts:config=deploy/helm/crds

.PHONY: install-crds
install-crds: generate
	kubectl apply -f deploy/helm/crds/

.PHONY: docker-build
docker-build: build
	docker build -t $(IMG) -f build/Dockerfile .
```

**`build/Dockerfile`:**

```dockerfile
FROM alpine:3.21
RUN adduser -D -u 10000 operator
COPY bin/myoperator /usr/local/bin/myoperator
USER 10000
ENTRYPOINT ["myoperator"]
```

The container image is tiny — Alpine base, pre-built binary, non-root user. You build the Go binary on the host (in the
Makefile), then COPY it in. This avoids multi-stage builds and keeps the image small.

### Output

- Repo with directories, Makefile, Dockerfile, .gitignore
- `go mod init` done
- Initial commit

---

## Stage 4: Implement the Types (1 day)

Now you turn your CRD design from stage 2 into Go code.

### The four files you need per API version

```
pkg/apis/yourproject.io/v1alpha1/
├── doc.go                        # Package-level markers
├── register.go                   # Scheme registration
├── types_yourinput.go            # Your input CRD types
└── types_youroutput.go           # Your output CRD types
```

**`doc.go`** — tells controller-gen to generate code for this package:

```go
// +kubebuilder:object:generate=true
// +groupName=yourproject.io
package v1alpha1
```

**`register.go`** — registers your types with the Kubernetes scheme:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "yourproject.io", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&YourInput{},
		&YourInputList{},
		&YourOutput{},
		&YourOutputList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
```

**Type files** — your CRD Go structs with kubebuilder markers:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={yi}
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[0].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type YourInput struct {
metav1.TypeMeta   `json:",inline"`
metav1.ObjectMeta `json:"metadata,omitempty"`
Spec   YourInputSpec   `json:"spec,omitempty"`
Status YourInputStatus `json:"status,omitempty"`
}
```

### Kubebuilder markers cheat sheet

| Marker                                | Where            | What it does                    |
|---------------------------------------|------------------|---------------------------------|
| `+kubebuilder:object:root=true`       | Above type       | Marks as a root CRD type        |
| `+kubebuilder:subresource:status`     | Above type       | Enables `/status` subresource   |
| `+kubebuilder:resource:shortName={x}` | Above type       | Adds `kubectl get x` shorthand  |
| `+kubebuilder:printcolumn:...`        | Above type       | Columns in `kubectl get` output |
| `+kubebuilder:validation:Required`    | Above field      | Field must be set               |
| `+kubebuilder:validation:Enum=A;B;C`  | Above type/field | Restricts to enum values        |
| `+kubebuilder:default=value`          | Above field      | Default value if not specified  |
| `+kubebuilder:validation:Minimum=1`   | Above field      | Minimum number                  |
| `+kubebuilder:validation:MinLength=1` | Above field      | Minimum string length           |
| `+kubebuilder:object:generate=true`   | In doc.go        | Generates DeepCopy methods      |
| `+groupName=yourproject.io`           | In doc.go        | Sets the API group              |

### Generate and verify

```bash
# Generate DeepCopy methods and CRD YAMLs
make generate

# Should create:
# pkg/apis/yourproject.io/v1alpha1/zz_generated.deepcopy.go
# deploy/helm/crds/yourproject.io_yourinputs.yaml
# deploy/helm/crds/yourproject.io_youroutputs.yaml

# Verify it compiles
go build ./...

# Inspect the generated CRD
cat deploy/helm/crds/yourproject.io_yourinputs.yaml
```

If `controller-gen` fails, the issue is almost always:

- Missing `+kubebuilder:object:root=true` on your root type
- Missing `TypeMeta`/`ObjectMeta` embedding
- Missing `List` types
- Type referenced in a field doesn't have DeepCopy (add `+kubebuilder:object:generate=true`)

### Output

- Go type files with full kubebuilder markers
- `make generate` produces CRD YAMLs
- `go build ./...` passes

---

## Stage 5: Build the Data Pipeline (3-5 days)

This is where you write your business logic. The key insight: **build it without Kubernetes first.**

Every operator has a data pipeline — input → processing → output. Build each stage as an independent Go package with
clear interfaces. Test each one in isolation. Don't touch controller-runtime until stage 6.

### Why this order matters

If you write the controller first and the business logic inside it, you get:

- Logic coupled to Kubernetes types and the reconcile loop
- No way to unit test without a cluster
- Spaghetti code where pipeline stages bleed into each other

If you build the pipeline first and the controller last, you get:

- Each stage testable with plain Go tests
- Clear interfaces between stages
- The controller becomes a thin wiring layer

### Designing your pipeline

Draw your data flow:

```
Input → [Stage 1] → [Stage 2] → [Stage 3] → Output
```

For Audicia:

```
Audit Log → Ingestor → Normalizer → Filter → Aggregator → Strategy → RBAC YAML
```

Each stage is a Go package with:

- A clear interface or public function
- Its own types (if needed)
- No dependency on Kubernetes client or controller-runtime (except for API types)

### Build order: inside out

Start with the stages that have **no dependencies on external systems** and work outward.

**Tier 1 — Pure logic, no I/O:**
These are pure functions. Input → output. Easiest to build and test.

- Normalizers (parse/transform data)
- Filters (accept/reject based on rules)
- Aggregators (combine/deduplicate)
- Strategy/policy engines (apply rules to generate output)

**Tier 2 — I/O adapters:**
These talk to the filesystem, network, or external systems. Harder to test (need mocks or temp files).

- File readers/tailers
- Webhook receivers
- External API clients

**Tier 3 — Kubernetes integration:**
This is the controller. Don't touch it until tiers 1 and 2 work.

### Writing each stage

For each pipeline stage:

1. **Define the interface first.** What goes in, what comes out.

```go
// Don't start with implementation. Start with this:
type Ingestor interface {
Start(ctx context.Context) (<-chan Event, error)
Checkpoint() Position
}
```

2. **Implement it.**

3. **Write unit tests.** Table-driven tests are the Go convention:

```go
func TestNormalize(t *testing.T) {
tests := []struct {
name     string
input    string
expected Output
}{
{name: "basic case", input: "...", expected: Output{...}},
{name: "edge case", input: "...", expected: Output{...}},
}
for _, tt := range tests {
t.Run(tt.name, func (t *testing.T) {
got := Normalize(tt.input)
if got != tt.expected {
t.Errorf("got %v, want %v", got, tt.expected)
}
})
}
}
```

4. **Run `go test ./pkg/yourstage/...`** and make sure it passes before moving on.

### For Audicia specifically — the build order

**Day 1: Strategy engine** (`pkg/strategy/`)

- Smallest stub, pure logic, no I/O
- Implement `renderRole()` and `renderBinding()` using `rbacv1` types + `sigs.k8s.io/yaml`
- Gotcha: you must set `TypeMeta` (apiVersion + Kind) explicitly — `yaml.Marshal` doesn't auto-populate it
- Gotcha: ServiceAccount subjects need `namespace`; Users/Groups need `apiGroup: rbac.authorization.k8s.io`
- Write 5-6 unit tests (ServiceAccount role, User clusterrole, empty rules, unknown verbs, non-resource URLs, YAML
  round-trip)

**Day 2: File ingestor** (`pkg/ingestor/`)

- Open file, seek to checkpoint offset, read JSON lines, parse into `audit.Event`, send on channel
- Use `github.com/fsnotify/fsnotify` to watch for new data instead of polling
- Handle log rotation: stat the file, compare inode, re-open if different
- Edge cases: file doesn't exist (error), malformed JSON (skip+log), channel full (block), context cancelled (clean
  exit)
- Platform note: inode detection uses `syscall.Stat_t` which only works on Linux. Skip it on Windows with a build tag.
  The operator runs in Linux containers.
- Write 4 tests (reads events, resumes from checkpoint, skips malformed lines, context cancellation)

**Day 3: Wire the rest together in the controller** (stage 6)

### Output

- Each pipeline stage in its own `pkg/` package
- Unit tests for each
- `make test` passes
- No Kubernetes cluster needed to test anything yet

---

## Stage 6: Wire the Controller (2-3 days)

Now you connect your pipeline to the Kubernetes reconcile loop. This is where most operator developers struggle, because
the reconcile model is unintuitive at first.

### How controller-runtime works (the mental model)

The reconciler is called when something changes:

- Your CRD was created, updated, or deleted
- A resource your CRD owns was changed
- A periodic resync fires

The reconciler receives a `Request` (just a name+namespace) and returns a `Result`. That's it. No event details, no
diff. You fetch the current state and decide what to do.

**The reconcile loop is NOT an event handler.** It's a level-triggered state reconciler. You don't get "X was created" —
you get "something changed, here's the name, go figure it out." This means your reconcile logic must be **idempotent**:
calling it twice with the same state produces the same result.

### The three files

**`pkg/operator/operator.go`** — creates the controller-runtime manager:

```go
func Start(ctx context.Context, info BuildInfo, config Config) error {
scheme := runtime.NewScheme()
clientgoscheme.AddToScheme(scheme)
yourv1alpha1.AddToScheme(scheme)

mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
Scheme:                 scheme,
HealthProbeBindAddress: config.HealthProbe,
LeaderElection:         config.LeaderElection,
LeaderElectionID:       "myoperator-lock",
})

// Register your controller
yourcontroller.SetupWithManager(mgr)

// Health checks
mgr.AddHealthzCheck("healthz", healthz.Ping)
mgr.AddReadyzCheck("readyz", healthz.Ping)

return mgr.Start(ctx)
}
```

**`pkg/operator/config.go`** — operator configuration from env vars.

**`pkg/controller/yourresource/controller.go`** — the reconciler:

```go
func SetupWithManager(mgr ctrl.Manager) error {
return ctrl.NewControllerManagedBy(mgr).
For(&v1alpha1.YourInput{}). // watch your input CRD
Owns(&v1alpha1.YourOutput{}). // watch output CRDs you create
Complete(&Reconciler{
Client: mgr.GetClient(),
Scheme: mgr.GetScheme(),
})
}
```

**`cmd/myoperator/main.go`** — thin entrypoint:

```go
var (
version = "dev" // injected via ldflags
commit = "none"
date    = "unknown"
)

func main() {
ctx, cancel := signal.NotifyContext(context.Background(),
syscall.SIGINT, syscall.SIGTERM)
defer cancel()

operator.Start(ctx, buildInfo, config)
}
```

### The reconcile pattern for continuous pipelines

Many operators do request-response: reconcile fires, do something, done. But if your operator runs a continuous
process (like tailing a file or listening on a webhook), you need **long-lived goroutines**.

The pattern:

```go
type Reconciler struct {
client.Client
Scheme     *runtime.Scheme
pipelines  map[types.NamespacedName]context.CancelFunc
mu         sync.Mutex
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 1. Fetch the resource
var source v1alpha1.YourInput
if err := r.Get(ctx, req.NamespacedName, &source); err != nil {
// Resource deleted — stop the pipeline
r.stopPipeline(req.NamespacedName)
return ctrl.Result{}, client.IgnoreNotFound(err)
}

// 2. If pipeline already running and spec hasn't changed — do nothing
r.mu.Lock()
if _, running := r.pipelines[req.NamespacedName]; running {
// Check if spec changed (compare generation)
// If unchanged, return
// If changed, stop and restart
}
r.mu.Unlock()

// 3. Start a new pipeline goroutine
pipelineCtx, cancel := context.WithCancel(ctx)
r.mu.Lock()
r.pipelines[req.NamespacedName] = cancel
r.mu.Unlock()

go r.runPipeline(pipelineCtx, source)

return ctrl.Result{}, nil
}
```

The pipeline goroutine runs your tier 1 and tier 2 logic, and periodically writes results back to the cluster.

### Updating status

With the status subresource enabled, you update status separately from spec:

```go
// Create or update the output CRD (spec + owner reference)
controllerutil.CreateOrUpdate(ctx, r.Client, output, func () error {
controllerutil.SetControllerReference(&source, output, r.Scheme)
output.Spec.Subject = subject
return nil
})

// Update status separately
output.Status.Results = results
output.Status.Conditions = conditions
r.Status().Update(ctx, output)
```

### Setting conditions

```go
import "k8s.io/apimachinery/pkg/api/meta"

meta.SetStatusCondition(&source.Status.Conditions, metav1.Condition{
Type:               "Ready",
Status:             metav1.ConditionTrue,
Reason:             "PipelineRunning",
Message:            "Pipeline is running.",
ObservedGeneration: source.Generation,
})
r.Status().Update(ctx, &source)
```

### Common pitfalls

**409 Conflict on status update:**
Two things try to update the same object. Use `retry.RetryOnConflict`:

```go
import "k8s.io/client-go/util/retry"

retry.RetryOnConflict(retry.DefaultRetry, func () error {
// Re-fetch, update, write
})
```

**Owner references across namespaces:**
Owner references only work within the same namespace. If your input CRD is namespace-scoped but your output might be
cluster-scoped, use a finalizer on the input to clean up on deletion instead of relying on owner references.

**Forgetting to re-register the scheme:**
If you see "no kind is registered for the type" at runtime, you forgot to call `AddToScheme` in your operator startup.

**Reconcile loop storms:**
Your pipeline goroutine updates status, which triggers another reconcile, which checks the pipeline, which triggers
another reconcile... Break the cycle by checking `ObservedGeneration` and only reacting to spec changes, not status
changes.

### Output

- Controller wired and running
- `make run` starts the operator, connects to your kubeconfig cluster
- Applying your input CRD triggers reconciliation (visible in logs)

---

## Stage 7: Test Locally (1 day)

### Create a local cluster

Use kind with your application-specific configuration. For operators that need special apiserver flags (like audit
logging), create a kind config:

```yaml
# operator/hack/kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: ClusterConfiguration
        apiServer:
          extraArgs:
            your-flag: "value"
```

```bash
make kind-cluster    # create cluster
make install-crds    # install your CRDs
make run             # run operator locally (in another terminal)
```

### Run the end-to-end test manually

```bash
# Apply your example input CRD
kubectl apply -f your-input.yaml

# Verify the operator reacted
kubectl describe yourinput my-config
# Look for conditions, events

# Generate some activity that the operator should detect
# (This is specific to your operator)

# Check the output
kubectl get youroutput -A
kubectl get youroutput <name> -o yaml
```

### The audit log access problem (if applicable)

If your operator reads files from the node filesystem (like audit logs), the file lives inside the kind Docker
container. Your operator running on the host can't read it.

Solutions:

1. **Copy it out** (dev only): `docker exec kind-control-plane cat /path/to/file > /tmp/file`
2. **Run inside kind** (production-like): `make docker-build && make kind-load && make helm-install`
3. **Use a different input method** (e.g., webhook instead of file)

### Output

- End-to-end loop working locally
- You can demonstrate: input → operator → output

---

## Stage 8: Package for Distribution (1-2 days)

### Helm chart

Create a Helm chart in `deploy/helm/`. This is how most operators are distributed.

```
deploy/helm/
├── Chart.yaml              # Chart metadata, version, kubeVersion constraint
├── values.yaml             # Configurable defaults
├── crds/                   # Generated CRD YAMLs (pre-rendered, not templates)
├── templates/
│   ├── _helpers.tpl        # Template helpers (name, labels, selectors)
│   ├── deployment.yaml     # Operator deployment
│   ├── serviceaccount.yaml
│   ├── rbac/
│   │   ├── clusterrole.yaml       # What the operator needs access to
│   │   └── clusterrolebinding.yaml
│   ├── monitor/
│   │   ├── service.yaml           # Metrics service
│   │   └── servicemonitor.yaml    # Prometheus ServiceMonitor (optional)
│   └── NOTES.txt           # Post-install instructions
└── .helmignore
```

**CRDs go in `crds/`, not `templates/`.**

Helm has special handling for the `crds/` directory: CRDs are installed before everything else, and are not deleted on
`helm uninstall` (to prevent accidental data loss). Never template CRDs — use `make generate` to produce them and commit
them.

**RBAC: least privilege.**

Your ClusterRole should request only what the operator needs:

- Read + status update on your input CRD
- Full CRUD + status on your output CRD
- Create/patch Events (for recording Kubernetes events)
- Get/create/update Leases (for leader election)

**Security context:**

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 10000
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: [ ALL ]
```

**values.yaml patterns:**

```yaml
replicaCount: 1

image:
  repository: ghcr.io/yourorg/myoperator
  tag: ""  # defaults to chart appVersion
  pullPolicy: IfNotPresent

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    memory: 256Mi

# Operator-specific config (injected as env vars)
operator:
  logLevel: 0
  metricsBindAddress: ":8080"
  leaderElection: true
```

### Verify

```bash
make helm-lint       # helm lint deploy/helm/
make helm-template   # helm template audicia deploy/helm/ — inspect rendered output
make helm-install    # helm install into cluster
```

### Output

- `helm install` deploys a working operator
- `helm uninstall` cleans up everything except CRDs
- `values.yaml` has sensible defaults

---

## Stage 9: Harden (ongoing)

### Unit tests

Every pipeline stage needs tests. Minimum coverage:

| Package         | What to test                                                 |
|-----------------|--------------------------------------------------------------|
| Normalizers     | Parsing edge cases, empty input, malformed input             |
| Filters         | Rule matching, ordering (first-match-wins), default behavior |
| Aggregators     | Deduplication, counting, concurrent access                   |
| Strategy/policy | Output validity (parse the YAML back), all code paths        |
| Ingestors       | File reading, resume, rotation, malformed data, cancellation |

### Integration tests with envtest

`sigs.k8s.io/controller-runtime/pkg/envtest` gives you a real etcd + apiserver without Docker or kind:

```go
testEnv = &envtest.Environment{
CRDDirectoryPaths: []string{filepath.Join("..", "..", "deploy", "helm", "crds")},
}
cfg, _ := testEnv.Start()
// Now you have a real Kubernetes API to test against
```

Test:

1. Create your input CRD → controller starts → output CRD appears
2. Delete input → output cleaned up (owner reference)
3. Update input spec → operator detects change and reacts
4. Malformed input → operator sets error condition, doesn't crash

### Observability

Define your Prometheus metrics upfront using `prometheus.NewCounterVec`, `prometheus.NewHistogram`, etc. Register them
with `controller-runtime/pkg/metrics.Registry`.

Typical operator metrics:

- `events_processed_total` (counter, labels: source, result)
- `reconcile_errors_total` (counter)
- `pipeline_latency_seconds` (histogram)
- `objects_managed_count` (gauge)

Wire them into your pipeline code — don't leave them as empty registrations.

### Edge cases every operator hits

- **Object size limits:** Kubernetes objects max ~1.5MB in etcd. If your status can grow unbounded, enforce limits.
- **Retention:** Old data should be cleaned up. Add a `retentionDays` or `maxItems` field.
- **Leader election:** If you run multiple replicas, only one should be active. controller-runtime handles this — just
  enable it in manager options.
- **Graceful shutdown:** Cancel goroutines, finish in-flight work, write final checkpoint.
- **Rate limiting:** Don't hammer the API server. Use the controller's built-in rate limiter and batch your status
  updates.

### Output

- `make test` passes with meaningful coverage
- Metrics are wired and visible at `/metrics`
- Edge cases documented and handled

---

## Stage 10: Ship (1-2 days)

### CI pipeline

At minimum:

```yaml
# .github/workflows/ci.yml
- go build ./...
- golangci-lint run ./...
- go test -race ./...
- helm lint deploy/helm/
- docker build
```

### Release process

1. Tag the release: `git tag -a v0.1.0 -m "v0.1.0"`
2. CI builds container image, pushes to registry (GHCR, Docker Hub)
3. CI packages Helm chart, pushes to chart repo
4. Sign the image with cosign (keyless/OIDC) for supply chain security
5. Generate SBOM

### Documentation

At minimum for a public project:

- **README.md** — what it does, quick start, configuration table
- **ARCHITECTURE.md** — data flow, component breakdown
- **CONTRIBUTING.md** — how to build, test, submit PRs
- **SECURITY.md** — vulnerability reporting, threat model
- **RELEASING.md** — version policy, release checklist

### Versioning

- Operator binary + Helm chart: SemVer (`v0.1.0`)
- CRD API: Kubernetes conventions (`v1alpha1` → `v1beta1` → `v1`)
- Start at `v1alpha1`. Promote when the API is stable. Don't rush to `v1`.

---

## Timelines

### Realistic estimates for a solo developer

| Stage                 | Time     | Cumulative |
|-----------------------|----------|------------|
| 1. Define the Problem | 1-2 days | 2 days     |
| 2. Design CRDs        | 2-3 days | 5 days     |
| 3. Set Up Repo        | 0.5 days | 5.5 days   |
| 4. Implement Types    | 1 day    | 6.5 days   |
| 5. Build Pipeline     | 3-5 days | 10 days    |
| 6. Wire Controller    | 2-3 days | 13 days    |
| 7. Test Locally       | 1 day    | 14 days    |
| 8. Package (Helm)     | 1-2 days | 15.5 days  |
| 9. Harden             | 3-5 days | 19 days    |
| 10. Ship              | 1-2 days | 21 days    |

**~3-4 weeks to a shippable v1alpha1 operator**, working full-time solo.

The first working demo (stages 1-7) takes about 2 weeks. The remaining week is making it production-grade.

If you've never built an operator before, add 50% for learning. If you have, subtract 30%.

### What you can cut for a prototype

If you just need a proof-of-concept:

- Skip stage 8 (Helm chart) — use `make run` locally
- Skip stage 9 (hardening) — no tests, no metrics, no edge cases
- Skip stage 10 (shipping) — no CI, no release process

This gets you to a working demo in ~1 week. But you'll pay for the skipped stages later when you try to run it in
production or hand it to someone else.

---

## Reference: Essential Reading

These are the docs you'll keep coming back to:

| Topic                         | Link                                                                                                       |
|-------------------------------|------------------------------------------------------------------------------------------------------------|
| controller-runtime Book       | https://book.kubebuilder.io                                                                                |
| Kubernetes API Conventions    | https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md |
| controller-runtime GoDoc      | https://pkg.go.dev/sigs.k8s.io/controller-runtime                                                          |
| kubebuilder Markers Reference | https://book.kubebuilder.io/reference/markers                                                              |
| Operator SDK Best Practices   | https://sdk.operatorframework.io/docs/best-practices/                                                      |
| Kubernetes RBAC Docs          | https://kubernetes.io/docs/reference/access-authn-authz/rbac/                                              |
| Kubernetes Audit Logging      | https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/                                                |

---

## Appendix: Audicia-Specific Status

Where Audicia stands mapped to the stages above:

| Stage              | Status         | Notes                                                                                  |
|--------------------|----------------|----------------------------------------------------------------------------------------|
| 1. Define Problem  | Done           | `PROBLEM_STATEMENT.md`, goals/non-goals in README                                      |
| 2. Design CRDs     | Done           | Two CRDs designed, examples in `docs/examples/`                                        |
| 3. Set Up Repo     | Done           | Full directory structure, Makefile, Dockerfile                                         |
| 4. Implement Types | Done           | Full kubebuilder markers, all types defined                                            |
| 5. Build Pipeline  | 70% done       | Normalizer, filter, aggregator working. Strategy rendering empty. File ingestor empty. |
| 6. Wire Controller | 10% done       | Controller registered, reconcile body is a stub.                                       |
| 7. Test Locally    | Not started    | Need stages 5-6 complete                                                               |
| 8. Package (Helm)  | Done           | Full Helm chart with RBAC, ServiceMonitor, values                                      |
| 9. Harden          | Not started    | No tests, metrics registered but not wired                                             |
| 10. Ship           | Partially done | Docs complete, no CI, no release pipeline                                              |

**Remaining work to first demo (stages 5-7):**

1. `pkg/strategy/strategy.go` — implement `renderRole()` and `renderBinding()` (~50 lines each)
2. `pkg/ingestor/file.go` — implement file tailing with fsnotify (~100 lines)
3. `pkg/controller/audiciasource/controller.go` — wire reconcile loop with goroutine-per-source (~150 lines)
4. `cmd/audicia/main.go` — load config from env vars (~10 lines)
5. Run `go mod tidy` + `make generate` (scaffold produces `go.sum` + DeepCopy)

**Estimated time: 2-3 focused days.**
