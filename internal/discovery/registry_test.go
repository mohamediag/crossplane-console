package discovery

import (
	"testing"

	"github.com/mohamediag/crossplane-console/internal/testutil"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func crd(name, group, kind, plural, scope string, categories []string, storageVersion string, isEstablished bool) *unstructured.Unstructured {
	cats := make([]interface{}, 0, len(categories))
	for _, c := range categories {
		cats = append(cats, c)
	}
	status := "False"
	if isEstablished {
		status = "True"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"group": group,
			"scope": scope,
			"names": map[string]interface{}{
				"kind": kind, "plural": plural, "categories": cats,
			},
			"versions": []interface{}{
				map[string]interface{}{"name": "v1alpha0", "storage": false},
				map[string]interface{}{"name": storageVersion, "storage": true},
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Established", "status": status},
			},
		},
	}}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		crd      *unstructured.Unstructured
		wantOK   bool
		wantCat  Category
		wantGVR  schema.GroupVersionResource
		wantNS   bool
	}{
		{
			name:    "managed namespaced (v2 .m. group)",
			crd:     crd("objects.kubernetes.m.crossplane.io", "kubernetes.m.crossplane.io", "Object", "objects", "Namespaced", []string{"crossplane", "managed", "kubernetes"}, "v1alpha1", true),
			wantOK:  true,
			wantCat: CategoryManaged,
			wantGVR: schema.GroupVersionResource{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Resource: "objects"},
			wantNS:  true,
		},
		{
			name:    "managed cluster-scoped (legacy v1 group)",
			crd:     crd("buckets.s3.aws.upbound.io", "s3.aws.upbound.io", "Bucket", "buckets", "Cluster", []string{"crossplane", "managed", "aws"}, "v1beta1", true),
			wantOK:  true,
			wantCat: CategoryManaged,
			wantGVR: schema.GroupVersionResource{Group: "s3.aws.upbound.io", Version: "v1beta1", Resource: "buckets"},
			wantNS:  false,
		},
		{
			name:    "composite",
			crd:     crd("apps.platform.homelab.io", "platform.homelab.io", "App", "apps", "Namespaced", []string{"composite"}, "v1alpha1", true),
			wantOK:  true,
			wantCat: CategoryComposite,
			wantGVR: schema.GroupVersionResource{Group: "platform.homelab.io", Version: "v1alpha1", Resource: "apps"},
			wantNS:  true,
		},
		{
			name:    "package provider",
			crd:     crd("providers.pkg.crossplane.io", "pkg.crossplane.io", "Provider", "providers", "Cluster", nil, "v1", true),
			wantOK:  true,
			wantCat: CategoryPackage,
			wantGVR: schema.GroupVersionResource{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"},
			wantNS:  false,
		},
		{
			name:   "package revision kinds ignored",
			crd:    crd("providerrevisions.pkg.crossplane.io", "pkg.crossplane.io", "ProviderRevision", "providerrevisions", "Cluster", nil, "v1", true),
			wantOK: false,
		},
		{
			name:   "unrelated CRD ignored",
			crd:    crd("httproutes.gateway.networking.k8s.io", "gateway.networking.k8s.io", "HTTPRoute", "httproutes", "Namespaced", nil, "v1", true),
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info, ok := Classify(tc.crd)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if info.Category != tc.wantCat || info.GVR != tc.wantGVR || info.Namespaced != tc.wantNS {
				t.Errorf("info = %+v, want cat=%s gvr=%v ns=%v", info, tc.wantCat, tc.wantGVR, tc.wantNS)
			}
		})
	}
}

func TestClassifyLiveFixtures(t *testing.T) {
	mrCRD := testutil.Load(t, "crd-object-managed.yaml")
	info, ok := Classify(mrCRD)
	if !ok || info.Category != CategoryManaged || !info.Namespaced {
		t.Errorf("live MR CRD classify = %+v ok=%v, want managed+namespaced", info, ok)
	}
	xrCRD := testutil.Load(t, "crd-app-composite.yaml")
	info, ok = Classify(xrCRD)
	if !ok || info.Category != CategoryComposite || !info.Namespaced {
		t.Errorf("live XR CRD classify = %+v ok=%v, want composite+namespaced", info, ok)
	}
}

func TestRegistryLifecycle(t *testing.T) {
	var added []TypeInfo
	var removed []schema.GroupVersionResource
	r := New(
		func(i TypeInfo) { added = append(added, i) },
		func(g schema.GroupVersionResource) { removed = append(removed, g) },
	)

	notEstablished := crd("apps.platform.homelab.io", "platform.homelab.io", "App", "apps", "Namespaced", []string{"composite"}, "v1alpha1", false)
	r.HandleCRDUpsert(notEstablished)
	if len(added) != 0 {
		t.Fatal("registered a CRD before Established=True")
	}

	// The same CRD becomes Established (informer update event).
	establishedCRD := crd("apps.platform.homelab.io", "platform.homelab.io", "App", "apps", "Namespaced", []string{"composite"}, "v1alpha1", true)
	r.HandleCRDUpsert(establishedCRD)
	r.HandleCRDUpsert(establishedCRD) // idempotent
	if len(added) != 1 {
		t.Fatalf("onAdd fired %d times, want 1", len(added))
	}

	if watched, namespaced := r.Lookup("platform.homelab.io/v1alpha1", "App"); !watched || !namespaced {
		t.Errorf("Lookup = %v,%v want watched+namespaced", watched, namespaced)
	}
	// Different served version, same group+kind: still watched.
	if watched, _ := r.Lookup("platform.homelab.io/v1beta9", "App"); !watched {
		t.Error("Lookup should match on group+kind regardless of version")
	}
	if watched, _ := r.Lookup("apps/v1", "Deployment"); watched {
		t.Error("unregistered kind reported as watched")
	}

	r.HandleCRDDelete("apps.platform.homelab.io")
	if len(removed) != 1 {
		t.Fatalf("onRemove fired %d times, want 1", len(removed))
	}
	if watched, _ := r.Lookup("platform.homelab.io/v1alpha1", "App"); watched {
		t.Error("deleted type still watched")
	}
}

func TestCrossplaneDetected(t *testing.T) {
	r := New(nil, nil)
	if r.CrossplaneDetected() {
		t.Fatal("detected before any CRD seen")
	}
	// Any apiextensions.crossplane.io CRD flips detection, classified or not.
	xrdCRD := crd("compositeresourcedefinitions.apiextensions.crossplane.io",
		"apiextensions.crossplane.io", "CompositeResourceDefinition",
		"compositeresourcedefinitions", "Cluster", nil, "v2", true)
	r.HandleCRDUpsert(xrdCRD)
	if !r.CrossplaneDetected() {
		t.Fatal("apiextensions.crossplane.io CRD did not flip detection")
	}
}
