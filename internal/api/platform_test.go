package api

import (
	"testing"

	"github.com/mohamediag/crossplane-console/internal/testutil"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestBuildPlatformLiveFixtures drives BuildPlatform with YAML captured from
// the live homelab cluster: one Provider, the App XRD, its Composition and
// one of its CompositionRevisions.
func TestBuildPlatformLiveFixtures(t *testing.T) {
	provider := testutil.Load(t, "provider-kubernetes.yaml")
	xrd := testutil.Load(t, "xrd-app.yaml")
	comp := testutil.Load(t, "composition-app.yaml")
	rev := testutil.Load(t, "compositionrevision-app.yaml")

	pc := testutil.Load(t, "clusterproviderconfig-default.yaml")

	s := BuildPlatform(PlatformInput{
		Packages:        []*unstructured.Unstructured{provider},
		Extensions:      []*unstructured.Unstructured{xrd, comp, rev},
		ProviderConfigs: []*unstructured.Unstructured{pc},
	})

	if len(s.Providers) != 1 {
		t.Fatalf("providers = %d, want 1", len(s.Providers))
	}
	p := s.Providers[0]
	if p.Name != "provider-kubernetes" || p.Health.State != "Healthy" {
		t.Errorf("provider = %s/%s, want provider-kubernetes/Healthy", p.Name, p.Health.State)
	}
	if p.Package == "" || p.CurrentRevision == "" {
		t.Errorf("provider package/revision empty: %+v", p)
	}

	if len(s.XRDs) != 1 {
		t.Fatalf("xrds = %d, want 1", len(s.XRDs))
	}
	x := s.XRDs[0]
	if x.Group != "platform.homelab.io" || x.Kind != "App" || x.Scope != "Namespaced" || !x.Established {
		t.Errorf("xrd = %+v, want platform.homelab.io/App Namespaced Established", x)
	}
	if len(x.Compositions) != 1 || x.Compositions[0] != "app-kubernetes" {
		t.Errorf("xrd.Compositions = %v, want [app-kubernetes]", x.Compositions)
	}
	if len(x.Versions) != 1 || x.Versions[0] != "v1alpha1" {
		t.Errorf("xrd.Versions = %v, want [v1alpha1]", x.Versions)
	}

	if len(s.Compositions) != 1 {
		t.Fatalf("compositions = %d, want 1", len(s.Compositions))
	}
	c := s.Compositions[0]
	if c.Mode != "Pipeline" || c.CompositeKind != "App" {
		t.Errorf("composition = %+v, want Pipeline mode serving App", c)
	}
	if len(c.Pipeline) != 1 || c.Pipeline[0].Function != "function-go-templating" {
		t.Errorf("pipeline = %v, want one step -> function-go-templating", c.Pipeline)
	}
	if c.RevisionCount != 1 || c.LatestRevisionName == "" {
		t.Errorf("revisions = count %d latest %q, want 1 revision with a name", c.RevisionCount, c.LatestRevisionName)
	}

	// Live ClusterProviderConfig fixture: InjectedIdentity, status.users=12.
	if len(s.ProviderConfigs) != 1 {
		t.Fatalf("providerConfigs = %d, want 1", len(s.ProviderConfigs))
	}
	got := s.ProviderConfigs[0]
	if got.Name != "default" || got.Kind != "ClusterProviderConfig" ||
		got.CredentialsSource != "InjectedIdentity" || got.UsedBy != 12 {
		t.Errorf("providerConfig = %+v, want default/ClusterProviderConfig/InjectedIdentity/12", got)
	}
}

func TestBuildPlatformRevisionSelection(t *testing.T) {
	mkRev := func(name string, revision int64) *unstructured.Unstructured {
		u := testutil.Obj("apiextensions.crossplane.io/v1", "CompositionRevision", "", name)
		u.SetLabels(map[string]string{"crossplane.io/composition-name": "demo"})
		_ = unstructured.SetNestedField(u.Object, revision, "spec", "revision")
		return u
	}
	comp := testutil.Obj("apiextensions.crossplane.io/v1", "Composition", "", "demo")
	_ = unstructured.SetNestedField(comp.Object, "demo.io/v1", "spec", "compositeTypeRef", "apiVersion")
	_ = unstructured.SetNestedField(comp.Object, "Demo", "spec", "compositeTypeRef", "kind")

	s := BuildPlatform(PlatformInput{Extensions: []*unstructured.Unstructured{
		comp, mkRev("demo-aaa", 1), mkRev("demo-bbb", 3), mkRev("demo-ccc", 2),
	}})
	if len(s.Compositions) != 1 {
		t.Fatalf("compositions = %d, want 1", len(s.Compositions))
	}
	c := s.Compositions[0]
	if c.RevisionCount != 3 || c.LatestRevision != 3 || c.LatestRevisionName != "demo-bbb" {
		t.Errorf("got count=%d latest=%d name=%s, want 3/3/demo-bbb",
			c.RevisionCount, c.LatestRevision, c.LatestRevisionName)
	}
}

func TestBuildPlatformEmpty(t *testing.T) {
	s := BuildPlatform(PlatformInput{})
	// All slices must be non-nil so the JSON renders [] not null.
	if s.Providers == nil || s.Functions == nil || s.Configurations == nil ||
		s.XRDs == nil || s.Compositions == nil || s.ProviderConfigs == nil ||
		s.Operations == nil {
		t.Fatal("empty summary must have non-nil slices")
	}
}

func TestBuildPlatformProviderConfigFallbackCount(t *testing.T) {
	// A ProviderConfig without status.users falls back to counting MRs
	// whose spec.providerConfigRef.name matches.
	pc := testutil.Obj("aws.m.upbound.io/v1beta1", "ProviderConfig", "", "aws-default")
	_ = unstructured.SetNestedField(pc.Object, "Secret", "spec", "credentials", "source")
	mkMR := func(name, ref string) *unstructured.Unstructured {
		u := testutil.Obj("s3.aws.m.upbound.io/v1beta1", "Bucket", "ns", name)
		_ = unstructured.SetNestedField(u.Object, ref, "spec", "providerConfigRef", "name")
		return u
	}
	// An MR from a DIFFERENT provider family with the same ref name must
	// not count (the live cluster has two configs both named "default").
	otherFamily := testutil.Obj("kubernetes.m.crossplane.io/v1alpha1", "Object", "ns", "k8s-obj")
	_ = unstructured.SetNestedField(otherFamily.Object, "aws-default", "spec", "providerConfigRef", "name")

	s := BuildPlatform(PlatformInput{
		ProviderConfigs: []*unstructured.Unstructured{pc},
		MRs: []*unstructured.Unstructured{
			mkMR("b1", "aws-default"), mkMR("b2", "aws-default"), mkMR("b3", "other"), otherFamily,
		},
	})
	if len(s.ProviderConfigs) != 1 || s.ProviderConfigs[0].UsedBy != 2 {
		t.Fatalf("providerConfigs = %+v, want aws-default usedBy=2 (family-scoped)", s.ProviderConfigs)
	}
}

func TestBuildPlatformOperations(t *testing.T) {
	op := testutil.WithConditions(
		testutil.Obj("ops.crossplane.io/v1alpha1", "CronOperation", "ops-ns", "nightly"),
		[2]string{"Ready", "True"})
	_ = unstructured.SetNestedField(op.Object, "0 2 * * *", "spec", "schedule")
	s := BuildPlatform(PlatformInput{Operations: []*unstructured.Unstructured{op}})
	if len(s.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(s.Operations))
	}
	got := s.Operations[0]
	if got.Kind != "CronOperation" || got.Schedule != "0 2 * * *" || got.Health.State != "Healthy" {
		t.Errorf("operation = %+v, want CronOperation with schedule and Healthy", got)
	}
}
