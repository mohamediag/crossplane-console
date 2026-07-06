import type { Delta, GraphResponse, K8sEvent } from "../types/api";

export interface StreamHandlers {
  onSnapshot: (snap: GraphResponse) => void;
  onDelta: (delta: Delta) => void;
  onEvent: (event: K8sEvent) => void;
  onStatus: (connected: boolean) => void;
}

// Opens /api/stream and reconnects with backoff. EventSource auto-reconnects
// on network errors, and every (re)connect replays a full snapshot, so the
// client can never be left permanently stale.
export function openStream(h: StreamHandlers): () => void {
  let es: EventSource | null = null;
  let closed = false;
  let retryMs = 1000;
  let retryTimer: number | undefined;

  const connect = () => {
    if (closed) return;
    es = new EventSource("/api/stream");
    es.addEventListener("open", () => {
      retryMs = 1000;
      h.onStatus(true);
    });
    es.addEventListener("snapshot", (e) => h.onSnapshot(JSON.parse(e.data)));
    es.addEventListener("delta", (e) => h.onDelta(JSON.parse(e.data)));
    es.addEventListener("k8sevent", (e) => h.onEvent(JSON.parse(e.data)));
    es.addEventListener("error", () => {
      h.onStatus(false);
      es?.close();
      retryTimer = window.setTimeout(connect, retryMs);
      retryMs = Math.min(retryMs * 2, 15000);
    });
  };

  connect();
  return () => {
    closed = true;
    window.clearTimeout(retryTimer);
    es?.close();
  };
}
