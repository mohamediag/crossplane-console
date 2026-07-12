// Command console runs the Crossplane observability console: a read-only
// REST + SSE API over an informer-backed graph of XRs, MRs and packages,
// serving the embedded SPA when built with -tags embedui.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mohamediag/crossplane-console/internal/api"
	"github.com/mohamediag/crossplane-console/internal/discovery"
	"github.com/mohamediag/crossplane-console/internal/engine"
	"github.com/mohamediag/crossplane-console/internal/events"
	"github.com/mohamediag/crossplane-console/internal/k8s"
	"github.com/mohamediag/crossplane-console/internal/watch"
	"github.com/mohamediag/crossplane-console/web"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
)

var version = "dev" // set via -ldflags at build time

func main() {
	var (
		kubeconfig = flag.String("kubeconfig", os.Getenv("KUBECONFIG_PATH"),
			"path to kubeconfig (default: in-cluster, then ~/.kube/config)")
		listen   = flag.String("listen", ":8080", "listen address")
		logLevel = flag.String("log-level", "info", "log level: debug|info|warn|error")
	)
	flag.Parse()

	log := newLogger(*logLevel)
	if err := run(log, *kubeconfig, *listen); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, kubeconfig, listen string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	clients, err := k8s.NewClients(kubeconfig)
	if err != nil {
		return err
	}
	log.Info("connected", "host", clients.Host, "version", version)

	var manager *watch.Manager
	var eng *engine.Engine

	registry := discovery.New(
		func(info discovery.TypeInfo) {
			manager.Ensure(ctx, info)
			eng.MarkDirty()
		},
		func(gvr schema.GroupVersionResource) {
			manager.Remove(gvr)
			eng.MarkDirty()
		},
	)
	manager = watch.NewManager(clients.Dynamic, func() { eng.MarkDirty() }, log)
	eng = engine.New(manager, registry, log)

	crdInformer := watch.NewCRDInformer(ctx, clients.Dynamic, registry)
	go crdInformer.Run(ctx.Done())
	go eng.Run(ctx)

	server := &api.Server{
		Engine:   eng,
		Manager:  manager,
		Registry: registry,
		Dynamic:  clients.Dynamic,
		Mapper: restmapper.NewDeferredDiscoveryRESTMapper(
			memory.NewMemCacheClient(clients.Typed.Discovery())),
		Log:      log,
		Version:  version,
		StaticFS: web.FS(),
	}

	eventStore := events.NewStore(ctx, clients.Dynamic, server.BroadcastEvent)
	server.Events = eventStore
	go eventStore.Run(ctx)

	handler := api.NewServer(server, crdInformer.HasSynced)

	// Non-blocking startup report: partial data is fine, silence is not.
	go func() {
		// The CRD informer must sync first: it is what discovers Crossplane
		// types and registers the per-type informers that WaitForSync waits on.
		// Without this, WaitForSync sees an empty manager and returns true
		// immediately, reporting "types: 0" before discovery has even run.
		if !cache.WaitForCacheSync(ctx.Done(), crdInformer.HasSynced) {
			return // context cancelled during startup
		}
		if manager.WaitForSync(ctx, 30*time.Second) {
			log.Info("all informers synced", "types", len(registry.Types()))
		} else {
			log.Warn("some informers not yet synced after 30s (provider mid-install?)")
		}
		if !registry.CrossplaneDetected() {
			log.Warn("no Crossplane CRDs detected — is Crossplane installed?")
		}
	}()

	httpServer := &http.Server{Addr: listen, Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "addr", listen)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
