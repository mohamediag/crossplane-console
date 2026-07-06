import { useEffect, useMemo, useState } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useGraphStore } from "../store/graphStore";
import { ResourceNode, type FlowNode } from "../components/graph/ResourceNode";
import { layoutDAG } from "../components/graph/layout";
import { DetailDrawer, useSelectedResource } from "../components/drawer/DetailDrawer";
import { Filters, useFilters } from "../components/list/Filters";
import type { GraphNode } from "../types/api";

const nodeTypes: NodeTypes = { resource: ResourceNode };

const PKG_TYPES = new Set(["provider", "function", "configuration"]);

export function GraphPage() {
  const nodes = useGraphStore((s) => s.nodes);
  const edges = useGraphStore((s) => s.edges);
  const topologyVersion = useGraphStore((s) => s.topologyVersion);
  const { selected, select } = useSelectedResource();
  const filters = useFilters();
  const [showPackages, setShowPackages] = useState(false);

  // Filter nodes: subtree-preserving namespace/kind/health filters on roots.
  const visible = useMemo(() => {
    const keep = new Set<string>();
    const childrenOf = new Map<string, string[]>();
    const hasParent = new Set<string>();
    for (const e of edges.values()) {
      childrenOf.set(e.from, [...(childrenOf.get(e.from) ?? []), e.to]);
      hasParent.add(e.to);
    }
    const mark = (id: string) => {
      if (keep.has(id)) return;
      keep.add(id);
      for (const c of childrenOf.get(id) ?? []) mark(c);
    };
    for (const n of nodes.values()) {
      if (hasParent.has(n.id)) continue;
      if (PKG_TYPES.has(n.nodeType) && !showPackages) continue;
      if (filters.namespace && n.namespace !== filters.namespace) continue;
      if (filters.kind && n.kind !== filters.kind) continue;
      if (filters.health && n.aggregate !== filters.health) continue;
      mark(n.id);
    }
    return keep;
  }, [nodes, edges, filters.namespace, filters.kind, filters.health, showPackages]);

  // Layout re-runs only when topology or filters change — never on
  // health-only deltas, so the graph doesn't jump under the user.
  const [laidOut, setLaidOut] = useState<FlowNode[]>([]);
  useEffect(() => {
    const flowNodes: FlowNode[] = [];
    for (const n of nodes.values()) {
      if (!visible.has(n.id)) continue;
      flowNodes.push({
        id: n.id,
        type: "resource",
        position: { x: 0, y: 0 },
        data: { resource: n },
      });
    }
    const flowEdges = toFlowEdges(edges, visible);
    setLaidOut(layoutDAG(flowNodes, flowEdges));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [topologyVersion, visible]);

  // Health/data updates flow into the laid-out nodes without moving them.
  const flowNodes = useMemo(
    () =>
      laidOut
        .filter((fn) => nodes.has(fn.id))
        .map((fn) => ({
          ...fn,
          data: { resource: nodes.get(fn.id) as GraphNode },
          selected: fn.id === selected,
        })),
    [laidOut, nodes, selected],
  );
  const flowEdges = useMemo(() => toFlowEdges(edges, visible), [edges, visible]);

  return (
    <div className="flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-3 border-b border-zinc-200 bg-white px-4 py-2">
          <Filters {...filters} />
          <label className="ml-auto flex items-center gap-1.5 text-xs text-zinc-600">
            <input
              type="checkbox"
              checked={showPackages}
              onChange={(e) => setShowPackages(e.target.checked)}
            />
            Show packages
          </label>
        </div>
        <div className="min-h-0 flex-1">
          <ReactFlow
            nodes={flowNodes}
            edges={flowEdges}
            nodeTypes={nodeTypes}
            onNodeClick={(_, node) => select(node.id)}
            onPaneClick={() => select(null)}
            fitView
            minZoom={0.1}
            proOptions={{ hideAttribution: true }}
            nodesDraggable
            nodesConnectable={false}
            edgesFocusable={false}
          >
            <Background gap={24} />
            <Controls showInteractive={false} />
            <MiniMap pannable zoomable />
          </ReactFlow>
        </div>
      </div>
      <DetailDrawer />
    </div>
  );
}

function toFlowEdges(edges: Map<string, import("../types/api").GraphEdge>, visible: Set<string>): Edge[] {
  const out: Edge[] = [];
  for (const e of edges.values()) {
    if (!visible.has(e.from) || !visible.has(e.to)) continue;
    out.push({
      id: e.id,
      source: e.from,
      target: e.to,
      animated: !e.validated,
      style: e.validated ? undefined : { strokeDasharray: "6 3" },
    });
  }
  return out;
}
