// Mirrors the Go JSON types in internal/graph and internal/api.

export type HealthState = "Healthy" | "Progressing" | "Unknown" | "Unhealthy" | "NA";

export type NodeType =
  | "xr"
  | "mr"
  | "k8s"
  | "provider"
  | "function"
  | "configuration"
  | "missing";

export interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

export interface Health {
  state: HealthState;
  ready?: Condition;
  synced?: Condition;
}

export interface Ref {
  apiVersion?: string;
  kind?: string;
  namespace?: string;
  name: string;
}

export interface GraphNode {
  id: string;
  uid?: string;
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
  nodeType: NodeType;
  health: Health;
  aggregate: HealthState;
  externalName?: string;
  compositionRef?: Ref;
  compositionRevision?: string;
  createdAt?: string;
}

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  type: string;
  validated: boolean;
}

export interface GraphResponse {
  revision: number;
  generatedAt: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface Delta {
  revision: number;
  upserts?: GraphNode[];
  removals?: string[];
  edgesAdded?: GraphEdge[];
  edgesRemoved?: string[];
}

export interface TypeStatus {
  gvr: string;
  kind: string;
  category: string;
  scope: string;
  synced: boolean;
  count: number;
}

export interface Meta {
  version: string;
  crossplaneDetected: boolean;
  revision: number;
  types: TypeStatus[];
  namespaces: string[];
  kinds: string[];
  typeCounts: { xr: number; mr: number; pkg: number };
}

export interface K8sEvent {
  type: string;
  reason: string;
  message: string;
  count: number;
  firstSeen?: string;
  lastSeen?: string;
  source?: string;
  involvedObject: {
    apiVersion?: string;
    kind?: string;
    namespace?: string;
    name?: string;
    uid?: string;
  };
}

export interface ResourceDetail {
  node: GraphNode | null;
  conditions: Condition[];
  yaml: string;
  owners: Ref[];
  children: GraphNode[];
}
