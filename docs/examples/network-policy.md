# NetworkPolicy

NetworkPolicy to restrict webhook ingress to the kube-apiserver only. Prevents
unauthorized clients from sending fabricated audit events.

**See also:** [Webhook Setup Guide](../guides/webhook-setup.md) |
[Security Model](../concepts/security-model.md)

Replace `<CONTROL-PLANE-IP>` with your control plane node IP
(`kubectl get nodes -o wide`).

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: audicia-webhook-ingress
  namespace: audicia-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: audicia-operator
  policyTypes:
    - Ingress
  ingress:
    - from:
        - ipBlock:
            cidr: <CONTROL-PLANE-IP>/32
      ports:
        - protocol: TCP
          port: 8443
```

## Notes

- **CIDR** — Use `/32` for a single control plane node. For HA clusters with
  multiple control plane nodes, either list each IP or use a broader CIDR that
  covers all of them.
- **CNI behavior** — On clusters with overlay networks (Calico, Cilium,
  Flannel), the source IP the pod sees may be the node's internal IP, a pod CIDR
  gateway, or a masqueraded address. If events don't flow after applying this
  policy, check `kubectl logs` to see what source IP Audicia receives.
- This can also be enabled via Helm: `webhook.networkPolicy.enabled=true` with
  `webhook.networkPolicy.controlPlaneCIDR`.
