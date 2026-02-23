# mTLS Setup Guide

This guide covers enabling mutual TLS (mTLS) for Audicia's webhook endpoint. With mTLS, only the kube-apiserver
(presenting a valid client certificate) can send audit events to Audicia — preventing spoofing and poisoning attacks.

**Prerequisite:** You should already have basic TLS webhook mode working. If not, start with the
[Webhook Setup Guide](webhook-setup.md) first.

---

## How mTLS Works

In basic TLS mode, Audicia presents a server certificate and the kube-apiserver verifies it (one-way TLS). With mTLS:

1. Audicia presents its server certificate (same as basic TLS).
2. The kube-apiserver presents a **client certificate** signed by a trusted CA.
3. Audicia verifies the client certificate against a CA bundle you provide.

This ensures only the kube-apiserver — not any arbitrary network client — can send audit events.

---

## Step 1: Create the Client CA Secret

The client CA is the CA that signed the kube-apiserver's client certificate. On kubeadm clusters, this is the cluster
CA at `/etc/kubernetes/pki/ca.crt`:

```bash
# The cluster CA is at /etc/kubernetes/pki/ca.crt on the control plane
cp /etc/kubernetes/pki/ca.crt ./cluster-ca.crt

# Create the Secret
kubectl create secret generic kube-apiserver-client-ca \
  --from-file=ca.crt=./cluster-ca.crt \
  -n audicia-system
```

## Step 2: Update the AudiciaSource

The `clientCASecretName` field on the AudiciaSource CR tells the controller to enable mTLS. Either:

**Option A: Apply the [hardened example](../examples/audicia-source-hardened.md):**

```bash
kubectl delete audiciasource realtime-audit -n audicia-system
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

**Option B: Patch your existing AudiciaSource:**

```bash
kubectl patch audiciasource <NAME> -n audicia-system --type=merge \
  -p '{"spec":{"webhook":{"clientCASecretName":"kube-apiserver-client-ca"}}}'
```

> **Important:** The `clientCASecretName` in the AudiciaSource CR is what enables mTLS, not the Helm value. The Helm
> value only mounts the volume in the Deployment.

## Step 3: Helm Upgrade

Mount the client CA volume by setting the Helm value:

```bash
helm upgrade audicia audicia/audicia-operator \
  --namespace audicia-system \
  --set webhook.enabled=true \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.clientCASecretName=kube-apiserver-client-ca
```

The pod will restart. Verify mTLS is active:

```bash
kubectl logs -n audicia-system deploy/audicia --tail=20 | grep mTLS
# Expected: "mTLS enabled" clientCA="/etc/audicia/webhook-client-ca/ca.crt"
```

## Step 4: Update the Webhook Kubeconfig

The kube-apiserver must now present a client certificate when connecting to Audicia's webhook. On the control plane
node, update the webhook kubeconfig to add client cert and key:

Replace `<WEBHOOK-IP>` with the Service ClusterIP (ClusterIP mode) or `127.0.0.1` (hostPort mode):

```yaml
apiVersion: v1
kind: Config
clusters:
  - name: audicia
    cluster:
      certificate-authority: /etc/kubernetes/pki/audicia-webhook-ca.crt
      server: https://<WEBHOOK-IP>:8443
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
```

See the [Webhook Kubeconfig (mTLS)](../examples/webhook-kubeconfig-mtls.md) example for the complete template.

> **Which client cert?** On kubeadm clusters, the apiserver's client certificate is at
> `/etc/kubernetes/pki/apiserver-kubelet-client.crt`. This cert is signed by the cluster CA — the same CA we put in
> the `kube-apiserver-client-ca` Secret. Both files are already inside the apiserver's `/etc/kubernetes/pki` volume
> mount, so no extra volumes are needed.

## Step 5: Restart the Apiserver

The webhook kubeconfig is loaded once at startup. After updating it:

```bash
# Move the manifest to trigger a restart
mv /etc/kubernetes/manifests/kube-apiserver.yaml /etc/kubernetes/kube-apiserver.yaml
sleep 5
mv /etc/kubernetes/kube-apiserver.yaml /etc/kubernetes/manifests/kube-apiserver.yaml
```

Wait for the apiserver to come back:

```bash
kubectl get nodes
```

## Verify mTLS Is Working

1. **Operator logs** should show `"mTLS enabled"` and no TLS errors:
   ```bash
   kubectl logs -n audicia-system deploy/audicia --tail=50
   ```

2. **Reports should start appearing** after generating API activity:
   ```bash
   kubectl get audiciapolicyreports --all-namespaces
   ```

3. **Unauthorized clients should be rejected.** Test from a pod without a client cert:
   ```bash
   kubectl run test-curl --rm -it --image=curlimages/curl -- \
     curl -k -v -X POST https://<SERVICE-NAME>.audicia-system.svc:8443/ -d '{}'
   # Expected: TLS handshake failure
   ```

---

## Troubleshooting

For mTLS-specific issues (`tls: client didn't provide a certificate`, `tls: bad certificate`, `mTLS enabled` not appearing), see [Troubleshooting](../troubleshooting.md).
