import type { GraphNode } from "../types/api";

export interface SearchResult {
  node: GraphNode;
  score: number;
}

// scoreMatch ranks how well `query` matches `text`:
//   exact match > prefix > word-boundary start > substring > no match (0).
// Case-insensitive; word boundaries are '-', '.', '/' (DNS-ish names).
export function scoreMatch(query: string, text: string): number {
  if (!query || !text) return 0;
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  if (t === q) return 100;
  if (t.startsWith(q)) return 80;
  const idx = t.indexOf(q);
  if (idx < 0) return 0;
  const prev = t[idx - 1];
  if (prev === "-" || prev === "." || prev === "/") return 60;
  return 40;
}

// searchNodes scores every node against the query on name, kind, namespace
// and external-name (name weighted highest), returning the best `limit`
// results sorted by score, then name for stability.
export function searchNodes(
  nodes: Iterable<GraphNode>,
  query: string,
  limit = 50,
): SearchResult[] {
  const trimmed = query.trim();
  if (!trimmed) return [];
  const results: SearchResult[] = [];
  for (const node of nodes) {
    if (node.nodeType === "missing") continue;
    const score = Math.max(
      scoreMatch(trimmed, node.name) * 2,
      scoreMatch(trimmed, node.kind),
      scoreMatch(trimmed, node.namespace ?? ""),
      scoreMatch(trimmed, node.externalName ?? ""),
    );
    if (score > 0) results.push({ node, score });
  }
  results.sort(
    (a, b) => b.score - a.score || a.node.name.localeCompare(b.node.name),
  );
  return results.slice(0, limit);
}
