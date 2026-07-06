// Package discovery turns the cluster's CRD stream into a live registry of
// Crossplane types. Every XR and MR type materializes as a CRD, so watching
// CRDs is both the discovery mechanism and the reaction mechanism for types
// appearing and disappearing with providers and XRDs — nothing is hardcoded.
package discovery

import (
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Category classifies a CRD for the console's purposes.
type Category string

const (
	CategoryManaged   Category = "managed"   // spec.names.categories contains "managed"
	CategoryComposite Category = "composite" // spec.names.categories contains "composite"
	CategoryPackage   Category = "package"   // pkg.crossplane.io Provider/Function/Configuration
)

// TypeInfo describes one watchable Crossplane type (storage version).
type TypeInfo struct {
	GVR        schema.GroupVersionResource
	Kind       string
	Namespaced bool
	Category   Category
}

// Registry tracks classified, Established CRDs. Feed it CRD objects via
// HandleCRDUpsert/HandleCRDDelete (wired to a CRD informer by the caller).
type Registry struct {
	mu       sync.RWMutex
	types    map[schema.GroupVersionResource]TypeInfo
	byCRD    map[string]schema.GroupVersionResource // CRD name -> GVR we registered
	xpSeen   bool                                   // any apiextensions.crossplane.io CRD seen
	onAdd    func(TypeInfo)
	onRemove func(schema.GroupVersionResource)
}

// New creates a Registry. onAdd fires when a classified CRD becomes
// Established; onRemove when it is deleted. Both may be nil.
func New(onAdd func(TypeInfo), onRemove func(schema.GroupVersionResource)) *Registry {
	return &Registry{
		types:    map[schema.GroupVersionResource]TypeInfo{},
		byCRD:    map[string]schema.GroupVersionResource{},
		onAdd:    onAdd,
		onRemove: onRemove,
	}
}

// HandleCRDUpsert classifies a CRD and registers it once Established=True.
// Safe to call repeatedly (informer add + update events).
func (r *Registry) HandleCRDUpsert(u *unstructured.Unstructured) {
	group, _, _ := unstructured.NestedString(u.Object, "spec", "group")
	if group == "apiextensions.crossplane.io" {
		r.mu.Lock()
		r.xpSeen = true
		r.mu.Unlock()
	}
	info, ok := Classify(u)
	if !ok || !established(u) {
		return
	}
	r.mu.Lock()
	_, known := r.types[info.GVR]
	if !known {
		r.types[info.GVR] = info
		r.byCRD[u.GetName()] = info.GVR
	}
	r.mu.Unlock()
	if !known && r.onAdd != nil {
		r.onAdd(info)
	}
}

// HandleCRDDelete unregisters a CRD by its metadata.name.
func (r *Registry) HandleCRDDelete(crdName string) {
	r.mu.Lock()
	gvr, ok := r.byCRD[crdName]
	if ok {
		delete(r.byCRD, crdName)
		delete(r.types, gvr)
	}
	r.mu.Unlock()
	if ok && r.onRemove != nil {
		r.onRemove(gvr)
	}
}

// Lookup implements graph.TypeLookup: is (apiVersion, kind) a watched XR/MR
// type, and if so is it namespaced? Matched on group+kind (refs may name a
// served version other than the storage version we registered).
func (r *Registry) Lookup(apiVersion, kind string) (watched bool, namespaced bool) {
	group := apiVersion
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		group = apiVersion[:i]
	} else {
		group = "" // core group
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, info := range r.types {
		if info.GVR.Group == group && info.Kind == kind &&
			(info.Category == CategoryManaged || info.Category == CategoryComposite) {
			return true, info.Namespaced
		}
	}
	return false, true
}

// Types returns a copy of all registered types.
func (r *Registry) Types() []TypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TypeInfo, 0, len(r.types))
	for _, info := range r.types {
		out = append(out, info)
	}
	return out
}

// CrossplaneDetected reports whether any apiextensions.crossplane.io CRD has
// been observed — the graceful-degradation signal for "Crossplane installed".
func (r *Registry) CrossplaneDetected() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.xpSeen
}

// Classify inspects a CRD (as unstructured) and returns the TypeInfo for its
// storage version, or ok=false if the console doesn't track this type.
func Classify(u *unstructured.Unstructured) (TypeInfo, bool) {
	group, _, _ := unstructured.NestedString(u.Object, "spec", "group")
	kind, _, _ := unstructured.NestedString(u.Object, "spec", "names", "kind")
	plural, _, _ := unstructured.NestedString(u.Object, "spec", "names", "plural")
	scope, _, _ := unstructured.NestedString(u.Object, "spec", "scope")
	categories, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "names", "categories")

	var category Category
	switch {
	case contains(categories, "managed"):
		category = CategoryManaged
	case contains(categories, "composite"):
		category = CategoryComposite
	case group == "pkg.crossplane.io" &&
		(kind == "Provider" || kind == "Function" || kind == "Configuration"):
		category = CategoryPackage
	default:
		return TypeInfo{}, false
	}

	version := storageVersion(u)
	if version == "" || plural == "" {
		return TypeInfo{}, false
	}
	return TypeInfo{
		GVR:        schema.GroupVersionResource{Group: group, Version: version, Resource: plural},
		Kind:       kind,
		Namespaced: scope != "Cluster",
		Category:   category,
	}, true
}

func storageVersion(u *unstructured.Unstructured) string {
	versions, _, _ := unstructured.NestedSlice(u.Object, "spec", "versions")
	for _, v := range versions {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if storage, _ := m["storage"].(bool); storage {
			name, _ := m["name"].(string)
			return name
		}
	}
	return ""
}

func established(u *unstructured.Unstructured) bool {
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] == "Established" && m["status"] == "True" {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
