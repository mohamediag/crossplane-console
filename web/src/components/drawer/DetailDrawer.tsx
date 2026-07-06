import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { fetchDetail } from "../../api/client";
import { HealthBadge } from "../graph/HealthBadge";
import { ConditionsTab } from "./ConditionsTab";
import { YamlTab } from "./YamlTab";
import { EventsTab } from "./EventsTab";

const TABS = ["Overview", "YAML", "Events"] as const;
type Tab = (typeof TABS)[number];

export function useSelectedResource() {
  const [params, setParams] = useSearchParams();
  const selected = params.get("selected");
  const select = (id: string | null) => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (id) next.set("selected", id);
        else next.delete("selected");
        return next;
      },
      { replace: true },
    );
  };
  return { selected, select };
}

export function DetailDrawer() {
  const { selected, select } = useSelectedResource();
  const [tab, setTab] = useState<Tab>("Overview");

  const detail = useQuery({
    queryKey: ["detail", selected],
    queryFn: () => fetchDetail(selected!),
    enabled: !!selected,
    refetchInterval: 5000,
  });

  if (!selected) return null;
  const [, kind, namespace, name] = selected.split("|");
  const node = detail.data?.node ?? null;

  return (
    <aside className="flex h-full w-[480px] shrink-0 flex-col border-l border-zinc-200 bg-white shadow-xl">
      <header className="border-b border-zinc-200 px-4 py-3">
        <div className="flex items-start justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              {node && <HealthBadge state={node.health.state} label />}
              <h2 className="truncate text-base font-semibold text-zinc-900" title={name}>
                {name}
              </h2>
            </div>
            <p className="mt-0.5 text-xs text-zinc-500">
              {kind}
              {namespace ? ` · ${namespace}` : " · cluster-scoped"}
            </p>
          </div>
          <button
            onClick={() => select(null)}
            className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700"
            aria-label="Close"
          >
            ✕
          </button>
        </div>
        <nav className="mt-3 flex gap-1">
          {TABS.map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`rounded-md px-3 py-1 text-sm ${tab === t ? "bg-zinc-900 text-white" : "text-zinc-600 hover:bg-zinc-100"}`}
            >
              {t}
            </button>
          ))}
        </nav>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {detail.isLoading && <p className="p-4 text-sm text-zinc-500">Loading…</p>}
        {detail.isError && (
          <p className="p-4 text-sm text-red-600">{(detail.error as Error).message}</p>
        )}
        {detail.data && tab === "Overview" && (
          <ConditionsTab detail={detail.data} onNavigate={select} />
        )}
        {detail.data && tab === "YAML" && <YamlTab yaml={detail.data.yaml} />}
        {detail.data && tab === "Events" && (
          <EventsTab
            uid={node?.uid}
            namespace={namespace || undefined}
            kind={kind}
            name={name}
          />
        )}
      </div>
    </aside>
  );
}
