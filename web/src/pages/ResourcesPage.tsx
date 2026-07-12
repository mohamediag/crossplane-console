import { useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useSearchParams } from "react-router-dom";
import { useGraphStore } from "../store/graphStore";
import { HealthBadge } from "../components/graph/HealthBadge";
import { DetailDrawer, useSelectedResource } from "../components/drawer/DetailDrawer";
import { Filters, useFilters } from "../components/list/Filters";
import { formatAge } from "../lib/format";
import type { GraphNode, ResourceListResponse } from "../types/api";

const TYPE_TABS = [
  { key: "", label: "All" },
  { key: "xr", label: "Composites" },
  { key: "mr", label: "Managed" },
  { key: "pkg", label: "Packages" },
];

const PKG_TYPES = new Set(["provider", "function", "configuration"]);

export function ResourcesPage() {
  const nodes = useGraphStore((s) => s.nodes);
  const filters = useFilters();
  const { select } = useSelectedResource();
  const [params, setParams] = useSearchParams();
  const typeTab = params.get("type") ?? "";

  // Rows come straight from the live store — the list stays current with SSE.
  const rows = useMemo(() => {
    const out: GraphNode[] = [];
    for (const n of nodes.values()) {
      if (typeTab === "xr" && n.nodeType !== "xr") continue;
      if (typeTab === "mr" && n.nodeType !== "mr") continue;
      if (typeTab === "pkg" && !PKG_TYPES.has(n.nodeType)) continue;
      if (typeTab === "" && n.nodeType === "missing") continue;
      if (filters.namespace && n.namespace !== filters.namespace) continue;
      if (filters.kind && n.kind !== filters.kind) continue;
      if (filters.health && n.health.state !== filters.health) continue;
      out.push(n);
    }
    return out.sort(
      (a, b) =>
        a.kind.localeCompare(b.kind) ||
        (a.namespace ?? "").localeCompare(b.namespace ?? "") ||
        a.name.localeCompare(b.name),
    );
  }, [nodes, typeTab, filters.namespace, filters.kind, filters.health]);

  // Kind-filtered lists get the CRD's own printer columns (SYNCED, READY,
  // REGION…) from the REST endpoint; rows themselves stay live from the store.
  const columnsQuery = useQuery({
    queryKey: ["resource-columns", filters.kind, filters.namespace, filters.health],
    queryFn: async (): Promise<ResourceListResponse> => {
      const q = new URLSearchParams({ kind: filters.kind, limit: "200" });
      if (filters.namespace) q.set("namespace", filters.namespace);
      if (filters.health) q.set("health", filters.health);
      const res = await fetch(`/api/resources?${q}`);
      if (!res.ok) throw new Error(`${res.status}`);
      return res.json();
    },
    enabled: filters.kind !== "",
    refetchInterval: 5000,
  });
  const printerCols = (filters.kind && columnsQuery.data?.columns) || [];
  const printerValues = (filters.kind && columnsQuery.data?.printerValues) || {};

  const gridTemplate =
    "minmax(140px,1fr) minmax(160px,1.4fr) minmax(140px,1fr) 110px" +
    (printerCols.length > 0
      ? printerCols.map(() => " minmax(90px,0.8fr)").join("")
      : " 150px");

  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 44,
    overscan: 15,
  });

  return (
    <div className="flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex flex-wrap items-center gap-3 border-b border-zinc-200 bg-white px-4 py-2">
          <nav className="flex gap-1">
            {TYPE_TABS.map((t) => (
              <button
                key={t.key}
                onClick={() =>
                  setParams((prev) => {
                    const next = new URLSearchParams(prev);
                    if (t.key) next.set("type", t.key);
                    else next.delete("type");
                    return next;
                  })
                }
                className={`rounded-md px-3 py-1 text-sm ${typeTab === t.key ? "bg-zinc-900 text-white" : "text-zinc-600 hover:bg-zinc-100"}`}
              >
                {t.label}
              </button>
            ))}
          </nav>
          <Filters {...filters} />
          <span className="ml-auto text-xs text-zinc-500">{rows.length} resources</span>
        </div>

        <div
          className="grid gap-2 border-b border-zinc-200 bg-zinc-50 px-4 py-2 text-xs font-semibold uppercase tracking-wide text-zinc-500"
          style={{ gridTemplateColumns: gridTemplate }}
        >
          <span>Kind</span>
          <span>Name</span>
          <span>Namespace</span>
          <span>Health</span>
          {printerCols.length > 0 ? (
            printerCols.map((c) => <span key={c.name}>{c.name}</span>)
          ) : (
            <span>External name</span>
          )}
        </div>

        <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto">
          <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
            {virtualizer.getVirtualItems().map((vi) => {
              const n = rows[vi.index];
              const vals = printerValues[n.id];
              return (
                <button
                  key={n.id}
                  onClick={() => select(n.id)}
                  className="grid w-full items-center gap-2 border-b border-zinc-100 px-4 text-left text-sm hover:bg-blue-50"
                  style={{
                    gridTemplateColumns: gridTemplate,
                    position: "absolute",
                    top: 0,
                    height: vi.size,
                    transform: `translateY(${vi.start}px)`,
                  }}
                >
                  <span className="truncate text-zinc-600">{n.kind}</span>
                  <span className="truncate font-medium text-zinc-900">{n.name}</span>
                  <span className="truncate text-zinc-500">{n.namespace ?? "—"}</span>
                  <HealthBadge state={n.health.state} label />
                  {printerCols.length > 0 ? (
                    printerCols.map((c, ci) => {
                      const v = vals?.[ci]?.value ?? "";
                      return (
                        <span key={c.name} className="truncate font-mono text-xs text-zinc-600">
                          {c.type === "date" && v ? formatAge(v) : v || "—"}
                        </span>
                      );
                    })
                  ) : (
                    <span className="truncate font-mono text-xs text-zinc-500">
                      {n.externalName ?? ""}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </div>
      </div>
      <DetailDrawer />
    </div>
  );
}
