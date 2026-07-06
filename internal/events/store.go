// Package events caches core/v1 Events via an informer and serves them by
// involvedObject. An informer (not on-demand listing) because the events tab
// needs recency-sorted live updates and feeds the SSE stream; the cluster's
// own event TTL (default 1h) bounds cache size, and deletes propagate.
package events

import (
	"context"
	"sort"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

// Event is a normalized core/v1 Event.
type Event struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Count     int64  `json:"count"`
	FirstSeen string `json:"firstSeen,omitempty"`
	LastSeen  string `json:"lastSeen,omitempty"`
	Source    string `json:"source,omitempty"`
	Involved  struct {
		APIVersion string `json:"apiVersion,omitempty"`
		Kind       string `json:"kind,omitempty"`
		Namespace  string `json:"namespace,omitempty"`
		Name       string `json:"name,omitempty"`
		UID        string `json:"uid,omitempty"`
	} `json:"involvedObject"`
}

// Store watches core/v1 Events cluster-wide.
type Store struct {
	informer cache.SharedIndexInformer
	onEvent  func(Event) // live feed for SSE; may be nil

	mu sync.RWMutex
}

var eventsGVR = schema.GroupVersionResource{Version: "v1", Resource: "events"}

// NewStore builds the events informer. onEvent fires for adds/updates once
// the cache has synced (no replay storm on startup).
func NewStore(ctx context.Context, client dynamic.Interface, onEvent func(Event)) *Store {
	s := &Store{onEvent: onEvent}
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.Resource(eventsGVR).Namespace(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.Resource(eventsGVR).Namespace(metav1.NamespaceAll).Watch(ctx, options)
		},
	}
	inf := cache.NewSharedIndexInformer(lw, &unstructured.Unstructured{}, 0, cache.Indexers{})
	_ = inf.SetTransform(stripEvent)
	_, _ = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if s.onEvent != nil && inf.HasSynced() {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					s.onEvent(Normalize(u))
				}
			}
		},
		UpdateFunc: func(_, obj interface{}) {
			if s.onEvent != nil {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					s.onEvent(Normalize(u))
				}
			}
		},
	})
	s.informer = inf
	return s
}

// Run blocks running the informer until ctx is cancelled.
func (s *Store) Run(ctx context.Context) { s.informer.Run(ctx.Done()) }

// HasSynced reports initial cache sync.
func (s *Store) HasSynced() bool { return s.informer.HasSynced() }

// Query filters cached events. Any empty argument matches everything.
// uid takes precedence over coordinate matching. Sorted by lastSeen desc.
func (s *Store) Query(uid, namespace, kind, name, eventType string, limit int) []Event {
	var out []Event
	for _, obj := range s.informer.GetStore().List() {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		e := Normalize(u)
		if uid != "" && e.Involved.UID != uid {
			continue
		}
		if uid == "" {
			if namespace != "" && e.Involved.Namespace != namespace {
				continue
			}
			if kind != "" && e.Involved.Kind != kind {
				continue
			}
			if name != "" && e.Involved.Name != name {
				continue
			}
		}
		if eventType != "" && e.Type != eventType {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Normalize converts an unstructured core/v1 Event.
func Normalize(u *unstructured.Unstructured) Event {
	var e Event
	get := func(fields ...string) string {
		v, _, _ := unstructured.NestedString(u.Object, fields...)
		return v
	}
	e.Type = get("type")
	e.Reason = get("reason")
	e.Message = get("message")
	if c, found, _ := unstructured.NestedInt64(u.Object, "count"); found {
		e.Count = c
	}
	e.FirstSeen = get("firstTimestamp")
	e.LastSeen = get("lastTimestamp")
	if e.LastSeen == "" {
		e.LastSeen = get("eventTime") // events.k8s.io-style singletons
	}
	e.Source = get("source", "component")
	if e.Source == "" {
		e.Source = get("reportingComponent")
	}
	e.Involved.APIVersion = get("involvedObject", "apiVersion")
	e.Involved.Kind = get("involvedObject", "kind")
	e.Involved.Namespace = get("involvedObject", "namespace")
	e.Involved.Name = get("involvedObject", "name")
	e.Involved.UID = get("involvedObject", "uid")
	return e
}

// stripEvent keeps events lean in cache.
func stripEvent(obj interface{}) (interface{}, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}
	u.SetManagedFields(nil)
	return u, nil
}
