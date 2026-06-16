import type { ChangeStatus } from '../api';

const styles: Record<ChangeStatus, string> = {
  added: 'bg-green-100 text-green-800',
  removed: 'bg-red-100 text-red-800',
  changed: 'bg-amber-100 text-amber-800',
  unchanged: 'bg-slate-100 text-slate-600',
};

export function StatusBadge({ status }: { status: ChangeStatus }) {
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${styles[status]}`}>
      {status}
    </span>
  );
}
