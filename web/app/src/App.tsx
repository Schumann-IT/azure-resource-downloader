import { GitCompareArrows, LayoutGrid } from 'lucide-react';
import { Link, Outlet, useLocation } from 'react-router-dom';

export default function App() {
  const { pathname } = useLocation();
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b bg-white">
        <div className="mx-auto flex max-w-7xl items-center gap-6 px-4 py-3">
          <Link to="/" className="text-lg font-semibold">
            Tenant Config Explorer
          </Link>
          <nav className="flex items-center gap-4 text-sm">
            <Link
              to="/"
              className={`flex items-center gap-1 ${pathname === '/' ? 'font-medium text-blue-700' : 'text-slate-600'}`}
            >
              <LayoutGrid size={16} /> Tenants
            </Link>
            <Link
              to="/diff"
              className={`flex items-center gap-1 ${pathname === '/diff' ? 'font-medium text-blue-700' : 'text-slate-600'}`}
            >
              <GitCompareArrows size={16} /> Diff
            </Link>
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
