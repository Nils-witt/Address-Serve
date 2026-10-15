import { useApi } from './useApi';
import { queryKeys } from '../api/queryKeys';
import { useApiQuery } from './useApiQuery';
import type { HouseNumber, HouseNumberFilter } from '../api/types';

const NO_HOUSE_NUMBERS: HouseNumber[] = [];

export function useHouseNumbers(streetId: string) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.houseNumbers(streetId),
      queryFn: () => api.listHouseNumbers({ streetId }),
    },
    NO_HOUSE_NUMBERS,
  );
}

/** House numbers across all streets, narrowed by `filter`. */
export function useAllHouseNumbers(filter: HouseNumberFilter) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.allHouseNumbers(filter),
      queryFn: () => api.listHouseNumbers(filter),
    },
    NO_HOUSE_NUMBERS,
  );
}
