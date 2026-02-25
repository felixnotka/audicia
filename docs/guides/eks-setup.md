# EKS Setup (CloudWatch Logs)

This guide walks through configuring Audicia to ingest audit logs from an Amazon
EKS cluster via CloudWatch Logs using IRSA (IAM Roles for Service Accounts).

## Prerequisites

- An EKS cluster
- Helm 3
- `eksctl` or `aws` CLI for IAM/OIDC setup

## Step 1: Enable EKS Audit Logging

EKS control plane logging is **disabled by default**. You must explicitly enable
the `audit` log type. Once enabled, audit events stream to CloudWatch Logs under
the log group `/aws/eks/<CLUSTER_NAME>/cluster`.

> **Cost note:** Enabling control plane logging incurs CloudWatch Logs charges.
> Consider setting a retention policy on the log group to control costs (e.g.,
> 30 or 90 days). AWS may also truncate very large audit log entries — see
> [EKS logging documentation](https://docs.aws.amazon.com/eks/latest/userguide/control-plane-logs.html)
> for details on limits.

```bash
# Enable audit logging
aws eks update-cluster-config \
  --name <CLUSTER_NAME> \
  --logging '{"clusterLogging":[{"types":["audit"],"enabled":true}]}'

# Optional: set log retention to control costs (default is indefinite)
aws logs put-retention-policy \
  --log-group-name "/aws/eks/<CLUSTER_NAME>/cluster" \
  --retention-in-days 90
```

Verify the log group exists and contains audit events:

```bash
aws logs describe-log-groups \
  --log-group-name-prefix "/aws/eks/<CLUSTER_NAME>/cluster"

aws logs filter-log-events \
  --log-group-name "/aws/eks/<CLUSTER_NAME>/cluster" \
  --log-stream-name-prefix "kube-apiserver-audit-" \
  --limit 5
```

## Step 2: Create an IAM Policy

Create an IAM policy that grants read access to the CloudWatch Logs group:

```bash
cat > audicia-cloudwatch-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:FilterLogEvents",
        "logs:DescribeLogStreams"
      ],
      "Resource": "arn:aws:logs:<REGION>:<ACCOUNT_ID>:log-group:/aws/eks/<CLUSTER_NAME>/cluster:*"
    }
  ]
}
EOF

aws iam create-policy \
  --policy-name AudiciaCloudWatchReadOnly \
  --policy-document file://audicia-cloudwatch-policy.json
```

Note the policy ARN from the output.

## Step 3: Set Up IRSA

IRSA allows Kubernetes ServiceAccounts to assume IAM roles without static
credentials.

**1. Create an OIDC provider for your cluster (if not already done):**

```bash
eksctl utils associate-iam-oidc-provider \
  --cluster <CLUSTER_NAME> \
  --approve
```

**2. Create an IAM role and associate it with the Audicia ServiceAccount:**

```bash
eksctl create iamserviceaccount \
  --cluster <CLUSTER_NAME> \
  --namespace audicia-system \
  --name audicia-operator \
  --attach-policy-arn arn:aws:iam::<ACCOUNT_ID>:policy/AudiciaCloudWatchReadOnly \
  --approve \
  --override-existing-serviceaccounts
```

Or manually, create the IAM role with a trust policy for the OIDC provider and
annotate the ServiceAccount:

```bash
OIDC_PROVIDER=$(aws eks describe-cluster --name <CLUSTER_NAME> \
  --query "cluster.identity.oidc.issuer" --output text | sed 's|https://||')

cat > trust-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::<ACCOUNT_ID>:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:audicia-system:audicia-operator",
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
EOF

aws iam create-role \
  --role-name audicia-operator \
  --assume-role-policy-document file://trust-policy.json

aws iam attach-role-policy \
  --role-name audicia-operator \
  --policy-arn arn:aws:iam::<ACCOUNT_ID>:policy/AudiciaCloudWatchReadOnly
```

> **Note:** The `--namespace` and `--name` must match the Helm chart defaults
> (`audicia-system:audicia-operator`).

## Step 4: Install with Helm

Create a `values-eks.yaml` file with your cluster-specific configuration:

```yaml
# values-eks.yaml
cloudAuditLog:
  enabled: true
  provider: AWSCloudWatch
  aws:
    logGroupName: "/aws/eks/<CLUSTER_NAME>/cluster"
    region: "<REGION>"

image:
  tag: "<VERSION>-aws"

serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: "arn:aws:iam::<ACCOUNT_ID>:role/audicia-operator"
```

Install with Helm:

```bash
helm repo add audicia https://charts.audicia.io

helm install audicia audicia/audicia-operator \
  -n audicia-system --create-namespace \
  --version <VERSION> \
  -f values-eks.yaml
```

> **Tip:** Pin `--version` to a specific chart version for reproducible
> deployments. Check in `values-eks.yaml` alongside your other infrastructure
> config.

The `eks.amazonaws.com/role-arn` ServiceAccount annotation is used by
[IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
(IAM Roles for Service Accounts). The IRSA mutating webhook injects
`AWS_ROLE_ARN`, `AWS_WEB_IDENTITY_TOKEN_FILE`, and a projected service account
token volume into the pod. The AWS SDK picks these up automatically via the
default credential chain.

## Step 5: Create an AudiciaSource

```yaml
apiVersion: audicia.io/v1alpha1
kind: AudiciaSource
metadata:
  name: eks-cloud-audit
  namespace: audicia-system
spec:
  sourceType: CloudAuditLog
  cloud:
    provider: AWSCloudWatch
    clusterIdentity: "arn:aws:eks:<REGION>:<ACCOUNT_ID>:cluster/<CLUSTER_NAME>"
    aws:
      logGroupName: "/aws/eks/<CLUSTER_NAME>/cluster"
      logStreamPrefix: "kube-apiserver-audit-"
      region: "<REGION>"
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  ignoreSystemUsers: true
  checkpoint:
    intervalSeconds: 30
    batchSize: 500
```

## Step 6: Verify

Check that the operator is ingesting events:

```bash
# Check AudiciaSource status
kubectl get audiciasource eks-cloud-audit -n audicia-system -o yaml

# Check operator logs
kubectl logs -l app.kubernetes.io/name=audicia -n audicia-system

# Check metrics
kubectl port-forward svc/audicia-metrics 8080:8080 -n audicia-system
curl localhost:8080/metrics | grep audicia_cloud
```

You should see `audicia_cloud_messages_received_total` incrementing and
`AudiciaPolicyReport` resources being created.

## Production Hardening

The steps above get Audicia running. For production environments, consider the
following additional measures.

### CloudWatch Log Retention and Encryption

By default, CloudWatch log groups retain data indefinitely, which can lead to
unexpected costs. Set a retention policy and optionally encrypt the log group
with a KMS key:

```bash
# Set retention (e.g., 90 days)
aws logs put-retention-policy \
  --log-group-name "/aws/eks/<CLUSTER_NAME>/cluster" \
  --retention-in-days 90

# Optional: encrypt log group with KMS
aws logs associate-kms-key \
  --log-group-name "/aws/eks/<CLUSTER_NAME>/cluster" \
  --kms-key-id "arn:aws:kms:<REGION>:<ACCOUNT_ID>:key/<KEY_ID>"
```

> **Note:** AWS may truncate very large audit log entries. See the
> [EKS control plane logging documentation](https://docs.aws.amazon.com/eks/latest/userguide/control-plane-logs.html)
> for details on size limits. This can affect audit fidelity for requests with
> very large bodies.

### IAM Policy Hardening

For regulated environments, add conditions to restrict the IAM policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:FilterLogEvents",
        "logs:DescribeLogStreams"
      ],
      "Resource": "arn:aws:logs:<REGION>:<ACCOUNT_ID>:log-group:/aws/eks/<CLUSTER_NAME>/cluster:*",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": "<REGION>"
        }
      }
    }
  ]
}
```

### Pod Security

Add a `securityContext` to the Helm values to run the operator as non-root:

```yaml
# values-eks.yaml (add to existing file)
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65534
  fsGroup: 65534

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

### Network Policy

Restrict the operator's network access. See the
[NetworkPolicy example](../examples/network-policy.md) for a ready-to-use
manifest that limits egress to the Kubernetes API server and CloudWatch
endpoints.

## Troubleshooting

| Symptom                   | Likely Cause                             | Fix                                                                                              |
| ------------------------- | ---------------------------------------- | ------------------------------------------------------------------------------------------------ |
| No messages received      | Audit logging not enabled on EKS cluster | Enable via `aws eks update-cluster-config --logging`                                             |
| No messages received      | Wrong log group name                     | Verify with `aws logs describe-log-groups`                                                       |
| AccessDeniedException     | Missing IAM permissions                  | Verify `logs:FilterLogEvents` permission on the log group                                        |
| Authentication error      | IRSA not configured                      | Check SA annotation and OIDC provider setup                                                      |
| `WebIdentityErr`          | Trust policy mismatch                    | Verify OIDC provider, namespace, and SA name in trust policy                                     |
| Events from wrong cluster | Shared log group                         | Set `clusterIdentity` to the EKS cluster ARN                                                     |
| High `cloud_lag_seconds`  | Large backlog or slow polling            | Increase `checkpoint.batchSize`, check network latency                                           |
| Truncated audit events    | AWS log entry size limits                | See [EKS logging docs](https://docs.aws.amazon.com/eks/latest/userguide/control-plane-logs.html) |

## Related

- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and
  design
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — Full `spec.cloud`
  field reference
- [Helm Values](../configuration/helm-values.md) — `cloudAuditLog` configuration
- [Metrics Reference](../reference/metrics.md) — Cloud metrics
