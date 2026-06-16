import { ArrowLeft } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api } from '../api';
import { MarkdownView } from '../components/MarkdownView';
import { useAsync } from '../hooks';

type Tab = 'config' | 'docs';

export default function ResourcePage() {
  const { tenant = '', provider = '', type = '', slug = '' } = useParams();
  const [tab, setTab] = useState<Tab>('config');
  const { data, error, loading } = useAsync(
    () => api.resource(tenant, provider, type, slug),
    [tenant, provider, type, slug],
  );

  if (loading) return <p className="text-slate-500">Loading resource…</p>;
  if (error) return <p className="text-red-600">Failed to load: {error}</p>;
  if (!data) return null;

  return (
    <div>
      <Link
        to={`/tenant/${encodeURIComponent(tenant)}`}
        className="mb-3 inline-flex items-center gap-1 text-sm text-blue-700"
      >
        <ArrowLeft size={14} /> {tenant}
      </Link>
      <h1 className="text-lg font-semibold">{slug}</h1>
      <p className="mb-4 text-sm text-slate-500">
        {provider} / {type}
      </p>

      <div className="mb-3 flex gap-1 border-b">
        <TabButton active={tab === 'config'} onClick={() => setTab('config')}>
          Configuration
        </TabButton>
        <TabButton active={tab === 'docs'} onClick={() => setTab('docs')}>
          Documentation
        </TabButton>
      </div>

      {tab === 'config' ? (
        <pre className="overflow-x-auto rounded-lg border bg-slate-900 p-4 text-xs leading-relaxed text-slate-100">
          {data.yaml}
        </pre>
      ) : data.doc ? (
        <MarkdownView source={data.doc} />
      ) : (
        <div className="rounded-lg border border-dashed bg-white p-6 text-center text-sm text-slate-500">
          Documentation has not been generated yet for this resource.
          <br />
          Run <code className="rounded bg-slate-100 px-1">npm run docgen</code> to create it.
        </div>
      )}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`-mb-px border-b-2 px-3 py-1.5 text-sm ${
        active ? 'border-blue-600 font-medium text-blue-700' : 'border-transparent text-slate-500'
      }`}
    >
      {children}
    </button>
  );
}
