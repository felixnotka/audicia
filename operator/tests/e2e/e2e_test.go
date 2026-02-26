//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

const (
	kindClusterName = "audicia-e2e"
	helmReleaseName = "audicia"
	helmNamespace   = "audicia-system"
)

var (
	k8sClient  client.Client
	clientset  *kubernetes.Clientset
	testScheme *runtime.Scheme

	// webhookCACert holds the CA certificate PEM used to verify the webhook TLS.
	// Set during TestMain setup.
	webhookCACert []byte
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	// Change to operator/ root so relative paths (hack/, build/, ../deploy/) work.
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintf(os.Stderr, "failed to chdir to operator root: %v\n", err)
		return 1
	}

	// Build scheme.
	testScheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add client-go scheme: %v\n", err)
		return 1
	}
	if err := audiciav1alpha1.AddToScheme(testScheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add audicia scheme: %v\n", err)
		return 1
	}

	// Ensure Kind cluster exists.
	if err := ensureKindCluster(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ensure kind cluster: %v\n", err)
		return 1
	}

	// Build and load Docker image.
	if err := buildAndLoadImage(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build/load image: %v\n", err)
		return 1
	}

	// Build clients from Kind kubeconfig.
	if err := buildClients(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build k8s clients: %v\n", err)
		return 1
	}

	// Install CRDs.
	if err := installCRDs(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to install CRDs: %v\n", err)
		return 1
	}

	// Generate webhook TLS certs and create Secret before Helm install.
	if err := setupWebhookTLS(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup webhook TLS: %v\n", err)
		return 1
	}

	// Helm install operator.
	if err := helmInstall(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to helm install: %v\n", err)
		return 1
	}

	// Grant operator SA cluster-admin (Helm chart does not include RBAC templates).
	if err := grantOperatorRBAC(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to grant operator RBAC: %v\n", err)
		return 1
	}

	// Wait for operator deployment to be ready.
	if err := waitForDeployment(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "operator deployment not ready: %v\n", err)
		return 1
	}

	// Run tests.
	code := m.Run()

	// Teardown unless E2E_KEEP_CLUSTER is set.
	if os.Getenv("E2E_KEEP_CLUSTER") == "" {
		teardown()
	} else {
		fmt.Println("E2E_KEEP_CLUSTER set, keeping cluster and Helm release")
	}

	return code
}

func ensureKindCluster() error {
	out, _ := runCmd("kind", "get", "clusters")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == kindClusterName {
			fmt.Printf("Kind cluster %q already exists, reusing\n", kindClusterName)
			return nil
		}
	}
	fmt.Printf("Creating Kind cluster %q...\n", kindClusterName)
	_, err := runCmdVerbose("kind", "create", "cluster",
		"--config", "hack/kind-e2e-config.yaml",
		"--name", kindClusterName)
	return err
}

func buildAndLoadImage() error {
	fmt.Println("Building Docker image audicia-operator:e2e...")
	if _, err := runCmdVerbose("docker", "build",
		"-t", "audicia-operator:e2e",
		"-f", "build/Dockerfile",
		"."); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	fmt.Println("Loading image into Kind...")
	if _, err := runCmd("kind", "load", "docker-image",
		"audicia-operator:e2e",
		"--name", kindClusterName); err != nil {
		// Fallback: pipe docker save into ctr import inside the Kind node.
		// Works around "failed to detect containerd snapshotter" in some environments.
		fmt.Println("kind load failed, falling back to docker save | ctr import...")
		nodeName := kindClusterName + "-control-plane"
		if _, err2 := runCmdPipe(
			exec.Command("docker", "save", "audicia-operator:e2e"),
			exec.Command("docker", "exec", "-i", nodeName,
				"ctr", "--namespace", "k8s.io", "images", "import", "--snapshotter", "overlayfs", "-"),
		); err2 != nil {
			return fmt.Errorf("ctr import fallback: %w (original kind load error: %v)", err2, err)
		}
	}
	return nil
}

func buildClients() error {
	kubeconfigBytes, err := runCmd("kind", "get", "kubeconfig", "--name", kindClusterName)
	if err != nil {
		return fmt.Errorf("get kubeconfig: %w", err)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfigBytes))
	if err != nil {
		return fmt.Errorf("parse kubeconfig: %w", err)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		return fmt.Errorf("create controller-runtime client: %w", err)
	}

	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create clientset: %w", err)
	}

	// Store config globally for SA token impersonation.
	restConfig = cfg
	return nil
}

func installCRDs() error {
	fmt.Println("Installing CRDs...")
	_, err := runCmdVerbose("kubectl", "apply",
		"-f", "../deploy/helm/crds/",
		"--context", "kind-"+kindClusterName)
	return err
}

func helmInstall(ctx context.Context) error {
	// Uninstall first if present (idempotent for reruns).
	_, _ = runCmd("helm", "uninstall", helmReleaseName,
		"--namespace", helmNamespace,
		"--kube-context", "kind-"+kindClusterName)

	fmt.Println("Helm installing operator...")
	_, err := runCmdVerbose("helm", "install", helmReleaseName, "../deploy/helm/",
		"--namespace", helmNamespace,
		"--create-namespace",
		"--kube-context", "kind-"+kindClusterName,
		"--set", "image.repository=audicia-operator",
		"--set", "image.tag=e2e",
		"--set", "image.pullPolicy=Never",
		"--set", "auditLog.enabled=true",
		"--set", "podSecurityContext.runAsUser=0",
		"--set", "podSecurityContext.runAsNonRoot=false",
		"--set", "webhook.enabled=true",
		"--set", "webhook.port=8443",
		"--set", "webhook.tlsSecretName=audicia-webhook-tls",
		"--set", "operator.leaderElection.enabled=false",
		"--set", "operator.logLevel=1",
		"--wait",
		"--timeout", "120s")
	return err
}

func grantOperatorRBAC(ctx context.Context) error {
	// The Helm chart creates a ServiceAccount but no ClusterRole/ClusterRoleBinding.
	// The operator needs broad permissions to read/write CRDs and read RBAC.
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "audicia-e2e-cluster-admin",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      helmReleaseName + "-audicia-operator",
				Namespace: helmNamespace,
			},
		},
	}
	if err := k8sClient.Create(ctx, crb); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("create cluster-admin binding: %w", err)
	}
	return nil
}

func waitForDeployment(ctx context.Context) error {
	fmt.Println("Waiting for operator deployment to be ready...")
	deployName := helmReleaseName + "-audicia-operator"

	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var dep appsv1.Deployment
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      deployName,
			Namespace: helmNamespace,
		}, &dep); err != nil {
			return false, nil
		}
		return dep.Status.ReadyReplicas > 0 && dep.Status.ReadyReplicas == *dep.Spec.Replicas, nil
	})
}

func teardown() {
	fmt.Println("Tearing down...")
	_, _ = runCmd("helm", "uninstall", helmReleaseName,
		"--namespace", helmNamespace,
		"--kube-context", "kind-"+kindClusterName)
	_, _ = runCmd("kubectl", "delete", "clusterrolebinding",
		"audicia-e2e-cluster-admin",
		"--context", "kind-"+kindClusterName)
	_, _ = runCmd("kind", "delete", "cluster", "--name", kindClusterName)
}

// runCmd executes a command and returns its combined output.
func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%s %v: %w\nstderr: %s", name, args, err, stderr.String())
	}
	return stdout.String(), nil
}

// runCmdVerbose runs a command with stdout/stderr piped to os.Stdout/os.Stderr.
func runCmdVerbose(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return buf.String(), fmt.Errorf("%s %v: %w", name, args, err)
	}
	return buf.String(), nil
}

// setupWebhookTLS generates self-signed TLS certificates and creates a TLS Secret
// in the operator namespace for the webhook ingestor.
func setupWebhookTLS(ctx context.Context) error {
	fmt.Println("Generating webhook TLS certificates...")

	certPEM, keyPEM, caPEM, err := generateWebhookTLS()
	if err != nil {
		return fmt.Errorf("generate TLS certs: %w", err)
	}
	webhookCACert = caPEM

	// Ensure the namespace exists before creating the Secret.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: helmNamespace}}
	if err := k8sClient.Create(ctx, ns); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create namespace %s: %w", helmNamespace, err)
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audicia-webhook-tls",
			Namespace: helmNamespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create TLS secret: %w", err)
		}
		// Update existing secret on rerun.
		if err := k8sClient.Update(ctx, secret); err != nil {
			return fmt.Errorf("update TLS secret: %w", err)
		}
	}

	fmt.Println("Webhook TLS secret created")
	return nil
}

// generateWebhookTLS creates a self-signed CA and a server certificate for the
// webhook endpoint. Returns PEM-encoded cert, key, and CA cert.
func generateWebhookTLS() (certPEM, keyPEM, caPEM []byte, err error) {
	// Generate CA key.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "audicia-e2e-ca"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create CA cert: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}

	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	// Generate server key.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate server key: %w", err)
	}

	svcName := helmReleaseName + "-audicia-operator-webhook"
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: svcName + "." + helmNamespace + ".svc"},
		DNSNames: []string{
			"localhost",
			svcName,
			svcName + "." + helmNamespace,
			svcName + "." + helmNamespace + ".svc",
			svcName + "." + helmNamespace + ".svc.cluster.local",
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create server cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal server key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	return certPEM, keyPEM, caPEM, nil
}

// runCmdPipe pipes the stdout of cmd1 into the stdin of cmd2.
func runCmdPipe(cmd1, cmd2 *exec.Cmd) (string, error) {
	pipe, err := cmd1.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	cmd2.Stdin = pipe
	var buf bytes.Buffer
	cmd2.Stdout = &buf
	cmd2.Stderr = os.Stderr

	if err := cmd1.Start(); err != nil {
		return "", fmt.Errorf("start cmd1: %w", err)
	}
	if err := cmd2.Start(); err != nil {
		return "", fmt.Errorf("start cmd2: %w", err)
	}
	if err := cmd1.Wait(); err != nil {
		return "", fmt.Errorf("cmd1 wait: %w", err)
	}
	if err := cmd2.Wait(); err != nil {
		return buf.String(), fmt.Errorf("cmd2 wait: %w", err)
	}
	return buf.String(), nil
}
