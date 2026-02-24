# Quick Start: EKS Cloud Ingestion

This tutorial walks you through setting up Audicia to ingest audit logs from an Amazon EKS cluster via
CloudWatch Logs. Unlike file or webhook mode, cloud ingestion works without control plane access — EKS
automatically sends audit logs to CloudWatch, and Audicia consumes them.

## Prerequisites

- An EKS cluster with audit logging enabled (enabled by default)
- The Audicia operator image built with the `aws` build tag
- Helm 3
- `eksctl` or `aws` CLI for IAM/OIDC setup

## Step 1: Verify EKS Audit Logs in CloudWatch

EKS automatically sends API server audit logs to CloudWatch Logs under the log group
`/aws/eks/<CLUSTER_NAME>/cluster`. Verify they exist:

```bash
aws logs filter-log-events \
  --log-group-name "/aws/eks/<CLUSTER_NAME>/cluster" \
  --log-stream-name-prefix "kube-apiserver-audit-" \
  --limit 5
```

If the log group doesn't exist, enable audit logging:

```bash
aws eks update-cluster-config \
  --name <CLUSTER_NAME> \
  --logging '{"clusterLogging":[{"types":["audit"],"enabled":true}]}'
```

## Step 2: Set Up IRSA

Create an IAM policy, role, and associate it with the Audicia ServiceAccount:

```bash
# Create IAM policy for CloudWatch read access
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

```bash
# Associate OIDC provider (if not already done)
eksctl utils associate-iam-oidc-provider \
  --cluster <CLUSTER_NAME> \
  --approve

# Create IAM role bound to the Audicia ServiceAccount
eksctl create iamserviceaccount \
  --cluster <CLUSTER_NAME> \
  --namespace audicia-system \
  --name audicia-operator \
  --attach-policy-arn arn:aws:iam::<ACCOUNT_ID>:policy/AudiciaCloudWatchReadOnly \
  --approve \
  --override-existing-serviceaccounts
```

> **Note:** The `--namespace` and `--name` must match the Helm chart defaults
> (`audicia-system:audicia-operator`).

## Step 3: Install Audicia

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
the required AWS credentials into the pod.

## Step 4: Create an AudiciaSource

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

```bash
kubectl apply -f eks-cloud-audit.yaml
```

## Step 5: Verify Events Flow

Check that the operator is ingesting events:

```bash
# Check AudiciaSource status
kubectl get audiciasource eks-cloud-audit -n audicia-system

# Check operator logs
kubectl logs -l app.kubernetes.io/name=audicia -n audicia-system --tail=20
```

You should see `audicia_cloud_messages_received_total` incrementing. After a flush cycle, policy reports start
appearing:

```bash
kubectl get audiciapolicyreports --all-namespaces
```

## What's Next

- [EKS Setup Guide](../guides/eks-setup.md) — Full guide with IRSA manual setup and troubleshooting
- [Cloud Ingestion Concept](../concepts/cloud-ingestion.md) — Architecture and design
- [Filter Recipes](../guides/filter-recipes.md) — Common filter configurations for production
- [Compliance Scoring](../concepts/compliance-scoring.md) — How RBAC drift detection works
