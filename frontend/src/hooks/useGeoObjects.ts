import { useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';
import { useApi } from './useApi';
import { queryKeys } from '../api/queryKeys';
import { useApiQuery } from './useApiQuery';
import type { GeoObject, GeoObjectFilter, HouseNumber } from '../api/types';

const NO_GEO_OBJECTS: GeoObject[] = [];
const NO_GEO_OBJECT_FILTER: GeoObjectFilter = {};

/** Geo objects narrowed by `filter`. */
export function useGeoObjects(filter: GeoObjectFilter) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.geoObjects(filter),
      queryFn: () => api.listGeoObjects(filter),
    },
    NO_GEO_OBJECTS,
  );
}

/** Every group in use, sorted, to suggest when filtering or editing. Shares
 * its cache with the unfiltered geo object list. */
export function useGeoObjectGroups(): string[] {
  const { data } = useGeoObjects(NO_GEO_OBJECT_FILTER);
  return useMemo(
    () => [...new Set(data.map((g) => g.group))].sort((a, b) => a.localeCompare(b)),
    [data],
  );
}

/** The house numbers of `streetIds` by ID, to label the geo objects at them.
 * Each street is fetched (and cached) on its own, under the same key as the
 * street page's list, so pass only the streets currently shown. */
export function useHouseNumbersById(streetIds: string[]): Map<string, HouseNumber> {
  const api = useApi();
  return useQueries({
    queries: streetIds.map((streetId) => ({
      queryKey: queryKeys.houseNumbers(streetId),
      queryFn: () => api.listHouseNumbers({ streetId }),
    })),
    combine: indexHouseNumbers,
  });
}

// Module-level so useQueries keeps the combined Map's identity between renders
// until a result changes.
function indexHouseNumbers(results: { data?: HouseNumber[] }[]): Map<string, HouseNumber> {
  const byId = new Map<string, HouseNumber>();
  for (const result of results) {
    for (const h of result.data ?? []) byId.set(h.id, h);
  }
  return byId;
}
