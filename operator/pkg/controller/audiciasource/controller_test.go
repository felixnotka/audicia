package audiciasource

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	authnv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/felixnotka/audicia/operator/pkg/aggregator"
	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/filter"
	"github.com/felixnotka/audicia/operator/pkg/ingestor"
	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
	"github.com/felixnotka/audicia/operator/pkg/normalizer"
	"github.com/felixnotka/audicia/operator/pkg/rbac"
	"github.com/felixnotka/audicia/operator/pkg/strategy"
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

// --- test helpers ---

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = audiciav1alpha1.AddToScheme(s)
	return s
}

func newTestReconciler(objs ...client.Object) *Reconciler {
	s := newTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(
			&audiciav1alpha1.AudiciaSource{},
			&audiciav1alpha1.AudiciaPolicyReport{},
		).
		Build()
	return &Reconciler{
		Client:    fakeClient,
		Scheme:    s,
		pipelines: make(map[types.NamespacedName]*pipelineState),
	}
}

// --- Reconcile ---

func TestReconcile_NotFound(t *testing.T) {
	r := newTestReconciler()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}}

	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue")
	}
}

func TestReconcile_NotFound_StopsPipeline(t *testing.T) {
	r := newTestReconciler()
	key := types.NamespacedName{Name: "deleted", Namespace: "default"}

	pipelineCtx, cancel := context.WithCancel(context.Background())
	r.pipelines[key] = &pipelineState{cancel: cancel, generation: 1}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue")
	}

	r.mu.Lock()
	_, exists := r.pipelines[key]
	r.mu.Unlock()
	if exists {
		t.Error("pipeline should have been removed for deleted source")
	}

	select {
	case <-pipelineCtx.Done():
	default:
		t.Error("pipeline context should have been cancelled")
	}
}

func TestReconcile_PipelineAlreadyRunning(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-source",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeK8sAuditLog,
			Location:   &audiciav1alpha1.FileLocation{Path: "/tmp/test.log"},
		},
	}

	r := newTestReconciler(source)
	key := types.NamespacedName{Name: "test-source", Namespace: "default"}

	_, pipelineCancel := context.WithCancel(context.Background())
	r.pipelines[key] = &pipelineState{cancel: pipelineCancel, generation: 1}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue for same-generation pipeline")
	}
}

func TestReconcile_StartsNewPipeline(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "new-source",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeK8sAuditLog,
			Location:   &audiciav1alpha1.FileLocation{Path: "/tmp/nonexistent.log"},
		},
	}

	r := newTestReconciler(source)
	key := types.NamespacedName{Name: "new-source", Namespace: "default"}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue")
	}

	r.mu.Lock()
	ps, exists := r.pipelines[key]
	r.mu.Unlock()
	if !exists {
		t.Fatal("expected pipeline to be registered")
	}
	if ps.generation != 1 {
		t.Errorf("expected generation=1, got %d", ps.generation)
	}

	// Clean up the pipeline goroutine.
	ps.cancel()
	time.Sleep(100 * time.Millisecond)

	// Verify Ready condition was set (PipelineStarting initially, then
	// PipelineRunning once the goroutine progresses — accept either).
	var updated audiciav1alpha1.AudiciaSource
	if err := r.Get(context.Background(), key, &updated); err != nil {
		t.Fatalf("get source: %v", err)
	}

	readyCond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if readyCond.Reason != "PipelineStarting" && readyCond.Reason != "PipelineRunning" {
		t.Errorf("expected reason=PipelineStarting or PipelineRunning, got %q", readyCond.Reason)
	}
}

func TestReconcile_RestartsPipelineOnSpecChange(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "changed-source",
			Namespace:  "default",
			Generation: 2,
		},
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeK8sAuditLog,
			Location:   &audiciav1alpha1.FileLocation{Path: "/tmp/test.log"},
		},
	}

	r := newTestReconciler(source)
	key := types.NamespacedName{Name: "changed-source", Namespace: "default"}

	oldCtx, oldCancel := context.WithCancel(context.Background())
	r.pipelines[key] = &pipelineState{cancel: oldCancel, generation: 1}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old pipeline context should be cancelled.
	select {
	case <-oldCtx.Done():
	default:
		t.Error("old pipeline context should have been cancelled")
	}

	// New pipeline should be registered with updated generation.
	r.mu.Lock()
	ps, exists := r.pipelines[key]
	r.mu.Unlock()
	if !exists {
		t.Fatal("expected new pipeline to be registered")
	}
	if ps.generation != 2 {
		t.Errorf("expected generation=2, got %d", ps.generation)
	}

	ps.cancel()
}

// --- stopPipeline ---

func TestStopPipeline(t *testing.T) {
	r := &Reconciler{
		pipelines: make(map[types.NamespacedName]*pipelineState),
	}

	key := types.NamespacedName{Name: "test", Namespace: "default"}
	pipelineCtx, cancel := context.WithCancel(context.Background())
	r.pipelines[key] = &pipelineState{cancel: cancel, generation: 1}

	r.stopPipeline(key)

	r.mu.Lock()
	_, exists := r.pipelines[key]
	r.mu.Unlock()

	if exists {
		t.Error("expected pipeline to be removed")
	}

	select {
	case <-pipelineCtx.Done():
	default:
		t.Error("expected pipeline context to be cancelled")
	}
}

func TestStopPipeline_NoOp(t *testing.T) {
	r := &Reconciler{
		pipelines: make(map[types.NamespacedName]*pipelineState),
	}

	key := types.NamespacedName{Name: "missing", Namespace: "default"}
	r.stopPipeline(key) // should not panic
}

// --- processEvent ---

func TestProcessEvent_Accepted(t *testing.T) {
	r := &Reconciler{}
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: false,
		},
	}

	chain, _ := filter.NewChain(nil)
	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	event := auditv1.Event{
		Verb: "get",
		User: authnv1.UserInfo{Username: "system:serviceaccount:default:my-sa"},
		ObjectRef: &auditv1.ObjectReference{
			Resource:   "pods",
			Namespace:  "default",
			APIVersion: "v1",
		},
		RequestURI: "/api/v1/namespaces/default/pods",
	}

	r.processEvent(event, source, chain, aggregators, subjects)

	if len(aggregators) != 1 {
		t.Errorf("expected 1 subject aggregator, got %d", len(aggregators))
	}
	if len(subjects) != 1 {
		t.Errorf("expected 1 subject, got %d", len(subjects))
	}
	for _, s := range subjects {
		if s.Name != "my-sa" {
			t.Errorf("expected subject name=my-sa, got %q", s.Name)
		}
		if s.Kind != audiciav1alpha1.SubjectKindServiceAccount {
			t.Errorf("expected subject kind=ServiceAccount, got %q", s.Kind)
		}
	}
}

func TestProcessEvent_DeniedByFilter(t *testing.T) {
	r := &Reconciler{}
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: false,
		},
	}

	chain, err := filter.NewChain([]audiciav1alpha1.Filter{
		{
			Action:           audiciav1alpha1.FilterActionDeny,
			NamespacePattern: "^denied-ns$",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	event := auditv1.Event{
		Verb: "get",
		User: authnv1.UserInfo{Username: "system:serviceaccount:default:my-sa"},
		ObjectRef: &auditv1.ObjectReference{
			Resource:  "pods",
			Namespace: "denied-ns",
		},
	}

	r.processEvent(event, source, chain, aggregators, subjects)

	if len(aggregators) != 0 {
		t.Errorf("expected 0 aggregators (event denied by filter), got %d", len(aggregators))
	}
}

func TestProcessEvent_SystemUserFiltered(t *testing.T) {
	r := &Reconciler{}
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: true,
		},
	}

	chain, _ := filter.NewChain(nil)
	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	event := auditv1.Event{
		Verb: "get",
		User: authnv1.UserInfo{Username: "system:kube-controller-manager"},
		ObjectRef: &auditv1.ObjectReference{
			Resource:  "pods",
			Namespace: "kube-system",
		},
	}

	r.processEvent(event, source, chain, aggregators, subjects)

	if len(aggregators) != 0 {
		t.Errorf("expected 0 aggregators (system user filtered), got %d", len(aggregators))
	}
}

func TestProcessEvent_MultipleSubjects(t *testing.T) {
	r := &Reconciler{}
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: false,
		},
	}

	chain, _ := filter.NewChain(nil)
	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	events := []auditv1.Event{
		{
			Verb: "get",
			User: authnv1.UserInfo{Username: "system:serviceaccount:default:sa-a"},
			ObjectRef: &auditv1.ObjectReference{
				Resource:  "pods",
				Namespace: "default",
			},
		},
		{
			Verb: "list",
			User: authnv1.UserInfo{Username: "system:serviceaccount:default:sa-b"},
			ObjectRef: &auditv1.ObjectReference{
				Resource:  "services",
				Namespace: "default",
			},
		},
	}

	for _, e := range events {
		r.processEvent(e, source, chain, aggregators, subjects)
	}

	if len(aggregators) != 2 {
		t.Errorf("expected 2 subject aggregators, got %d", len(aggregators))
	}
	if len(subjects) != 2 {
		t.Errorf("expected 2 subjects, got %d", len(subjects))
	}
}

// --- populateReportStatus ---

func TestPopulateReportStatus(t *testing.T) {
	r := &Reconciler{} // nil Resolver = skip compliance
	report := &audiciav1alpha1.AudiciaPolicyReport{}
	subject := audiciav1alpha1.Subject{
		Kind:      audiciav1alpha1.SubjectKindServiceAccount,
		Name:      "test-sa",
		Namespace: "default",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("pods", "get", "default", time.Now()),
	}
	manifests := []string{"apiVersion: rbac.authorization.k8s.io/v1\nkind: Role"}

	r.populateReportStatus(context.Background(), report, subject, rules, manifests, 5, logr.Discard())

	if len(report.Status.ObservedRules) != 1 {
		t.Errorf("expected 1 observed rule, got %d", len(report.Status.ObservedRules))
	}
	if report.Status.EventsProcessed != 5 {
		t.Errorf("expected 5 events processed, got %d", report.Status.EventsProcessed)
	}
	if report.Status.SuggestedPolicy == nil {
		t.Fatal("expected non-nil suggested policy")
	}
	if len(report.Status.SuggestedPolicy.Manifests) != 1 {
		t.Errorf("expected 1 manifest, got %d", len(report.Status.SuggestedPolicy.Manifests))
	}
	if report.Status.LastProcessedTime == nil {
		t.Error("expected non-nil LastProcessedTime")
	}

	readyCond := meta.FindStatusCondition(report.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition")
	}
	if readyCond.Status != metav1.ConditionTrue {
		t.Errorf("expected Ready=True, got %s", readyCond.Status)
	}
	if readyCond.Reason != "PolicyGenerated" {
		t.Errorf("expected reason=PolicyGenerated, got %q", readyCond.Reason)
	}
}

func TestPopulateReportStatus_NoManifests(t *testing.T) {
	r := &Reconciler{}
	report := &audiciav1alpha1.AudiciaPolicyReport{}
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "test-sa"}

	r.populateReportStatus(context.Background(), report, subject, nil, nil, 0, logr.Discard())

	if report.Status.SuggestedPolicy != nil {
		t.Error("expected nil suggested policy for empty manifests")
	}
}

// --- setCondition ---

func TestSetCondition(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cond-source",
			Namespace: "default",
		},
	}

	r := newTestReconciler(source)

	err := r.setCondition(context.Background(), source, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "Testing",
		Message: "test condition",
	})
	if err != nil {
		t.Fatalf("setCondition: %v", err)
	}

	var updated audiciav1alpha1.AudiciaSource
	if err := r.Get(context.Background(), types.NamespacedName{Name: "cond-source", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("get source: %v", err)
	}

	cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatal("expected Ready condition")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected status=False, got %s", cond.Status)
	}
	if cond.Reason != "Testing" {
		t.Errorf("expected reason=Testing, got %q", cond.Reason)
	}
}

// --- flushSubjectReport ---

func TestFlushSubjectReport(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "flush-source",
			Namespace: "default",
		},
	}

	r := newTestReconciler(&source)
	engine := strategy.NewEngine(audiciav1alpha1.PolicyStrategy{})
	subject := audiciav1alpha1.Subject{
		Kind:      audiciav1alpha1.SubjectKindServiceAccount,
		Name:      "test-sa",
		Namespace: "default",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("pods", "get", "default", time.Now()),
	}

	err := r.flushSubjectReport(context.Background(), source, engine, subject, rules, 3, logr.Discard())
	if err != nil {
		t.Fatalf("flushSubjectReport: %v", err)
	}

	reportName := fmt.Sprintf("report-%s", sanitizeName(subject.Name))
	var report audiciav1alpha1.AudiciaPolicyReport
	if err := r.Get(context.Background(), types.NamespacedName{Name: reportName, Namespace: "default"}, &report); err != nil {
		t.Fatalf("get report: %v", err)
	}

	if report.Spec.Subject.Name != "test-sa" {
		t.Errorf("expected subject name=test-sa, got %q", report.Spec.Subject.Name)
	}
	if report.Status.EventsProcessed != 3 {
		t.Errorf("expected events processed=3, got %d", report.Status.EventsProcessed)
	}
	if len(report.Status.ObservedRules) != 1 {
		t.Errorf("expected 1 observed rule, got %d", len(report.Status.ObservedRules))
	}
	if report.Status.SuggestedPolicy == nil || len(report.Status.SuggestedPolicy.Manifests) == 0 {
		t.Error("expected non-empty suggested policy manifests")
	}

	readyCond := meta.FindStatusCondition(report.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatal("expected Ready condition on report")
	}
	if readyCond.Status != metav1.ConditionTrue {
		t.Errorf("expected Ready=True, got %s", readyCond.Status)
	}
}

// --- restoreCloudCheckpoint ---

func TestRestoreCloudCheckpoint_Empty(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{}
	pos := restoreCloudCheckpoint(source)
	if pos.PartitionOffsets != nil {
		t.Error("expected nil PartitionOffsets for empty source")
	}
	if pos.LastTimestamp != "" {
		t.Error("expected empty LastTimestamp for empty source")
	}
}

func TestRestoreCloudCheckpoint_WithData(t *testing.T) {
	ts := metav1.Now()
	source := audiciav1alpha1.AudiciaSource{
		Status: audiciav1alpha1.AudiciaSourceStatus{
			CloudCheckpoint: &audiciav1alpha1.CloudCheckpointStatus{
				PartitionOffsets: map[string]string{"0": "100", "1": "200"},
			},
			LastTimestamp: &ts,
		},
	}

	pos := restoreCloudCheckpoint(source)
	if len(pos.PartitionOffsets) != 2 {
		t.Errorf("expected 2 partition offsets, got %d", len(pos.PartitionOffsets))
	}
	if pos.PartitionOffsets["0"] != "100" {
		t.Errorf("expected partition 0 offset=100, got %q", pos.PartitionOffsets["0"])
	}
	if pos.LastTimestamp == "" {
		t.Error("expected non-empty LastTimestamp")
	}
}

// --- createCloudIngestor ---

func TestCreateCloudIngestor_NilConfig(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			SourceType: audiciav1alpha1.SourceTypeCloudAuditLog,
			Cloud:      nil,
		},
	}

	_, err := createIngestor(source, logr.Discard())
	if err == nil {
		t.Error("expected error for nil cloud config")
	}
}

// --- processEvent edge cases ---

func TestProcessEvent_NilObjectRef(t *testing.T) {
	r := &Reconciler{}
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: false,
		},
	}

	chain, _ := filter.NewChain(nil)
	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	event := auditv1.Event{
		Verb:      "get",
		User:      authnv1.UserInfo{Username: "system:serviceaccount:default:my-sa"},
		ObjectRef: nil, // No ObjectRef — resource/namespace should be empty.
	}

	r.processEvent(event, source, chain, aggregators, subjects)

	if len(aggregators) != 1 {
		t.Errorf("expected 1 aggregator, got %d", len(aggregators))
	}
}

func TestProcessEvent_ExplicitTimestamp(t *testing.T) {
	r := &Reconciler{}
	source := audiciav1alpha1.AudiciaSource{
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: false,
		},
	}

	chain, _ := filter.NewChain(nil)
	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	ts := metav1.NewMicroTime(time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))
	event := auditv1.Event{
		Verb:                     "list",
		User:                     authnv1.UserInfo{Username: "system:serviceaccount:default:ts-sa"},
		ObjectRef:                &auditv1.ObjectReference{Resource: "pods", Namespace: "default"},
		RequestReceivedTimestamp: ts,
	}

	r.processEvent(event, source, chain, aggregators, subjects)

	for _, agg := range aggregators {
		rules := agg.Rules()
		if len(rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(rules))
		}
		if rules[0].FirstSeen.Year() != 2025 {
			t.Errorf("expected event timestamp year=2025, got %d", rules[0].FirstSeen.Year())
		}
	}
}

// --- setSourceCondition ---

func TestSetSourceCondition(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cond-source-2",
			Namespace: "default",
		},
	}

	r := newTestReconciler(source)
	key := types.NamespacedName{Name: "cond-source-2", Namespace: "default"}

	r.setSourceCondition(context.Background(), key, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "PipelineRunning",
		Message: "running",
	})

	var updated audiciav1alpha1.AudiciaSource
	if err := r.Get(context.Background(), key, &updated); err != nil {
		t.Fatalf("get source: %v", err)
	}
	cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatal("expected Ready condition")
	}
	if cond.Reason != "PipelineRunning" {
		t.Errorf("expected reason=PipelineRunning, got %q", cond.Reason)
	}
}

func TestSetSourceCondition_NotFound(t *testing.T) {
	r := newTestReconciler()
	key := types.NamespacedName{Name: "missing", Namespace: "default"}

	// Should not panic when source doesn't exist.
	r.setSourceCondition(context.Background(), key, metav1.Condition{
		Type:   "Ready",
		Status: metav1.ConditionFalse,
		Reason: "Test",
	})
}

// --- flushCheckpoint ---

type fakeIngestor struct {
	pos ingestor.Position
}

func (f *fakeIngestor) Start(_ context.Context) (<-chan auditv1.Event, error) {
	return nil, nil
}

func (f *fakeIngestor) Checkpoint() ingestor.Position {
	return f.pos
}

func TestFlushCheckpoint(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ckpt-source",
			Namespace: "default",
		},
	}

	r := newTestReconciler(source)
	key := types.NamespacedName{Name: "ckpt-source", Namespace: "default"}

	// Note: Inode (uint64) causes a panic in the fake client's structured-merge-diff,
	// so we only test FileOffset and LastTimestamp here.
	ing := &fakeIngestor{pos: ingestor.Position{
		FileOffset:    42000,
		LastTimestamp: "2025-06-15T12:00:00Z",
	}}

	r.flushCheckpoint(context.Background(), key, ing)

	var updated audiciav1alpha1.AudiciaSource
	if err := r.Get(context.Background(), key, &updated); err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updated.Status.FileOffset != 42000 {
		t.Errorf("expected FileOffset=42000, got %d", updated.Status.FileOffset)
	}
	if updated.Status.LastTimestamp == nil {
		t.Fatal("expected non-nil LastTimestamp")
	}
}

func TestFlushCheckpoint_NotFound(t *testing.T) {
	r := newTestReconciler()
	key := types.NamespacedName{Name: "missing", Namespace: "default"}
	ing := &fakeIngestor{pos: ingestor.Position{FileOffset: 100}}

	// Should not panic when source doesn't exist.
	r.flushCheckpoint(context.Background(), key, ing)
}

// --- flushReports ---

func TestFlushReports(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "flush-multi-source",
			Namespace: "default",
		},
	}

	r := newTestReconciler(&source)
	engine := strategy.NewEngine(audiciav1alpha1.PolicyStrategy{})

	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	// Add two subjects with rules.
	for _, name := range []string{"sa-alpha", "sa-beta"} {
		key := fmt.Sprintf("ServiceAccount/default/%s", name)
		aggregators[key] = aggregator.New()
		subjects[key] = audiciav1alpha1.Subject{
			Kind:      audiciav1alpha1.SubjectKindServiceAccount,
			Name:      name,
			Namespace: "default",
		}
		aggregators[key].Add(normalizer.CanonicalRule{
			APIGroup: "", Resource: "pods",
			Verb: "get", Namespace: "default",
		}, time.Now())
	}

	r.flushReports(context.Background(), types.NamespacedName{Name: "flush-multi-source", Namespace: "default"}, source, engine, aggregators, subjects)

	// Both subjects should have reports.
	for _, name := range []string{"sa-alpha", "sa-beta"} {
		reportName := fmt.Sprintf("report-%s", sanitizeName(name))
		var report audiciav1alpha1.AudiciaPolicyReport
		if err := r.Get(context.Background(), types.NamespacedName{Name: reportName, Namespace: "default"}, &report); err != nil {
			t.Errorf("expected report for %s: %v", name, err)
		}
	}
}

// --- flushSubjectReport cross-namespace ---

func TestFlushSubjectReport_CrossNamespace(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "xns-source",
			Namespace: "audicia-system",
		},
	}

	r := newTestReconciler(&source)
	engine := strategy.NewEngine(audiciav1alpha1.PolicyStrategy{})
	subject := audiciav1alpha1.Subject{
		Kind:      audiciav1alpha1.SubjectKindServiceAccount,
		Name:      "cross-sa",
		Namespace: "other-ns", // Different from source namespace.
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("pods", "get", "other-ns", time.Now()),
	}

	err := r.flushSubjectReport(context.Background(), source, engine, subject, rules, 1, logr.Discard())
	if err != nil {
		t.Fatalf("flushSubjectReport: %v", err)
	}

	// Report should be in the subject's namespace, not the source's.
	reportName := fmt.Sprintf("report-%s", sanitizeName(subject.Name))
	var report audiciav1alpha1.AudiciaPolicyReport
	if err := r.Get(context.Background(), types.NamespacedName{Name: reportName, Namespace: "other-ns"}, &report); err != nil {
		t.Fatalf("expected report in other-ns: %v", err)
	}
}

// --- populateReportStatus with Resolver ---

func TestPopulateReportStatus_WithResolver(t *testing.T) {
	s := newTestScheme()
	_ = rbacv1.AddToScheme(s)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "test-role", Namespace: "default"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
		},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-binding", Namespace: "default"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "test-role"},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "test-sa", Namespace: "default"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(role, binding).
		Build()

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   s,
		Resolver: rbac.NewResolver(fakeClient),
	}

	report := &audiciav1alpha1.AudiciaPolicyReport{}
	subject := audiciav1alpha1.Subject{
		Kind:      audiciav1alpha1.SubjectKindServiceAccount,
		Name:      "test-sa",
		Namespace: "default",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeObservedRule("pods", "get", "default", time.Now()),
	}

	r.populateReportStatus(context.Background(), report, subject, rules, []string{"kind: Role"}, 1, logr.Discard())

	if report.Status.Compliance == nil {
		t.Fatal("expected non-nil compliance (Resolver is set)")
	}
	if report.Status.Compliance.Score == 0 {
		t.Error("expected non-zero compliance score")
	}
}

// --- flushCloudCheckpoint ---

type fakeParser struct{}

func (fakeParser) Parse([]byte) ([]auditv1.Event, error) { return nil, nil }

func TestFlushCloudCheckpoint(t *testing.T) {
	source := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-ckpt-src",
			Namespace: "default",
		},
	}

	r := newTestReconciler(source)
	key := types.NamespacedName{Name: "cloud-ckpt-src", Namespace: "default"}

	ing := cloud.NewCloudIngestor(
		cloud.NewFakeSource(), fakeParser{}, nil,
		cloud.CloudPosition{
			PartitionOffsets: map[string]string{"0": "42", "1": "99"},
			LastTimestamp:    "2025-06-15T12:00:00Z",
		},
		"test",
	)

	r.flushCloudCheckpoint(context.Background(), key, ing, logr.Discard())

	var updated audiciav1alpha1.AudiciaSource
	if err := r.Get(context.Background(), key, &updated); err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updated.Status.CloudCheckpoint == nil {
		t.Fatal("expected non-nil CloudCheckpoint")
	}
	if updated.Status.CloudCheckpoint.PartitionOffsets["0"] != "42" {
		t.Errorf("expected partition 0 offset=42, got %q", updated.Status.CloudCheckpoint.PartitionOffsets["0"])
	}
	if updated.Status.CloudCheckpoint.PartitionOffsets["1"] != "99" {
		t.Errorf("expected partition 1 offset=99, got %q", updated.Status.CloudCheckpoint.PartitionOffsets["1"])
	}
	if updated.Status.LastTimestamp == nil {
		t.Fatal("expected non-nil LastTimestamp")
	}
}

func TestFlushCloudCheckpoint_NotFound(t *testing.T) {
	r := newTestReconciler()
	key := types.NamespacedName{Name: "missing", Namespace: "default"}

	ing := cloud.NewCloudIngestor(
		cloud.NewFakeSource(), fakeParser{}, nil,
		cloud.CloudPosition{}, "test",
	)

	// Should not panic when source doesn't exist.
	r.flushCloudCheckpoint(context.Background(), key, ing, logr.Discard())
}

// --- eventLoop ---

func TestEventLoop_ProcessesEventsAndFlushes(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "evloop-source",
			Namespace: "default",
		},
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			IgnoreSystemUsers: false,
			Checkpoint: audiciav1alpha1.CheckpointConfig{
				IntervalSeconds: 1, // 1 second flush interval for fast test.
			},
		},
	}

	r := newTestReconciler(&source)
	key := types.NamespacedName{Name: "evloop-source", Namespace: "default"}

	engine := strategy.NewEngine(audiciav1alpha1.PolicyStrategy{})
	filterChain, _ := filter.NewChain(nil)
	ing := &fakeIngestor{}

	events := make(chan auditv1.Event, 10)

	// Send some events.
	events <- auditv1.Event{
		Verb: "get",
		User: authnv1.UserInfo{Username: "system:serviceaccount:default:loop-sa"},
		ObjectRef: &auditv1.ObjectReference{
			Resource: "pods", Namespace: "default",
		},
	}
	events <- auditv1.Event{
		Verb: "list",
		User: authnv1.UserInfo{Username: "system:serviceaccount:default:loop-sa"},
		ObjectRef: &auditv1.ObjectReference{
			Resource: "pods", Namespace: "default",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		r.eventLoop(ctx, key, source, engine, filterChain, ing, events)
		close(done)
	}()

	// Wait for the checkpoint ticker to fire and flush.
	time.Sleep(2 * time.Second)

	// Cancel context to trigger final flush and shutdown.
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("eventLoop did not exit after context cancellation")
	}

	// Verify a report was created.
	reportName := fmt.Sprintf("report-%s", sanitizeName("loop-sa"))
	var report audiciav1alpha1.AudiciaPolicyReport
	if err := r.Get(context.Background(), types.NamespacedName{Name: reportName, Namespace: "default"}, &report); err != nil {
		t.Fatalf("expected report for loop-sa: %v", err)
	}
	if report.Status.EventsProcessed < 2 {
		t.Errorf("expected at least 2 events processed, got %d", report.Status.EventsProcessed)
	}
}

func TestEventLoop_ChannelClosed(t *testing.T) {
	source := audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "evloop-close-source",
			Namespace: "default",
		},
		Spec: audiciav1alpha1.AudiciaSourceSpec{
			Checkpoint: audiciav1alpha1.CheckpointConfig{
				IntervalSeconds: 60,
			},
		},
	}

	r := newTestReconciler(&source)
	key := types.NamespacedName{Name: "evloop-close-source", Namespace: "default"}

	engine := strategy.NewEngine(audiciav1alpha1.PolicyStrategy{})
	filterChain, _ := filter.NewChain(nil)
	ing := &fakeIngestor{}

	events := make(chan auditv1.Event, 10)

	// Close the channel immediately — eventLoop should exit cleanly.
	close(events)

	done := make(chan struct{})
	go func() {
		r.eventLoop(context.Background(), key, source, engine, filterChain, ing, events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("eventLoop did not exit after channel close")
	}
}
