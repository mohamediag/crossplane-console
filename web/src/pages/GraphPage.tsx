import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type NodeTypes,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useGraphStore } from "../store/graphStore";
import { ResourceNode, type FlowNode } from "../components/graph/ResourceNode";
import {
  NamespaceCard,
  type NamespaceCardData,
  type NamespaceCardNode,
} from "../components/graph/NamespaceCard";
import { layoutDAG, CARD_WIDTH, CARD_HEIGHT } from "../components/graph/layout";
import { DetailDrawer, useSelectedResource } from "../components/drawer/DetailDrawer";
import { Filters, useFilters } from "../components/list/Filters";
import type { GraphNode, HealthState } from "../types/api";

type AppNode = FlowNode | NamespaceCardNode;

const nodeTypes: NodeTypes = { resource: ResourceNode, nsCard: NamespaceCard };

const PKG_TYPES = new Set(["provider", "function", "configuration"]);
const CLUSTER_NS = "(cluster-scoped)";

const SEVERITY: Record<HealthState, number> = {
  Unhealthy: 4,
  Unknown: 3,
  Progressing: 2,
  Healthy: 1,
  NA: 0,
};

const GRID_COLS = 4;
const GRID_GAP = 24;
const SECTION_GAP = 64;

// ArgoCD-style graph: one tile per namespace; clicking a tile expands it into
// that namespace's left-to-right resource tree (tile → root XRs → children).
export function GraphPage() {
  const nodes = useGraphStore((s) => s.nodes);
  const edges = useGraphStore((s) => s.edges);
  const { selected, select } = useSelectedResource();
  const filters = useFilters();
  const [showPackages, setShowPackages] = useState(false);
  const [params, setParams] = useSearchParams();
  const flowRef = useRef<ReactFlowInstance<AppNode, Edge> | null>(null);

  const expanded = useMemo(
    () => new Set((params.get("expanded") ?? "").split(",").filter(Boolean)),
    [params],
  );
  const toggleNamespace = (ns: string) => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        const set = new Set((next.get("expanded") ?? "").split(",").filter(Boolean));
        if (set.has(ns)) set.delete(ns);
        else set.add(ns);
        if (set.size > 0) next.set("expanded", [...set].join(","));
        else next.delete("expanded");
        return next;
      },
      { replace: true },
    );
  };

  // Deep links (search palette, drawer child links) auto-expand the
  // selected node's namespace so the node is actually visible.
  useEffect(() => {
    if (!selected) return;
    const node = nodes.get(selected);
    if (!node) return;
    const ns = node.namespace || CLUSTER_NS;
    if (!expanded.has(ns)) toggleNamespace(ns);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, nodes]);

  // Group filtered nodes by namespace; cluster-scoped objects live under a
  // pseudo-group, packages only when toggled on.
  const groups = useMemo(() => {
    const byNS = new Map<string, GraphNode[]>();
    for (const n of nodes.values()) {
      if (PKG_TYPES.has(n.nodeType) && !showPackages) continue;
      if (filters.kind && n.kind !== filters.kind) continue;
      if (filters.health && n.aggregate !== filters.health) continue;
      const ns = n.namespace || CLUSTER_NS;
      if (filters.namespace && ns !== filters.namespace) continue;
      byNS.set(ns, [...(byNS.get(ns) ?? []), n]);
    }
    return new Map([...byNS.entries()].sort((a, b) => a[0].localeCompare(b[0])));
  }, [nodes, filters.namespace, filters.kind, filters.health, showPackages]);

  const expandedKey = [...expanded].sort().join(",");

  // Stable identity for group MEMBERSHIP (namespace → node IDs). `groups`
  // itself is a fresh Map on every SSE delta; keying the layout effect on
  // membership keeps health-only updates from re-running dagre + fitView.
  const membershipKey = useMemo(() => {
    const parts: string[] = [];
    for (const [ns, members] of groups) {
      parts.push(`${ns}:${members.map((m) => m.id).sort().join(",")}`);
    }
    return parts.join("|");
  }, [groups]);

  // Layout: collapsed tiles in a grid on top; each expanded namespace becomes
  // its own LR dagre section stacked below. Re-runs only on topology, filter
  // or expand changes — never on health-only deltas.
  const [laidOut, setLaidOut] = useState<{ nodes: AppNode[]; edges: Edge[] }>({
    nodes: [],
    edges: [],
  });
  useEffect(() => {
    const outNodes: AppNode[] = [];
    const outEdges: Edge[] = [];

    const collapsed = [...groups.keys()].filter((ns) => !expanded.has(ns));
    collapsed.forEach((ns, i) => {
      outNodes.push({
        id: `ns:${ns}`,
        type: "nsCard",
        position: {
          x: (i % GRID_COLS) * (CARD_WIDTH + GRID_GAP),
          y: Math.floor(i / GRID_COLS) * (CARD_HEIGHT + GRID_GAP),
        },
        data: buildCardData(ns, groups.get(ns) ?? [], false),
      });
    });
    let offsetY =
      collapsed.length > 0
        ? Math.ceil(collapsed.length / GRID_COLS) * (CARD_HEIGHT + GRID_GAP) + SECTION_GAP
        : 0;

    for (const ns of groups.keys()) {
      if (!expanded.has(ns)) continue;
      const members = groups.get(ns) ?? [];
      const memberIDs = new Set(members.map((n) => n.id));
      const sectionNodes: AppNode[] = [
        {
          id: `ns:${ns}`,
          type: "nsCard",
          position: { x: 0, y: 0 },
          data: buildCardData(ns, members, true),
        },
        ...members.map(
          (n): FlowNode => ({
            id: n.id,
            type: "resource",
            position: { x: 0, y: 0 },
            data: { resource: n },
          }),
        ),
      ];
      const sectionEdges: Edge[] = [];
      const hasParent = new Set<string>();
      for (const e of edges.values()) {
        if (memberIDs.has(e.from) && memberIDs.has(e.to)) {
          hasParent.add(e.to);
          if (e.type === "uses") {
            // XR→Composition: authoritative spec relation, distinct amber styling.
            sectionEdges.push({
              id: e.id,
              source: e.from,
              target: e.to,
              style: { stroke: "#f59e0b", strokeDasharray: "1 3" },
            });
          } else {
            sectionEdges.push({
              id: e.id,
              source: e.from,
              target: e.to,
              animated: !e.validated,
              style: e.validated ? undefined : { strokeDasharray: "6 3" },
            });
          }
        }
      }
      for (const n of members) {
        if (!hasParent.has(n.id)) {
          sectionEdges.push({
            id: `ns-edge:${ns}:${n.id}`,
            source: `ns:${ns}`,
            target: n.id,
            style: { strokeDasharray: "2 4", stroke: "#d4d4d8" },
          });
        }
      }
      const section = layoutDAG(sectionNodes, sectionEdges);
      for (const n of section.nodes) {
        outNodes.push({ ...n, position: { x: n.position.x, y: n.position.y + offsetY } });
      }
      outEdges.push(...sectionEdges);
      offsetY += section.height + SECTION_GAP;
    }

    setLaidOut({ nodes: outNodes, edges: outEdges });
    requestAnimationFrame(() => flowRef.current?.fitView({ padding: 0.15, duration: 300 }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [membershipKey, expandedKey]);

  // Health/data updates flow into the laid-out nodes without moving them.
  const flowNodes = useMemo(
    () =>
      laidOut.nodes
        .map((fn): AppNode | null => {
          if (fn.type === "nsCard") {
            const ns = fn.id.slice(3);
            const members = groups.get(ns);
            return members
              ? { ...(fn as NamespaceCardNode), data: buildCardData(ns, members, expanded.has(ns)) }
              : null;
          }
          const live = nodes.get(fn.id);
          return live
            ? { ...(fn as FlowNode), data: { resource: live }, selected: fn.id === selected }
            : null;
        })
        .filter((n): n is AppNode => n !== null),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [laidOut, nodes, groups, selected, expandedKey],
  );

  return (
    <div className="flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-3 border-b border-zinc-200 bg-white px-4 py-2">
          <Filters {...filters} />
          {expanded.size > 0 && (
            <button
              onClick={() =>
                setParams(
                  (prev) => {
                    const next = new URLSearchParams(prev);
                    next.delete("expanded");
                    return next;
                  },
                  { replace: true },
                )
              }
              className="rounded-md border border-zinc-200 px-2.5 py-1 text-xs text-zinc-500 hover:bg-zinc-50"
            >
              Collapse all
            </button>
          )}
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
          <ReactFlow<AppNode, Edge>
            nodes={flowNodes}
            edges={laidOut.edges}
            nodeTypes={nodeTypes}
            onInit={(instance) => {
              flowRef.current = instance;
            }}
            onNodeClick={(_, node) => {
              if (node.type === "nsCard") toggleNamespace(node.id.slice(3));
              else select(node.id);
            }}
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

function buildCardData(
  ns: string,
  members: GraphNode[],
  isExpanded: boolean,
): NamespaceCardData {
  let xr = 0;
  let mr = 0;
  let other = 0;
  let agg: HealthState = "NA";
  for (const n of members) {
    if (n.nodeType === "xr") xr++;
    else if (n.nodeType === "mr") mr++;
    else other++;
    if (SEVERITY[n.aggregate] > SEVERITY[agg]) agg = n.aggregate;
  }
  return {
    namespace: ns,
    xrCount: xr,
    mrCount: mr,
    otherCount: other,
    aggregate: agg,
    expanded: isExpanded,
  };
}
