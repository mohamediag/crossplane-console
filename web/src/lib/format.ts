// formatAge renders an RFC3339 timestamp as a kubectl-style age ("42d", "3h").
export function formatAge(timestamp: string): string {
  const then = Date.parse(timestamp);
  if (Number.isNaN(then)) return timestamp;
  let seconds = Math.max(0, Math.floor((Date.now() - then) / 1000));
  const units: [number, string][] = [
    [86400 * 365, "y"],
    [86400, "d"],
    [3600, "h"],
    [60, "m"],
  ];
  for (const [size, suffix] of units) {
    if (seconds >= size) return `${Math.floor(seconds / size)}${suffix}`;
  }
  return `${seconds}s`;
}
