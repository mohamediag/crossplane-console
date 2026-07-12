import { describe, expect, it } from "vitest";
import { scoreMatch, searchNodes } from "./search";
import type { GraphNode } from "../types/api";

const node = (name: string, over: Partial<GraphNode> = {}): GraphNode => ({
  id: `g/v1|K|ns|${name}`,
  apiVersion: "g/v1",
  kind: "Object",
  namespace: "ns",
  name,
  nodeType: "mr",
  health: { state: "Healthy" },
  aggregate: "Healthy",
  ...over,
});

describe("scoreMatch", () => {
  it("ranks exact > prefix > word-boundary > substring > miss", () => {
    expect(scoreMatch("api", "api")).toBe(100);
    expect(scoreMatch("api", "api-server")).toBe(80);
    expect(scoreMatch("server", "api-server")).toBe(60);
    expect(scoreMatch("erv", "api-server")).toBe(40);
    expect(scoreMatch("xyz", "api-server")).toBe(0);
  });

  it("is case-insensitive", () => {
    expect(scoreMatch("APP", "app-kubernetes")).toBe(80);
  });

  it("handles empty inputs", () => {
    expect(scoreMatch("", "abc")).toBe(0);
    expect(scoreMatch("abc", "")).toBe(0);
  });
});

describe("searchNodes", () => {
  const nodes = [
    node("sample-service"),
    node("sample-service-deployment", { externalName: "sample-service-deployment" }),
    node("other-thing"),
    node("ghost", { nodeType: "missing" }),
    node("svc-in-sample-ns", { namespace: "sample-application-dev" }),
  ];

  it("matches on name with highest weight and sorts by score", () => {
    const results = searchNodes(nodes, "sample");
    expect(results.length).toBeGreaterThanOrEqual(3);
    expect(results[0].node.name).toBe("sample-service");
  });

  it("matches on kind and namespace too", () => {
    expect(searchNodes(nodes, "Object").length).toBe(4); // all non-missing
    expect(
      searchNodes(nodes, "sample-application-dev").some(
        (r) => r.node.name === "svc-in-sample-ns",
      ),
    ).toBe(true);
  });

  it("excludes missing placeholder nodes", () => {
    expect(searchNodes(nodes, "ghost").length).toBe(0);
  });

  it("returns empty for blank query and respects the limit", () => {
    expect(searchNodes(nodes, "  ").length).toBe(0);
    expect(searchNodes(nodes, "sample", 1).length).toBe(1);
  });
});
