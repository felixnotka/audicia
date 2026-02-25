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

	"github.com/felixnotka/audicia/operator/pkg/aggregator"
	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/filter"
	"github.com/felixnotka/audicia/operator/pkg/ingestor"
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
	if result.Requeue {
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
	if result.Requeue {
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
	if result.Requeue {
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
	if result.Requeue {
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
