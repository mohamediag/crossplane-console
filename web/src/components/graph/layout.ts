import dagre from "@dagrejs/dagre";
import type { Edge, Node } from "@xyflow/react";

export const NODE_WIDTH = 240;
export const NODE_HEIGHT = 72;
export const CARD_WIDTH = 250;
export const CARD_HEIGHT = 112;

function dims(n: Node): { width: number; height: number } {
  if (n.type === "nsCard") return { width: CARD_WIDTH, height: CARD_HEIGHT };
  return { width: NODE_WIDTH, height: NODE_HEIGHT };
}

// layoutDAG lays out one section left-to-right (ArgoCD style: roots on the
// left, children fanning right, siblings stacked vertically) and reports the
// section's bounding box so sections can be stacked. Single seam for layout:
// swap in ELK here if graphs outgrow dagre.
export function layoutDAG<T extends Node>(
  nodes: T[],
  edges: Edge[],
): { nodes: T[]; width: number; height: number } {
  if (nodes.length === 0) return { nodes, width: 0, height: 0 };
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", ranksep: 90, nodesep: 24 });
  g.setDefaultEdgeLabel(() => ({}));
  for (const n of nodes) g.setNode(n.id, dims(n));
  for (const e of edges) g.setEdge(e.source, e.target);
  dagre.layout(g);

  let maxX = 0;
  let maxY = 0;
  const out = nodes.map((n) => {
    const pos = g.node(n.id);
    const d = dims(n);
    const x = pos.x - d.width / 2;
    const y = pos.y - d.height / 2;
    maxX = Math.max(maxX, x + d.width);
    maxY = Math.max(maxY, y + d.height);
    return { ...n, position: { x, y } };
  });
  return { nodes: out, width: maxX, height: maxY };
}
