import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import type { PlatformPackage, PlatformSummary } from "../types/api";
import { HealthBadge } from "../components/graph/HealthBadge";

async function fetchPlatform(): Promise<PlatformSummary> {
  const res = await fetch("/api/platform");
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export function PlatformPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["platform"],
    queryFn: fetchPlatform,
    refetchInterval: 10000,
  });

  if (isLoading) return <p className="p-6 text-sm text-zinc-500">Loading platform…</p>;
  if (isError)
    return <p className="p-6 text-sm text-red-600">{(error as Error).message}</p>;
  if (!data) return null;

  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="mx-auto max-w-5xl space-y-8">
        <PackageSection title="Providers" items={data.providers} />
        <PackageSection title="Functions" items={data.functions} />
        {data.configurations.length > 0 && (
          <PackageSection title="Configurations" items={data.configurations} />
        )}

        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Provider Configs
          </h2>
          {data.providerConfigs.length === 0 && (
            <p className="text-sm text-zinc-500">No provider configs.</p>
          )}
          {data.providerConfigs.length > 0 && (
            <div className="overflow-hidden rounded-lg border border-zinc-200 bg-white">
              <table className="w-full text-sm">
                <thead className="bg-zinc-50 text-left text-xs uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-4 py-2">Name</th>
                    <th className="px-4 py-2">Kind</th>
                    <th className="px-4 py-2">Provider group</th>
                    <th className="px-4 py-2">Credentials</th>
                    <th className="px-4 py-2">Used by</th>
                  </tr>
                </thead>
                <tbody>
                  {data.providerConfigs.map((pc) => (
                    <tr key={`${pc.group}/${pc.name}`} className="border-t border-zinc-100">
                      <td className="px-4 py-2 font-medium text-zinc-900">{pc.name}</td>
                      <td className="px-4 py-2 text-zinc-600">{pc.kind}</td>
                      <td className="px-4 py-2 font-mono text-xs text-zinc-600">{pc.group}</td>
                      <td className="px-4 py-2 text-zinc-600">{pc.credentialsSource || "—"}</td>
                      <td className="px-4 py-2">
                        <span
                          className={`rounded px-1.5 py-0.5 text-xs font-semibold ${pc.usedBy > 0 ? "bg-emerald-100 text-emerald-700" : "bg-zinc-100 text-zinc-500"}`}
                        >
                          {pc.usedBy} resource{pc.usedBy === 1 ? "" : "s"}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Operations
          </h2>
          {data.operations.length === 0 && (
            <p className="text-sm text-zinc-500">
              No operations. Crossplane v2 Operations (Operation, CronOperation,
              WatchOperation) will appear here when created.
            </p>
          )}
          {data.operations.length > 0 && (
            <div className="overflow-hidden rounded-lg border border-zinc-200 bg-white">
              <table className="w-full text-sm">
                <thead className="bg-zinc-50 text-left text-xs uppercase tracking-wide text-zinc-500">
                  <tr>
                    <th className="px-4 py-2">Name</th>
                    <th className="px-4 py-2">Kind</th>
                    <th className="px-4 py-2">Namespace</th>
                    <th className="px-4 py-2">Schedule</th>
                    <th className="px-4 py-2">Health</th>
                  </tr>
                </thead>
                <tbody>
                  {data.operations.map((op) => (
                    <tr key={`${op.namespace}/${op.name}`} className="border-t border-zinc-100">
                      <td className="px-4 py-2 font-medium text-zinc-900">{op.name}</td>
                      <td className="px-4 py-2 text-zinc-600">{op.kind}</td>
                      <td className="px-4 py-2 text-zinc-600">{op.namespace || "—"}</td>
                      <td className="px-4 py-2 font-mono text-xs text-zinc-600">
                        {op.schedule || "—"}
                      </td>
                      <td className="px-4 py-2">
                        <HealthBadge state={op.health.state} label />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Composite Resource Definitions
          </h2>
          {data.xrds.length === 0 && (
            <p className="text-sm text-zinc-500">No XRDs installed.</p>
          )}
          <div className="overflow-hidden rounded-lg border border-zinc-200 bg-white">
            <table className="w-full text-sm">
              <thead className="bg-zinc-50 text-left text-xs uppercase tracking-wide text-zinc-500">
                <tr>
                  <th className="px-4 py-2">Kind</th>
                  <th className="px-4 py-2">Group</th>
                  <th className="px-4 py-2">Scope</th>
                  <th className="px-4 py-2">Versions</th>
                  <th className="px-4 py-2">Established</th>
                  <th className="px-4 py-2">Compositions</th>
                </tr>
              </thead>
              <tbody>
                {data.xrds.map((x) => (
                  <tr key={x.name} className="border-t border-zinc-100">
                    <td className="px-4 py-2">
                      <Link
                        to={`/resources?kind=${encodeURIComponent(x.kind)}`}
                        className="font-medium text-blue-700 hover:underline"
                        title={`List ${x.kind} resources`}
                      >
                        {x.kind}
                      </Link>
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-zinc-600">{x.group}</td>
                    <td className="px-4 py-2 text-zinc-600">{x.scope}</td>
                    <td className="px-4 py-2 font-mono text-xs text-zinc-600">
                      {(x.versions ?? []).join(", ")}
                    </td>
                    <td className="px-4 py-2">
                      <span
                        className={`rounded px-1.5 py-0.5 text-xs font-semibold ${x.established ? "bg-emerald-100 text-emerald-700" : "bg-amber-100 text-amber-700"}`}
                      >
                        {x.established ? "True" : "False"}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-zinc-600">
                      {(x.compositions ?? []).join(", ") || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Compositions
          </h2>
          {data.compositions.length === 0 && (
            <p className="text-sm text-zinc-500">No Compositions installed.</p>
          )}
          <div className="space-y-3">
            {data.compositions.map((c) => (
              <div key={c.name} className="rounded-lg border border-zinc-200 bg-white p-4">
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                  <div>
                    <span className="font-semibold text-zinc-900">{c.name}</span>
                    <span className="ml-2 text-xs text-zinc-500">
                      serves {c.compositeKind}
                      {c.compositeApiVersion ? ` (${c.compositeApiVersion})` : ""}
                    </span>
                  </div>
                  <span className="text-xs text-zinc-500">
                    {c.mode || "Resources"} mode · {c.revisionCount} revision
                    {c.revisionCount === 1 ? "" : "s"}
                    {c.latestRevisionName ? ` · latest ${c.latestRevisionName}` : ""}
                  </span>
                </div>
                {(c.pipeline ?? []).length > 0 && (
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <span className="text-xs text-zinc-400">pipeline:</span>
                    {(c.pipeline ?? []).map((p, i) => (
                      <span key={p.step} className="flex items-center gap-2">
                        {i > 0 && <span className="text-zinc-300">→</span>}
                        <span
                          className="rounded-md border border-violet-200 bg-violet-50 px-2 py-1 text-xs text-violet-800"
                          title={`step: ${p.step}`}
                        >
                          {p.step}
                          <span className="ml-1 text-violet-500">({p.function})</span>
                        </span>
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}

function PackageSection({ title, items }: { title: string; items: PlatformPackage[] }) {
  return (
    <section>
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        {title}
      </h2>
      {items.length === 0 && <p className="text-sm text-zinc-500">None installed.</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {items.map((p) => (
          <div key={p.name} className="rounded-lg border border-zinc-200 bg-white p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="truncate font-semibold text-zinc-900" title={p.name}>
                {p.name}
              </span>
              <HealthBadge state={p.health.state} label />
            </div>
            {p.package && (
              <p className="mt-1 truncate font-mono text-xs text-zinc-500" title={p.package}>
                {p.package}
              </p>
            )}
            <div className="mt-2 flex gap-3 text-xs text-zinc-500">
              {p.health.ready && (
                <span>
                  Healthy: <b>{p.health.ready.status}</b>
                </span>
              )}
              {p.health.synced && (
                <span>
                  Installed: <b>{p.health.synced.status}</b>
                </span>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
