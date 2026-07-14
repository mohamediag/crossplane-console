package graph

import (
	"testing"
	"time"

	"github.com/mohamediag/crossplane-console/internal/testutil"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// homelabLookup mimics the registry for the types in the live fixtures.
func homelabLookup(apiVersion, kind string) (bool, bool) {
	switch {
	case kind == "App" && apiVersion == "platform.homelab.io/v1alpha1":
		return true, true
	case kind == "Object" && apiVersion == "kubernetes.m.crossplane.io/v1alpha1":
		return true, true
	}
	return false, true
}

// TestBuildLiveFixtures uses YAML captured from the live homelab cluster:
// the sample-service XR (Ready=False) plus one of its four Object MRs.
func TestBuildLiveFixtures(t *testing.T) {
	xr := testutil.Load(t, "xr-sample-service.yaml")
	mr := testutil.Load(t, "mr-object-deployment.yaml")
	comp := testutil.Obj("apiextensions.crossplane.io/v1", "Composition", "", "app-kubernetes")

	snap := Build(Input{
		XRs:          []*unstructured.Unstructured{xr},
		MRs:          []*unstructured.Unstructured{mr},
		Compositions: []*unstructured.Unstructured{comp},
		Lookup:       homelabLookup,
		Now:          time.Now(),
		Revision:     1,
	})

	// 1 XR + 4 referenced children (1 cached MR + 3 missing placeholders)
	// + 1 Composition the XR resolves to.
	if got := len(snap.Nodes); got != 6 {
		t.Fatalf("nodes = %d, want 6", got)
	}
	// 4 resourceRefs edges + 1 XR→Composition "uses" edge.
	if got := len(snap.Edges); got != 5 {
		t.Fatalf("edges = %d, want 5", got)
	}

	xrID := NodeID("platform.homelab.io/v1alpha1", "App", "sample-application-dev", "sample-service")
	xrNode := snap.Nodes[xrID]
	if xrNode == nil {
		t.Fatalf("XR node %q missing", xrID)
	}
	if xrNode.Health.State != StateUnhealthy {
		t.Errorf("XR own health = %s, want Unhealthy (live Ready=False)", xrNode.Health.State)
	}
	if xrNode.Aggregate != StateUnhealthy {
		t.Errorf("XR aggregate = %s, want Unhealthy", xrNode.Aggregate)
	}
	if xrNode.Composition == nil || xrNode.Composition.Name != "app-kubernetes" {
		t.Errorf("XR composition ref = %+v, want app-kubernetes", xrNode.Composition)
	}
	if xrNode.CompositionRevision == "" {
		t.Errorf("XR composition revision empty, want spec.crossplane.compositionRevisionRef.name")
	}

	// The Composition node: projected into the XR's namespace, linked by a
	// validated "uses" edge.
	compID := NodeID("apiextensions.crossplane.io/v1", "Composition", "sample-application-dev", "app-kubernetes")
	compNode := snap.Nodes[compID]
	if compNode == nil {
		t.Fatalf("Composition node %q missing", compID)
	}
	if compNode.NodeType != NodeComposition {
		t.Errorf("Composition nodeType = %s, want %s", compNode.NodeType, NodeComposition)
	}
	compEdge := snap.Edges[EdgeID(xrID, compID)]
	if compEdge == nil {
		t.Fatalf("XR→Composition edge missing")
	}
	if compEdge.Type != "uses" || !compEdge.Validated {
		t.Errorf("composition edge = %+v, want type=uses validated=true", compEdge)
	}

	// The cached MR: healthy, validated edge, external-name surfaced.
	mrID := NodeID("kubernetes.m.crossplane.io/v1alpha1", "Object", "sample-application-dev", "sample-service-deployment")
	mrNode := snap.Nodes[mrID]
	if mrNode == nil {
		t.Fatalf("MR node %q missing", mrID)
	}
	if mrNode.Health.State != StateHealthy {
		t.Errorf("MR health = %s, want Healthy", mrNode.Health.State)
	}
	if mrNode.ExternalName != "sample-service-deployment" {
		t.Errorf("MR externalName = %q", mrNode.ExternalName)
	}
	edge := snap.Edges[EdgeID(xrID, mrID)]
	if edge == nil {
		t.Fatal("edge XR→MR missing")
	}
	if !edge.Validated {
		t.Error("edge XR→MR not validated despite matching ownerReference")
	}

	// The three uncached refs become missing placeholders with Unknown health.
	missingID := NodeID("kubernetes.m.crossplane.io/v1alpha1", "Object", "sample-application-dev", "sample-service-configmap")
	missing := snap.Nodes[missingID]
	if missing == nil {
		t.Fatalf("placeholder node %q missing", missingID)
	}
	if missing.NodeType != NodeMissing || missing.Health.State != StateUnknown {
		t.Errorf("placeholder = %s/%s, want missing/Unknown", missing.NodeType, missing.Health.State)
	}
	if e := snap.Edges[EdgeID(xrID, missingID)]; e == nil || e.Validated {
		t.Errorf("edge to missing child should exist and be unvalidated, got %+v", e)
	}
}

// withCompositionRef sets spec.crossplane.compositionRef.name on an XR.
func withCompositionRef(u *unstructured.Unstructured, name string) *unstructured.Unstructured {
	_ = unstructured.SetNestedField(u.Object, name, "spec", "crossplane", "compositionRef", "name")
	return u
}

func TestBuildCompositionEdges(t *testing.T) {
	xrAPI := "platform.homelab.io/v1alpha1"
	compAPI := "apiextensions.crossplane.io/v1"

	t.Run("cached composition becomes a node in the XR's namespace", func(t *testing.T) {
		xr := withCompositionRef(testutil.Obj(xrAPI, "App", "ns1", "app1"), "my-comp")
		comp := testutil.Obj(compAPI, "Composition", "", "my-comp")

		s := Build(Input{
			XRs:          []*unstructured.Unstructured{xr},
			Compositions: []*unstructured.Unstructured{comp},
			Lookup:       homelabLookup,
			Now:          time.Now(),
			Revision:     1,
		})

		xrID := NodeID(xrAPI, "App", "ns1", "app1")
		compID := NodeID(compAPI, "Composition", "ns1", "my-comp") // projected into ns1
		n := s.Nodes[compID]
		if n == nil {
			t.Fatalf("composition node %q missing", compID)
		}
		if n.NodeType != NodeComposition {
			t.Errorf("nodeType = %s, want %s", n.NodeType, NodeComposition)
		}
		if n.Namespace != "ns1" {
			t.Errorf("namespace = %q, want ns1 (projected)", n.Namespace)
		}
		e := s.Edges[EdgeID(xrID, compID)]
		if e == nil || e.Type != "uses" || !e.Validated {
			t.Errorf("uses edge = %+v, want type=uses validated=true", e)
		}
		// A cached (present) composition must not worsen the XR aggregate.
		if s.Nodes[xrID].Aggregate == StateUnknown {
			t.Errorf("XR aggregate = Unknown, cached composition should not poison it")
		}
	})

	t.Run("dangling compositionRef becomes a missing node and poisons aggregate", func(t *testing.T) {
		xr := withCompositionRef(testutil.Obj(xrAPI, "App", "ns1", "app2"), "ghost")

		s := Build(Input{
			XRs:      []*unstructured.Unstructured{xr},
			Lookup:   homelabLookup,
			Now:      time.Now(),
			Revision: 1,
		})

		compID := NodeID(compAPI, "Composition", "ns1", "ghost")
		n := s.Nodes[compID]
		if n == nil {
			t.Fatalf("dangling composition node %q missing", compID)
		}
		if n.NodeType != NodeMissing || n.Health.State != StateUnknown {
			t.Errorf("dangling = %s/%s, want missing/Unknown", n.NodeType, n.Health.State)
		}
		xrID := NodeID(xrAPI, "App", "ns1", "app2")
		if s.Nodes[xrID].Aggregate != StateUnknown {
			t.Errorf("XR aggregate = %s, want Unknown (poisoned by dangling composition)", s.Nodes[xrID].Aggregate)
		}
	})

	t.Run("same composition used by XRs in two namespaces yields two nodes", func(t *testing.T) {
		xrA := withCompositionRef(testutil.Obj(xrAPI, "App", "nsA", "a"), "shared")
		xrB := withCompositionRef(testutil.Obj(xrAPI, "App", "nsB", "b"), "shared")
		comp := testutil.Obj(compAPI, "Composition", "", "shared")

		s := Build(Input{
			XRs:          []*unstructured.Unstructured{xrA, xrB},
			Compositions: []*unstructured.Unstructured{comp},
			Lookup:       homelabLookup,
			Now:          time.Now(),
			Revision:     1,
		})

		if s.Nodes[NodeID(compAPI, "Composition", "nsA", "shared")] == nil {
			t.Error("composition node for nsA missing")
		}
		if s.Nodes[NodeID(compAPI, "Composition", "nsB", "shared")] == nil {
			t.Error("composition node for nsB missing")
		}
	})

	t.Run("XR without compositionRef gets no composition node or edge", func(t *testing.T) {
		xr := testutil.Obj(xrAPI, "App", "ns1", "plain")
		s := Build(Input{
			XRs:      []*unstructured.Unstructured{xr},
			Lookup:   homelabLookup,
			Now:      time.Now(),
			Revision: 1,
		})
		if len(s.Edges) != 0 {
			t.Errorf("edges = %d, want 0", len(s.Edges))
		}
		for _, n := range s.Nodes {
			if n.NodeType == NodeComposition || n.Kind == "Composition" {
				t.Errorf("unexpected composition node %+v", n)
			}
		}
	})
}

func TestBuildTable(t *testing.T) {
	xrAPI, mrAPI := "platform.homelab.io/v1alpha1", "kubernetes.m.crossplane.io/v1alpha1"
	lookup := homelabLookup

	tests := []struct {
		name   string
		in     func() Input
		verify func(t *testing.T, s *Snapshot)
	}{
		{
			name: "legacy v1 spec.resourceRefs fallback",
			in: func() Input {
				xr := testutil.Obj(xrAPI, "App", "ns1", "legacy")
				_ = unstructured.SetNestedSlice(xr.Object, []interface{}{
					map[string]interface{}{"apiVersion": mrAPI, "kind": "Object", "name": "child"},
				}, "spec", "resourceRefs")
				return Input{XRs: []*unstructured.Unstructured{xr}, Lookup: lookup, Now: time.Now()}
			},
			verify: func(t *testing.T, s *Snapshot) {
				if len(s.Edges) != 1 {
					t.Fatalf("edges = %d, want 1 (legacy path not read)", len(s.Edges))
				}
			},
		},
		{
			name: "explicit ref namespace wins over parent namespace",
			in: func() Input {
				xr := testutil.WithResourceRefs(testutil.Obj(xrAPI, "App", "ns1", "xr"),
					map[string]interface{}{"apiVersion": mrAPI, "kind": "Object", "name": "c", "namespace": "other"})
				return Input{XRs: []*unstructured.Unstructured{xr}, Lookup: lookup, Now: time.Now()}
			},
			verify: func(t *testing.T, s *Snapshot) {
				want := NodeID(mrAPI, "Object", "other", "c")
				if s.Nodes[want] == nil {
					t.Fatalf("child node with explicit namespace missing, nodes=%v", keys(s))
				}
			},
		},
		{
			name: "cluster-scoped builtin ref gets no namespace",
			in: func() Input {
				xr := testutil.WithResourceRefs(testutil.Obj(xrAPI, "App", "ns1", "xr"),
					map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "target-ns"})
				return Input{XRs: []*unstructured.Unstructured{xr}, Lookup: lookup, Now: time.Now()}
			},
			verify: func(t *testing.T, s *Snapshot) {
				want := NodeID("v1", "Namespace", "", "target-ns")
				n := s.Nodes[want]
				if n == nil {
					t.Fatalf("cluster-scoped child missing, nodes=%v", keys(s))
				}
				if n.NodeType != NodeK8s || n.Health.State != StateNA {
					t.Errorf("plain k8s child = %s/%s, want k8s/NA", n.NodeType, n.Health.State)
				}
			},
		},
		{
			name: "unwatched namespaced kind renders as k8s node, never missing",
			in: func() Input {
				xr := testutil.WithResourceRefs(testutil.Obj(xrAPI, "App", "ns1", "xr"),
					map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "name": "web"})
				return Input{XRs: []*unstructured.Unstructured{xr}, Lookup: lookup, Now: time.Now()}
			},
			verify: func(t *testing.T, s *Snapshot) {
				n := s.Nodes[NodeID("apps/v1", "Deployment", "ns1", "web")]
				if n == nil {
					t.Fatalf("k8s child missing, nodes=%v", keys(s))
				}
				if n.NodeType != NodeK8s {
					t.Errorf("nodeType = %s, want k8s", n.NodeType)
				}
			},
		},
		{
			name: "nested XR: unhealthy grandchild poisons both ancestors",
			in: func() Input {
				root := testutil.WithResourceRefs(
					testutil.WithConditions(testutil.Obj(xrAPI, "App", "ns1", "root"), [2]string{"Ready", "True"}),
					map[string]interface{}{"apiVersion": xrAPI, "kind": "App", "name": "mid"})
				mid := testutil.WithResourceRefs(
					testutil.WithConditions(testutil.Obj(xrAPI, "App", "ns1", "mid"), [2]string{"Ready", "True"}),
					map[string]interface{}{"apiVersion": mrAPI, "kind": "Object", "name": "leaf"})
				leaf := testutil.WithConditions(testutil.Obj(mrAPI, "Object", "ns1", "leaf"),
					[2]string{"Ready", "False"})
				return Input{
					XRs: []*unstructured.Unstructured{root, mid},
					MRs: []*unstructured.Unstructured{leaf},
					Lookup: lookup, Now: time.Now(),
				}
			},
			verify: func(t *testing.T, s *Snapshot) {
				rootN := s.Nodes[NodeID(xrAPI, "App", "ns1", "root")]
				midN := s.Nodes[NodeID(xrAPI, "App", "ns1", "mid")]
				if rootN.Health.State != StateHealthy {
					t.Errorf("root own health = %s, want Healthy", rootN.Health.State)
				}
				if rootN.Aggregate != StateUnhealthy || midN.Aggregate != StateUnhealthy {
					t.Errorf("aggregates root=%s mid=%s, want Unhealthy both", rootN.Aggregate, midN.Aggregate)
				}
			},
		},
		{
			name: "ref cycle terminates",
			in: func() Input {
				a := testutil.WithResourceRefs(
					testutil.WithConditions(testutil.Obj(xrAPI, "App", "ns1", "a"), [2]string{"Ready", "True"}),
					map[string]interface{}{"apiVersion": xrAPI, "kind": "App", "name": "b"})
				b := testutil.WithResourceRefs(
					testutil.WithConditions(testutil.Obj(xrAPI, "App", "ns1", "b"), [2]string{"Ready", "True"}),
					map[string]interface{}{"apiVersion": xrAPI, "kind": "App", "name": "a"})
				return Input{XRs: []*unstructured.Unstructured{a, b}, Lookup: lookup, Now: time.Now()}
			},
			verify: func(t *testing.T, s *Snapshot) {
				// Just completing is the main assertion; cycle members
				// see an Unknown from the cut-off back-edge.
				if len(s.Nodes) != 2 {
					t.Fatalf("nodes = %d, want 2", len(s.Nodes))
				}
			},
		},
		{
			name: "packages get Healthy/Installed-based health",
			in: func() Input {
				p := testutil.WithConditions(
					testutil.Obj("pkg.crossplane.io/v1", "Provider", "", "provider-kubernetes"),
					[2]string{"Healthy", "True"}, [2]string{"Installed", "True"})
				return Input{Packages: []*unstructured.Unstructured{p}, Lookup: lookup, Now: time.Now()}
			},
			verify: func(t *testing.T, s *Snapshot) {
				n := s.Nodes[NodeID("pkg.crossplane.io/v1", "Provider", "", "provider-kubernetes")]
				if n == nil || n.NodeType != NodeProvider || n.Health.State != StateHealthy {
					t.Fatalf("provider node = %+v, want provider/Healthy", n)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.verify(t, Build(tc.in()))
		})
	}
}

func keys(s *Snapshot) []string {
	out := make([]string, 0, len(s.Nodes))
	for id := range s.Nodes {
		out = append(out, id)
	}
	return out
}
