import { useApi } from './useApi';
import { queryKeys } from '../api/queryKeys';
import { useApiQuery } from './useApiQuery';
import type { APIToken } from '../api/types';

const NO_API_TOKENS: APIToken[] = [];

/** The signed-in user's API tokens, newest first. */
export function useAPITokens() {
  const api = useApi();
  return useApiQuery(
    { queryKey: queryKeys.apiTokens, queryFn: () => api.listAPITokens() },
    NO_API_TOKENS,
  );
}
