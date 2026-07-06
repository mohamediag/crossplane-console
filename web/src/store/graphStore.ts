import { create } from "zustand";
import type { Delta, GraphEdge, GraphNode, GraphResponse, K8sEvent } from "../types/api";

const EVENT_BUFFER_CAP = 500;

interface GraphState {
  nodes: Map<string, GraphNode>;
  edges: Map<string, GraphEdge>;
  revision: number;
  connected: boolean;
  /** Bumped only when the node/edge ID SET changes — layout re-runs on this,
   *  so health-only updates never make the graph jump. */
  topologyVersion: number;
  /** True when a delta revision gap was detected; the stream effect refetches. */
  needsResync: boolean;
  liveEvents: K8sEvent[];

  replaceAll: (snap: GraphResponse) => void;
  applyDelta: (delta: Delta) => void;
  setConnected: (connected: boolean) => void;
  pushEvent: (event: K8sEvent) => void;
  clearResync: () => void;
}

export const useGraphStore = create<GraphState>((set, get) => ({
  nodes: new Map(),
  edges: new Map(),
  revision: 0,
  connected: false,
  topologyVersion: 0,
  needsResync: false,
  liveEvents: [],

  replaceAll: (snap) => {
    const nodes = new Map(snap.nodes.map((n) => [n.id, n]));
    const edges = new Map(snap.edges.map((e) => [e.id, e]));
    set({
      nodes,
      edges,
      revision: snap.revision,
      needsResync: false,
      topologyVersion: get().topologyVersion + 1,
    });
  },

  applyDelta: (delta) => {
    const state = get();
    // Revisions are consecutive; a gap means we missed a delta (dropped as a
    // slow client or server restart) and must resync from a full snapshot.
    if (state.revision !== 0 && delta.revision <= state.revision) return; // dup
    if (state.revision !== 0 && delta.revision !== state.revision + 1) {
      set({ needsResync: true });
      return;
    }
    const nodes = new Map(state.nodes);
    const edges = new Map(state.edges);
    let topologyChanged = false;
    for (const n of delta.upserts ?? []) {
      if (!nodes.has(n.id)) topologyChanged = true;
      nodes.set(n.id, n);
    }
    for (const id of delta.removals ?? []) {
      if (nodes.delete(id)) topologyChanged = true;
    }
    for (const e of delta.edgesAdded ?? []) {
      if (!edges.has(e.id)) topologyChanged = true;
      edges.set(e.id, e);
    }
    for (const id of delta.edgesRemoved ?? []) {
      if (edges.delete(id)) topologyChanged = true;
    }
    set({
      nodes,
      edges,
      revision: delta.revision,
      topologyVersion: topologyChanged ? state.topologyVersion + 1 : state.topologyVersion,
    });
  },

  setConnected: (connected) => set({ connected }),

  pushEvent: (event) =>
    set((s) => ({ liveEvents: [event, ...s.liveEvents].slice(0, EVENT_BUFFER_CAP) })),

  clearResync: () => set({ needsResync: false }),
}));
