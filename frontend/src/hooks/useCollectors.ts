import { useApi } from './useApi';
import { queryKeys } from '../api/queryKeys';
import { useApiQuery } from './useApiQuery';
import type { Collector, CollectorRun } from '../api/types';

const NO_COLLECTORS: Collector[] = [];
const NO_RUNS: CollectorRun[] = [];

/** How often the collector list refreshes, so statuses stay current. */
const STATUS_REFRESH_MS = 30_000;

/** Every collector with its status and last run, by name. */
export function useCollectors() {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.collectors,
      queryFn: () => api.listCollectors(),
      refetchInterval: STATUS_REFRESH_MS,
    },
    NO_COLLECTORS,
  );
}

/** The runs of the collector with `collectorId`, newest first; nothing while
 * it is null. */
export function useCollectorRuns(collectorId: string | null) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.collectorRuns(collectorId ?? ''),
      queryFn: () => api.listCollectorRuns(collectorId ?? ''),
      enabled: collectorId !== null,
      refetchInterval: STATUS_REFRESH_MS,
    },
    NO_RUNS,
  );
}
