# EKS Setup (CloudWatch Logs)

This guide walks through configuring Audicia to ingest audit logs from an Amazon EKS cluster via
CloudWatch Logs using IRSA (IAM Roles for Service Accounts).

## Prerequisites

- An EKS cluster with audit logging enabled (enabled by default — logs go to CloudWatch)
- The Audicia operator image built with the `aws` build tag
- Helm 3
- `eksctl` or `aws` CLI for IAM/OIDC setup

## Step 1: Verify EKS Audit Logs in CloudWatch

EKS automatically sends API server audit logs to CloudWatch Logs. The log group follows the naming convention:

```
/aws/eks/<CLUSTER_NAME>/cluster
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

If the log group doesn't exist, enable audit logging in the EKS console or via CLI:

```bash
aws eks update-cluster-config \
  --name <CLUSTER_NAME> \
  --logging '{"clusterLogging":[{"types":["audit"],"enabled":true}]}'
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

IRSA allows Kubernetes ServiceAccounts to assume IAM roles without static credentials.

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

Or manually, create the IAM role with a trust policy for the OIDC provider and annotate the ServiceAccount:

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

```bash
helm repo add audicia https://charts.audicia.io

helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set cloudAuditLog.enabled=true \
  --set cloudAuditLog.provider=AWSCloudWatch \
  --set cloudAuditLog.aws.logGroupName="/aws/eks/<CLUSTER_NAME>/cluster" \
  --set cloudAuditLog.aws.region="<REGION>" \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::<ACCOUNT_ID>:role/audicia-operator"
```

The `eks.amazonaws.com/role-arn` ServiceAccount annotation triggers the EKS Pod Identity webhook to inject
`AWS_ROLE_ARN`, `AWS_WEB_IDENTITY_TOKEN_FILE`, and the projected token volume into the pod. The AWS SDK
picks these up automatically.

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

You should see `audicia_cloud_messages_received_total` incrementing and `AudiciaPolicyReport` resources being created.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| No messages received | Audit logging not enabled on EKS cluster | Enable via `aws eks update-cluster-config --logging` |
| No messages received | Wrong log group name | Verify with `aws logs describe-log-groups` |
| AccessDeniedException | Missing IAM permissions | Verify `logs:FilterLogEvents` permission on the log group |
| Authentication error | IRSA not configured | Check SA annotation and OIDC provider setup |
| `WebIdentityErr` | Trust policy mismatch | Verify OIDC provider, namespace, and SA name in trust policy |
| Events from wrong cluster | Shared log group | Set `clusterIdentity` to the EKS cluster ARN |
| High `cloud_lag_seconds` | Large backlog or slow polling | Increase `checkpoint.batchSize`, check network latency |

## Related

- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and design
- [AudiciaSource CRD](../reference/crd-audiciasource.md) — Full `spec.cloud` field reference
- [Helm Values](../configuration/helm-values.md) — `cloudAuditLog` configuration
- [Metrics Reference](../reference/metrics.md) — Cloud metrics
