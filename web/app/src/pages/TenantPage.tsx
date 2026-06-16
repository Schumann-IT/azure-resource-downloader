import { ChevronDown, ChevronRight, FileText } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { ResourceTypeNode } from '../api';
import { api } from '../api';
import { useAsync } from '../hooks';

export default function TenantPage() {
  const { tenant = '' } = useParams();
  const { data, error, loading } = useAsync(() => api.tree(tenant), [tenant]);
  const [query, setQuery] = useState('');

  const providers = useMemo(() => {
    if (!data) return [];
    const q = query.trim().toLowerCase();
    if (!q) return data.providers;
    return data.providers
      .map((p) => ({
        ...p,
        resourceTypes: p.resourceTypes
          .map((rt) => ({
            ...rt,
            resources: rt.resources.filter(
              (r) =>
                r.slug.toLowerCase().includes(q) ||
                (r.displayName ?? '').toLowerCase().includes(q) ||
                rt.resourceType.toLowerCase().includes(q),
            ),
          }))
          .filter((rt) => rt.resources.length > 0),
      }))
      .filter((p) => p.resourceTypes.length > 0);
  }, [data, query]);

  if (loading) return <p className="text-slate-500">Loading tree…</p>;
  if (error) return <p className="text-red-600">Failed to load: {error}</p>;

  return (
    <div>
      <h1 className="mb-1 text-xl font-semibold">{tenant}</h1>
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search resources and types…"
        className="mb-4 w-full max-w-md rounded border px-3 py-1.5 text-sm"
      />
      <div className="space-y-4">
        {providers.map((p) => (
          <section key={p.provider}>
            <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">
              {p.provider}
            </h2>
            <div className="space-y-1">
              {p.resourceTypes.map((rt) => (
                <TypeNode key={rt.resourceType} tenant={tenant} node={rt} expanded={!!query} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

function TypeNode({
  tenant,
  node,
  expanded,
}: {
  tenant: string;
  node: ResourceTypeNode;
  expanded: boolean;
}) {
  const [open, setOpen] = useState(expanded);
  return (
    <div className="rounded border bg-white">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-slate-50"
      >
        {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        <span className="font-medium">{node.resourceType}</span>
        <span className="text-slate-400">({node.resources.length})</span>
      </button>
      {open && (
        <ul className="border-t">
          {node.resources.map((r) => (
            <li key={r.slug}>
              <Link
                to={`/tenant/${encodeURIComponent(tenant)}/${encodeURIComponent(node.provider)}/${encodeURIComponent(node.resourceType)}/${encodeURIComponent(r.slug)}`}
                className="flex items-center gap-2 px-8 py-1.5 text-sm text-slate-700 hover:bg-blue-50"
              >
                <FileText size={14} className={r.hasDoc ? 'text-green-600' : 'text-slate-400'} />
                {r.displayName ?? r.slug}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
