import { useEffect, useState } from 'react';

export interface AsyncState<T> {
  data?: T;
  error?: string;
  loading: boolean;
}

/** Minimal data-fetching hook; re-runs when any dependency changes. */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): AsyncState<T> {
  const [state, setState] = useState<AsyncState<T>>({ loading: true });
  useEffect(() => {
    let active = true;
    setState({ loading: true });
    fn()
      .then((data) => active && setState({ data, loading: false }))
      .catch((err: unknown) =>
        active && setState({ error: String(err), loading: false }),
      );
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return state;
}
