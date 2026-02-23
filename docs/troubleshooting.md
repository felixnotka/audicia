# Troubleshooting

Common issues and their solutions, organized by symptom.

---

## No AudiciaPolicyReports appear

**Possible causes:**

1. **Audit logging not enabled.** Audicia needs audit events to work. Verify the audit log exists and contains events:
   ```bash
   head -5 /var/log/kube-audit.log
   ```
   If empty or missing, check that `--audit-policy-file` and `--audit-log-path` (or `--audit-webhook-config-file`) are set on the kube-apiserver. See [Audit Policy](guides/audit-policy.md).

2. **No AudiciaSource CR applied.** Check that a source exists and is Ready:
   ```bash
   kubectl get audiciasources -n audicia-system
   kubectl describe audiciasource -n audicia-system
   ```

3. **All events filtered out.** If your filter chain is too aggressive, no events pass through. Temporarily remove all filters to confirm events flow:
   ```yaml
   spec:
     ignoreSystemUsers: false
     filters: []
   ```
   Then add filters back one at a time. See [Filter Recipes](guides/filter-recipes.md).

4. **Reports flush on a timer.** Reports are written every `checkpoint.intervalSeconds` (default 30s). Wait at least one interval after generating API activity.

---

## Webhook: No events arrive

The webhook receiver is running but the kube-apiserver is not sending events.

1. **Check apiserver logs** (most important step):
   ```bash
   cat /var/log/pods/kube-system_kube-apiserver-*/kube-apiserver/*.log | grep -i webhook | tail -10
   ```
   - `no such host` — DNS resolution failure. See [DNS resolution failure](#dns-resolution-failure) below.
   - `tls: bad certificate` — TLS cert SAN doesn't match the server address. Regenerate the cert with the correct IP (ClusterIP or `127.0.0.1` for hostPort).
   - `connection refused` — Audicia isn't running or the Service has no endpoints.
   - Connection timeout / no errors — ClusterIP may be unreachable from the host. See [ClusterIP unreachable](#clusterip-unreachable-from-host-namespace).
   - No webhook errors at all — The apiserver may not have the `--audit-webhook-config-file` flag.

2. **Verify the webhook kubeconfig uses an IP address, not a DNS name:**
   ```bash
   cat /etc/kubernetes/audit-webhook-kubeconfig.yaml
   ```
   The `server:` field must be an IP address (`https://<CLUSTER-IP>:8443` or `https://127.0.0.1:8443` for hostPort mode), NOT a `.svc.cluster.local` name. The apiserver uses `hostNetwork: true` and cannot resolve cluster DNS.

3. **Check that the audit policy allows the events you expect:**
   ```bash
   cat /etc/kubernetes/audit-policy.yaml
   ```
   The policy must have at least `Metadata` level for the resources you care about.

4. **Check that the webhook Service has endpoints:**
   ```bash
   kubectl get svc -n audicia-system
   kubectl get endpoints -n audicia-system
   ```

5. **Check for NetworkPolicies blocking traffic:**
   ```bash
   kubectl get networkpolicy -n audicia-system
   ```
   See [NetworkPolicy blocking traffic](#networkpolicy-blocking-traffic) below.

6. **Check the apiserver has the webhook flag:**
   ```bash
   ps aux | grep audit-webhook
   ```

---

## ClusterIP unreachable from host namespace

The kube-apiserver runs with `hostNetwork: true`. On some CNIs — particularly Cilium in
kube-proxy-free mode — ClusterIP traffic from the host namespace is not routed to pods.
The apiserver silently drops audit events (in batch mode) or logs connection timeouts.

**Symptoms:** Audicia pod is running, webhook HTTPS server is listening, but `curl -k
https://<CLUSTER-IP>:8443` hangs from the control plane node. Curling the pod IP directly
works fine.

**Fix:** Use hostPort mode. See the [Kube-Proxy-Free Guide](guides/kube-proxy-free.md)
for the complete setup including diagnosis steps, hostPort configuration, and TLS
certificate generation.

---

## DNS resolution failure

The apiserver logs show:

```
dial tcp: lookup audicia-....svc on 185.12.64.1:53: no such host
```

The kube-apiserver is trying to resolve the Service DNS name using the node's DNS resolver (e.g., your ISP's DNS), not the cluster's CoreDNS. This happens because the apiserver runs with `hostNetwork: true`.

**Fix:** Use an IP address in the webhook kubeconfig:

```bash
# Get the ClusterIP
kubectl get svc -n audicia-system

# Update the kubeconfig — replace the DNS name with the ClusterIP
vi /etc/kubernetes/audit-webhook-kubeconfig.yaml
```

Also ensure the TLS certificate has the IP as a SAN. If it only has DNS SANs, regenerate it. See [Webhook Setup Guide](guides/webhook-setup.md#step-1-generate-a-self-signed-tls-certificate).

After updating, restart the apiserver:

```bash
mv /etc/kubernetes/manifests/kube-apiserver.yaml /etc/kubernetes/kube-apiserver.yaml
sleep 5
mv /etc/kubernetes/kube-apiserver.yaml /etc/kubernetes/manifests/kube-apiserver.yaml
```

---

## NetworkPolicy blocking traffic

If you applied a NetworkPolicy in the `audicia-system` namespace, it may silently block all traffic to the webhook.

**Symptoms:** Audicia pod is running, webhook HTTPS server is listening, but no events arrive. The kube-apiserver logs show no webhook errors (it silently drops events when the connection times out in batch mode).

**Diagnose:**

```bash
kubectl get networkpolicy -n audicia-system -o yaml
```

Check the `ingress.from.ipBlock.cidr` — it must include the IP your kube-apiserver uses to reach the pod network:

```bash
kubectl get nodes -o wide | grep control-plane
```

**Fix:**

```bash
# Option 1: Remove the restriction entirely
kubectl delete networkpolicy audicia-webhook-ingress -n audicia-system

# Option 2: Update with the correct control plane IP
kubectl patch networkpolicy audicia-webhook-ingress -n audicia-system --type=json \
  -p='[{"op":"replace","path":"/spec/ingress/0/from/0/ipBlock/cidr","value":"<CONTROL-PLANE-IP>/32"}]'
```

> **Note:** On clusters with overlay networks (Calico, Cilium, Flannel), the source IP may be the node's internal IP, a pod CIDR gateway, or a masqueraded address. If in doubt, remove the NetworkPolicy first, confirm events flow, then use `kubectl logs` to identify the actual source IP.

---

## Apiserver crash loop after adding webhook flag

If you added `--audit-webhook-config-file` before Audicia was running (or before the CA cert was in place), the kube-apiserver will crash loop and `kubectl` stops working.

**Symptoms:**

```
$ kubectl get nodes
The connection to the server <IP>:6443 was refused
```

**Check the apiserver logs:**

```bash
ls -lt /var/log/pods/kube-system_kube-apiserver-*/kube-apiserver/
cat /var/log/pods/kube-system_kube-apiserver-<POD-ID>/kube-apiserver/<LATEST>.log
```

| Error | Cause |
|-------|-------|
| `unable to read certificate-authority ... no such file or directory` | CA cert not at the path in the webhook kubeconfig |
| `initializing audit webhook: invalid configuration` | Webhook kubeconfig is malformed or references missing files |
| `dial tcp ...: connection refused` | Webhook Service doesn't exist or Audicia isn't running |

**Fix:** Remove the webhook flag to recover:

```bash
sed -i '/audit-webhook-config-file/d' /etc/kubernetes/manifests/kube-apiserver.yaml
```

The kubelet will restart the apiserver. Due to crash backoff, this may take up to 2 minutes:

```bash
crictl ps | grep apiserver
```

Once `kubectl` works again, follow the [Webhook Setup Guide](guides/webhook-setup.md) in the correct order: install Audicia first, verify it's running, then add the apiserver flag.

---

## mTLS: `tls: client didn't provide a certificate`

The apiserver is connecting without a client certificate. The webhook kubeconfig doesn't have `client-certificate` and `client-key` in the `users` section.

**Fix:** Update the kubeconfig to the mTLS format and restart the apiserver. See [Webhook Kubeconfig (mTLS)](examples/webhook-kubeconfig-mtls.md).

---

## mTLS: `tls: bad certificate`

The apiserver presented a client certificate, but it wasn't signed by the CA in the `kube-apiserver-client-ca` Secret.

**Verify the Secret has the correct CA:**

```bash
kubectl get secret kube-apiserver-client-ca -n audicia-system \
  -o jsonpath='{.data.ca\.crt}' | base64 -d | openssl x509 -text -noout | head -5
```

This should show your cluster's CA (same issuer as `/etc/kubernetes/pki/ca.crt`).

---

## mTLS: `mTLS enabled` not in operator logs

The `AudiciaSource` CR doesn't have `clientCASecretName` set. The Helm value only mounts the volume — the CR tells the controller to enable mTLS.

**Fix:**

```bash
kubectl patch audiciasource <NAME> -n audicia-system --type=merge \
  -p '{"spec":{"webhook":{"clientCASecretName":"kube-apiserver-client-ca"}}}'
```

---

## File mode: `IngestionError` on AudiciaSource

The operator can't read the audit log file.

**Common causes:**

- **Wrong path** — Verify `spec.location.path` matches `--audit-log-path` on the apiserver.
- **Permission denied** — The operator pod needs read access. File mode typically requires `runAsUser: 0`.
- **Pod not on control plane** — File mode needs `nodeSelector` and `tolerations` for control plane scheduling.

Check pod logs:

```bash
kubectl logs -n audicia-system -l app.kubernetes.io/name=audicia-operator
```

---

## Reports keep growing / report too large

Reports grow as new rules are observed. Without limits, they can approach etcd's 1.5MB object size limit.

**Fix:** Set retention and size limits on the AudiciaSource:

```yaml
spec:
  limits:
    maxRulesPerReport: 200
    retentionDays: 30
```

Rules not seen within the retention window are dropped during flush. When the max is exceeded, the oldest rules (by `lastSeen`) are dropped first.
