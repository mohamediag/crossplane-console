import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchEvents } from "../../api/client";
import { useGraphStore } from "../../store/graphStore";
import type { K8sEvent } from "../../types/api";

export function EventsTab({
  uid,
  namespace,
  kind,
  name,
}: {
  uid?: string;
  namespace?: string;
  kind?: string;
  name?: string;
}) {
  const [typeFilter, setTypeFilter] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(true);

  const query = useQuery({
    queryKey: ["events", uid, namespace, kind, name, typeFilter],
    queryFn: () => fetchEvents({ involvedUid: uid, namespace, kind, name, type: typeFilter }),
    refetchInterval: autoRefresh ? 5000 : false,
  });

  // Merge live SSE events for this object on top of the REST result.
  const liveEvents = useGraphStore((s) => s.liveEvents);
  const merged = useMemo(() => {
    const base = query.data?.items ?? [];
    if (!autoRefresh) return base;
    const matches = liveEvents.filter((e) =>
      uid
        ? e.involvedObject.uid === uid
        : e.involvedObject.kind === kind &&
          e.involvedObject.name === name &&
          (!namespace || e.involvedObject.namespace === namespace),
    );
    const seen = new Set<string>();
    const out: K8sEvent[] = [];
    for (const e of [...matches, ...base]) {
      const key = `${e.reason}|${e.message}|${e.lastSeen}`;
      if (seen.has(key)) continue;
      if (typeFilter && e.type !== typeFilter) continue;
      seen.add(key);
      out.push(e);
    }
    return out.sort((a, b) => (b.lastSeen ?? "").localeCompare(a.lastSeen ?? ""));
  }, [query.data, liveEvents, uid, namespace, kind, name, typeFilter, autoRefresh]);

  return (
    <div className="p-4">
      <div className="mb-3 flex items-center justify-between">
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="rounded border border-zinc-300 px-2 py-1 text-sm"
        >
          <option value="">All types</option>
          <option value="Normal">Normal</option>
          <option value="Warning">Warning</option>
        </select>
        <label className="flex items-center gap-1.5 text-xs text-zinc-600">
          <input
            type="checkbox"
            checked={autoRefresh}
            onChange={(e) => setAutoRefresh(e.target.checked)}
          />
          Auto-refresh
        </label>
      </div>
      {query.isLoading && <p className="text-sm text-zinc-500">Loading…</p>}
      {merged.length === 0 && !query.isLoading && (
        <p className="text-sm text-zinc-500">No events for this object.</p>
      )}
      <ul className="space-y-2">
        {merged.map((e, i) => (
          <li
            key={i}
            className={`rounded-md border p-2.5 text-sm ${e.type === "Warning" ? "border-amber-300 bg-amber-50" : "border-zinc-200"}`}
          >
            <div className="flex items-center justify-between">
              <span className="font-medium text-zinc-900">{e.reason}</span>
              <span className="text-xs text-zinc-500">
                {e.count > 1 && `×${e.count} · `}
                {e.lastSeen && new Date(e.lastSeen).toLocaleString()}
              </span>
            </div>
            <p className="mt-1 text-xs text-zinc-700">{e.message}</p>
            {e.source && <p className="mt-1 text-[11px] text-zinc-400">{e.source}</p>}
          </li>
        ))}
      </ul>
    </div>
  );
}
