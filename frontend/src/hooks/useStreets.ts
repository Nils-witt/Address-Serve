import { useApi } from './useApi';
import { queryKeys } from '../api/queryKeys';
import { useApiQuery } from './useApiQuery';
import type { District, Street, StreetFilter } from '../api/types';

const NO_STREETS: Street[] = [];
const NO_CITIES: string[] = [];
const NO_DISTRICTS: District[] = [];

export function useStreets(filter: StreetFilter) {
  const api = useApi();
  return useApiQuery(
    { queryKey: queryKeys.streets(filter), queryFn: () => api.listStreets(filter) },
    NO_STREETS,
  );
}

export function useStreet(streetId: string) {
  const api = useApi();
  return useApiQuery<Street | null>(
    { queryKey: queryKeys.street(streetId), queryFn: () => api.getStreet(streetId) },
    null,
  );
}

export function useCities() {
  const api = useApi();
  return useApiQuery({ queryKey: queryKeys.cities, queryFn: () => api.listCities() }, NO_CITIES);
}

/** Districts of `city`, or of every city when it is empty. */
export function useDistricts(city: string) {
  const api = useApi();
  return useApiQuery(
    { queryKey: queryKeys.districts(city), queryFn: () => api.listDistricts(city) },
    NO_DISTRICTS,
  );
}
