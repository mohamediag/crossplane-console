package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var metaGetOptions = metav1.GetOptions{}

const heartbeatInterval = 15 * time.Second

// GET /api/stream — Server-Sent Events.
//
// Protocol:
//
//	event: snapshot   (once on connect; same JSON shape as /api/graph)
//	event: delta      ({revision, upserts, removals, edgesAdded, edgesRemoved})
//	event: k8sevent   (normalized Kubernetes event)
//	: ping            (comment heartbeat every 15s; keeps Envoy/Gateway happy)
//
// Deltas carry consecutive revisions; a client seeing a gap refetches
// /api/graph. Debouncing happens upstream: the engine's 500ms rebuild tick
// coalesces informer bursts into at most two deltas per second.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Subscribe BEFORE snapshotting so no delta between snapshot and
	// subscription is lost (duplicates are fine, gaps are not).
	deltaCh, cancelDeltas := s.Engine.Subscribe()
	defer cancelDeltas()
	eventCh, cancelEvents := s.eventHub.subscribe()
	defer cancelEvents()

	send := func(event string, v any) bool {
		data, err := json.Marshal(v)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send("snapshot", snapshotResponse(s.Engine.Snapshot(), nil)) {
		return
	}

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case delta, ok := <-deltaCh:
			if !ok { // dropped as a slow client; force reconnect
				return
			}
			if !send("delta", delta) {
				return
			}
		case ev := <-eventCh:
			if !send("k8sevent", ev) {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
