# Webhook Mode — Setup Guide

Step-by-step guide for setting up Audicia's webhook-based audit event ingestion on a
kubeadm bare-metal cluster. Tested on Kubernetes v1.35.0.

This guide also applies to k3s, RKE2, and any self-managed cluster where you have access
to kube-apiserver flags.

> **Not supported on managed Kubernetes.** EKS, GKE, and AKS do not expose kube-apiserver
> flags or allow custom audit webhook configuration.
> See [Platform Compatibility](../reference/features.md#platform-compatibility).

---

## Prerequisites

- A kubeadm cluster with SSH access to the control plane node.
- `openssl` installed on the control plane node (or your workstation).
- The Audicia Helm chart (`helm repo add audicia https://charts.audicia.io`).
- An audit policy already configured (`--audit-policy-file`). If you're already using
  file-based audit logging, you have one.

---

## Critical: Order of Operations

> **The kube-apiserver validates the webhook config at startup. If the webhook endpoint
> is unreachable, the CA cert is missing, or the Service doesn't exist, the apiserver
> will refuse to start and enter a crash loop. This takes down your entire cluster.**
>
> **You MUST follow this order:**
>
> 1. Generate TLS certificates
> 2. Create Kubernetes Secrets
> 3. Install Audicia with webhook enabled
> 4. Verify Audicia pod and Service are running
> 5. Create the webhook kubeconfig file on the control plane
> 6. **Only then** add the `--audit-webhook-config-file` flag to the apiserver

If you add the apiserver flag before Audicia is running, the apiserver will crash loop.
See [Recovery: Apiserver Crash Loop](#recovery-apiserver-crash-loop) at the bottom.

---

## Step 1: Generate a Self-Signed TLS Certificate

Run this on the control plane node (or your workstation — you'll need the files in both
places).

> **Important:** You need the webhook Service's ClusterIP for the certificate SAN. If this
> is a fresh install, first do Steps 2-3 to create the namespace and install Audicia, note
> the ClusterIP from `kubectl get svc -n audicia-system`, then come back here. If
> reinstalling, the ClusterIP may change — regenerate the cert.

```bash
# First, get the Service ClusterIP (after Helm install):
CLUSTER_IP=$(kubectl get svc -n audicia-system -l app.kubernetes.io/name=audicia-operator -o jsonpath='{.items[?(@.spec.ports[0].name=="webhook")].spec.clusterIP}')
echo "Webhook Service ClusterIP: $CLUSTER_IP"

# Generate the certificate with both DNS and IP SANs:
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout webhook-server.key \
  -out webhook-server.crt \
  -days 365 \
  -subj "/CN=audicia-webhook" \
  -addext "subjectAltName=DNS:audicia-audicia-operator-webhook.audicia-system.svc,DNS:audicia-audicia-operator-webhook.audicia-system.svc.cluster.local,IP:${CLUSTER_IP}"
```

The SAN (Subject Alternative Name) **must** include the Service ClusterIP. The
kube-apiserver runs with `hostNetwork: true` and uses the node's DNS resolver, which
**cannot resolve** Kubernetes `.svc.cluster.local` names. The webhook kubeconfig must
therefore use the ClusterIP, and the certificate must have that IP in its SAN.

> **Why not the DNS name?** The kube-apiserver static pod uses `hostNetwork: true`, so it
> uses the node's `/etc/resolv.conf` (typically pointing to a public DNS or systemd-resolved).
> Cluster-internal DNS names like `*.svc.cluster.local` are only resolvable via CoreDNS,
> which runs inside the cluster's pod network. The apiserver cannot reach CoreDNS for name
> resolution. Using the ClusterIP bypasses this entirely.

---

## Step 2: Create Kubernetes Secrets

```bash
# Create the namespace if it doesn't exist
kubectl create namespace audicia-system --dry-run=client -o yaml | kubectl apply -f -

# TLS Secret for Audicia's webhook HTTPS server
kubectl create secret tls audicia-webhook-tls \
  --cert=webhook-server.crt \
  --key=webhook-server.key \
  -n audicia-system
```

### Optional: mTLS Client CA Secret

If you want the webhook server to verify that requests come from your kube-apiserver
(recommended for production), create a Secret with the cluster's CA certificate:

```bash
# Run this on the control plane node where /etc/kubernetes/pki/ is accessible
kubectl create secret generic kube-apiserver-client-ca \
  --from-file=ca.crt=/etc/kubernetes/pki/ca.crt \
  -n audicia-system
```

This enables mTLS: only clients presenting a certificate signed by the cluster CA (i.e.,
the kube-apiserver) can send events to the webhook.

---

## Step 3: Install Audicia with Webhook Mode

### Basic TLS (no mTLS)

```bash
helm install audicia audicia/audicia \
  --create-namespace --namespace audicia-system \
  --set image.repository=felixnotka/audicia-operator \
  --set image.tag=latest \
  --set webhook.enabled=true \
  --set webhook.port=8443 \
  --set webhook.tlsSecretName=audicia-webhook-tls
```

### With mTLS (recommended for production)

```bash
helm install audicia audicia/audicia \
  --create-namespace --namespace audicia-system \
  --set image.repository=felixnotka/audicia-operator \
  --set image.tag=latest \
  --set webhook.enabled=true \
  --set webhook.port=8443 \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.clientCASecretName=kube-apiserver-client-ca
```

### Upgrading from Basic TLS to mTLS

If you already have a working basic TLS install and want to add mTLS, follow **all four
steps** below. The apiserver must present a client certificate, so both the kubeconfig and
the Helm values need updating.

**Step A: Create the client CA secret** (if not already present):

```bash
kubectl create secret generic kube-apiserver-client-ca \
  --from-file=ca.crt=/etc/kubernetes/pki/ca.crt \
  -n audicia-system
```

**Step B: Switch the AudiciaSource to the hardened example:**

```bash
# Delete the basic TLS AudiciaSource
kubectl delete audiciasource realtime-audit -n audicia-system

# Apply the hardened example (includes clientCASecretName)
# See: docs/examples/audicia-source-hardened.md for full manifest
kubectl apply -f - <<EOF
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: realtime-audit-hardened
  namespace: audicia-system
spec:
  sourceType: Webhook
  webhook:
    port: 8443
    tlsSecretName: audicia-webhook-tls
    clientCASecretName: kube-apiserver-client-ca
    rateLimitPerSecond: 100
    maxRequestBodyBytes: 1048576
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  checkpoint:
    intervalSeconds: 30
    batchSize: 500
  limits:
    maxRulesPerReport: 200
    retentionDays: 30
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
EOF
```

Or if you prefer to patch your existing AudiciaSource in-place:

```bash
kubectl patch audiciasource realtime-audit -n audicia-system --type=merge \
  -p '{"spec":{"webhook":{"clientCASecretName":"kube-apiserver-client-ca"}}}'
```

**Step C: Helm upgrade** to mount the client CA volume:

```bash
helm upgrade audicia audicia/audicia \
  --namespace audicia-system \
  --set image.repository=felixnotka/audicia-operator \
  --set image.tag=latest \
  --set webhook.enabled=true \
  --set webhook.port=8443 \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.clientCASecretName=kube-apiserver-client-ca
```

The pod will restart. Verify mTLS is active:

```bash
kubectl logs -n audicia-system deploy/audicia --tail=20 | grep mTLS
# Expected: "mTLS enabled" clientCA="/etc/audicia/webhook-client-ca/ca.crt"
```

**Step D: Update the webhook kubeconfig** to present the apiserver's client certificate:

On the control plane node, update `/etc/kubernetes/audit-webhook-kubeconfig.yaml` to add
client cert and key. See [Step 6](#step-6-create-the-webhook-kubeconfig-on-the-control-plane)
for the full mTLS kubeconfig format. The key additions are:

```yaml
users:
  - name: kube-apiserver
    user:
      client-certificate: /etc/kubernetes/pki/apiserver-kubelet-client.crt
      client-key: /etc/kubernetes/pki/apiserver-kubelet-client.key
```

Then restart the apiserver:

```bash
mv /etc/kubernetes/manifests/kube-apiserver.yaml /etc/kubernetes/kube-apiserver.yaml
sleep 5
mv /etc/kubernetes/kube-apiserver.yaml /etc/kubernetes/manifests/kube-apiserver.yaml
```

> **Why does the apiserver need a restart?** The webhook kubeconfig is loaded once at
> startup. Unlike the TLS server cert, the client certificate fields require an update to
> the kubeconfig file and an apiserver restart to take effect.

> **Note:** Webhook mode does NOT need `nodeSelector`, `tolerations`, or `runAsUser: 0`.
> The pod can run on any node — no hostPath, no control plane scheduling.

---

## Step 4: Verify Audicia Is Running

**Do not proceed to Step 5 until these checks pass.**

```bash
# Pod should be Running
kubectl get pods -n audicia-system

# Service should exist with port 8443 — note the CLUSTER-IP
kubectl get svc -n audicia-system

# Expected output includes:
#   audicia-audicia-operator-webhook   ClusterIP   10.x.x.x   <none>   8443/TCP
```

> **Write down the ClusterIP** (e.g., `10.111.100.194`). You will need it for the webhook
> kubeconfig in Step 6 and the TLS certificate SAN in Step 1.

Verify the webhook container port and TLS volume:

```bash
kubectl get deploy -n audicia-system -o jsonpath='{.items[0].spec.template.spec.containers[0].ports}' | jq .
kubectl get deploy -n audicia-system -o jsonpath='{.items[0].spec.template.spec.containers[0].volumeMounts}' | jq .
```

---

## Step 5: Create the Webhook AudiciaSource

Apply the [Webhook AudiciaSource](../examples/audicia-source-webhook.md) example or create your own:

```bash
kubectl apply -f - <<EOF
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
EOF
```

Verify the pipeline started:

```bash
kubectl describe audiciasource realtime-audit -n audicia-system
# Look for: Condition Ready=True, Reason=PipelineRunning
```

Check operator logs:

```bash
kubectl logs -n audicia-system deploy/audicia -f | grep webhook
# Expected: "starting webhook HTTPS server" port=8443
```

If using mTLS, you should also see: `"mTLS enabled" clientCA="/etc/audicia/webhook-client-ca/ca.crt"`

---

## Step 6: Create the Webhook Kubeconfig on the Control Plane

SSH to the control plane node and create the kubeconfig that tells the kube-apiserver
where to send audit events.

> **You MUST use the Service ClusterIP, not the DNS name.** The kube-apiserver runs with
> `hostNetwork: true` and cannot resolve `.svc.cluster.local` names — it uses the node's
> DNS resolver, not CoreDNS. If you use the DNS name, the apiserver will silently drop all
> audit events with `dial tcp: lookup ... no such host`.

Get the ClusterIP from Step 4, then create the kubeconfig. Example templates are in the
documentation:

- [Webhook Kubeconfig (Basic TLS)](../examples/webhook-kubeconfig.md) — basic TLS (no mTLS)
- [Webhook Kubeconfig (mTLS)](../examples/webhook-kubeconfig-mtls.md) — mTLS (recommended for production)

### Basic TLS kubeconfig (no mTLS)

```bash
# Replace <CLUSTER-IP> with the actual IP from: kubectl get svc -n audicia-system
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
users:
  - name: audicia
current-context: audicia
EOF
```

### mTLS kubeconfig (recommended for production)

If you enabled mTLS (with `clientCASecretName`), the apiserver must present a client
certificate. Add the `client-certificate` and `client-key` fields to the `users` section:

```bash
# Replace <CLUSTER-IP> with the actual IP from: kubectl get svc -n audicia-system
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

> **Which client cert?** On kubeadm clusters, the apiserver's client certificate is at
> `/etc/kubernetes/pki/apiserver-kubelet-client.crt` (with key `.key`). This cert is signed
> by the cluster CA (`/etc/kubernetes/pki/ca.crt`) — the same CA we put in the
> `kube-apiserver-client-ca` Secret. Both files are already inside the apiserver's
> `/etc/kubernetes/pki` volume mount, so no extra volumes are needed.

For example, if the ClusterIP is `10.111.100.194` (mTLS):

```bash
cat > /etc/kubernetes/audit-webhook-kubeconfig.yaml << 'EOF'
apiVersion: v1
kind: Config
clusters:
  - name: audicia
    cluster:
      certificate-authority: /etc/kubernetes/pki/audicia-webhook-ca.crt
      server: https://10.111.100.194:8443
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

The `certificate-authority` tells the kube-apiserver how to verify Audicia's TLS
certificate. Since we used a self-signed cert, the cert itself is the CA:

```bash
cp webhook-server.crt /etc/kubernetes/pki/audicia-webhook-ca.crt
```

> **Why this path?** The kube-apiserver container mounts `/etc/kubernetes/pki` as a
> read-only volume (standard kubeadm setup). Putting the CA cert here makes it visible
> inside the container without adding extra volume mounts.
>
> **ClusterIP stability:** The ClusterIP is stable across pod restarts and Helm upgrades
> as long as the Service is not deleted and recreated. If you run `helm uninstall` and
> `helm install` (which recreates the Service), the ClusterIP may change — update the
> kubeconfig and regenerate the TLS certificate.

---

## Step 7: Add the Apiserver Flag

> **Only do this after Steps 1-6 are complete and verified.** The apiserver validates
> the webhook config at startup. If anything is wrong, it crash-loops.

Edit the kube-apiserver static pod manifest:

```bash
vi /etc/kubernetes/manifests/kube-apiserver.yaml
```

Add the webhook flag to the `command` list. Place it near the other `--audit-*` flags:

```yaml
    - --audit-webhook-config-file=/etc/kubernetes/audit-webhook-kubeconfig.yaml
```

You also need to mount the kubeconfig file into the container. Add a volumeMount and
volume:

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

> **Note:** The `/etc/kubernetes/pki` directory is already mounted by kubeadm, so the
> CA cert at `/etc/kubernetes/pki/audicia-webhook-ca.crt` is automatically visible.
> You only need an extra mount for the kubeconfig file itself.

Save the file. The kube-apiserver will restart automatically (kubelet watches static
pod manifests). Wait 30-60 seconds:

```bash
kubectl get nodes
```

If it doesn't respond within 2 minutes, see [Recovery](#recovery-apiserver-crash-loop).

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
kubectl logs -n audicia-system deploy/audicia --tail=50
```

---

## Dual Mode: File + Webhook

You can run both ingestion modes simultaneously. Each `AudiciaSource` gets its own
pipeline goroutine. The kube-apiserver supports both `--audit-log-path` (file) and
`--audit-webhook-config-file` (webhook) at the same time.

```bash
helm install audicia audicia/audicia \
  --create-namespace --namespace audicia-system \
  --set image.repository=felixnotka/audicia-operator \
  --set image.tag=latest \
  --set auditLog.enabled=true \
  --set auditLog.hostPath=/var/log/kubernetes/audit/audit.log \
  --set webhook.enabled=true \
  --set webhook.port=8443 \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.clientCASecretName=kube-apiserver-client-ca \
  --set nodeSelector."kubernetes\.io/hostname"=<CONTROL-PLANE-NODE> \
  --set tolerations[0].key="node-role.kubernetes.io/control-plane" \
  --set tolerations[0].operator="Exists" \
  --set tolerations[0].effect="NoSchedule" \
  --set podSecurityContext.runAsUser=0 \
  --set podSecurityContext.runAsNonRoot=false
```

> **Note:** Dual mode requires control plane scheduling because the file ingestor needs
> hostPath access. If you only use webhook mode, none of the nodeSelector/tolerations/root
> settings are needed.

Then apply both AudiciaSource CRs. See the [File Mode](../examples/audicia-source-file.md)
and [Webhook Mode](../examples/audicia-source-webhook.md) example pages for the full manifests.

---

## Kube-apiserver Webhook Reference

The kube-apiserver supports tuning the webhook backend:

| Flag                              | Default | Description                                       |
|-----------------------------------|---------|---------------------------------------------------|
| `--audit-webhook-config-file`     | —       | Path to the webhook kubeconfig (required).        |
| `--audit-webhook-mode`            | `batch` | `batch` (recommended) or `blocking`.              |
| `--audit-webhook-batch-max-wait`  | `30s`   | Max time to buffer events before sending a batch. |
| `--audit-webhook-batch-max-size`  | `400`   | Max events per batch.                             |
| `--audit-webhook-initial-backoff` | `10s`   | Backoff after a failed webhook request.           |
| `--audit-policy-file`             | —       | Shared by both file and webhook backends.         |

The default batch mode sends events in `EventList` batches every 30 seconds or when the
buffer reaches 400 events — whichever comes first.

---

## Recovery: Apiserver Crash Loop

If the kube-apiserver crash-loops after adding the webhook flag, see [Troubleshooting](../troubleshooting.md#apiserver-crash-loop-after-adding-webhook-flag).

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

For common webhook issues (no events, mTLS failures, DNS resolution, NetworkPolicy), see [Troubleshooting](../troubleshooting.md).
