import { useApi } from './useApi';
import { queryKeys } from '../api/queryKeys';
import { useApiQuery } from './useApiQuery';
import type { HouseNumber } from '../api/types';

const NO_HOUSE_NUMBERS: HouseNumber[] = [];

export function useHouseNumbers(streetId: string) {
  const api = useApi();
  return useApiQuery(
    {
      queryKey: queryKeys.houseNumbers(streetId),
      queryFn: () => api.listHouseNumbers(streetId),
    },
    NO_HOUSE_NUMBERS,
  );
}
