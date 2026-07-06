import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { fetchMeta } from "../../api/client";

export interface FilterState {
  namespace: string;
  kind: string;
  health: string;
  set: (key: "namespace" | "kind" | "health", value: string) => void;
}

export function useFilters(): FilterState {
  const [params, setParams] = useSearchParams();
  return {
    namespace: params.get("namespace") ?? "",
    kind: params.get("kind") ?? "",
    health: params.get("health") ?? "",
    set: (key, value) =>
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (value) next.set(key, value);
          else next.delete(key);
          return next;
        },
        { replace: true },
      ),
  };
}

const HEALTH_STATES = ["Healthy", "Progressing", "Unknown", "Unhealthy", "NA"];

export function Filters({ namespace, kind, health, set }: FilterState) {
  const meta = useQuery({ queryKey: ["meta"], queryFn: fetchMeta, refetchInterval: 10000 });
  return (
    <div className="flex items-center gap-2">
      <Select
        value={namespace}
        onChange={(v) => set("namespace", v)}
        placeholder="All namespaces"
        options={meta.data?.namespaces ?? []}
      />
      <Select
        value={kind}
        onChange={(v) => set("kind", v)}
        placeholder="All kinds"
        options={meta.data?.kinds ?? []}
      />
      <Select
        value={health}
        onChange={(v) => set("health", v)}
        placeholder="Any health"
        options={HEALTH_STATES}
      />
    </div>
  );
}

function Select({
  value,
  onChange,
  placeholder,
  options,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  options: string[];
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm text-zinc-700"
    >
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );
}
