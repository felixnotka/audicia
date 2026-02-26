# Demo Script

This walkthrough demonstrates Audicia's core value loop: **observe access
patterns, generate a policy, apply it, verify. **

## Prerequisites

- A running Kubernetes cluster with audit logging enabled (see
  [Installation](../getting-started/installation.md))
- Audicia operator installed
- `kubectl` configured

---

## Step 0: Enable Audit Logging

Most clusters do not have audit logging enabled by default. Audicia requires it.

Ensure your kube-apiserver has `--audit-policy-file` and `--audit-log-path`
configured. See [Installation](../getting-started/installation.md) for setup
instructions and [Audit Policy](audit-policy.md) for tuning.

---

## Step 1: The Problem — Alice Can't List Pods

Alice is a developer. She has a kubeconfig with her identity
(`alice@example.com`) but no RBAC Role in the `dev` namespace.

```bash
# As Alice
kubectl get pods -n dev
```

**Result:**

```
Error from server (Forbidden): pods is forbidden: User "alice@example.com"
cannot list resource "pods" in API group "" in the namespace "dev"
```

The kube-apiserver audit log now contains this event:

```json
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "Metadata",
  "auditID": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "verb": "list",
  "user": {
    "username": "alice@example.com"
  },
  "objectRef": {
    "resource": "pods",
    "namespace": "dev",
    "apiGroup": ""
  },
  "responseStatus": {
    "code": 403
  }
}
```

---

## Step 2: Configure Audicia to Watch

Create an `AudiciaSource` pointing to the audit log:

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: dev-cluster-audit
spec:
  sourceType: K8sAuditLog
  location:
    path: /var/log/kubernetes/audit/audit.log
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
  filters:
    - action: Deny
      userPattern: "^system:.*"
```

Save the above manifest and apply it:

```bash
kubectl apply -f audicia-source.yaml
```

Audicia begins tailing the audit log from the current position. Check that it
started successfully:

```bash
kubectl describe audiciasource dev-cluster-audit
```

Look for the `Ready` condition in the status and the `IngestionStarted` event.

---

## Step 3: Audicia Generates a Policy Report

After processing the audit events, Audicia creates an `AudiciaPolicyReport`:

```bash
kubectl get audiciapolicyreports -n dev
```

**Output:**

```
NAME                  SUBJECT            KIND   COMPLIANCE   SCORE   AGE
report-alice-example  alice@example.com  User                          30s
```

Inspect the report:

```bash
kubectl get audiciapolicyreport report-alice-example -n dev -o yaml
```

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaPolicyReport
metadata:
  name: report-alice-example
  namespace: dev
spec:
  subject:
    kind: User
    name: alice@example.com
status:
  observedRules:
    - apiGroups: [""]
      resources: ["pods"]
      verbs: ["list"]
      firstSeen: "2026-02-14T10:00:00Z"
      lastSeen: "2026-02-14T10:00:00Z"
      count: 1
  suggestedPolicy:
    manifests:
      - |
          apiVersion: rbac.authorization.k8s.io/v1
          kind: Role
          metadata:
            name: suggested-alice-role
            namespace: dev
          rules:
            - apiGroups: [""]
              resources: ["pods"]
              verbs: ["list"]
      - |
          apiVersion: rbac.authorization.k8s.io/v1
          kind: RoleBinding
          metadata:
            name: suggested-alice-binding
            namespace: dev
          roleRef:
            apiGroup: rbac.authorization.k8s.io
            kind: Role
            name: suggested-alice-role
          subjects:
            - kind: User
              name: alice@example.com
              apiGroup: rbac.authorization.k8s.io
  conditions:
    - type: Ready
      status: "True"
```

Audicia observed Alice's `list pods` attempt and generated the **minimal** Role
and RoleBinding to satisfy exactly that access.

---

## Step 4: Apply the Policy

Extract and apply the suggested manifests:

```bash
# Review first (dry-run)
kubectl get audiciapolicyreport report-alice-example -n dev \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  | kubectl apply --dry-run=client -f -

# Apply for real
kubectl get audiciapolicyreport report-alice-example -n dev \
  -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
  | kubectl apply -f -
```

**Output:**

```
role.rbac.authorization.k8s.io/suggested-alice-role created
rolebinding.rbac.authorization.k8s.io/suggested-alice-binding created
```

> **Tip:** For GitOps workflows, pipe the output to a file in your policy repo
> instead:
>
> ```bash
> kubectl get audiciapolicyreport report-alice-example -n dev \
>   -o jsonpath='{range .status.suggestedPolicy.manifests[*]}{@}{"\n---\n"}{end}' \
>   > policies/dev/alice-rbac.yaml
> git add policies/dev/alice-rbac.yaml && git commit -m "Add RBAC for alice (Audicia suggestion)"
> ```

---

## Step 5: Verify — Alice Can Now List Pods

```bash
# As Alice
kubectl get pods -n dev
```

**Result:**

```
NAME                     READY   STATUS    RESTARTS   AGE
backend-api-7d4f8b6c5-x2k9m   1/1     Running   0          2h
frontend-84c9b7d6f-p3n7q       1/1     Running   0          2h
```

**Access granted.** Alice has exactly the permissions she needs — nothing more.

---

## The Loop

This is the Audicia workflow:

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐     ┌─────────────┐
│  403 Denied  │────►│  Audit Event  │────►│  Audicia      │────►│  Apply Role  │
│  (Red)       │     │  Generated    │     │  Policy Report│     │  (Green)     │
└─────────────┘     └──────────────┘     └──────────────┘     └─────────────┘
      ▲                                                              │
      └──────────────────────────────────────────────────────────────┘
                            Continuous Refinement
```

As Alice's usage evolves (she starts watching pods, checking logs), Audicia
updates the report incrementally. The policy grows organically to match real
behavior — always minimal, always correct.

---

## Troubleshooting

If reports aren't appearing or the pipeline isn't working as expected, see
[Troubleshooting](../troubleshooting.md).
