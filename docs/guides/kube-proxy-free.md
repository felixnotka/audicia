# Kube-Proxy-Free Clusters (Cilium, eBPF)

Running Audicia on clusters that replace kube-proxy with a CNI-native
implementation (e.g. Cilium with `kubeProxyReplacement: true`). This guide
covers the networking gotchas and their solutions for **both file mode and
webhook mode**.

> **Standard kube-proxy clusters?** You don't need this guide. Follow the normal
> [Installation](../getting-started/installation.md) or
> [Webhook Setup Guide](webhook-setup.md).

---

## The Problem

On standard clusters, kube-proxy programs iptables rules that translate
ClusterIP addresses into pod IPs. This routing works in both directions — from
pods to the Kubernetes API server, and from the host namespace to pod
ClusterIPs.

On kube-proxy-free clusters, the CNI handles service routing differently.
Cilium, for example, uses eBPF socket-level load balancing. Depending on the
Cilium version and configuration, **ClusterIP traffic may not be routed
correctly** between the host namespace and pod network. This breaks Audicia in
two ways:

1. **File mode (pod → apiserver):** The operator pod cannot reach the Kubernetes
   API server ClusterIP (`10.96.0.1:443`) to start its informer caches.
2. **Webhook mode (apiserver → pod):** The kube-apiserver (which runs with
   `hostNetwork: true`) cannot reach Audicia's webhook Service via its
   ClusterIP.

### File Mode Symptoms

- Operator pod starts but crashes with: `dial tcp 10.96.0.1:443: i/o timeout`
- Operator logs show `failed to prime RBAC cache informer` or the manager fails
  to start
- The pod never reaches `Ready`

### Webhook Mode Symptoms

- Audicia pod is running, webhook HTTPS server is listening on port 8443
- `curl -k https://<POD-IP>:8443` from the control plane node **works**
- `curl -k https://<CLUSTER-IP>:8443` from the control plane node **hangs or
  times out**
- No audit events arrive despite the apiserver having the webhook flag
  configured
- No errors in apiserver logs (batch mode silently drops failed deliveries)

---

## File Mode: hostNetwork {#file-mode-hostnetwork}

The simplest fix for file mode: run the operator pod with `hostNetwork: true`.
This makes the pod share the node's network namespace, bypassing the CNI
datapath entirely. The pod can then reach the Kubernetes API server directly
through the host's network stack.

This is safe because file-mode pods already run on the control plane node with
`hostPath` access to the audit log. Adding `hostNetwork` does not grant any
additional privilege beyond what `hostPath` already implies.

```bash
helm install audicia audicia/audicia-operator \
  --create-namespace --namespace audicia-system \
  --set auditLog.enabled=true \
  --set auditLog.hostPath=/var/log/kubernetes/audit/audit.log \
  --set hostNetwork=true \
  --set nodeSelector."node-role\.kubernetes\.io/control-plane"="" \
  --set tolerations[0].key=node-role.kubernetes.io/control-plane \
  --set tolerations[0].effect=NoSchedule \
  --set podSecurityContext.runAsUser=0 \
  --set podSecurityContext.runAsNonRoot=false
```

The Helm chart automatically sets `dnsPolicy: ClusterFirstWithHostNet` when
`hostNetwork` is enabled, so cluster DNS (e.g. for leader election lease
lookups) continues to work.

> **Already deployed without hostNetwork?** Patch the existing deployment:
>
> ```bash
> kubectl patch deployment -n audicia-system audicia-audicia-operator \
>   --type=json -p='[
>     {"op":"add","path":"/spec/template/spec/hostNetwork","value":true},
>     {"op":"add","path":"/spec/template/spec/dnsPolicy","value":"ClusterFirstWithHostNet"}
>   ]'
> ```

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
reach it via loopback:

```bash
helm install audicia audicia/audicia-operator \
  --create-namespace --namespace audicia-system \
  --set webhook.enabled=true \
  --set webhook.port=8443 \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.hostPort=true \
  --set nodeSelector."node-role\.kubernetes\.io/control-plane"="" \
  --set tolerations[0].key=node-role.kubernetes.io/control-plane \
  --set tolerations[0].effect=NoSchedule
```

To add mTLS, append:

```
--set webhook.clientCASecretName=kube-apiserver-client-ca
```

See the [Webhook Setup Guide](webhook-setup.md#optional-mtls-client-ca-secret)
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
instead. Set `webhook.service.nodePort` to a port in the 30000-32767 range:

```bash
helm install audicia audicia/audicia-operator \
  --create-namespace --namespace audicia-system \
  --set webhook.enabled=true \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.service.nodePort=30443
```

The webhook kubeconfig then uses the node IP and NodePort:

```yaml
server: https://<NODE-IP>:30443
```

And the TLS certificate SAN must include that node IP.

> **Caveat:** NodePort still goes through the CNI's service routing layer. On
> some Cilium configurations, NodePort works from the host namespace even when
> ClusterIP doesn't, but this is not guaranteed. hostPort is the most reliable
> option.

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

### Cilium socket LB

Cilium's `socketLB.enabled: true` (also known as `bpf-lb-sock`) is designed to
enable host-to-ClusterIP routing. However:

- It may require a **node reboot** after enabling (not just a Cilium agent
  restart)
- Behavior depends on your `socketLB.hostNamespaceOnly` setting
- It doesn't work in all kernel versions and Cilium configurations

hostPort bypasses all of this and is the recommended workaround.

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

---

## Related

- [Installation](../getting-started/installation.md) — Standard installation
  guide
- [Webhook Setup Guide](webhook-setup.md) — Full webhook setup (ClusterIP mode)
- [mTLS Setup](webhook-setup.md#upgrading-from-basic-tls-to-mtls) — Mutual TLS
  for webhook security
- [Troubleshooting](../troubleshooting.md) — Common issues and solutions
