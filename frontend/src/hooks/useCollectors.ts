import { useQuery } from '@tanstack/react-query';
import { ApiError } from '../api/ApiClient';
import { queryKeys } from '../api/queryKeys';
import type { Collector, CollectorRun } from '../api/types';
import { useApi } from './useApi';
import { useApiQuery } from './useApiQuery';

/** How often to look again while a run is in progress. */
const RUN_POLL_MS = 2000;

const NO_COLLECTORS: Collector[] = [];
const NO_RUNS: CollectorRun[] = [];

const whileRunning = (running: boolean) => (running ? RUN_POLL_MS : false);

/** Every known collector, refreshed while one of them runs. */
export function useCollectors() {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.collectors,
      queryFn: () => api.listCollectors(),
      pollInterval: (collectors) =>
        whileRunning(collectors?.some((c) => c.lastRun?.status === 'running') ?? false),
    },
    NO_COLLECTORS,
  );
}

/** Whether the user may manage collectors, i.e. has the admin claim. Shares
 * its cache entry with useCollectors, so the collectors page opens with the
 * list already loaded. */
export function useCollectorAccess(): boolean {
  const api = useApi();
  const { isSuccess, error } = useQuery({
    queryKey: queryKeys.collectors,
    queryFn: () => api.listCollectors(),
  });
  // 403: no admin claim; 404: collectors are off on this server. Any other
  // failure (a network error, say) still shows the page, which shows it.
  const refused = error instanceof ApiError && (error.status === 403 || error.status === 404);
  return isSuccess || (error !== null && !refused);
}

/** A collector's runs, newest first, refreshed while one is running. */
export function useCollectorRuns(collectorId: string | null) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.collectorRuns(collectorId ?? ''),
      queryFn: () => api.listCollectorRuns(collectorId ?? ''),
      enabled: collectorId !== null,
      fresh: true,
      pollInterval: (runs) => whileRunning(runs?.some((r) => r.status === 'running') ?? false),
    },
    NO_RUNS,
  );
}

/** One run with its issues and log, refreshed until it ends. */
export function useCollectorRun(runId: string | null) {
  const api = useApi();
  return useApiQuery<CollectorRun | null>(
    {
      queryKey: queryKeys.collectorRun(runId ?? ''),
      queryFn: () => api.getCollectorRun(runId ?? ''),
      enabled: runId !== null,
      fresh: true,
      pollInterval: (run) => whileRunning(run?.status === 'running'),
    },
    null,
  );
}
