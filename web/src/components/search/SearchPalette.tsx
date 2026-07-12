import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useGraphStore } from "../../store/graphStore";
import { searchNodes } from "../../lib/search";
import { HealthBadge } from "../graph/HealthBadge";

// Global command palette: ⌘K / Ctrl+K (or "/" outside inputs) → fuzzy search
// over every cached resource; Enter jumps to the node in the graph.
export function SearchPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const nodes = useGraphStore((s) => s.nodes);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const inField =
        e.target instanceof HTMLElement &&
        (e.target.tagName === "INPUT" ||
          e.target.tagName === "TEXTAREA" ||
          e.target.isContentEditable);
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      } else if (e.key === "/" && !inField) {
        e.preventDefault();
        setOpen(true);
      } else if (e.key === "Escape") {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      // Focus after the modal renders.
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  const results = useMemo(
    () => searchNodes(nodes.values(), query, 50),
    [nodes, query],
  );

  const go = (id: string) => {
    setOpen(false);
    // Clear graph filters so the selected node's subtree is visible.
    navigate(`/graph?selected=${encodeURIComponent(id)}`);
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/30 pt-[12vh]"
      onClick={() => setOpen(false)}
    >
      <div
        className="w-[600px] max-w-[90vw] overflow-hidden rounded-xl bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setActive(0);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setActive((a) => Math.min(a + 1, results.length - 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setActive((a) => Math.max(a - 1, 0));
            } else if (e.key === "Enter" && results[active]) {
              go(results[active].node.id);
            }
          }}
          placeholder="Search resources by name, kind, namespace…"
          className="w-full border-b border-zinc-200 px-4 py-3 text-sm outline-none"
        />
        <ul className="max-h-[50vh] overflow-y-auto">
          {query.trim() !== "" && results.length === 0 && (
            <li className="px-4 py-3 text-sm text-zinc-500">No matches.</li>
          )}
          {results.map((r, i) => (
            <li key={r.node.id}>
              <button
                onClick={() => go(r.node.id)}
                onMouseEnter={() => setActive(i)}
                className={`flex w-full items-center gap-3 px-4 py-2 text-left text-sm ${i === active ? "bg-blue-50" : ""}`}
              >
                <HealthBadge state={r.node.health.state} />
                <span className="rounded bg-zinc-100 px-1.5 py-0.5 text-[11px] font-medium text-zinc-600">
                  {r.node.kind}
                </span>
                <span className="truncate font-medium text-zinc-900">{r.node.name}</span>
                <span className="ml-auto truncate text-xs text-zinc-400">
                  {r.node.namespace ?? "cluster"}
                </span>
              </button>
            </li>
          ))}
        </ul>
        <div className="border-t border-zinc-100 px-4 py-2 text-[11px] text-zinc-400">
          ↑↓ navigate · Enter open in graph · Esc close
        </div>
      </div>
    </div>
  );
}
