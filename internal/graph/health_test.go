package graph

import (
	"testing"
	"time"

	"github.com/mohamediag/crossplane-console/internal/testutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestComputeHealth(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		obj   func() *unstructured.Unstructured
		want  string
	}{
		{"ready true", func() *unstructured.Unstructured {
			return testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"), [2]string{"Ready", "True"})
		}, StateHealthy},
		{"ready false", func() *unstructured.Unstructured {
			return testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"), [2]string{"Ready", "False"})
		}, StateUnhealthy},
		{"synced false without ready", func() *unstructured.Unstructured {
			return testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"), [2]string{"Synced", "False"})
		}, StateUnhealthy},
		{"ready false beats synced true", func() *unstructured.Unstructured {
			return testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"),
				[2]string{"Synced", "True"}, [2]string{"Ready", "False"})
		}, StateUnhealthy},
		{"no conditions at all", func() *unstructured.Unstructured {
			return testutil.Obj("g/v1", "K", "ns", "a")
		}, StateNA},
		{"conditions but no ready, young object", func() *unstructured.Unstructured {
			u := testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"), [2]string{"Synced", "True"})
			u.SetCreationTimestamp(metav1.NewTime(now.Add(-time.Minute)))
			return u
		}, StateProgressing},
		{"conditions but no ready, old object", func() *unstructured.Unstructured {
			u := testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"), [2]string{"Synced", "True"})
			u.SetCreationTimestamp(metav1.NewTime(now.Add(-time.Hour)))
			return u
		}, StateUnknown},
		{"ready unknown, old object", func() *unstructured.Unstructured {
			u := testutil.WithConditions(testutil.Obj("g/v1", "K", "ns", "a"), [2]string{"Ready", "Unknown"})
			u.SetCreationTimestamp(metav1.NewTime(now.Add(-time.Hour)))
			return u
		}, StateUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeHealth(tc.obj(), now); got.State != tc.want {
				t.Errorf("state = %s, want %s", got.State, tc.want)
			}
		})
	}
}

func TestComputeHealthKeepsConditionDetails(t *testing.T) {
	u := testutil.Obj("g/v1", "K", "ns", "a")
	_ = unstructured.SetNestedSlice(u.Object, []interface{}{
		map[string]interface{}{
			"type": "Ready", "status": "False", "reason": "Creating",
			"message": "Unready resources: configmap", "lastTransitionTime": "2026-05-10T18:32:34Z",
		},
	}, "status", "conditions")
	h := ComputeHealth(u, time.Now())
	if h.Ready == nil || h.Ready.Reason != "Creating" || h.Ready.Message == "" || h.Ready.LastTransitionTime == "" {
		t.Fatalf("Ready condition details not preserved: %+v", h.Ready)
	}
}

func TestPackageHealth(t *testing.T) {
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want string
	}{
		{"healthy installed", testutil.WithConditions(testutil.Obj("pkg.crossplane.io/v1", "Provider", "", "p"),
			[2]string{"Healthy", "True"}, [2]string{"Installed", "True"}), StateHealthy},
		{"unhealthy", testutil.WithConditions(testutil.Obj("pkg.crossplane.io/v1", "Provider", "", "p"),
			[2]string{"Healthy", "False"}, [2]string{"Installed", "True"}), StateUnhealthy},
		{"installing", testutil.WithConditions(testutil.Obj("pkg.crossplane.io/v1", "Provider", "", "p"),
			[2]string{"Installed", "True"}), StateProgressing},
		{"no conditions", testutil.Obj("pkg.crossplane.io/v1", "Provider", "", "p"), StateNA},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PackageHealth(tc.obj); got.State != tc.want {
				t.Errorf("state = %s, want %s", got.State, tc.want)
			}
		})
	}
}
