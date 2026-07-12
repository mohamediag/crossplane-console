import { useEffect } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { openStream } from "./api/sse";
import { fetchGraph, fetchMeta } from "./api/client";
import { useGraphStore } from "./store/graphStore";
import { GraphPage } from "./pages/GraphPage";
import { ResourcesPage } from "./pages/ResourcesPage";
import { PlatformPage } from "./pages/PlatformPage";
import { SearchPalette } from "./components/search/SearchPalette";

export default function App() {
  const connected = useGraphStore((s) => s.connected);
  const needsResync = useGraphStore((s) => s.needsResync);
  const meta = useQuery({ queryKey: ["meta"], queryFn: fetchMeta, refetchInterval: 30000 });

  // One SSE subscription for the whole app.
  useEffect(() => {
    const store = useGraphStore.getState();
    return openStream({
      onSnapshot: store.replaceAll,
      onDelta: store.applyDelta,
      onEvent: store.pushEvent,
      onStatus: store.setConnected,
    });
  }, []);

  // Revision gap → refetch the full graph.
  useEffect(() => {
    if (!needsResync) return;
    fetchGraph().then(useGraphStore.getState().replaceAll).catch(() => {});
  }, [needsResync]);

  return (
    <div className="flex h-screen flex-col bg-zinc-50">
      <header className="flex items-center gap-6 border-b border-zinc-200 bg-white px-4 py-2.5">
        <h1 className="text-sm font-bold tracking-tight text-zinc-900">
          Crossplane <span className="text-violet-600">Console</span>
        </h1>
        <nav className="flex gap-1">
          {[
            { to: "/graph", label: "Graph" },
            { to: "/resources", label: "Resources" },
            { to: "/platform", label: "Platform" },
          ].map((l) => (
            <NavLink
              key={l.to}
              to={l.to}
              className={({ isActive }) =>
                `rounded-md px-3 py-1 text-sm ${isActive ? "bg-zinc-900 text-white" : "text-zinc-600 hover:bg-zinc-100"}`
              }
            >
              {l.label}
            </NavLink>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-3 text-xs">
          <button
            onClick={() =>
              window.dispatchEvent(
                new KeyboardEvent("keydown", { key: "k", metaKey: true }),
              )
            }
            className="flex items-center gap-2 rounded-md border border-zinc-200 px-2.5 py-1 text-zinc-500 hover:bg-zinc-50"
            title="Search (⌘K)"
          >
            <span>Search</span>
            <kbd className="rounded bg-zinc-100 px-1 text-[10px]">⌘K</kbd>
          </button>
          {meta.data && !meta.data.crossplaneDetected && (
            <span className="rounded bg-amber-100 px-2 py-1 font-medium text-amber-800">
              Crossplane not detected in this cluster
            </span>
          )}
          {meta.data?.types.some((t) => !t.synced) && (
            <span className="rounded bg-sky-100 px-2 py-1 font-medium text-sky-800">
              Some types still syncing…
            </span>
          )}
          <span className="flex items-center gap-1.5 text-zinc-500">
            <span
              className={`h-2 w-2 rounded-full ${connected ? "bg-emerald-500" : "bg-red-500"}`}
            />
            {connected ? "live" : "reconnecting"}
          </span>
        </div>
      </header>
      <main className="min-h-0 flex-1">
        <Routes>
          <Route path="/graph" element={<GraphPage />} />
          <Route path="/resources" element={<ResourcesPage />} />
          <Route path="/platform" element={<PlatformPage />} />
          <Route path="*" element={<Navigate to="/graph" replace />} />
        </Routes>
      </main>
      <SearchPalette />
    </div>
  );
}
