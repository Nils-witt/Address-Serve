import { useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { queryKeys } from '../api/queryKeys';

/** Returns a function that refetches every cached query of a family after a
 * mutation, e.g. `invalidate('streets')` after creating a street. */
export function useInvalidate() {
  const queryClient = useQueryClient();
  return useCallback(
    async (family: keyof typeof queryKeys.under) => {
      await Promise.all(
        queryKeys.under[family].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      );
    },
    [queryClient],
  );
}
