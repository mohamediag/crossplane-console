import type { ResourceDetail } from "../../types/api";
import { HealthBadge } from "../graph/HealthBadge";
import { makeNodeId } from "../../lib/ids";

export function ConditionsTab({
  detail,
  onNavigate,
}: {
  detail: ResourceDetail;
  onNavigate: (id: string) => void;
}) {
  const { node, conditions, owners, children } = detail;
  return (
    <div className="space-y-5 p-4">
      {node && (
        <section className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
          <Field label="API version" value={node.apiVersion} />
          <Field label="Aggregate health" value={<HealthBadge state={node.aggregate} label />} />
          {node.externalName && <Field label="External name" value={node.externalName} mono />}
          {node.compositionRef && (
            <Field label="Composition" value={node.compositionRef.name} mono />
          )}
          {node.compositionRevision && (
            <Field label="Composition revision" value={node.compositionRevision} mono />
          )}
          {node.createdAt && (
            <Field label="Created" value={new Date(node.createdAt).toLocaleString()} />
          )}
        </section>
      )}

      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">
          Conditions
        </h3>
        {conditions.length === 0 && <p className="text-sm text-zinc-500">No conditions.</p>}
        <ul className="space-y-2">
          {conditions.map((c) => (
            <li key={c.type} className="rounded-md border border-zinc-200 p-2.5">
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium text-zinc-900">{c.type}</span>
                <span
                  className={`rounded px-1.5 py-0.5 text-xs font-semibold ${c.status === "True" ? "bg-emerald-100 text-emerald-700" : c.status === "False" ? "bg-red-100 text-red-700" : "bg-amber-100 text-amber-700"}`}
                >
                  {c.status}
                </span>
              </div>
              <div className="mt-1 text-xs text-zinc-500">
                {c.reason}
                {c.lastTransitionTime && ` · ${new Date(c.lastTransitionTime).toLocaleString()}`}
              </div>
              {c.message && <p className="mt-1 text-xs text-zinc-700">{c.message}</p>}
            </li>
          ))}
        </ul>
      </section>

      {owners.length > 0 && (
        <LinkSection title="Owners" onNavigate={onNavigate}>
          {owners.map((o) => ({
            id: makeNodeId(o.apiVersion ?? "", o.kind ?? "", o.namespace ?? "", o.name),
            label: `${o.kind}/${o.name}`,
          }))}
        </LinkSection>
      )}

      {children.length > 0 && (
        <LinkSection title="Children" onNavigate={onNavigate}>
          {children.map((c) => ({ id: c.id, label: `${c.kind}/${c.name}`, state: c.health.state }))}
        </LinkSection>
      )}
    </div>
  );
}

function Field({
  label,
  value,
  mono,
}: {
  label: string;
  value: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-zinc-500">{label}</dt>
      <dd className={`truncate text-zinc-900 ${mono ? "font-mono text-xs" : ""}`}>{value}</dd>
    </div>
  );
}

function LinkSection({
  title,
  children,
  onNavigate,
}: {
  title: string;
  children: { id: string; label: string; state?: string }[];
  onNavigate: (id: string) => void;
}) {
  return (
    <section>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">{title}</h3>
      <ul className="space-y-1">
        {children.map((item) => (
          <li key={item.id}>
            <button
              onClick={() => onNavigate(item.id)}
              className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-sm text-blue-700 hover:bg-blue-50"
            >
              {item.state && <HealthBadge state={item.state as never} />}
              <span className="truncate">{item.label}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
