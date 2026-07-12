import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { HealthState } from "../../types/api";
import { HealthBadge } from "./HealthBadge";

export interface NamespaceCardData extends Record<string, unknown> {
  namespace: string;
  xrCount: number;
  mrCount: number;
  otherCount: number;
  aggregate: HealthState;
  expanded: boolean;
}

export type NamespaceCardNode = Node<NamespaceCardData, "nsCard">;

const AGG_BORDER: Record<HealthState, string> = {
  Healthy: "border-emerald-300",
  Progressing: "border-sky-300",
  Unknown: "border-amber-300",
  Unhealthy: "border-red-300",
  NA: "border-zinc-200",
};

// NamespaceCard is the ArgoCD-style tile: one square per namespace. Collapsed
// it stands alone in the tile grid; expanded it becomes the root of that
// namespace's left-to-right resource tree.
export function NamespaceCard({ data }: NodeProps<NamespaceCardNode>) {
  const d = data;
  return (
    <div
      className={`h-[112px] w-[250px] rounded-xl border-2 bg-white px-4 py-3 shadow-sm transition-shadow hover:shadow-md ${AGG_BORDER[d.aggregate]} ${d.expanded ? "ring-2 ring-blue-400" : ""}`}
      title={d.expanded ? "Click to collapse" : "Click to expand"}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white">
          NS
        </span>
        <HealthBadge state={d.aggregate} label />
      </div>
      <div className="mt-1.5 truncate text-sm font-semibold text-zinc-900" title={d.namespace}>
        {d.namespace}
      </div>
      <div className="mt-1.5 flex gap-3 text-xs text-zinc-500">
        <span>
          <b className="text-zinc-700">{d.xrCount}</b> XR{d.xrCount === 1 ? "" : "s"}
        </span>
        <span>
          <b className="text-zinc-700">{d.mrCount}</b> MR{d.mrCount === 1 ? "" : "s"}
        </span>
        {d.otherCount > 0 && (
          <span>
            <b className="text-zinc-700">{d.otherCount}</b> other
          </span>
        )}
        <span className="ml-auto text-zinc-400">{d.expanded ? "▾" : "▸"}</span>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-zinc-400" />
    </div>
  );
}
