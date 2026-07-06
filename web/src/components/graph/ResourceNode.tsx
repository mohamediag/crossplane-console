import { Handle, Position, type NodeProps, type Node } from "@xyflow/react";
import type { GraphNode } from "../../types/api";
import { HealthBadge } from "./HealthBadge";

export type FlowNode = Node<{ resource: GraphNode }, "resource">;

const TYPE_LABELS: Record<string, string> = {
  xr: "XR",
  mr: "MR",
  k8s: "K8s",
  provider: "Provider",
  function: "Function",
  configuration: "Config",
  missing: "Missing",
};

const TYPE_STYLES: Record<string, string> = {
  xr: "border-violet-400 bg-violet-50",
  mr: "border-sky-400 bg-white",
  k8s: "border-zinc-300 bg-zinc-50",
  provider: "border-teal-400 bg-teal-50",
  function: "border-teal-300 bg-white",
  configuration: "border-teal-300 bg-white",
  missing: "border-dashed border-red-300 bg-red-50",
};

// Aggregate ring: a healthy-looking node with a sick subtree gets a red ring.
function ringClass(n: GraphNode): string {
  if (n.aggregate === n.health.state) return "";
  return n.aggregate === "Unhealthy" ? "ring-2 ring-red-400" : "ring-2 ring-amber-300";
}

export function ResourceNode({ data, selected }: NodeProps<FlowNode>) {
  const n = data.resource;
  return (
    <div
      className={`w-[240px] rounded-lg border-2 px-3 py-2 shadow-sm ${TYPE_STYLES[n.nodeType] ?? TYPE_STYLES.k8s} ${ringClass(n)} ${selected ? "outline outline-2 outline-blue-500" : ""}`}
    >
      <Handle type="target" position={Position.Top} className="!bg-zinc-400" />
      <div className="flex items-center justify-between gap-2">
        <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white">
          {TYPE_LABELS[n.nodeType] ?? n.nodeType}
        </span>
        <HealthBadge state={n.health.state} />
      </div>
      <div className="mt-1 truncate text-sm font-semibold text-zinc-900" title={n.name}>
        {n.name}
      </div>
      <div className="truncate text-xs text-zinc-500" title={`${n.kind} · ${n.namespace ?? "cluster"}`}>
        {n.kind}
        {n.namespace ? ` · ${n.namespace}` : ""}
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-zinc-400" />
    </div>
  );
}
