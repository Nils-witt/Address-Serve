import { useQuery } from '@tanstack/react-query';
import { ApiError } from '../api/ApiClient';
import { queryKeys } from '../api/queryKeys';
import type { Collector, CollectorRun } from '../api/types';
import { useApi } from './useApi';
import { useApiQuery } from './useApiQuery';

const NO_COLLECTORS: Collector[] = [];
const NO_RUNS: CollectorRun[] = [];

/** Every known collector. Kept current while a run goes on by
 * useCollectorEvents, which must be mounted alongside. */
export function useCollectors() {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.collectors,
      queryFn: () => api.listCollectors(),
    },
    NO_COLLECTORS,
  );
}

/** Whether the user may manage collectors, i.e. is in the admin group. Shares
 * its cache entry with useCollectors, so the collectors page opens with the
 * list already loaded. */
export function useCollectorAccess(): boolean {
  const api = useApi();
  const { isSuccess, error } = useQuery({
    queryKey: queryKeys.collectors,
    queryFn: () => api.listCollectors(),
  });
  // 403: not in the admin group; 404: collectors are off on this server. Any other
  // failure (a network error, say) still shows the page, which shows it.
  const refused = error instanceof ApiError && (error.status === 403 || error.status === 404);
  return isSuccess || (error !== null && !refused);
}

/** A collector's runs, newest first, kept current by useCollectorEvents. */
export function useCollectorRuns(collectorId: string | null) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.collectorRuns(collectorId ?? ''),
      queryFn: () => api.listCollectorRuns(collectorId ?? ''),
      enabled: collectorId !== null,
      fresh: true,
    },
    NO_RUNS,
  );
}

/** One run with its issues and log, refetched by useCollectorEvents whenever
 * the run changes. */
export function useCollectorRun(runId: string | null) {
  const api = useApi();
  return useApiQuery<CollectorRun | null>(
    {
      queryKey: queryKeys.collectorRun(runId ?? ''),
      queryFn: () => api.getCollectorRun(runId ?? ''),
      enabled: runId !== null,
      fresh: true,
    },
    null,
  );
}
