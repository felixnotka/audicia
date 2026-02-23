# Quick Start: Webhook Ingestion

This tutorial walks you through setting up Audicia to receive real-time audit events via the kube-apiserver's webhook
backend. This is the recommended mode for production — it provides sub-second event delivery and doesn't require
control plane node scheduling.

> **Kube-proxy-free cluster (Cilium, eBPF)?** ClusterIP may not be routable from the host namespace.
> See the dedicated [Kube-Proxy-Free Guide](../guides/kube-proxy-free.md) instead.

## Prerequisites

- Audicia installed with `webhook.enabled=true` (see [Installation](installation.md))
- `kubectl` configured
- Access to the control plane node (for apiserver configuration)

## Step 1: Create TLS Certificates

The webhook receiver requires TLS. For production, use certificates from your PKI. For testing:

```bash
# Get the webhook Service ClusterIP
CLUSTER_IP=$(kubectl get svc -n audicia-system -l app.kubernetes.io/name=audicia-operator -o jsonpath='{.items[0].spec.clusterIP}')

# Generate a self-signed certificate with the ClusterIP as a SAN
openssl req -x509 -newkey rsa:4096 -keyout webhook-server.key -out webhook-server.crt \
  -days 365 -nodes -subj '/CN=audicia-webhook' \
  -addext "subjectAltName=IP:${CLUSTER_IP}"
```

## Step 2: Create the TLS Secret

```bash
kubectl create secret tls audicia-webhook-tls \
  --cert=webhook-server.crt --key=webhook-server.key -n audicia-system
```

## Step 3: Install Audicia with Webhook Mode

```bash
helm upgrade audicia audicia/audicia-operator -n audicia-system \
  --set webhook.enabled=true \
  --set webhook.tlsSecretName=audicia-webhook-tls
```

## Step 4: Create an AudiciaSource

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: webhook-audit
  namespace: audicia-system
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
    - action: Deny
      userPattern: "^system:apiserver$"
  checkpoint:
    intervalSeconds: 30
  limits:
    maxRulesPerReport: 200
    retentionDays: 30
```

```bash
kubectl apply -f - <<EOF
# (paste the YAML above)
EOF
```

## Step 5: Configure the kube-apiserver

Create the webhook kubeconfig on the control plane node. Use the ClusterIP from Step 1:

```bash
echo $CLUSTER_IP
```

SSH to the control plane and create the kubeconfig:

```bash
cat > /etc/kubernetes/audit-webhook-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
  - name: audicia
    cluster:
      certificate-authority: /etc/kubernetes/pki/audicia-webhook-ca.crt
      server: https://<CLUSTER-IP>:8443
contexts:
  - name: audicia
    context:
      cluster: audicia
users:
  - name: audicia
current-context: audicia
EOF
```

> **Important:** You must use an IP address, not a DNS name. The kube-apiserver uses `hostNetwork: true` and cannot
> resolve `.svc.cluster.local` names.

Copy the webhook CA certificate:

```bash
# Copy the self-signed cert as the CA
cp webhook-server.crt /etc/kubernetes/pki/audicia-webhook-ca.crt
```

Add the webhook flag to the apiserver manifest:

```bash
# Edit /etc/kubernetes/manifests/kube-apiserver.yaml
# Add this flag:
#   - --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

The apiserver will restart automatically.

See the [Webhook Kubeconfig](../examples/webhook-kubeconfig.md) example for the complete template.

## Step 6: Verify Events Flow

Check the operator logs:

```bash
kubectl logs -n audicia-system -l app.kubernetes.io/name=audicia-operator --tail=20
```

You should see:

```
"starting webhook HTTPS server" port=8443
```

After a few seconds of API activity:

```bash
kubectl get audiciapolicyreports --all-namespaces
```

Reports should start appearing for active subjects.

## Optional: Enable mTLS

For production, enable mTLS so only the kube-apiserver (presenting a valid client certificate) can send events.
See the [mTLS Setup Guide](../guides/mtls-setup.md) or the
[Webhook Setup Guide](../guides/webhook-setup.md#upgrading-from-basic-tls-to-mtls) for the full walkthrough.

## What's Next

- [mTLS Setup](../guides/mtls-setup.md) — Harden the webhook with mutual TLS
- [Webhook Setup Guide](../guides/webhook-setup.md) — Complete reference for webhook configuration
- [Filter Recipes](../guides/filter-recipes.md) — Production filter configurations
