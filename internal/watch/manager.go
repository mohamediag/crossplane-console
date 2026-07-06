// Package watch manages one dynamic informer per discovered type. A shared
// informer factory can't stop individual informers, and Crossplane types come
// and go with providers and XRDs, so each GVR gets its own lifecycle.
package watch

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/mohamediag/crossplane-console/internal/discovery"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

// TypeStatus is per-type informer state, surfaced via /api/meta so the UI can
// show "provider installing" instead of a silently incomplete graph.
type TypeStatus struct {
	GVR      string `json:"gvr"`
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Scope    string `json:"scope"`
	Synced   bool   `json:"synced"`
	Count    int    `json:"count"`
}

type handle struct {
	info     discovery.TypeInfo
	informer cache.SharedIndexInformer
	cancel   context.CancelFunc
}

// Manager owns the per-GVR informers.
type Manager struct {
	client   dynamic.Interface
	onChange func()
	log      *slog.Logger

	mu      sync.RWMutex
	handles map[schema.GroupVersionResource]*handle
}

func NewManager(client dynamic.Interface, onChange func(), log *slog.Logger) *Manager {
	return &Manager{
		client:   client,
		onChange: onChange,
		log:      log,
		handles:  map[schema.GroupVersionResource]*handle{},
	}
}

// Ensure starts an informer for the type if one isn't already running.
func (m *Manager) Ensure(ctx context.Context, info discovery.TypeInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handles[info.GVR]; ok {
		return
	}
	ictx, cancel := context.WithCancel(ctx)
	inf := m.newInformer(ictx, info.GVR)
	m.handles[info.GVR] = &handle{info: info, informer: inf, cancel: cancel}
	m.log.Info("starting informer", "gvr", info.GVR.String(), "category", string(info.Category))
	go inf.Run(ictx.Done())
}

// Remove stops and forgets the informer for a deleted type. The next graph
// rebuild naturally drops its objects since the store is gone.
func (m *Manager) Remove(gvr schema.GroupVersionResource) {
	m.mu.Lock()
	h, ok := m.handles[gvr]
	if ok {
		delete(m.handles, gvr)
	}
	m.mu.Unlock()
	if ok {
		m.log.Info("stopping informer", "gvr", gvr.String())
		h.cancel()
		if m.onChange != nil {
			m.onChange()
		}
	}
}

// ListByCategory returns all cached objects of every type in the category.
func (m *Manager) ListByCategory(cat discovery.Category) []*unstructured.Unstructured {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*unstructured.Unstructured
	for _, h := range m.handles {
		if h.info.Category != cat {
			continue
		}
		for _, obj := range h.informer.GetStore().List() {
			if u, ok := obj.(*unstructured.Unstructured); ok {
				out = append(out, u)
			}
		}
	}
	return out
}

// GetByCoordinates finds a cached object by apiVersion/kind/namespace/name
// across all informers. Returns nil if not cached.
func (m *Manager) GetByCoordinates(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.handles {
		gv := h.info.GVR.GroupVersion().String()
		if h.info.Kind != kind {
			continue
		}
		// Match on group+kind; the ref may use a different served version.
		if gv != apiVersion && h.info.GVR.Group != groupOf(apiVersion) {
			continue
		}
		key := name
		if namespace != "" {
			key = namespace + "/" + name
		}
		obj, exists, err := h.informer.GetStore().GetByKey(key)
		if err != nil || !exists {
			continue
		}
		if u, ok := obj.(*unstructured.Unstructured); ok {
			return u
		}
	}
	return nil
}

// Status reports per-type sync state, sorted by GVR for stable output.
func (m *Manager) Status() []TypeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]TypeStatus, 0, len(m.handles))
	for gvr, h := range m.handles {
		scope := "Namespaced"
		if !h.info.Namespaced {
			scope = "Cluster"
		}
		out = append(out, TypeStatus{
			GVR:      gvr.String(),
			Kind:     h.info.Kind,
			Category: string(h.info.Category),
			Scope:    scope,
			Synced:   h.informer.HasSynced(),
			Count:    len(h.informer.GetStore().ListKeys()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GVR < out[j].GVR })
	return out
}

// WaitForSync blocks until all current informers have synced or the timeout
// elapses; it returns false on timeout. Used only to make startup logs and
// /readyz meaningful — the console serves partial data happily.
func (m *Manager) WaitForSync(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return false
		}
		all := true
		for _, st := range m.Status() {
			if !st.Synced {
				all = false
				break
			}
		}
		if all {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) newInformer(ctx context.Context, gvr schema.GroupVersionResource) cache.SharedIndexInformer {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return m.client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return m.client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
		},
	}
	inf := cache.NewSharedIndexInformer(lw, &unstructured.Unstructured{}, 0, cache.Indexers{})
	_ = inf.SetTransform(StripTransform)
	_, _ = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { m.changed() },
		UpdateFunc: func(any, any) { m.changed() },
		DeleteFunc: func(any) { m.changed() },
	})
	return inf
}

func (m *Manager) changed() {
	if m.onChange != nil {
		m.onChange()
	}
}

// StripTransform drops managedFields and the last-applied annotation before
// caching — typically 30-50% of object bytes and never rendered by the UI.
func StripTransform(obj interface{}) (interface{}, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}
	u.SetManagedFields(nil)
	ann := u.GetAnnotations()
	if _, ok := ann["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		u.SetAnnotations(ann)
	}
	return u, nil
}

func groupOf(apiVersion string) string {
	for i := 0; i < len(apiVersion); i++ {
		if apiVersion[i] == '/' {
			return apiVersion[:i]
		}
	}
	return ""
}
