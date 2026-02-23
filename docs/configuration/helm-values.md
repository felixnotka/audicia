# Helm Values Reference

Complete reference for all Helm chart configuration values.

```bash
helm repo add audicia https://charts.audicia.io
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace
```

---

## Deployment

| Value              | Type    | Default                       | Description                                                        |
|--------------------|---------|-------------------------------|--------------------------------------------------------------------|
| `replicaCount`     | integer | `1`                           | Number of replicas. Only 1 is needed (leader election handles HA). |
| `image.repository` | string  | `felixnotka/audicia-operator` | Container image repository.                                        |
| `image.pullPolicy` | string  | `Always`                      | Image pull policy.                                                 |
| `image.tag`        | string  | `""`                          | Image tag override. When empty, defaults to chart `appVersion`. Does **not** pull `latest`. |
| `imagePullSecrets` | list    | `[]`                          | Image pull secrets for private registries.                         |
| `nameOverride`     | string  | `""`                          | Override the release name.                                         |
| `fullnameOverride` | string  | `""`                          | Override the full release name.                                    |

## Service Account

| Value                        | Type    | Default | Description                                                  |
|------------------------------|---------|---------|--------------------------------------------------------------|
| `serviceAccount.create`      | boolean | `true`  | Whether to create a ServiceAccount.                          |
| `serviceAccount.annotations` | object  | `{}`    | Annotations to add to the ServiceAccount.                    |
| `serviceAccount.name`        | string  | `""`    | Name of the ServiceAccount. If not set, a name is generated. |

## Pod Configuration

| Value                                      | Type    | Default | Description                    |
|--------------------------------------------|---------|---------|--------------------------------|
| `podAnnotations`                           | object  | `{}`    | Pod annotations.               |
| `podSecurityContext.runAsNonRoot`          | boolean | `true`  | Run as non-root user.          |
| `podSecurityContext.runAsUser`             | integer | `10000` | User ID.                       |
| `podSecurityContext.fsGroup`               | integer | `10000` | Filesystem group.              |
| `securityContext.allowPrivilegeEscalation` | boolean | `false` | Disallow privilege escalation. |
| `securityContext.readOnlyRootFilesystem`   | boolean | `true`  | Read-only root filesystem.     |
| `securityContext.capabilities.drop`        | list    | `[ALL]` | Drop all Linux capabilities.   |

## Resources

| Value                       | Type   | Default | Description     |
|-----------------------------|--------|---------|-----------------|
| `resources.requests.cpu`    | string | `100m`  | CPU request.    |
| `resources.requests.memory` | string | `128Mi` | Memory request. |
| `resources.limits.cpu`      | string | `500m`  | CPU limit.      |
| `resources.limits.memory`   | string | `256Mi` | Memory limit.   |

## Scheduling

| Value          | Type   | Default | Description                                                                   |
|----------------|--------|---------|-------------------------------------------------------------------------------|
| `nodeSelector` | object | `{}`    | Node selector. Set `node-role.kubernetes.io/control-plane: ""` for file mode. |
| `tolerations`  | list   | `[]`    | Tolerations. Add control-plane toleration for file mode.                      |
| `affinity`     | object | `{}`    | Affinity rules.                                                               |

## Operator Runtime

Runtime settings for the Audicia operator. These are exposed as Helm values and set as environment variables on the operator container.

| Value                             | Type    | Default | Env Var                     | Description                                              |
|-----------------------------------|---------|---------|-----------------------------|----------------------------------------------------------|
| `operator.metricsBindAddress`     | string  | `:8080` | `METRICS_BIND_ADDRESS`      | Prometheus metrics endpoint bind address.                |
| `operator.healthProbeBindAddress` | string  | `:8081` | `HEALTH_PROBE_BIND_ADDRESS` | Health probe (liveness/readiness) bind address.          |
| `operator.leaderElection.enabled` | boolean | `true`  | `LEADER_ELECTION_ENABLED`   | Enable leader election for HA.                           |
| `operator.logLevel`               | integer | `0`     | `LOG_LEVEL`                 | Log verbosity (0=info, 1=debug, 2=trace).                |

### Additional Runtime Environment Variables

These environment variables are not exposed as top-level Helm values but can be set via `extraEnv` or by customizing the Deployment template:

| Env Var                     | Default                 | Description                                             |
|-----------------------------|-------------------------|---------------------------------------------------------|
| `LEADER_ELECTION_ID`        | `audicia-operator-lock` | Lease resource name for leader election.                |
| `LEADER_ELECTION_NAMESPACE` | `audicia-system`        | Namespace for the Lease (auto-set from pod namespace).  |
| `CONCURRENT_RECONCILES`     | `1`                     | Number of parallel reconcile loops.                     |
| `SYNC_PERIOD`               | `10m`                   | Minimum interval between full cache resynchronizations. |

### Logging Levels

| Level     | Content                                                                 |
|-----------|-------------------------------------------------------------------------|
| 0 (info)  | Reconcile events, pipeline start/stop, report updates, errors.          |
| 1 (debug) | Skipped malformed lines, compliance skip reasons, inode check warnings. |
| 2 (trace) | Available but not currently used.                                       |

### Health Probes

| Probe     | Endpoint   | Port | Description                                  |
|-----------|------------|------|----------------------------------------------|
| Liveness  | `/healthz` | 8081 | Basic ping check — operator process is alive |
| Readiness | `/readyz`  | 8081 | Operator is ready to process events          |

Both use standard `healthz.Ping` checks. Ports are configurable via `operator.healthProbeBindAddress`.

## Audit Log (File Mode)

| Value               | Type    | Default                   | Description                           |
|---------------------|---------|---------------------------|---------------------------------------|
| `auditLog.enabled`  | boolean | `false`                   | Enable mounting the audit log volume. |
| `auditLog.hostPath` | string  | `/var/log/kube-audit.log` | Host path to the audit log file.      |

When enabled, mounts the host file as a read-only volume. Requires control plane scheduling (nodeSelector +
tolerations) and typically `runAsUser: 0` for hostPath read access.

## Webhook (Webhook Mode)

| Value                          | Type    | Default | Description                                                                                    |
|--------------------------------|---------|---------|------------------------------------------------------------------------------------------------|
| `webhook.enabled`              | boolean | `false` | Enable the webhook audit event receiver.                                                       |
| `webhook.port`                 | integer | `8443`  | HTTPS port for the webhook receiver.                                                           |
| `webhook.tlsSecretName`        | string  | `""`    | Name of a TLS Secret (must contain `tls.crt` and `tls.key`). Required when webhook is enabled. |
| `webhook.clientCASecretName`   | string  | `""`    | Name of a Secret containing `ca.crt` for mTLS. Optional but recommended for production.        |
| `webhook.hostPort`             | boolean | `false` | Expose the webhook port on the host via hostPort. Recommended for Cilium / kube-proxy-free clusters where ClusterIP is unreachable from the host namespace. Requires control plane node scheduling. |
| `webhook.service.clusterIP`              | string  | `""`    | Fixed ClusterIP for the webhook Service. Survives uninstall/reinstall cycles.       |
| `webhook.service.nodePort`               | string  | `""`    | Fixed NodePort (30000-32767). When set, the Service type is changed to NodePort.    |
| `webhook.networkPolicy.enabled`          | boolean | `false` | Create a NetworkPolicy restricting webhook ingress to the kube-apiserver.           |
| `webhook.networkPolicy.controlPlaneCIDR` | string  | `""`    | CIDR of your control plane node(s). Required when networkPolicy is enabled.         |

When enabled, adds:

- Webhook containerPort (with `hostPort` if `webhook.hostPort` is true)
- TLS Secret volume + volumeMount at `/etc/audicia/webhook-tls`
- Client CA Secret volume + volumeMount at `/etc/audicia/webhook-client-ca` (only when `clientCASecretName` is set)
- A ClusterIP or NodePort Service for the webhook endpoint
- A NetworkPolicy (only when `webhook.networkPolicy.enabled` is true)

For `hostPort` and `nodePort` usage on kube-proxy-free clusters, see the
[Kube-Proxy-Free Guide](../guides/kube-proxy-free.md).

## Monitoring

| Value                     | Type    | Default | Description                                                        |
|---------------------------|---------|---------|--------------------------------------------------------------------|
| `serviceMonitor.enabled`  | boolean | `false` | Create a Prometheus ServiceMonitor for automatic scrape discovery. |
| `serviceMonitor.labels`   | object  | `{}`    | Additional labels for the ServiceMonitor.                          |
| `serviceMonitor.interval` | string  | `30s`   | Scrape interval.                                                   |

---

## Example: File Mode (Control Plane)

```bash
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set auditLog.enabled=true \
  --set auditLog.hostPath=/var/log/kube-audit.log \
  --set nodeSelector."node-role\.kubernetes\.io/control-plane"="" \
  --set tolerations[0].key=node-role.kubernetes.io/control-plane \
  --set tolerations[0].effect=NoSchedule \
  --set podSecurityContext.runAsUser=0 \
  --set podSecurityContext.runAsNonRoot=false
```

## Example: Webhook Mode (ClusterIP)

```bash
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set webhook.enabled=true \
  --set webhook.tlsSecretName=audicia-webhook-tls
```

## Example: Webhook Mode with mTLS

```bash
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set webhook.enabled=true \
  --set webhook.tlsSecretName=audicia-webhook-tls \
  --set webhook.clientCASecretName=kube-apiserver-client-ca \
  --set webhook.service.clusterIP=<CLUSTER-IP> \
  --set webhook.networkPolicy.enabled=true \
  --set webhook.networkPolicy.controlPlaneCIDR=<CONTROL-PLANE-IP>/32
```

Replace `<CLUSTER-IP>` with a free IP from your service CIDR and `<CONTROL-PLANE-IP>` with your control plane
node IP (`kubectl get nodes -o wide`).
