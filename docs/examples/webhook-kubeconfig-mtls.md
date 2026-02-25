# Webhook Kubeconfig (mTLS)

Kubeconfig with mutual TLS for the kube-apiserver audit webhook backend. The
apiserver presents a client certificate so Audicia can verify the caller.

**See also:** [mTLS Setup Guide](../guides/mtls-setup.md) |
[Webhook Setup Guide](../guides/webhook-setup.md)

Replace `<CLUSTER-IP>` with your webhook Service ClusterIP
(`kubectl get svc -n audicia-system`).

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
      user: kube-apiserver
users:
  - name: kube-apiserver
    user:
      client-certificate: /etc/kubernetes/pki/apiserver-kubelet-client.crt
      client-key: /etc/kubernetes/pki/apiserver-kubelet-client.key
current-context: audicia
```

## Notes

- **Client certificate** — On kubeadm clusters, the apiserver's client cert is
  at `/etc/kubernetes/pki/apiserver-kubelet-client.crt`. This cert is signed by
  the cluster CA — the same CA referenced in the `kube-apiserver-client-ca`
  Secret.
- Both cert files are already inside the apiserver's `/etc/kubernetes/pki`
  volume mount, so no extra volumes are needed.
- The kube-apiserver must be restarted after updating the kubeconfig (it loads
  the config once at startup).
