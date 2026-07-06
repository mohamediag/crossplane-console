import type { GraphResponse, K8sEvent, Meta, ResourceDetail } from "../types/api";

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `${res.status} ${res.statusText}`);
  }
  return res.json();
}

export const fetchGraph = () => getJSON<GraphResponse>("/api/graph");

export const fetchMeta = () => getJSON<Meta>("/api/meta");

export const fetchDetail = (id: string) =>
  getJSON<ResourceDetail>(`/api/resource?id=${encodeURIComponent(id)}`);

export const fetchEvents = (params: {
  involvedUid?: string;
  namespace?: string;
  kind?: string;
  name?: string;
  type?: string;
  limit?: number;
}) => {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
  }
  return getJSON<{ items: K8sEvent[] }>(`/api/events?${q}`);
};
