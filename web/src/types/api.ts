// Mirrors the Go JSON types in internal/graph and internal/api.

export type HealthState = "Healthy" | "Progressing" | "Unknown" | "Unhealthy" | "NA";

export type NodeType =
  | "xr"
  | "mr"
  | "k8s"
  | "provider"
  | "function"
  | "configuration"
  | "composition"
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

export interface PrinterValue {
  name: string;
  type: string;
  value: string;
}

export interface ResourceDetail {
  node: GraphNode | null;
  conditions: Condition[];
  yaml: string;
  owners: Ref[];
  children: GraphNode[];
  printerColumns?: PrinterValue[];
  pipeline?: PipelineStep[];
  providerConfigRef?: Ref;
}

export interface ResourceListResponse {
  total: number;
  items: GraphNode[];
  columns?: { name: string; type: string }[];
  printerValues?: Record<string, PrinterValue[]>;
}

export interface PlatformPackage {
  name: string;
  kind: string;
  package?: string;
  currentRevision?: string;
  health: Health;
}

export interface PipelineStep {
  step: string;
  function: string;
}

export interface PlatformComposition {
  name: string;
  compositeApiVersion?: string;
  compositeKind?: string;
  mode?: string;
  pipeline?: PipelineStep[];
  revisionCount: number;
  latestRevision?: number;
  latestRevisionName?: string;
}

export interface PlatformXRD {
  name: string;
  group: string;
  kind: string;
  scope: string;
  versions?: string[];
  established: boolean;
  compositions?: string[];
}

export interface PlatformProviderConfig {
  name: string;
  kind: string;
  group: string;
  namespace?: string;
  credentialsSource?: string;
  usedBy: number;
}

export interface PlatformOperation {
  name: string;
  kind: string;
  namespace?: string;
  schedule?: string;
  health: Health;
}

export interface PlatformSummary {
  providers: PlatformPackage[];
  functions: PlatformPackage[];
  configurations: PlatformPackage[];
  xrds: PlatformXRD[];
  compositions: PlatformComposition[];
  providerConfigs: PlatformProviderConfig[];
  operations: PlatformOperation[];
}
