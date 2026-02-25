# AudiciaSource: Cloud (EKS)

An AudiciaSource configured for cloud-based ingestion from an EKS cluster via
CloudWatch Logs using IRSA.

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
    clusterIdentity: "arn:aws:eks:eu-central-1:123456789012:cluster/my-cluster"
    aws:
      logGroupName: "/aws/eks/my-cluster/cluster"
      logStreamPrefix: "kube-apiserver-audit-"
      region: "eu-central-1"
  policyStrategy:
    scopeMode: NamespaceStrict
    verbMerge: Smart
    wildcards: Forbidden
  ignoreSystemUsers: true
  filters:
    - action: Deny
      userPattern: "^system:node:.*"
    - action: Deny
      userPattern: "^system:kube-.*"
  checkpoint:
    intervalSeconds: 30
    batchSize: 500
  limits:
    maxRulesPerReport: 200
    retentionDays: 30
```

## Key Fields

| Field                       | Value           | Notes                                                        |
| --------------------------- | --------------- | ------------------------------------------------------------ |
| `sourceType`                | `CloudAuditLog` | Selects the cloud ingestion path                             |
| `cloud.provider`            | `AWSCloudWatch` | AWS CloudWatch Logs adapter                                  |
| `cloud.clusterIdentity`     | EKS cluster ARN | Used to filter events if multiple clusters share a log group |
| `cloud.aws.logGroupName`    | Log group path  | EKS default: `/aws/eks/<cluster>/cluster`                    |
| `cloud.aws.logStreamPrefix` | Stream prefix   | Filters to `kube-apiserver-audit-` streams only              |
| `cloud.aws.region`          | AWS region      | If empty, uses `AWS_REGION` from environment                 |

## Authentication

Authentication uses IRSA (IAM Roles for Service Accounts). The ServiceAccount
must be annotated with the IAM role ARN:

```bash
helm install audicia audicia/audicia-operator -n audicia-system --create-namespace \
  --set image.tag=<VERSION>-aws \
  --set cloudAuditLog.enabled=true \
  --set cloudAuditLog.provider=AWSCloudWatch \
  --set cloudAuditLog.aws.logGroupName="/aws/eks/my-cluster/cluster" \
  --set cloudAuditLog.aws.region="eu-central-1" \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::123456789012:role/audicia-operator"
```

The IAM role needs the following permissions on the CloudWatch Logs group:

- `logs:FilterLogEvents`
- `logs:DescribeLogStreams`

See the [EKS Setup Guide](../guides/eks-setup.md) for full IRSA setup including
IAM role creation, OIDC provider configuration, and trust policy.

## Related

- [EKS Setup Guide](../guides/eks-setup.md) — Step-by-step walkthrough
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture
  overview
- [AudiciaSource CRD Reference](../reference/crd-audiciasource.md) — Full field
  reference
