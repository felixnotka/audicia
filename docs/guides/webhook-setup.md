# Webhook Mode — Setup Guide

Step-by-step guide for setting up Audicia's webhook-based audit event ingestion
on a kubeadm bare-metal cluster. Tested on Kubernetes v1.35.0.

This guide also applies to k3s, RKE2, and any self-managed cluster where you
have access to kube-apiserver flags.

> **Not supported on managed Kubernetes.** EKS, GKE, and AKS do not expose
> kube-apiserver flags. See
> [Platform Compatibility](../reference/features.md#platform-compatibility).

---

## Prerequisites

- A kubeadm cluster with SSH access to the control plane node.
- `openssl` installed on the control plane node (or your workstation).
- The Audicia Helm chart (`helm repo add audicia https://charts.audicia.io`).
- An audit policy already configured (`--audit-policy-file`). If you're already
  using file-based audit logging, you have one.

---

## Critical: Order of Operations

> **The kube-apiserver validates the webhook config at startup. If the webhook
> endpoint is unreachable, the CA cert is missing, or the Service doesn't exist,
> the apiserver will refuse to start and enter a crash loop. This takes down
> your entire cluster.**
>
> **You MUST follow this order:**
>
> 1. Create the namespace and Secrets
> 2. Install Audicia with webhook enabled (pod stays Pending until TLS secret
>    exists)
> 3. Generate the TLS certificate using the Service ClusterIP, then create the
>    TLS secret (pod starts)
> 4. Verify Audicia is running
> 5. Create the webhook kubeconfig on the control plane
> 6. **Only then** add the `--audit-webhook-config-file` flag to the apiserver

If you add the apiserver flag before Audicia is running, the apiserver will
crash loop. See [Recovery: Apiserver Crash Loop](#recovery-apiserver-crash-loop)
at the bottom.

---

## Step 1: Create the Namespace and Secrets

```bash
kubectl create namespace audicia-system --dry-run=client -o yaml | kubectl apply -f -
```

Create the mTLS client CA Secret so the webhook only accepts requests from your
kube-apiserver:

```bash
# Run this on the control plane node where /etc/kubernetes/pki/ is accessible
kubectl create secret generic kube-apiserver-client-ca \
  --from-file=ca.crt=/etc/kubernetes/pki/ca.crt \
  -n audicia-system
```

---

## Step 2: Install Audicia with Webhook Mode

Create a `values-webhook.yaml`:

```yaml
# values-webhook.yaml
webhook:
  enabled: true
  port: 8443
  tlsSecretName: audicia-webhook-tls
  clientCASecretName: kube-apiserver-client-ca # remove for basic TLS without mTLS
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-webhook.yaml
```

The pod will stay in **Pending** because the TLS Secret (`audicia-webhook-tls`)
doesn't exist yet — that's expected. The install creates the webhook Service,
which we need for the next step.

---

## Step 3: Generate the TLS Certificate and Create the TLS Secret

Now that the Service exists, get the webhook ClusterIP:

```bash
CLUSTER_IP=$(kubectl get svc -n audicia-system -l app.kubernetes.io/name=audicia-operator -o jsonpath='{.items[?(@.spec.ports[0].name=="webhook")].spec.clusterIP}')
echo "Webhook Service ClusterIP: $CLUSTER_IP"
```

Generate a self-signed certificate with the ClusterIP as a SAN:

```bash
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout webhook-server.key \
  -out webhook-server.crt \
  -days 365 \
  -subj "/CN=audicia-webhook" \
  -addext "subjectAltName=DNS:audicia-operator-webhook.audicia-system.svc,DNS:audicia-operator-webhook.audicia-system.svc.cluster.local,IP:${CLUSTER_IP}"
```

Create the TLS Secret — the pod starts automatically once this exists:

```bash
kubectl create secret tls audicia-webhook-tls \
  --cert=webhook-server.crt \
  --key=webhook-server.key \
  -n audicia-system
```

---

## Step 4: Verify Audicia Is Running

**Do not proceed to Step 5 until these checks pass.**

```bash
# Pod should be Running
kubectl get pods -n audicia-system

# Service should exist with port 8443 — note the CLUSTER-IP for Step 6
kubectl get svc -n audicia-system

# Expected output includes:
#   audicia-operator-webhook   ClusterIP   10.x.x.x   <none>   8443/TCP
```

---

## Step 5: Create the Webhook AudiciaSource

Save the following manifest as `realtime-audit.yaml` (see the
[Webhook AudiciaSource](../examples/audicia-source-webhook.md) example for
customization options):

```yaml
# realtime-audit.yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: realtime-audit
  namespace: audicia-system
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
    rateLimitPerSecond: 100
    maxRequestBodyBytes: 1048576
  policyStrategy:
    scopeMode: ClusterScopeAllowed
    verbMerge: Exact
    wildcards: Forbidden
  filters:
    - action: Deny
      userPattern: "^system:anonymous$"
    - action: Deny
      userPattern: "^system:kube-proxy$"
    - action: Deny
      userPattern: "^system:kube-scheduler$"
    - action: Deny
      userPattern: "^system:kube-controller-manager$"
    - action: Allow
      userPattern: ".*"
```

```bash
kubectl apply -f realtime-audit.yaml
```

Verify the pipeline started:

```bash
kubectl describe audiciasource realtime-audit -n audicia-system
# Look for: Condition Ready=True, Reason=PipelineRunning
```

Check operator logs:

```bash
kubectl logs -n audicia-system deploy/audicia-operator -f | grep webhook
# Expected: "starting webhook HTTPS server" port=8443
# With mTLS: "mTLS enabled" clientCA="/etc/audicia/webhook-client-ca/ca.crt"
```

---

## Step 6: Create the Webhook Kubeconfig on the Control Plane

SSH to the control plane node and create the kubeconfig that tells the
kube-apiserver where to send audit events.

> **You MUST use the ClusterIP, not the DNS name.** The kube-apiserver uses
> `hostNetwork: true` and cannot resolve `.svc.cluster.local` names. If you use
> the DNS name, the apiserver will silently drop all audit events.

Copy the self-signed certificate as the CA:

```bash
cp webhook-server.crt /etc/kubernetes/pki/audicia-webhook-ca.crt
```

Create the kubeconfig. Replace `<CLUSTER-IP>` with the IP from Step 4:

```bash
cat > /etc/kubernetes/audit-webhook-kubeconfig.yaml << 'EOF'
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
      user: kube-apiserver
users:
  - name: kube-apiserver
    user:
      client-certificate: /etc/kubernetes/pki/apiserver-kubelet-client.crt
      client-key: /etc/kubernetes/pki/apiserver-kubelet-client.key
current-context: audicia
EOF
```

The `client-certificate` and `client-key` fields enable mTLS. If you skipped
mTLS, see the [basic TLS kubeconfig example](../examples/webhook-kubeconfig.md)
instead.

---

## Step 7: Add the Apiserver Flag

Edit the kube-apiserver static pod manifest:

```bash
vi /etc/kubernetes/manifests/kube-apiserver.yaml
```

Add the webhook flag to the `command` list, near the other `--audit-*` flags:

```yaml
- --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

Mount the kubeconfig file into the container:

```yaml
volumeMounts:
  # ... existing mounts ...
  - mountPath: /etc/kubernetes/audit-webhook-kubeconfig.yaml
    name: audit-webhook-config
    readOnly: true

volumes:
  # ... existing volumes ...
  - hostPath:
      path: /etc/kubernetes/audit-webhook-kubeconfig.yaml
      type: File
    name: audit-webhook-config
```

Save the file. The kube-apiserver will restart automatically (kubelet watches
static pod manifests). Wait 30-60 seconds:

```bash
kubectl get nodes
```

If it doesn't respond within 2 minutes, see
[Recovery](#recovery-apiserver-crash-loop).

---

## Step 8: Verify Events Are Flowing

Generate some API activity:

```bash
kubectl get pods -n default
kubectl get configmaps -n default
kubectl get secrets -n default
```

Check for reports:

```bash
kubectl get audiciapolicyreports --all-namespaces
```

Check operator logs:

```bash
kubectl logs -n audicia-system deploy/audicia-operator --tail=50
```

---

## Upgrading from Basic TLS to mTLS

If you have an existing basic TLS install, follow these steps to add mTLS.

**Step A: Create the client CA secret:**

```bash
kubectl create secret generic kube-apiserver-client-ca \
  --from-file=ca.crt=/etc/kubernetes/pki/ca.crt \
  -n audicia-system
```

**Step B: Helm upgrade** with `clientCASecretName`:

```bash
helm upgrade audicia audicia/audicia-operator \
  -n audicia-system \
  -f values-webhook.yaml
```

(Ensure your `values-webhook.yaml` includes
`clientCASecretName: kube-apiserver-client-ca`.)

**Step C: Update the AudiciaSource:**

```bash
kubectl patch audiciasource realtime-audit -n audicia-system --type=merge \
  -p '{"spec":{"webhook":{"clientCASecretName":"kube-apiserver-client-ca"}}}'
```

**Step D: Update the webhook kubeconfig** to present the apiserver's client
certificate (see
[Step 6](#step-6-create-the-webhook-kubeconfig-on-the-control-plane) for the
full format), then restart the apiserver:

```bash
mv /etc/kubernetes/manifests/kube-apiserver.yaml /etc/kubernetes/kube-apiserver.yaml
sleep 5
mv /etc/kubernetes/kube-apiserver.yaml /etc/kubernetes/manifests/kube-apiserver.yaml
```

---

## Dual Mode: File + Webhook

You can run both ingestion modes simultaneously. The kube-apiserver supports
both `--audit-log-path` (file) and `--audit-webhook-config-file` (webhook) at
the same time.

Create a `values-dual.yaml`:

```yaml
# values-dual.yaml
auditLog:
  enabled: true
  hostPath: /var/log/kubernetes/audit/audit.log

webhook:
  enabled: true
  port: 8443
  tlsSecretName: audicia-webhook-tls
  clientCASecretName: kube-apiserver-client-ca

hostNetwork: true

nodeSelector:
  kubernetes.io/hostname: "<CONTROL-PLANE-NODE>"

tolerations:
  - key: node-role.kubernetes.io/control-plane
    operator: Exists
    effect: NoSchedule

podSecurityContext:
  runAsUser: 0
  runAsNonRoot: false
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-dual.yaml
```

Then apply both AudiciaSource CRs. See the
[File Mode](../examples/audicia-source-file.md) and
[Webhook Mode](../examples/audicia-source-webhook.md) example pages for the full
manifests.

---

## Kube-apiserver Webhook Reference

| Flag                              | Default | Description                                       |
| --------------------------------- | ------- | ------------------------------------------------- |
| `--audit-webhook-config-file`     | —       | Path to the webhook kubeconfig (required).        |
| `--audit-webhook-mode`            | `batch` | `batch` (recommended) or `blocking`.              |
| `--audit-webhook-batch-max-wait`  | `30s`   | Max time to buffer events before sending a batch. |
| `--audit-webhook-batch-max-size`  | `400`   | Max events per batch.                             |
| `--audit-webhook-initial-backoff` | `10s`   | Backoff after a failed webhook request.           |
| `--audit-policy-file`             | —       | Shared by both file and webhook backends.         |

---

## Recovery: Apiserver Crash Loop

If the kube-apiserver crash-loops after adding the webhook flag, see
[Troubleshooting](../troubleshooting.md#apiserver-crash-loop-after-adding-webhook-flag).

---

## Uninstall / Disable Webhook Mode

To disable webhook mode without breaking the cluster:

**1. Remove the apiserver flag FIRST** (before uninstalling Audicia):

```bash
sed -i '/audit-webhook-config-file/d' /etc/kubernetes/manifests/kube-apiserver.yaml
```

Wait for the apiserver to restart (~30 seconds).

**2. Then uninstall Audicia:**

```bash
helm uninstall audicia -n audicia-system
```

**3. Clean up Secrets and CRDs if desired:**

```bash
kubectl delete secret audicia-webhook-tls -n audicia-system
kubectl delete secret kube-apiserver-client-ca -n audicia-system
kubectl delete crd audiciasources.audicia.io audiciapolicyreports.audicia.io
kubectl delete namespace audicia-system
```

**4. Optionally remove the CA cert and kubeconfig from the control plane:**

```bash
rm /etc/kubernetes/pki/audicia-webhook-ca.crt
rm /etc/kubernetes/audit-webhook-kubeconfig.yaml
```

---

## Troubleshooting

For common webhook issues (no events, mTLS failures, DNS resolution,
NetworkPolicy), see [Troubleshooting](../troubleshooting.md).
