# Installation

## Prerequisites

- A Kubernetes cluster (kubeadm, k3s, or RKE2)
- `kubectl` configured and pointing at your cluster
- `helm` v3 installed
- Audit logging enabled on the kube-apiserver (see
  [Audit Policy Guide](../guides/audit-policy.md))

## Enable Audit Logging

Most clusters do not have audit logging enabled by default. Audicia requires it.

Add these flags to your kube-apiserver manifest
(`/etc/kubernetes/manifests/kube-apiserver.yaml`):

```yaml
spec:
  containers:
    - command:
        - kube-apiserver
        # ... existing flags ...
        - --audit-policy-file=/etc/kubernetes/audit-policy.yaml
        - --audit-log-path=/var/log/kubernetes/audit/audit.log # For file-based ingestion
        # OR for webhook ingestion:
        - --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

Copy the audit policy to the control plane:

```bash
cp operator/hack/audit-policy.yaml /etc/kubernetes/audit-policy.yaml
```

See the [Audit Policy Guide](../guides/audit-policy.md) for tuning
recommendations.

### Verify Audit Logging

```bash
head -5 /var/log/kubernetes/audit/audit.log
```

You should see JSON audit events. If the file is empty or missing, check your
apiserver flags.

## Install Audicia

```bash
helm repo add audicia https://charts.audicia.io
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace
```

### For File-Based Ingestion

The operator needs to run on a control plane node with access to the audit log
file. Create a `values-file.yaml`:

```yaml
# values-file.yaml
auditLog:
  enabled: true
  hostPath: /var/log/kubernetes/audit/audit.log

hostNetwork: true

nodeSelector:
  node-role.kubernetes.io/control-plane: ""

tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-file.yaml
```

> **Why `hostNetwork: true`?** On Cilium and other kube-proxy-free CNIs, pods on
> control plane nodes cannot reach the Kubernetes service ClusterIP.
> `hostNetwork` lets the pod use the node's network stack, bypassing the CNI
> datapath. This is safe because the pod already runs on the control plane with
> `hostPath` access. See the
> [Kube-Proxy-Free Guide](../guides/kube-proxy-free.md#file-mode-hostnetwork).
> If you are certain your cluster uses kube-proxy, you can remove this setting.

> **Permission denied?** Audit logs are typically owned by root. The operator
> runs as non-root (UID 10000) by default, so it cannot read the log without
> elevated privileges.
>
> **Option A: Run as root** — add the following to your `values-file.yaml`:
>
> ```yaml
> podSecurityContext:
>   runAsUser: 0
>   runAsNonRoot: false
> ```
>
> **Option B: Relax file permissions on the host** (keeps the operator
> non-root):
>
> ```bash
> chmod 644 /var/log/kubernetes/audit/audit.log
> ```
>
> Note that some kube-apiserver configurations reset file permissions on
> restart. If you choose this approach, verify the permissions persist after an
> apiserver restart.

### For Webhook Ingestion

The operator can run on any node. You need a TLS certificate:

```bash
# Create TLS certificate (self-signed for testing)
# Use the ClusterIP as the SAN (get it after Helm install from kubectl get svc -n audicia-system)
openssl req -x509 -newkey rsa:4096 -keyout webhook-server.key -out webhook-server.crt \
  -days 365 -nodes -subj '/CN=audicia-webhook' \
  -addext "subjectAltName=IP:<CLUSTER-IP>"

# Create the TLS secret
kubectl create namespace audicia-system
kubectl create secret tls audicia-webhook-tls \
  --cert=webhook-server.crt --key=webhook-server.key -n audicia-system
```

Create a `values-webhook.yaml`:

```yaml
# values-webhook.yaml
webhook:
  enabled: true
  tlsSecretName: audicia-webhook-tls
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system \
  -f values-webhook.yaml
```

> **ClusterIP unreachable?** On Cilium or other kube-proxy-free CNIs, the
> kube-apiserver may not be able to route traffic to a ClusterIP. See the
> [Kube-Proxy-Free Guide](../guides/kube-proxy-free.md) for the hostPort-based
> setup.

After installing, you must configure the kube-apiserver to send audit events to
the webhook. This requires adding a flag and restarting the apiserver. See the
[Webhook Setup Guide](../guides/webhook-setup.md) for the full walkthrough
including kube-apiserver configuration and mTLS.

> **How to restart the kube-apiserver.** On kubeadm clusters, the kube-apiserver
> runs as a static pod managed by the kubelet. To restart it after changing its
> manifest:
>
> ```bash
> # Move the manifest out of the watched directory
> mv /etc/kubernetes/manifests/kube-apiserver.yaml /etc/kubernetes/kube-apiserver.yaml
>
> # Wait for the old pod to terminate
> sleep 5
>
> # Move it back — kubelet will start a new pod
> mv /etc/kubernetes/kube-apiserver.yaml /etc/kubernetes/manifests/kube-apiserver.yaml
> ```
>
> The apiserver should be back within 30-60 seconds. Verify with
> `kubectl get nodes`. If it doesn't come back within 2 minutes, check the
> static pod logs with `crictl logs $(crictl ps --name kube-apiserver -q)`.

## Verify Installation

```bash
# Check the operator pod is running
kubectl get pods -n audicia-system

# Check CRDs are registered
kubectl get crd audiciasources.audicia.io audiciapolicyreports.audicia.io

# Check operator logs
kubectl logs -n audicia-system -l app.kubernetes.io/name=audicia-operator
```

## What's Next

- [Quick Start: File Ingestion](quick-start-file.md) — Create your first
  AudiciaSource and generate reports
- [Quick Start: Webhook Ingestion](quick-start-webhook.md) — Real-time audit
  events via HTTPS
- [Quick Start: AKS Cloud Ingestion](quick-start-aks.md) — Ingest audit logs
  from AKS via Event Hub
- [Webhook Setup Guide](../guides/webhook-setup.md) — Full webhook configuration
  with TLS and mTLS
