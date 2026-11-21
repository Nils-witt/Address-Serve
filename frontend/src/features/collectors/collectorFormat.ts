import type { ChipProps } from '@mui/material';
import type { CollectorStatus, RunStatus, SyncStats } from '../../api/types';

export const collectorStatusColor: Record<CollectorStatus, ChipProps['color']> = {
  ok: 'success',
  invalid: 'error',
  missing: 'warning',
};

export const runStatusColor: Record<RunStatus, ChipProps['color']> = {
  running: 'info',
  succeeded: 'success',
  failed: 'error',
  cancelled: 'default',
};

/** Sums of what a run did across streets, house numbers and geo objects. */
export function totals(stats: SyncStats) {
  const all = [stats.streets, stats.houseNumbers, stats.geoObjects];
  const sum = (key: keyof SyncStats['streets']) => all.reduce((n, s) => n + s[key], 0);
  return {
    created: sum('created'),
    updated: sum('updated') + sum('adopted'),
    deleted: sum('deleted'),
    problems: sum('skipped') + sum('conflicts'),
  };
}
