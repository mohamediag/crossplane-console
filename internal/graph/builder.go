package graph

import (
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TypeLookup reports whether a (apiVersion, kind) is a watched Crossplane
// type (XR or MR) and, if so, whether it is namespaced. Unwatched kinds are
// plain Kubernetes children: rendered from the ref alone, never "missing".
type TypeLookup func(apiVersion, kind string) (watched bool, namespaced bool)

// Input is everything Build needs. All slices come straight from informer
// stores; Build never talks to the API server.
type Input struct {
	XRs      []*unstructured.Unstructured
	MRs      []*unstructured.Unstructured
	Packages []*unstructured.Unstructured
	// Compositions are the apiextensions.crossplane.io Compositions an XR may
	// resolve to via spec.crossplane.compositionRef. Only those actually
	// referenced by an XR become graph nodes.
	Compositions []*unstructured.Unstructured
	Lookup       TypeLookup
	Now          time.Time
	Revision     int64
}

// Common cluster-scoped built-in kinds, for namespace-defaulting of refs to
// unwatched types. Anything not listed defaults to the parent XR's namespace.
var clusterScopedBuiltins = map[string]bool{
	"Namespace": true, "Node": true, "PersistentVolume": true,
	"ClusterRole": true, "ClusterRoleBinding": true, "StorageClass": true,
	"CustomResourceDefinition": true, "IngressClass": true, "PriorityClass": true,
}

// Build constructs a full snapshot from cached objects.
//
// Edges are extracted top-down from each XR's spec.crossplane.resourceRefs
// (v2) with a fallback to spec.resourceRefs (legacy v1 XRs), and validated
// bottom-up by checking the child's ownerReferences for the parent's UID.
func Build(in Input) *Snapshot {
	s := &Snapshot{
		Revision:    in.Revision,
		GeneratedAt: in.Now,
		Nodes:       map[string]*Node{},
		Edges:       map[string]*Edge{},
	}
	lookup := in.Lookup
	if lookup == nil {
		lookup = func(string, string) (bool, bool) { return false, true }
	}

	byUID := map[string]*Node{}
	add := func(n *Node) *Node {
		s.Nodes[n.ID] = n
		if n.UID != "" {
			byUID[n.UID] = n
		}
		return n
	}

	for _, u := range in.XRs {
		n := baseNode(u, NodeXR, in.Now)
		n.Composition = compositionRef(u)
		n.CompositionRevision = firstNonEmpty(
			nestedString(u, "spec", "crossplane", "compositionRevisionRef", "name"),
			nestedString(u, "spec", "compositionRevisionRef", "name"),
		)
		add(n)
	}
	for _, u := range in.MRs {
		add(baseNode(u, NodeMR, in.Now))
	}
	for _, u := range in.Packages {
		n := baseNode(u, packageNodeType(u.GetKind()), in.Now)
		n.Health = PackageHealth(u)
		add(n)
	}

	// ownerReferences index: child UID sets per parent UID for edge validation.
	childOwners := map[string]map[string]bool{} // childNodeID -> set of owner UIDs
	for _, list := range [][]*unstructured.Unstructured{in.XRs, in.MRs} {
		for _, u := range list {
			id := NodeID(u.GetAPIVersion(), u.GetKind(), u.GetNamespace(), u.GetName())
			owners := map[string]bool{}
			for _, ref := range u.GetOwnerReferences() {
				owners[string(ref.UID)] = true
			}
			childOwners[id] = owners
		}
	}

	// Edges from resourceRefs.
	for _, u := range in.XRs {
		parentID := NodeID(u.GetAPIVersion(), u.GetKind(), u.GetNamespace(), u.GetName())
		parentUID := string(u.GetUID())
		for _, ref := range resourceRefs(u) {
			childID, watched := refNodeID(ref, u.GetNamespace(), lookup)
			if _, exists := s.Nodes[childID]; !exists {
				nodeType := NodeK8s
				if watched {
					// A watched type we can't find in cache: genuinely missing
					// (being deleted, or provider not ready yet).
					nodeType = NodeMissing
				}
				add(&Node{
					ID:         childID,
					APIVersion: ref.APIVersion,
					Kind:       ref.Kind,
					Namespace:  nsFromID(childID),
					Name:       ref.Name,
					NodeType:   nodeType,
					Health:     Health{State: StateNA},
				})
				if nodeType == NodeMissing {
					s.Nodes[childID].Health.State = StateUnknown
				}
			}
			e := &Edge{
				ID:   EdgeID(parentID, childID),
				From: parentID, To: childID,
				Type:      "composes",
				Validated: childOwners[childID][parentUID],
			}
			s.Edges[e.ID] = e
		}
	}

	addCompositionEdges(s, in, add)

	rollup(s)
	return s
}

// addCompositionEdges links each XR to the Composition it resolves to via
// spec.crossplane.compositionRef. Compositions are cluster-scoped, but we
// project a node into each referencing XR's namespace so the relation renders
// inside the namespace-scoped graph. Only Compositions actually referenced by
// a live XR (and present in the informer cache) become nodes; a dangling
// compositionRef surfaces as a "missing" node so the gap is visible.
func addCompositionEdges(s *Snapshot, in Input, add func(*Node) *Node) {
	if len(in.XRs) == 0 {
		return
	}
	// Index cached Compositions by name (cluster-scoped ⇒ name is unique).
	compByName := map[string]*unstructured.Unstructured{}
	for _, u := range in.Compositions {
		if u.GetKind() == "Composition" {
			compByName[u.GetName()] = u
		}
	}

	for _, u := range in.XRs {
		ref := compositionRef(u)
		if ref == nil {
			continue
		}
		parentID := NodeID(u.GetAPIVersion(), u.GetKind(), u.GetNamespace(), u.GetName())
		ns := u.GetNamespace() // project the Composition alongside its XR
		compID := NodeID(ref.APIVersion, ref.Kind, ns, ref.Name)

		if _, exists := s.Nodes[compID]; !exists {
			comp, found := compByName[ref.Name]
			n := &Node{
				ID:         compID,
				APIVersion: ref.APIVersion,
				Kind:       ref.Kind,
				Namespace:  ns,
				Name:       ref.Name,
				NodeType:   NodeComposition,
				Health:     Health{State: StateNA},
			}
			if !found {
				// Referenced but absent from cache: dangling ref.
				n.NodeType = NodeMissing
				n.Health.State = StateUnknown
			} else {
				n.UID = string(comp.GetUID())
				n.CreatedAt = comp.GetCreationTimestamp().Time
			}
			add(n)
		}

		e := &Edge{
			ID:   EdgeID(parentID, compID),
			From: parentID, To: compID,
			Type:      "uses",
			Validated: true, // compositionRef is an authoritative spec field
		}
		s.Edges[e.ID] = e
	}
}

// rollup computes each node's Aggregate as the worst state in its subtree,
// with a visited set so a (never-expected) ref cycle cannot hang the server.
func rollup(s *Snapshot) {
	children := map[string][]string{}
	for _, e := range s.Edges {
		children[e.From] = append(children[e.From], e.To)
	}
	memo := map[string]string{}
	var visit func(id string, path map[string]bool) string
	visit = func(id string, path map[string]bool) string {
		if agg, ok := memo[id]; ok {
			return agg
		}
		n, ok := s.Nodes[id]
		if !ok || path[id] {
			return StateUnknown
		}
		path[id] = true
		agg := n.Health.State
		for _, c := range children[id] {
			agg = worse(agg, visit(c, path))
		}
		delete(path, id)
		memo[id] = agg
		return agg
	}
	// Sort for determinism (map iteration order varies).
	ids := make([]string, 0, len(s.Nodes))
	for id := range s.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s.Nodes[id].Aggregate = visit(id, map[string]bool{})
	}
}

func baseNode(u *unstructured.Unstructured, nodeType string, now time.Time) *Node {
	return &Node{
		ID:           NodeID(u.GetAPIVersion(), u.GetKind(), u.GetNamespace(), u.GetName()),
		UID:          string(u.GetUID()),
		APIVersion:   u.GetAPIVersion(),
		Kind:         u.GetKind(),
		Namespace:    u.GetNamespace(),
		Name:         u.GetName(),
		NodeType:     nodeType,
		Health:       ComputeHealth(u, now),
		ExternalName: u.GetAnnotations()["crossplane.io/external-name"],
		CreatedAt:    u.GetCreationTimestamp().Time,
	}
}

// resourceRefs reads spec.crossplane.resourceRefs (Crossplane v2, verified
// live) falling back to spec.resourceRefs (legacy v1 cluster-scoped XRs).
func resourceRefs(u *unstructured.Unstructured) []Ref {
	raw, found, _ := unstructured.NestedSlice(u.Object, "spec", "crossplane", "resourceRefs")
	if !found {
		raw, found, _ = unstructured.NestedSlice(u.Object, "spec", "resourceRefs")
		if !found {
			return nil
		}
	}
	refs := make([]Ref, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		r := Ref{
			APIVersion: str(m["apiVersion"]),
			Kind:       str(m["kind"]),
			Namespace:  str(m["namespace"]),
			Name:       str(m["name"]),
		}
		if r.Name == "" { // refs may be pre-allocated with no name yet
			continue
		}
		refs = append(refs, r)
	}
	return refs
}

// refNodeID resolves a ref to a node ID, defaulting the namespace: explicit
// ref namespace wins; watched cluster-scoped types get none; everything else
// inherits the parent XR's namespace (v2 refs verified to omit namespace).
func refNodeID(ref Ref, parentNS string, lookup TypeLookup) (id string, watched bool) {
	watched, namespaced := lookup(ref.APIVersion, ref.Kind)
	ns := ref.Namespace
	if ns == "" {
		switch {
		case watched && !namespaced:
			ns = ""
		case !watched && clusterScopedBuiltins[ref.Kind]:
			ns = ""
		default:
			ns = parentNS
		}
	}
	return NodeID(ref.APIVersion, ref.Kind, ns, ref.Name), watched
}

func compositionRef(u *unstructured.Unstructured) *Ref {
	name := firstNonEmpty(
		nestedString(u, "spec", "crossplane", "compositionRef", "name"),
		nestedString(u, "spec", "compositionRef", "name"),
	)
	if name == "" {
		return nil
	}
	return &Ref{Name: name, Kind: "Composition", APIVersion: "apiextensions.crossplane.io/v1"}
}

func packageNodeType(kind string) string {
	switch kind {
	case "Provider":
		return NodeProvider
	case "Function":
		return NodeFunction
	case "Configuration":
		return NodeConfiguration
	default:
		return NodeK8s
	}
}

func nestedString(u *unstructured.Unstructured, fields ...string) string {
	v, _, _ := unstructured.NestedString(u.Object, fields...)
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// nsFromID extracts the namespace segment back out of a node ID.
func nsFromID(id string) string {
	// id = apiVersion|kind|namespace|name
	parts := [4]string{}
	start, idx := 0, 0
	for i := 0; i < len(id) && idx < 3; i++ {
		if id[i] == '|' {
			parts[idx] = id[start:i]
			start = i + 1
			idx++
		}
	}
	if idx == 3 {
		return parts[2]
	}
	return ""
}
