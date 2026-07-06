import dagre from "@dagrejs/dagre";
import type { Edge, Node } from "@xyflow/react";

export const NODE_WIDTH = 240;
export const NODE_HEIGHT = 72;

// Single seam for layout: swap in ELK here if graphs outgrow dagre.
export function layoutDAG<T extends Node>(nodes: T[], edges: Edge[]): T[] {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "TB", ranksep: 70, nodesep: 40 });
  g.setDefaultEdgeLabel(() => ({}));
  for (const n of nodes) g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  for (const e of edges) g.setEdge(e.source, e.target);
  dagre.layout(g);
  return nodes.map((n) => {
    const pos = g.node(n.id);
    return {
      ...n,
      position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 },
    };
  });
}
