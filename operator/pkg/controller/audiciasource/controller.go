package audiciasource

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/felixnotka/audicia/operator/pkg/aggregator"
	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/diff"
	"github.com/felixnotka/audicia/operator/pkg/filter"
	"github.com/felixnotka/audicia/operator/pkg/ingestor"
	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
	"github.com/felixnotka/audicia/operator/pkg/metrics"
	"github.com/felixnotka/audicia/operator/pkg/normalizer"
	"github.com/felixnotka/audicia/operator/pkg/rbac"
	"github.com/felixnotka/audicia/operator/pkg/strategy"
)

// pipelineState tracks a running pipeline goroutine for one AudiciaSource.
type pipelineState struct {
	cancel     context.CancelFunc
	generation int64
}

// Reconciler reconciles AudiciaSource objects.
type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Resolver  *rbac.Resolver
	mu        sync.Mutex
	pipelines map[types.NamespacedName]*pipelineState
}

// SetupWithManager registers the AudiciaSource controller with the manager.
func SetupWithManager(mgr ctrl.Manager, maxConcurrent int) error {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&audiciav1alpha1.AudiciaSource{}).
		Owns(&audiciav1alpha1.AudiciaPolicyReport{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrent}).
		Complete(&Reconciler{
			Client:    mgr.GetClient(),
			Scheme:    mgr.GetScheme(),
			Resolver:  rbac.NewResolver(mgr.GetClient()),
			pipelines: make(map[types.NamespacedName]*pipelineState),
		})
}

// Reconcile handles a single reconciliation for an AudiciaSource resource.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling AudiciaSource", "name", req.NamespacedName)

	// Fetch the AudiciaSource instance.
	var source audiciav1alpha1.AudiciaSource
	if err := r.Get(ctx, req.NamespacedName, &source); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Resource deleted — stop the pipeline.
			r.stopPipeline(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if pipeline is already running for this source.
	r.mu.Lock()
	existing, running := r.pipelines[req.NamespacedName]
	if running && existing.generation == source.Generation {
		// Pipeline is running and spec hasn't changed — nothing to do.
		r.mu.Unlock()
		return ctrl.Result{}, nil
	}
	r.mu.Unlock()

	// Stop existing pipeline if spec changed.
	if running {
		r.stopPipeline(req.NamespacedName)
	}

	// Build and start a new pipeline.
	pipelineCtx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.pipelines[req.NamespacedName] = &pipelineState{
		cancel:     cancel,
		generation: source.Generation,
	}
	r.mu.Unlock()

	// Set initial condition.
	if err := r.setCondition(ctx, &source, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "PipelineStarting",
		Message:            "Ingestion pipeline is starting.",
		ObservedGeneration: source.Generation,
	}); err != nil {
		logger.Error(err, "failed to set starting condition")
	}

	go r.runPipeline(pipelineCtx, req.NamespacedName, source)

	logger.Info("pipeline started", "sourceType", source.Spec.SourceType)
	return ctrl.Result{}, nil
}

// stopPipeline cancels and removes a running pipeline.
func (r *Reconciler) stopPipeline(key types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ps, ok := r.pipelines[key]; ok {
		ps.cancel()
		delete(r.pipelines, key)
	}
}

// runPipeline runs the full ingestion pipeline for a single AudiciaSource.
func (r *Reconciler) runPipeline(ctx context.Context, key types.NamespacedName, source audiciav1alpha1.AudiciaSource) {
	logger := ctrl.Log.WithName("pipeline").WithValues("source", key)

	// 1. Create the ingestor based on source type.
	ing, err := createIngestor(source, logger)
	if err != nil {
		return
	}

	// 2. Create the filter chain.
	filterChain, err := filter.NewChain(source.Spec.Filters)
	if err != nil {
		logger.Error(err, "failed to compile filter chain")
		return
	}

	// 3. Create the strategy engine.
	engine := strategy.NewEngine(source.Spec.PolicyStrategy)

	// 4. Start ingestion.
	events, err := ing.Start(ctx)
	if err != nil {
		logger.Error(err, "failed to start ingestor")
		return
	}

	// Set Ready condition.
	r.setSourceCondition(ctx, key, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "PipelineRunning",
		Message:            "Ingestion pipeline is running.",
		ObservedGeneration: source.Generation,
	})

	// 5. Process events through the pipeline.
	r.eventLoop(ctx, key, source, engine, filterChain, ing, events)
}

// createIngestor builds the appropriate ingestor for the source type.
func createIngestor(source audiciav1alpha1.AudiciaSource, logger logr.Logger) (ingestor.Ingestor, error) {
	switch source.Spec.SourceType {
	case audiciav1alpha1.SourceTypeK8sAuditLog:
		return createFileIngestor(source, logger)
	case audiciav1alpha1.SourceTypeWebhook:
		return createWebhookIngestor(source, logger)
	case audiciav1alpha1.SourceTypeCloudAuditLog:
		return createCloudIngestor(source, logger)
	default:
		logger.Error(nil, "unknown source type", "sourceType", source.Spec.SourceType)
		return nil, fmt.Errorf("unknown source type: %s", source.Spec.SourceType)
	}
}

func createFileIngestor(source audiciav1alpha1.AudiciaSource, logger logr.Logger) (ingestor.Ingestor, error) {
	if source.Spec.Location == nil {
		logger.Error(nil, "K8sAuditLog source requires location config")
		return nil, fmt.Errorf("K8sAuditLog source requires location config")
	}
	startPos := ingestor.Position{
		FileOffset: source.Status.FileOffset,
		Inode:      source.Status.Inode,
	}
	batchSize := int(source.Spec.Checkpoint.BatchSize)
	if batchSize == 0 {
		batchSize = 500
	}
	return ingestor.NewFileIngestor(source.Spec.Location.Path, startPos, batchSize), nil
}

func createWebhookIngestor(source audiciav1alpha1.AudiciaSource, logger logr.Logger) (ingestor.Ingestor, error) {
	if source.Spec.Webhook == nil {
		logger.Error(nil, "Webhook source requires webhook config")
		return nil, fmt.Errorf("webhook source requires webhook config")
	}

	// TLS cert/key are mounted by the Helm chart from the Secret named in
	// spec.webhook.tlsSecretName. The mount path is a convention:
	//   /etc/audicia/webhook-tls/tls.crt
	//   /etc/audicia/webhook-tls/tls.key
	const tlsMountPath = "/etc/audicia/webhook-tls"
	tlsCertFile := path.Join(tlsMountPath, "tls.crt")
	tlsKeyFile := path.Join(tlsMountPath, "tls.key")

	wh := ingestor.NewWebhookIngestor(
		source.Spec.Webhook.Port,
		tlsCertFile, tlsKeyFile,
	)
	wh.MaxRequestBodyBytes = source.Spec.Webhook.MaxRequestBodyBytes
	wh.RateLimitPerSecond = source.Spec.Webhook.RateLimitPerSecond

	// Optional mTLS: if a client CA Secret is specified, mount its ca.crt
	// and configure the webhook server to require client certificates.
	if source.Spec.Webhook.ClientCASecretName != "" {
		const clientCAMountPath = "/etc/audicia/webhook-client-ca"
		wh.ClientCAFile = path.Join(clientCAMountPath, "ca.crt")
	}

	return wh, nil
}

func createCloudIngestor(source audiciav1alpha1.AudiciaSource, logger logr.Logger) (ingestor.Ingestor, error) {
	if source.Spec.Cloud == nil {
		logger.Error(nil, "CloudAuditLog source requires cloud config")
		return nil, fmt.Errorf("CloudAuditLog source requires cloud config")
	}

	msgSource, parser, err := cloud.BuildAdapter(source.Spec.Cloud)
	if err != nil {
		logger.Error(err, "failed to build cloud adapter", "provider", source.Spec.Cloud.Provider)
		return nil, fmt.Errorf("building cloud adapter: %w", err)
	}

	startPos := restoreCloudCheckpoint(source)

	var validator *cloud.ClusterIdentityValidator
	if source.Spec.Cloud.ClusterIdentity != "" {
		validator = &cloud.ClusterIdentityValidator{
			ExpectedIdentity: source.Spec.Cloud.ClusterIdentity,
		}
	}

	return cloud.NewCloudIngestor(msgSource, parser, validator, startPos, string(source.Spec.Cloud.Provider)), nil
}

// restoreCloudCheckpoint rebuilds CloudPosition from the AudiciaSource status.
func restoreCloudCheckpoint(source audiciav1alpha1.AudiciaSource) cloud.CloudPosition {
	pos := cloud.CloudPosition{}
	if source.Status.CloudCheckpoint != nil && source.Status.CloudCheckpoint.PartitionOffsets != nil {
		pos.PartitionOffsets = source.Status.CloudCheckpoint.PartitionOffsets
	}
	if source.Status.LastTimestamp != nil {
		pos.LastTimestamp = source.Status.LastTimestamp.Format(time.RFC3339)
	}
	return pos
}

// eventLoop processes incoming audit events and periodically flushes reports.
func (r *Reconciler) eventLoop(
	ctx context.Context,
	key types.NamespacedName,
	source audiciav1alpha1.AudiciaSource,
	engine *strategy.Engine,
	filterChain *filter.Chain,
	ing ingestor.Ingestor,
	events <-chan auditv1.Event,
) {
	logger := ctrl.Log.WithName("pipeline").WithValues("source", key)
	aggregators := make(map[string]*aggregator.Aggregator)
	subjects := make(map[string]audiciav1alpha1.Subject)

	checkpointInterval := time.Duration(source.Spec.Checkpoint.IntervalSeconds) * time.Second
	if checkpointInterval == 0 {
		checkpointInterval = 30 * time.Second
	}
	checkpointTicker := time.NewTicker(checkpointInterval)
	defer checkpointTicker.Stop()

	dirty := false

	for {
		select {
		case <-ctx.Done():
			// Pipeline shutting down. Do a final flush.
			if dirty {
				r.flushReports(context.Background(), key, source, engine, aggregators, subjects)
				r.flushCheckpoint(context.Background(), key, ing)
			}
			return

		case event, ok := <-events:
			if !ok {
				// Channel closed (ingestor stopped).
				logger.Info("ingestor channel closed")
				return
			}

			r.processEvent(event, source, filterChain, aggregators, subjects)
			dirty = true

		case <-checkpointTicker.C:
			if !dirty {
				continue
			}
			start := time.Now()
			r.flushReports(ctx, key, source, engine, aggregators, subjects)
			r.flushCheckpoint(ctx, key, ing)
			metrics.PipelineLatencySeconds.Observe(time.Since(start).Seconds())
			dirty = false
		}
	}
}

// processEvent runs a single audit event through filter -> normalizer -> aggregator.
func (r *Reconciler) processEvent(
	event auditv1.Event,
	source audiciav1alpha1.AudiciaSource,
	filterChain *filter.Chain,
	aggregators map[string]*aggregator.Aggregator,
	subjects map[string]audiciav1alpha1.Subject,
) {
	username := ""
	if event.User.Username != "" {
		username = event.User.Username
	}

	namespace := ""
	if event.ObjectRef != nil {
		namespace = event.ObjectRef.Namespace
	}

	// Filter.
	if !filterChain.Allow(username, namespace) {
		metrics.EventsFilteredTotal.WithLabelValues("deny").Inc()
		return
	}

	// Normalize subject.
	subject, include := normalizer.NormalizeSubject(username, source.Spec.IgnoreSystemUsers)
	if !include {
		metrics.EventsFilteredTotal.WithLabelValues("system_user").Inc()
		return
	}

	// Normalize event into a canonical rule.
	resource := ""
	subresource := ""
	apiGroup := ""
	if event.ObjectRef != nil {
		resource = event.ObjectRef.Resource
		subresource = event.ObjectRef.Subresource
		apiGroup = event.ObjectRef.APIGroup
	}
	rule := normalizer.NormalizeEvent(
		resource,
		subresource,
		apiGroup,
		event.Verb,
		namespace,
		event.RequestURI,
		event.ObjectRef != nil,
	)

	// Aggregate per subject.
	subjectKey := subjectKeyString(subject)
	if _, exists := aggregators[subjectKey]; !exists {
		aggregators[subjectKey] = aggregator.New()
		subjects[subjectKey] = subject
	}

	eventTime := time.Now()
	if !event.RequestReceivedTimestamp.Time.IsZero() {
		eventTime = event.RequestReceivedTimestamp.Time
	}
	aggregators[subjectKey].Add(rule, eventTime)

	metrics.EventsProcessedTotal.WithLabelValues(string(source.Spec.SourceType), "accepted").Inc()
}

// flushReports creates or updates AudiciaPolicyReport resources for each subject.
func (r *Reconciler) flushReports(
	ctx context.Context,
	key types.NamespacedName,
	source audiciav1alpha1.AudiciaSource,
	engine *strategy.Engine,
	aggregators map[string]*aggregator.Aggregator,
	subjects map[string]audiciav1alpha1.Subject,
) {
	logger := ctrl.Log.WithName("pipeline").WithValues("source", key)

	for subjectKey, agg := range aggregators {
		subject := subjects[subjectKey]
		rules := compactRules(agg.Rules(), source.Spec.Limits, subject.Name, logger)

		if err := r.flushSubjectReport(ctx, source, engine, subject, rules, agg.EventsProcessed(), logger); err != nil {
			logger.Error(err, "failed to flush report", "subject", subject.Name)
			metrics.ReconcileErrorsTotal.Inc()
		}
	}
}

// compactRules applies retention and truncation limits to observed rules.
func compactRules(rules []audiciav1alpha1.ObservedRule, limits audiciav1alpha1.LimitsConfig, subjectName string, logger logr.Logger) []audiciav1alpha1.ObservedRule {
	retentionDays := int(limits.RetentionDays)
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := metav1.NewTime(time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour))
	retained := make([]audiciav1alpha1.ObservedRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.LastSeen.Before(&cutoff) {
			retained = append(retained, rule)
		}
	}
	rules = retained

	// Sort by LastSeen descending for truncation (keep most recent).
	sort.Slice(rules, func(i, j int) bool {
		return rules[j].LastSeen.Before(&rules[i].LastSeen)
	})

	maxRules := int(limits.MaxRulesPerReport)
	if maxRules <= 0 {
		maxRules = 200
	}
	if len(rules) > maxRules {
		logger.Info("compacting rules", "subject", subjectName,
			"total", len(rules), "max", maxRules,
			"dropped", len(rules)-maxRules)
		rules = rules[:maxRules]
	}
	return rules
}

// flushSubjectReport creates/updates a single AudiciaPolicyReport for one subject.
func (r *Reconciler) flushSubjectReport(
	ctx context.Context,
	source audiciav1alpha1.AudiciaSource,
	engine *strategy.Engine,
	subject audiciav1alpha1.Subject,
	rules []audiciav1alpha1.ObservedRule,
	eventsProcessed int64,
	logger logr.Logger,
) error {
	manifests, err := engine.GenerateManifests(subject, rules)
	if err != nil {
		return fmt.Errorf("generating manifests: %w", err)
	}

	reportName := fmt.Sprintf("report-%s", sanitizeName(subject.Name))
	reportNamespace := source.Namespace
	if subject.Kind == audiciav1alpha1.SubjectKindServiceAccount && subject.Namespace != "" {
		reportNamespace = subject.Namespace
	}

	report := &audiciav1alpha1.AudiciaPolicyReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: reportNamespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, report, func() error {
		if reportNamespace == source.Namespace {
			if err := controllerutil.SetControllerReference(&source, report, r.Scheme); err != nil {
				return err
			}
		}
		report.Spec.Subject = subject
		return nil
	})
	if err != nil {
		return fmt.Errorf("creating/updating report: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		logger.Info("report spec updated", "report", reportName, "result", result)
	}

	// Update status with retry for conflict/not-found races.
	err = retry.OnError(retry.DefaultRetry, func(err error) bool {
		return errors.IsConflict(err) || errors.IsNotFound(err)
	}, func() error {
		if err := r.Get(ctx, types.NamespacedName{Name: reportName, Namespace: reportNamespace}, report); err != nil {
			return err
		}
		r.populateReportStatus(ctx, report, subject, rules, manifests, eventsProcessed, logger)
		return r.Status().Update(ctx, report)
	})
	if err != nil {
		return fmt.Errorf("updating report status: %w", err)
	}

	metrics.ReportsUpdatedTotal.Inc()
	metrics.ReportRulesCount.WithLabelValues(reportName).Set(float64(len(rules)))
	metrics.RulesGeneratedTotal.Add(float64(len(rules)))
	return nil
}

// populateReportStatus fills in the status fields of an AudiciaPolicyReport.
func (r *Reconciler) populateReportStatus(
	ctx context.Context,
	report *audiciav1alpha1.AudiciaPolicyReport,
	subject audiciav1alpha1.Subject,
	rules []audiciav1alpha1.ObservedRule,
	manifests []string,
	eventsProcessed int64,
	logger logr.Logger,
) {
	now := metav1.Now()
	report.Status.ObservedRules = rules
	report.Status.EventsProcessed = eventsProcessed
	report.Status.LastProcessedTime = &now
	if len(manifests) > 0 {
		report.Status.SuggestedPolicy = &audiciav1alpha1.SuggestedPolicy{
			Manifests: manifests,
		}
	}

	if r.Resolver != nil {
		effective, err := r.Resolver.EffectiveRules(ctx, subject)
		if err != nil {
			logger.V(1).Info("skipping compliance evaluation", "subject", subject.Name, "error", err)
		} else {
			report.Status.Compliance = diff.Evaluate(rules, effective)
		}
	}

	meta.SetStatusCondition(&report.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "PolicyGenerated",
		Message: fmt.Sprintf("Generated %d rules for %s", len(rules), subject.Name),
	})
}

// flushCheckpoint persists the ingestor checkpoint back to the AudiciaSource status.
func (r *Reconciler) flushCheckpoint(ctx context.Context, key types.NamespacedName, ing ingestor.Ingestor) {
	logger := ctrl.Log.WithName("pipeline").WithValues("source", key)

	// Cloud ingestors have partition-based checkpoints.
	if cloudIng, ok := ing.(*cloud.CloudIngestor); ok {
		r.flushCloudCheckpoint(ctx, key, cloudIng, logger)
		return
	}

	// File/webhook checkpoint path (unchanged).
	pos := ing.Checkpoint()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var source audiciav1alpha1.AudiciaSource
		if err := r.Get(ctx, key, &source); err != nil {
			return err
		}

		source.Status.FileOffset = pos.FileOffset
		source.Status.Inode = pos.Inode
		if pos.LastTimestamp != "" {
			t, err := time.Parse(time.RFC3339, pos.LastTimestamp)
			if err == nil {
				mt := metav1.NewTime(t)
				source.Status.LastTimestamp = &mt
			}
		}

		return r.Status().Update(ctx, &source)
	})
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "failed to update checkpoint")
		}
	} else {
		metrics.CheckpointLagSeconds.WithLabelValues(key.String()).Set(0)
	}
}

// flushCloudCheckpoint persists cloud-specific partition offsets to AudiciaSource status.
func (r *Reconciler) flushCloudCheckpoint(ctx context.Context, key types.NamespacedName, ing *cloud.CloudIngestor, logger logr.Logger) {
	cp := ing.CloudCheckpoint()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var source audiciav1alpha1.AudiciaSource
		if err := r.Get(ctx, key, &source); err != nil {
			return err
		}

		if source.Status.CloudCheckpoint == nil {
			source.Status.CloudCheckpoint = &audiciav1alpha1.CloudCheckpointStatus{}
		}
		source.Status.CloudCheckpoint.PartitionOffsets = cp.PartitionOffsets

		if cp.LastTimestamp != "" {
			t, err := time.Parse(time.RFC3339, cp.LastTimestamp)
			if err == nil {
				mt := metav1.NewTime(t)
				source.Status.LastTimestamp = &mt
			}
		}

		return r.Status().Update(ctx, &source)
	})
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "failed to update cloud checkpoint")
		}
	} else {
		metrics.CheckpointLagSeconds.WithLabelValues(key.String()).Set(0)
	}
}

// setCondition updates a condition on the AudiciaSource status.
func (r *Reconciler) setCondition(ctx context.Context, source *audiciav1alpha1.AudiciaSource, condition metav1.Condition) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, types.NamespacedName{Name: source.Name, Namespace: source.Namespace}, source); err != nil {
			return err
		}
		meta.SetStatusCondition(&source.Status.Conditions, condition)
		return r.Status().Update(ctx, source)
	})
}

// setSourceCondition is a convenience wrapper for setting conditions by key.
func (r *Reconciler) setSourceCondition(ctx context.Context, key types.NamespacedName, condition metav1.Condition) {
	var source audiciav1alpha1.AudiciaSource
	if err := r.Get(ctx, key, &source); err != nil {
		return
	}
	_ = r.setCondition(ctx, &source, condition)
}

// subjectKeyString returns a unique string key for a subject.
func subjectKeyString(s audiciav1alpha1.Subject) string {
	if s.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", s.Kind, s.Namespace, s.Name)
	}
	return fmt.Sprintf("%s/%s", s.Kind, s.Name)
}

// sanitizeName converts a subject name into a valid Kubernetes object name.
func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "@", "-at-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	// Trim to max 63 characters (Kubernetes name limit).
	if len(s) > 63 {
		s = s[:63]
	}
	// Remove trailing hyphens.
	s = strings.TrimRight(s, "-")
	return s
}
