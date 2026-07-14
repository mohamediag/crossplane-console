// Package engine ties discovery, watch caches and the graph builder together:
// informer events mark a dirty flag; a 500ms ticker rebuilds the full snapshot
// when dirty and fans the diff out to SSE subscribers. Full rebuilds (not
// incremental updates) are deliberate — at hundreds-of-MRs scale a rebuild is
// sub-10ms and eliminates the whole class of incremental-index bugs. The tick
// interval doubles as SSE debouncing.
package engine

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mohamediag/crossplane-console/internal/discovery"
	"github.com/mohamediag/crossplane-console/internal/graph"
	"github.com/mohamediag/crossplane-console/internal/watch"
)

const rebuildInterval = 500 * time.Millisecond

// Engine owns the current snapshot and its subscribers.
type Engine struct {
	manager  *watch.Manager
	registry *discovery.Registry
	log      *slog.Logger

	dirty    atomic.Bool
	revision atomic.Int64

	mu   sync.RWMutex
	snap *graph.Snapshot

	subMu sync.Mutex
	subs  map[chan *graph.Delta]struct{}
}

func New(manager *watch.Manager, registry *discovery.Registry, log *slog.Logger) *Engine {
	e := &Engine{
		manager:  manager,
		registry: registry,
		log:      log,
		subs:     map[chan *graph.Delta]struct{}{},
	}
	e.snap = &graph.Snapshot{Nodes: map[string]*graph.Node{}, Edges: map[string]*graph.Edge{}}
	return e
}

// MarkDirty schedules a rebuild on the next tick. Cheap; called from every
// informer event handler.
func (e *Engine) MarkDirty() { e.dirty.Store(true) }

// Snapshot returns the current snapshot (immutable once published).
func (e *Engine) Snapshot() *graph.Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.snap
}

// Subscribe registers a delta channel. The returned cancel must be called.
// Channels are buffered; a subscriber that falls behind has its channel
// closed and must resync from a full snapshot (revision gaps are detectable).
func (e *Engine) Subscribe() (<-chan *graph.Delta, func()) {
	ch := make(chan *graph.Delta, 16)
	e.subMu.Lock()
	e.subs[ch] = struct{}{}
	e.subMu.Unlock()
	cancel := func() {
		e.subMu.Lock()
		if _, ok := e.subs[ch]; ok {
			delete(e.subs, ch)
			close(ch)
		}
		e.subMu.Unlock()
	}
	return ch, cancel
}

// Run drives the rebuild loop until ctx is done.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(rebuildInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.dirty.Swap(false) {
				e.rebuild()
			}
		}
	}
}

func (e *Engine) rebuild() {
	start := time.Now()
	next := graph.Build(graph.Input{
		XRs:          e.manager.ListByCategory(discovery.CategoryComposite),
		MRs:          e.manager.ListByCategory(discovery.CategoryManaged),
		Packages:     e.manager.ListByCategory(discovery.CategoryPackage),
		Compositions: e.manager.ListByCategory(discovery.CategoryExtension),
		Lookup:       e.registry.Lookup,
		Now:          time.Now(),
		Revision:     e.revision.Add(1),
	})

	e.mu.Lock()
	prev := e.snap
	e.snap = next
	e.mu.Unlock()

	delta := graph.Diff(prev, next)
	if delta.Empty() {
		return
	}
	e.log.Debug("graph rebuilt",
		"revision", next.Revision, "nodes", len(next.Nodes), "edges", len(next.Edges),
		"upserts", len(delta.Upserts), "removals", len(delta.Removals),
		"took", time.Since(start).String())

	e.subMu.Lock()
	for ch := range e.subs {
		select {
		case ch <- delta:
		default:
			// Slow client: drop it; it reconnects and gets a fresh snapshot.
			delete(e.subs, ch)
			close(ch)
		}
	}
	e.subMu.Unlock()
}
