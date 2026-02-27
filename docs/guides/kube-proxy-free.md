# Kube-Proxy-Free Clusters (Cilium, eBPF)

Guide for running Audicia on clusters that replace kube-proxy with a CNI-native
implementation (e.g. Cilium with `kubeProxyReplacement: true`). Covers both file
mode and webhook mode.

If your cluster uses standard kube-proxy, you don't need this guide — follow the
normal [Installation](../getting-started/installation.md) or
[Webhook Setup Guide](webhook-setup.md).

---

## The Problem

On kube-proxy-free clusters, **ClusterIP traffic may not be routed correctly**
between the host namespace and pod network. This breaks Audicia in two ways:

- **File mode (pod → apiserver):** The operator pod cannot reach the Kubernetes
  API server ClusterIP. Symptoms: pod crashes with
  `dial tcp 10.96.0.1:443: i/o timeout`, never reaches `Ready`.

- **Webhook mode (apiserver → pod):** The kube-apiserver (running with
  `hostNetwork: true`) cannot reach Audicia's webhook Service via ClusterIP.
  Symptoms: `curl -k https://<POD-IP>:8443` works but
  `curl -k https://<CLUSTER-IP>:8443` hangs. No audit events arrive.

---

## File Mode: hostNetwork

Run the operator pod with `hostNetwork: true` to bypass the CNI datapath.

Create a `values-file.yaml`:

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

podSecurityContext:
  runAsUser: 0
  runAsNonRoot: false
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-file.yaml
```

---

## Webhook Mode: hostPort

Instead of routing through a ClusterIP Service, expose the webhook directly on
the node using `hostPort`. The kube-apiserver connects via `127.0.0.1`
(loopback), which bypasses service routing entirely.

### Step 1: Generate the TLS Certificate

The certificate SAN must be `127.0.0.1` (not a ClusterIP):

```bash
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout webhook-server.key \
  -out webhook-server.crt \
  -days 365 \
  -subj "/CN=audicia-webhook" \
  -addext "subjectAltName=IP:127.0.0.1"
```

### Step 2: Create the TLS Secret

```bash
kubectl create namespace audicia-system --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret tls audicia-webhook-tls \
  --cert=webhook-server.crt \
  --key=webhook-server.key \
  -n audicia-system
```

### Step 3: Install with hostPort Enabled

The operator must be scheduled on the control plane node so the apiserver can
reach it via loopback.

Create a `values-webhook-hostport.yaml`:

```yaml
# values-webhook-hostport.yaml
webhook:
  enabled: true
  port: 8443
  tlsSecretName: audicia-webhook-tls
  hostPort: true
  clientCASecretName: kube-apiserver-client-ca # optional: remove for basic TLS

nodeSelector:
  node-role.kubernetes.io/control-plane: ""

tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-webhook-hostport.yaml
```

See the
[Webhook Setup Guide](webhook-setup.md#step-1-create-the-namespace-and-secrets)
for creating the client CA Secret.

### Step 4: Create the Webhook Kubeconfig

On the control plane node:

```bash
# Copy the self-signed cert as the CA
cp webhook-server.crt /etc/kubernetes/pki/audicia-webhook-ca.crt

# Create the kubeconfig (basic TLS)
cat > /etc/kubernetes/audit-webhook-kubeconfig.yaml << 'EOF'
apiVersion: v1
kind: Config
clusters:
  - name: audicia
    cluster:
      certificate-authority: /etc/kubernetes/pki/audicia-webhook-ca.crt
      server: https://127.0.0.1:8443
contexts:
  - name: audicia
    context:
      cluster: audicia
users:
  - name: audicia
current-context: audicia
EOF
```

For mTLS, add client certificate fields to the `users` section. See the
[mTLS kubeconfig example](../examples/webhook-kubeconfig-mtls.md).

### Step 5: Add the Apiserver Flag

Follow
[Step 7 of the Webhook Setup Guide](webhook-setup.md#step-7-add-the-apiserver-flag)
to add `--audit-webhook-config-file` and the volume mount to the apiserver
manifest.

---

## Alternative: NodePort Mode

If hostPort doesn't fit your setup (e.g. port conflicts), you can use NodePort
instead. Note that NodePort still goes through the CNI's service routing layer
and is not guaranteed to work on all Cilium configurations.

Create a `values-webhook-nodeport.yaml`:

```yaml
# values-webhook-nodeport.yaml
webhook:
  enabled: true
  tlsSecretName: audicia-webhook-tls
  service:
    nodePort: 30443
```

```bash
helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  -f values-webhook-nodeport.yaml
```

The webhook kubeconfig then uses the node IP and NodePort
(`server: https://<NODE-IP>:30443`), and the TLS certificate SAN must include
that node IP.

---

## Diagnosing ClusterIP Routing

If you're unsure whether your cluster has this issue, run these commands on the
**control plane node**:

```bash
# Check if pod IP works (should work on any cluster)
POD_IP=$(kubectl get pod -n audicia-system -l app.kubernetes.io/name=audicia-operator \
  -o jsonpath='{.items[0].status.podIP}')
curl -k https://${POD_IP}:8443 -v --connect-timeout 5

# Check if ClusterIP works
CLUSTER_IP=$(kubectl get svc -n audicia-system \
  -l app.kubernetes.io/name=audicia-operator \
  -o jsonpath='{.items[?(@.spec.ports[0].name=="webhook")].spec.clusterIP}')
curl -k https://${CLUSTER_IP}:8443 -v --connect-timeout 5
```

If the pod IP responds but the ClusterIP hangs, you need hostPort or NodePort
mode.

---

## Helm Values Reference

| Value                      | Type    | Default | Description                                                                  |
| -------------------------- | ------- | ------- | ---------------------------------------------------------------------------- |
| `hostNetwork`              | boolean | `false` | Use the host network namespace (file mode fix).                              |
| `dnsPolicy`                | string  | `""`    | DNS policy override. Auto-set to `ClusterFirstWithHostNet` with hostNetwork. |
| `webhook.hostPort`         | boolean | `false` | Expose the webhook port directly on the host via hostPort.                   |
| `webhook.service.nodePort` | string  | `""`    | Fixed NodePort (30000-32767). Changes the Service type to NodePort when set. |

See the full [Helm Values Reference](../configuration/helm-values.md) for all
options.
