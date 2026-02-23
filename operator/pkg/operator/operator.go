package operator

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/controller/audiciasource"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(audiciav1alpha1.AddToScheme(scheme))
}

// BuildInfo holds build-time metadata injected via ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Start initializes and runs the operator.
func Start(ctx context.Context, buildInfo BuildInfo, config Config) error {
	logger := zap.New(zap.UseDevMode(config.LogLevel > 0))
	ctrl.SetLogger(logger)

	setupLog := ctrl.Log.WithName("setup")
	setupLog.Info("starting audicia operator",
		"version", buildInfo.Version,
		"commit", buildInfo.Commit,
		"date", buildInfo.Date,
	)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: config.MetricsBindAddress,
		},
		HealthProbeBindAddress:  config.HealthProbeBindAddress,
		LeaderElection:          config.LeaderElectionEnabled,
		LeaderElectionID:        config.LeaderElectionID,
		LeaderElectionNamespace: config.LeaderElectionNamespace,
		Cache: cache.Options{
			SyncPeriod: &config.SyncPeriod,
		},
	})
	if err != nil {
		return fmt.Errorf("unable to create manager: %w", err)
	}

	// Register controllers.
	if err := audiciasource.SetupWithManager(mgr, config.ConcurrentReconciles); err != nil {
		return fmt.Errorf("unable to create AudiciaSource controller: %w", err)
	}

	// Prime RBAC informer caches so the compliance resolver has warm data
	// on its first evaluation. GetInformer registers the type with the cache
	// but does not block — actual sync happens when the manager starts.
	rbacTypes := []client.Object{
		&rbacv1.ClusterRole{},
		&rbacv1.ClusterRoleBinding{},
		&rbacv1.Role{},
		&rbacv1.RoleBinding{},
	}
	for _, obj := range rbacTypes {
		if _, err := mgr.GetCache().GetInformer(ctx, obj); err != nil {
			setupLog.Error(err, "failed to prime RBAC cache informer", "type", fmt.Sprintf("%T", obj))
			// Non-fatal: compliance will degrade gracefully.
		}
	}

	// Health checks.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager exited with error: %w", err)
	}

	return nil
}
