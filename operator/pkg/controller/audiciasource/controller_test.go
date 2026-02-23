package audiciasource

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/ingestor"
)

func makeObservedRule(resource, verb, ns string, lastSeen time.Time) audiciav1alpha1.ObservedRule {
	return audiciav1alpha1.ObservedRule{
		APIGroups: []string{""},
		Resources: []string{resource},
		Verbs:     []string{verb},
		Namespace: ns,
		FirstSeen: metav1.NewTime(lastSeen.Add(-time.Hour)),
		LastSeen:  metav1.NewTime(lastSeen),
		Count:     1,
	}
}

// --- compactRules ---

func TestCompactRules_NoRules(t *testing.T) {
	limits := audiciav1alpha1.LimitsConfig{MaxRulesPerReport: 200, RetentionDays: 30}
	result := compactRules(nil, limits, "test", logr.Discard())
	if len(result) != 0 {
		t.Errorf("got %d rules, want 0", len(result))
	}
}

func TestCompactRules_RetentionFiltering(t *testing.T) {
	now := time.Now()
	recent := now.Add(-24 * time.Hour)   // 1 day ago — should be kept.
	old := now.Add(-60 * 24 * time.Hour) // 60 days ago — should be dropped.

	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("pods", "get", "default", recent),
		makeObservedRule("secrets", "get", "default", old),
	}

	limits := audiciav1alpha1.LimitsConfig{MaxRulesPerReport: 200, RetentionDays: 30}
	result := compactRules(rules, limits, "test", logr.Discard())
	if len(result) != 1 {
		t.Errorf("got %d rules, want 1 (old rule should be dropped)", len(result))
	}
	if result[0].Resources[0] != "pods" {
		t.Errorf("expected pods rule to survive, got %s", result[0].Resources[0])
	}
}

func TestCompactRules_Truncation(t *testing.T) {
	now := time.Now()
	var rules []audiciav1alpha1.ObservedRule
	for i := 0; i < 10; i++ {
		rules = append(rules, makeObservedRule("pods", "get", "default", now.Add(-time.Duration(i)*time.Minute)))
	}

	limits := audiciav1alpha1.LimitsConfig{MaxRulesPerReport: 5, RetentionDays: 30}
	result := compactRules(rules, limits, "test", logr.Discard())
	if len(result) != 5 {
		t.Errorf("got %d rules, want 5 (truncated)", len(result))
	}
}

func TestCompactRules_TruncationKeepsMostRecent(t *testing.T) {
	now := time.Now()
	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("old-resource", "get", "default", now.Add(-2*time.Hour)),
		makeObservedRule("new-resource", "get", "default", now),
	}

	limits := audiciav1alpha1.LimitsConfig{MaxRulesPerReport: 1, RetentionDays: 30}
	result := compactRules(rules, limits, "test", logr.Discard())
	if len(result) != 1 {
		t.Fatalf("got %d rules, want 1", len(result))
	}
	// Most recent rule should survive.
	if result[0].Resources[0] != "new-resource" {
		t.Errorf("expected new-resource to survive truncation, got %s", result[0].Resources[0])
	}
}

func TestCompactRules_DefaultLimits(t *testing.T) {
	now := time.Now()
	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("pods", "get", "default", now),
	}

	// Zero values should use defaults (200 max, 30 days retention).
	limits := audiciav1alpha1.LimitsConfig{}
	result := compactRules(rules, limits, "test", logr.Discard())
	if len(result) != 1 {
		t.Errorf("got %d rules, want 1", len(result))
	}
}

// --- createIngestor ---

func TestCreateIngestor_K8sAuditLog(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeK8sAuditLog,
			Location:   &audiciav1alpha1.FileLocation{Path: "/var/log/audit.log"},
		},
	}

	ing, err := createIngestor(source, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if ing == nil {
		t.Fatal("expected non-nil ingestor")
	}
}

func TestCreateIngestor_K8sAuditLog_NilLocation(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeK8sAuditLog,
			Location:   nil,
		},
	}

	_, err := createIngestor(source, logr.Discard())
	if err == nil {
		t.Error("expected error for nil location")
	}
}

func TestCreateIngestor_Webhook(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeWebhook,
			Webhook: &audiciav1alpha1.WebhookConfig{
				Port:          8443,
				TLSSecretName: "tls-secret",
			},
		},
	}

	ing, err := createIngestor(source, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if ing == nil {
		t.Fatal("expected non-nil ingestor")
	}
}

func TestCreateIngestor_Webhook_TLSPathsSet(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeWebhook,
			Webhook: &audiciav1alpha1.WebhookConfig{
				Port:                8443,
				TLSSecretName:       "tls-secret",
				RateLimitPerSecond:  50,
				MaxRequestBodyBytes: 2097152,
			},
		},
	}

	ing, err := createIngestor(source, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	wh, ok := ing.(*ingestor.WebhookIngestor)
	if !ok {
		t.Fatal("expected *ingestor.WebhookIngestor")
	}

	if wh.TLSCertFile != "/etc/audicia/webhook-tls/tls.crt" {
		t.Errorf("TLSCertFile = %q, want /etc/audicia/webhook-tls/tls.crt", wh.TLSCertFile)
	}
	if wh.TLSKeyFile != "/etc/audicia/webhook-tls/tls.key" {
		t.Errorf("TLSKeyFile = %q, want /etc/audicia/webhook-tls/tls.key", wh.TLSKeyFile)
	}
	if wh.RateLimitPerSecond != 50 {
		t.Errorf("RateLimitPerSecond = %d, want 50", wh.RateLimitPerSecond)
	}
	if wh.MaxRequestBodyBytes != 2097152 {
		t.Errorf("MaxRequestBodyBytes = %d, want 2097152", wh.MaxRequestBodyBytes)
	}
}

func TestCreateIngestor_Webhook_MTLSEnabled(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeWebhook,
			Webhook: &audiciav1alpha1.WebhookConfig{
				Port:               8443,
				TLSSecretName:      "tls-secret",
				ClientCASecretName: "client-ca-secret",
			},
		},
	}

	ing, err := createIngestor(source, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	wh, ok := ing.(*ingestor.WebhookIngestor)
	if !ok {
		t.Fatal("expected *ingestor.WebhookIngestor")
	}

	if wh.ClientCAFile != "/etc/audicia/webhook-client-ca/ca.crt" {
		t.Errorf("ClientCAFile = %q, want /etc/audicia/webhook-client-ca/ca.crt", wh.ClientCAFile)
	}
}

func TestCreateIngestor_Webhook_MTLSDisabledWhenEmpty(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeWebhook,
			Webhook: &audiciav1alpha1.WebhookConfig{
				Port:               8443,
				TLSSecretName:      "tls-secret",
				ClientCASecretName: "",
			},
		},
	}

	ing, err := createIngestor(source, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	wh, ok := ing.(*ingestor.WebhookIngestor)
	if !ok {
		t.Fatal("expected *ingestor.WebhookIngestor")
	}

	if wh.ClientCAFile != "" {
		t.Errorf("ClientCAFile = %q, want empty (mTLS should be disabled)", wh.ClientCAFile)
	}
}

func TestCreateIngestor_Webhook_NilConfig(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeWebhook,
			Webhook:    nil,
		},
	}

	_, err := createIngestor(source, logr.Discard())
	if err == nil {
		t.Error("expected error for nil webhook config")
	}
}

func TestCreateIngestor_UnknownSourceType(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: "UnknownType",
		},
	}

	_, err := createIngestor(source, logr.Discard())
	if err == nil {
		t.Error("expected error for unknown source type")
	}
}

func TestCreateIngestor_K8sAuditLog_DefaultBatchSize(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeK8sAuditLog,
			Location:   &audiciav1alpha1.FileLocation{Path: "/var/log/audit.log"},
			Checkpoint: audiciav1alpha1.CheckpointConfig{BatchSize: 0},
		},
	}

	ing, err := createIngestor(source, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if ing == nil {
		t.Fatal("expected non-nil ingestor")
	}
}

// --- sanitizeName ---

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"backend", "backend"},
		{"UPPER", "upper"},
		{"alice@example.com", "alice-at-example-com"},
		{"system:kube-scheduler", "system-kube-scheduler"},
		{"ns/sa-name", "ns-sa-name"},
		{"dotted.name", "dotted-name"},
	}
	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeName_Truncation(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "a"
	}
	got := sanitizeName(long)
	if len(got) > 63 {
		t.Errorf("sanitizeName output length = %d, want <= 63", len(got))
	}
}

func TestSanitizeName_TrailingDash(t *testing.T) {
	got := sanitizeName("test.")
	if got[len(got)-1] == '-' {
		t.Errorf("sanitizeName(%q) = %q, should not end with dash", "test.", got)
	}
}

// --- subjectKeyString ---

func TestSubjectKeyString_WithNamespace(t *testing.T) {
	s := audiciav1alpha1.Subject{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"}
	got := subjectKeyString(s)
	if got != "ServiceAccount/prod/backend" {
		t.Errorf("got %q, want ServiceAccount/prod/backend", got)
	}
}

func TestSubjectKeyString_WithoutNamespace(t *testing.T) {
	s := audiciav1alpha1.Subject{Kind: "User", Name: "alice"}
	got := subjectKeyString(s)
	if got != "User/alice" {
		t.Errorf("got %q, want User/alice", got)
	}
}
