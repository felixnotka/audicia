package operator

import (
	"testing"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

func TestSchemeRegistration_AudiciaSource(t *testing.T) {
	gvk := audiciav1alpha1.SchemeGroupVersion.WithKind("AudiciaSource")
	obj, err := scheme.New(gvk)
	if err != nil {
		t.Fatalf("AudiciaSource not registered in scheme: %v", err)
	}
	if _, ok := obj.(*audiciav1alpha1.AudiciaSource); !ok {
		t.Errorf("scheme returned %T, expected *AudiciaSource", obj)
	}
}

func TestSchemeRegistration_AudiciaPolicyReport(t *testing.T) {
	gvk := audiciav1alpha1.SchemeGroupVersion.WithKind("AudiciaPolicyReport")
	obj, err := scheme.New(gvk)
	if err != nil {
		t.Fatalf("AudiciaPolicyReport not registered in scheme: %v", err)
	}
	if _, ok := obj.(*audiciav1alpha1.AudiciaPolicyReport); !ok {
		t.Errorf("scheme returned %T, expected *AudiciaPolicyReport", obj)
	}
}

func TestSchemeRegistration_AudiciaSourceList(t *testing.T) {
	gvk := audiciav1alpha1.SchemeGroupVersion.WithKind("AudiciaSourceList")
	obj, err := scheme.New(gvk)
	if err != nil {
		t.Fatalf("AudiciaSourceList not registered in scheme: %v", err)
	}
	if _, ok := obj.(*audiciav1alpha1.AudiciaSourceList); !ok {
		t.Errorf("scheme returned %T, expected *AudiciaSourceList", obj)
	}
}

func TestConfig_FieldDefaults(t *testing.T) {
	cfg := Config{}

	// Zero values for Go struct; env tags define the runtime defaults.
	if cfg.MetricsBindAddress != "" {
		t.Errorf("expected empty MetricsBindAddress, got %q", cfg.MetricsBindAddress)
	}
	if cfg.HealthProbeBindAddress != "" {
		t.Errorf("expected empty HealthProbeBindAddress, got %q", cfg.HealthProbeBindAddress)
	}
	if cfg.LeaderElectionEnabled {
		t.Error("expected LeaderElectionEnabled=false as Go zero value")
	}
	if cfg.ConcurrentReconciles != 0 {
		t.Errorf("expected ConcurrentReconciles=0, got %d", cfg.ConcurrentReconciles)
	}
	if cfg.SyncPeriod != 0 {
		t.Errorf("expected SyncPeriod=0, got %v", cfg.SyncPeriod)
	}
}
