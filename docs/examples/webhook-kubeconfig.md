# Webhook Kubeconfig (Basic TLS)

Kubeconfig for the kube-apiserver audit webhook backend. Tells the apiserver
where to send audit events.

**See also:** [Webhook Setup Guide](../guides/webhook-setup.md)

Replace `<CLUSTER-IP>` with your webhook Service ClusterIP
(`kubectl get svc -n audicia-system`). You must use the ClusterIP, not a DNS
name — the apiserver uses `hostNetwork: true` and cannot resolve
`.svc.cluster.local` names.

```yaml
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
```

## Notes

- **`certificate-authority`** — The CA that signed Audicia's webhook TLS
  certificate. For self-signed certs, the cert itself is the CA.
- **ClusterIP stability** — The ClusterIP is stable across pod restarts and Helm
  upgrades. It only changes if the Service is deleted and recreated (e.g.,
  `helm uninstall` + `helm install`). Pin the ClusterIP via
  `webhook.service.clusterIP` in the Helm values to avoid this.
- For mTLS, use the [mTLS kubeconfig](webhook-kubeconfig-mtls.md) instead.
