// Package api exposes the read-only REST + SSE surface and serves the SPA.
package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mohamediag/crossplane-console/internal/discovery"
	"github.com/mohamediag/crossplane-console/internal/engine"
	"github.com/mohamediag/crossplane-console/internal/events"
	"github.com/mohamediag/crossplane-console/internal/watch"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

// Server wires all backend components behind one mux.
type Server struct {
	Engine   *engine.Engine
	Manager  *watch.Manager
	Registry *discovery.Registry
	Events   *events.Store
	Dynamic  dynamic.Interface
	Mapper   meta.RESTMapper // for on-demand GETs of unwatched kinds; may be nil
	Log      *slog.Logger
	Version  string
	StaticFS fs.FS // built SPA (web/dist); may be nil in dev

	eventHub eventHub
	ready    func() bool
}

// NewServer builds the HTTP handler. ready gates /readyz.
func NewServer(s *Server, ready func() bool) http.Handler {
	s.ready = ready
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if s.ready != nil && !s.ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /api/graph", s.handleGraph)
	mux.HandleFunc("GET /api/resources", s.handleResources)
	mux.HandleFunc("GET /api/resource", s.handleResourceDetail)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/meta", s.handleMeta)
	mux.HandleFunc("GET /api/platform", s.handlePlatform)
	mux.HandleFunc("GET /api/stream", s.handleStream)

	if s.StaticFS != nil {
		mux.Handle("GET /", spaHandler(s.StaticFS))
	}

	return recoverMiddleware(s.Log, logMiddleware(s.Log, gzipMiddleware(mux)))
}

// BroadcastEvent feeds a live Kubernetes event to all SSE clients. Wired to
// the events store's onEvent callback in main.
func (s *Server) BroadcastEvent(e events.Event) { s.eventHub.broadcast(e) }

// ---- middleware ----

func logMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/stream" {
			log.Debug("request", "method", r.Method, "path", r.URL.Path, "took", time.Since(start).String())
		}
	})
}

func recoverMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic serving request", "path", r.URL.Path, "panic", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// gzipMiddleware compresses JSON responses. It must not touch the SSE stream
// (compression buffers writes, which breaks flushing).
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stream" ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		w.Header().Set("Content-Encoding", "gzip")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, w: gz}, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	w io.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) { return g.w.Write(b) }

// ---- SPA serving with index.html fallback ----

func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(dist, path); err != nil {
			// Client-side route: serve the app shell.
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// ---- shared JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---- event hub (Kubernetes events → SSE clients) ----

type eventHub struct {
	mu   sync.Mutex
	subs map[chan events.Event]struct{}
}

func (h *eventHub) subscribe() (chan events.Event, func()) {
	ch := make(chan events.Event, 64)
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[chan events.Event]struct{}{}
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *eventHub) broadcast(e events.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default: // drop for slow clients; events are advisory
		}
	}
}
