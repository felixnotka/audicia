# Installation

## Prerequisites

- A Kubernetes cluster (kubeadm, k3s, or RKE2)
- `kubectl` configured and pointing at your cluster
- `helm` v3 installed
- Audit logging enabled on the kube-apiserver (see [Audit Policy Guide](../guides/audit-policy.md))

## Enable Audit Logging

Most clusters do not have audit logging enabled by default. Audicia requires it.

Add these flags to your kube-apiserver manifest (`/etc/kubernetes/manifests/kube-apiserver.yaml`):

```yaml
spec:
  containers:
  - command:
    - kube-apiserver
    # ... existing flags ...
    - --audit-policy-file=/etc/kubernetes/audit-policy.yaml
    - --audit-log-path=/var/log/kube-audit.log          # For file-based ingestion
    # OR for webhook ingestion:
    - --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

Copy the audit policy to the control plane:

```bash
cp operator/hack/audit-policy.yaml /etc/kubernetes/audit-policy.yaml
```

See the [Audit Policy Guide](../guides/audit-policy.md) for tuning recommendations.

### Verify Audit Logging

```bash
head -5 /var/log/kube-audit.log
```

You should see JSON audit events. If the file is empty or missing, check your apiserver flags.

## Install Audicia

```bash
helm repo add audicia https://charts.audicia.io
helm install audicia audicia/audicia -n audicia-system --create-namespace
```

### For File-Based Ingestion

The operator needs to run on a control plane node with access to the audit log file:

```bash
helm install audicia audicia/audicia -n audicia-system --create-namespace \
  --set auditLog.enabled=true \
  --set auditLog.hostPath=/var/log/kube-audit.log \
  --set nodeSelector."node-role\.kubernetes\.io/control-plane"="" \
  --set tolerations[0].key=node-role.kubernetes.io/control-plane \
  --set tolerations[0].effect=NoSchedule
```

### For Webhook Ingestion

The operator can run on any node. You need a TLS certificate:

```bash
# Create TLS certificate (self-signed for testing)
openssl req -x509 -newkey rsa:4096 -keyout webhook-server.key -out webhook-server.crt \
  -days 365 -nodes -subj '/CN=audicia-webhook' \
  -addext "subjectAltName=IP:<CLUSTER-IP>"

# Create the TLS secret
kubectl create namespace audicia-system
kubectl create secret tls audicia-webhook-tls \
  --cert=webhook-server.crt --key=webhook-server.key -n audicia-system

# Install with webhook mode
helm install audicia audicia/audicia -n audicia-system \
  --set webhook.enabled=true \
  --set webhook.tlsSecretName=audicia-webhook-tls
```

See the [Webhook Setup Guide](../guides/webhook-setup.md) for the full walkthrough including kube-apiserver
configuration and mTLS.

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

- [Quick Start: File Ingestion](quick-start-file.md) — Create your first AudiciaSource and generate reports
- [Quick Start: Webhook Ingestion](quick-start-webhook.md) — Real-time audit events via HTTPS
- [Webhook Setup Guide](../guides/webhook-setup.md) — Full webhook configuration with TLS and mTLS
