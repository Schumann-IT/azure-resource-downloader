import { useState } from 'react';
import type { TenantDiffResult } from '../api';
import { api } from '../api';
import { StatusBadge } from '../components/StatusBadge';
import { useAsync } from '../hooks';

const VOLATILE = ['id', 'lastModifiedDateTime'];

export default function DiffPage() {
  const { data: tenantsData } = useAsync(() => api.tenants(), []);
  const tenants = tenantsData?.tenants ?? [];

  const [left, setLeft] = useState('');
  const [right, setRight] = useState('');
  const [ignoreVolatile, setIgnoreVolatile] = useState(true);
  const [result, setResult] = useState<TenantDiffResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  async function run() {
    if (!left || !right) return;
    setBusy(true);
    setErr('');
    try {
      setResult(await api.diff(left, right, ignoreVolatile ? VOLATILE : []));
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Diff tenants</h1>
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <TenantSelect label="Left" value={left} onChange={setLeft} tenants={tenants} />
        <TenantSelect label="Right" value={right} onChange={setRight} tenants={tenants} />
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={ignoreVolatile}
            onChange={(e) => setIgnoreVolatile(e.target.checked)}
          />
          Ignore volatile fields ({VOLATILE.join(', ')})
        </label>
        <button
          onClick={run}
          disabled={!left || !right || busy}
          className="rounded bg-blue-600 px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
        >
          {busy ? 'Comparing…' : 'Compare'}
        </button>
      </div>

      {err && <p className="text-red-600">{err}</p>}
      {result && <DiffResult result={result} />}
    </div>
  );
}

function TenantSelect({
  label,
  value,
  onChange,
  tenants,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  tenants: string[];
}) {
  return (
    <label className="flex flex-col text-sm">
      <span className="mb-1 text-slate-500">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded border px-2 py-1.5"
      >
        <option value="">Select tenant…</option>
        {tenants.map((t) => (
          <option key={t} value={t}>
            {t}
          </option>
        ))}
      </select>
    </label>
  );
}

function DiffResult({ result }: { result: TenantDiffResult }) {
  const s = result.summary;
  const groups = result.groups.filter((g) => g.summary.added + g.summary.removed + g.summary.changed > 0);
  return (
    <div>
      <div className="mb-4 flex gap-4 text-sm">
        <Counter label="added" value={s.added} className="text-green-700" />
        <Counter label="removed" value={s.removed} className="text-red-700" />
        <Counter label="changed" value={s.changed} className="text-amber-700" />
        <Counter label="unchanged" value={s.unchanged} className="text-slate-500" />
      </div>
      {groups.length === 0 ? (
        <p className="text-slate-500">No differences between the selected tenants.</p>
      ) : (
        <div className="space-y-3">
          {groups.map((g) => (
            <div key={`${g.provider}/${g.resourceType}`} className="rounded border bg-white">
              <div className="border-b px-3 py-2 text-sm font-medium">
                {g.provider} / {g.resourceType}
              </div>
              <ul className="divide-y">
                {g.entries
                  .filter((e) => e.status !== 'unchanged')
                  .map((e) => (
                    <li key={e.slug} className="flex items-center justify-between px-3 py-1.5 text-sm">
                      <span>{e.slug}</span>
                      <span className="flex items-center gap-2">
                        {e.changes && (
                          <span className="text-xs text-slate-400">
                            +{e.changes.added} -{e.changes.removed} ~{e.changes.changed}
                          </span>
                        )}
                        <StatusBadge status={e.status} />
                      </span>
                    </li>
                  ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function Counter({ label, value, className }: { label: string; value: number; className: string }) {
  return (
    <span className={`font-medium ${className}`}>
      {value} <span className="font-normal text-slate-500">{label}</span>
    </span>
  );
}
