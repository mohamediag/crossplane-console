package watch

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

var crdGVR = schema.GroupVersionResource{
	Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
}

// CRDSink receives CRD lifecycle callbacks (implemented by discovery.Registry).
type CRDSink interface {
	HandleCRDUpsert(u *unstructured.Unstructured)
	HandleCRDDelete(crdName string)
}

// NewCRDInformer returns an informer on CustomResourceDefinitions feeding the
// sink. Updates are re-fed as upserts so a CRD that becomes Established after
// creation (the usual case for provider installs) still gets registered.
func NewCRDInformer(ctx context.Context, client dynamic.Interface, sink CRDSink) cache.SharedIndexInformer {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.Resource(crdGVR).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.Resource(crdGVR).Watch(ctx, options)
		},
	}
	inf := cache.NewSharedIndexInformer(lw, &unstructured.Unstructured{}, 0, cache.Indexers{})
	_ = inf.SetTransform(StripTransform)
	upsert := func(obj interface{}) {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			sink.HandleCRDUpsert(u)
		}
	}
	_, _ = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    upsert,
		UpdateFunc: func(_, obj interface{}) { upsert(obj) },
		DeleteFunc: func(obj interface{}) {
			if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tomb.Obj
			}
			if u, ok := obj.(*unstructured.Unstructured); ok {
				sink.HandleCRDDelete(u.GetName())
			}
		},
	})
	return inf
}
