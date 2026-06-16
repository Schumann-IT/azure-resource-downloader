import { Building2 } from 'lucide-react';
import { Link } from 'react-router-dom';
import { api } from '../api';
import { useAsync } from '../hooks';

export default function TenantsPage() {
  const { data, error, loading } = useAsync(() => api.tenants(), []);

  if (loading) return <p className="text-slate-500">Loading tenants…</p>;
  if (error) return <p className="text-red-600">Failed to load: {error}</p>;

  const tenants = data?.tenants ?? [];
  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Tenants</h1>
      {tenants.length === 0 ? (
        <p className="text-slate-500">No tenants found under the output directory.</p>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {tenants.map((t) => (
            <li key={t}>
              <Link
                to={`/tenant/${encodeURIComponent(t)}`}
                className="flex items-center gap-3 rounded-lg border bg-white p-4 hover:border-blue-400 hover:shadow-sm"
              >
                <Building2 className="text-blue-600" />
                <span className="font-medium">{t}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
