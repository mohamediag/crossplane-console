import type { HealthState } from "../../types/api";

const COLORS: Record<HealthState, string> = {
  Healthy: "bg-emerald-500",
  Progressing: "bg-sky-500",
  Unknown: "bg-amber-500",
  Unhealthy: "bg-red-500",
  NA: "bg-zinc-400",
};

export const healthTextColors: Record<HealthState, string> = {
  Healthy: "text-emerald-600",
  Progressing: "text-sky-600",
  Unknown: "text-amber-600",
  Unhealthy: "text-red-600",
  NA: "text-zinc-500",
};

export function HealthBadge({ state, label }: { state: HealthState; label?: boolean }) {
  return (
    <span className="inline-flex items-center gap-1.5" title={state}>
      <span className={`h-2.5 w-2.5 rounded-full ${COLORS[state]}`} />
      {label && <span className={`text-xs font-medium ${healthTextColors[state]}`}>{state}</span>}
    </span>
  );
}
