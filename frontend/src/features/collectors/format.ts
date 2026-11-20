import type { TFunction } from 'i18next';
import type { CollectorRun } from '../../api/types';

/** An interval in the largest whole unit, e.g. "6 hours" or "90 minutes". */
export function formatInterval(minutes: number, t: TFunction): string {
  if (minutes % (24 * 60) === 0) return t('collectors.days', { count: minutes / (24 * 60) });
  if (minutes % 60 === 0) return t('collectors.hours', { count: minutes / 60 });
  return t('collectors.minutes', { count: minutes });
}

/** What a run wrote and deleted, e.g. "120 streets, 1,800 house numbers,
 * 4 geo objects, 3 deleted". */
export function formatRunCounts(run: CollectorRun, t: TFunction, locale: string): string {
  const n = (value: number) => value.toLocaleString(locale);
  const parts = [
    t('collectors.countStreets', { count: run.streets, formatted: n(run.streets) }),
    t('collectors.countHouseNumbers', { count: run.houseNumbers, formatted: n(run.houseNumbers) }),
    t('collectors.countGeoObjects', { count: run.geoObjects, formatted: n(run.geoObjects) }),
  ];
  if (run.deleted) {
    parts.push(t('collectors.countDeleted', { count: run.deleted, formatted: n(run.deleted) }));
  }
  return parts.join(', ');
}
