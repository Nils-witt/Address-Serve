import { useEffect, useState } from 'react';
import { type QueryClient, useQueryClient } from '@tanstack/react-query';
import { EVENTS_CLOSE_TOKEN_EXPIRED } from '../api/ApiClient';
import { queryKeys } from '../api/queryKeys';
import type { Collector, CollectorRun, CollectorRunEvent } from '../api/types';
import { useApi } from './useApi';

/** `connecting` before the first connection, `live` while connected, and
 * `reconnecting` after it was lost, while updates may be missing. */
export type LiveState = 'connecting' | 'live' | 'reconnecting';

/** Waits before reconnecting: doubling from the first, up to the last. */
const RETRY_MIN_MS = 1000;
const RETRY_MAX_MS = 30_000;

/** Puts a run update into the cached collector list and run history, and
 * refetches the run's details (issues, log) if they are shown. */
function applyRun(queryClient: QueryClient, run: CollectorRun) {
  // Only one run per collector runs at a time, so the one updating is its
  // newest.
  queryClient.setQueryData<Collector[]>(queryKeys.collectors, (collectors) =>
    collectors?.map((c) => (c.id === run.collectorId ? { ...c, lastRun: run } : c)),
  );
  queryClient.setQueryData<CollectorRun[]>(queryKeys.collectorRuns(run.collectorId), (runs) => {
    if (!runs) return runs;
    const i = runs.findIndex((r) => r.id === run.id);
    return i >= 0 ? runs.with(i, run) : [run, ...runs];
  });
  void queryClient.invalidateQueries({ queryKey: queryKeys.collectorRun(run.id), exact: true });
}

/** Follows collector runs over the server's WebSocket while mounted, keeping
 * the collector queries current without polling. Reconnects when the
 * connection drops, and reloads the collector queries on every connect,
 * since updates sent meanwhile are not replayed. */
export function useCollectorEvents(): LiveState {
  const api = useApi();
  const queryClient = useQueryClient();
  const [state, setState] = useState<LiveState>('connecting');

  useEffect(() => {
    let socket: WebSocket | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let retryMs = RETRY_MIN_MS;
    let stopped = false;

    const connect = () => {
      socket = api.openCollectorEvents();

      socket.onopen = () => {
        retryMs = RETRY_MIN_MS;
        setState('live');
        void queryClient.invalidateQueries({ queryKey: ['collectors'] });
        void queryClient.invalidateQueries({ queryKey: ['collector-runs'] });
      };

      socket.onmessage = (message: MessageEvent<string>) => {
        const event = JSON.parse(message.data) as CollectorRunEvent;
        if (event.type === 'run') applyRun(queryClient, event.run);
      };

      socket.onclose = (close) => {
        if (stopped) return;
        // An expired token is routine: reconnect with the renewed one at once.
        if (close.code === EVENTS_CLOSE_TOKEN_EXPIRED) {
          connect();
          return;
        }
        setState('reconnecting');
        retryTimer = setTimeout(connect, retryMs);
        retryMs = Math.min(retryMs * 2, RETRY_MAX_MS);
      };
    };

    connect();

    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      socket?.close();
    };
  }, [api, queryClient]);

  return state;
}
