import { beforeEach, describe, expect, it } from "vitest";
import { useGraphStore } from "./graphStore";
import type { GraphNode } from "../types/api";

const node = (id: string, state = "Healthy"): GraphNode => ({
  id,
  apiVersion: "g/v1",
  kind: "K",
  name: id,
  nodeType: "mr",
  health: { state: state as GraphNode["health"]["state"] },
  aggregate: state as GraphNode["aggregate"],
});

describe("graphStore", () => {
  beforeEach(() => {
    useGraphStore.getState().replaceAll({ revision: 1, generatedAt: "", nodes: [node("a")], edges: [] });
    useGraphStore.getState().clearResync();
  });

  it("applies consecutive deltas", () => {
    useGraphStore.getState().applyDelta({ revision: 2, upserts: [node("b")] });
    const s = useGraphStore.getState();
    expect(s.revision).toBe(2);
    expect(s.nodes.size).toBe(2);
    expect(s.needsResync).toBe(false);
  });

  it("ignores duplicate/old revisions", () => {
    useGraphStore.getState().applyDelta({ revision: 1, upserts: [node("dup")] });
    expect(useGraphStore.getState().nodes.has("dup")).toBe(false);
  });

  it("flags resync on revision gap", () => {
    useGraphStore.getState().applyDelta({ revision: 5, upserts: [node("late")] });
    const s = useGraphStore.getState();
    expect(s.needsResync).toBe(true);
    expect(s.nodes.has("late")).toBe(false);
  });

  it("bumps topologyVersion only on ID-set changes", () => {
    const before = useGraphStore.getState().topologyVersion;
    // Health-only update to existing node "a": no topology change.
    useGraphStore.getState().applyDelta({ revision: 2, upserts: [node("a", "Unhealthy")] });
    expect(useGraphStore.getState().topologyVersion).toBe(before);
    expect(useGraphStore.getState().nodes.get("a")!.health.state).toBe("Unhealthy");
    // New node: topology change.
    useGraphStore.getState().applyDelta({ revision: 3, upserts: [node("b")] });
    expect(useGraphStore.getState().topologyVersion).toBe(before + 1);
  });

  it("removals drop nodes and bump topology", () => {
    const before = useGraphStore.getState().topologyVersion;
    useGraphStore.getState().applyDelta({ revision: 2, removals: ["a"] });
    const s = useGraphStore.getState();
    expect(s.nodes.size).toBe(0);
    expect(s.topologyVersion).toBe(before + 1);
  });
});
