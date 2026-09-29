import { type ReactNode, useMemo } from 'react';
import { ApiClient } from '../api/ApiClient';
import { useAuth } from '../hooks/useAuth.ts';
import { ApiContext } from './ApiContext.ts';

export function ApiProvider({ children }: { children: ReactNode }) {
  const { getToken, refreshSession, expireSession } = useAuth();

  // All three callbacks are stable, so this is one client for the session's
  // lifetime: it reads the token per request, so renewing it doesn't hand
  // every consumer a new client (and re-run their fetches).
  const instance = useMemo(
    () => new ApiClient({ getToken, refreshSession, onSessionExpired: expireSession }),
    [getToken, refreshSession, expireSession],
  );

  return <ApiContext.Provider value={instance}>{children}</ApiContext.Provider>;
}
