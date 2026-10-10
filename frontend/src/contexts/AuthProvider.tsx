import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ErrorResponse, type User, type UserManager } from 'oidc-client-ts';
import { Alert, Box } from '@mui/material';
import { fetchUIConfig } from '../api/ApiClient.ts';
import { createUserManager, displayName, isUsable } from '../api/oidc.ts';
import type { UIConfig } from '../api/types.ts';
import RouteFallback from '../components/RouteFallback.tsx';
import { errorMessage } from '../lib/errors.ts';
import { AuthContext, type AuthState } from './AuthContext.ts';

interface LoginState {
  from?: string | null;
}

/** How a renewal ended: a new token is in place, the provider refused (the
 * session is over), or it failed for a reason that may pass (offline). */
type Renewal = 'renewed' | 'refused' | 'failed';

/** Background renewal starts this long before the access token expires. */
const RENEW_BEFORE_EXPIRY_S = 60;
/** Backoff for failed background renewals: doubles from base up to max. */
const RETRY_BASE_MS = 5_000;
const RETRY_MAX_MS = 5 * 60_000;
/** setTimeout fires immediately for delays past 2^31-1 ms. */
const MAX_TIMEOUT_MS = 2 ** 31 - 1;

/** Loads the server's OIDC config, then provides the session (see AuthState). */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [setup, setSetup] = useState<{ config: UIConfig; manager: UserManager } | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchUIConfig()
      .then((config) => {
        if (!cancelled) setSetup({ config, manager: createUserManager(config) });
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(errorMessage(err));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (error) {
    return (
      <Box className="route-fallback">
        <Alert severity="error">{error}</Alert>
      </Box>
    );
  }
  if (!setup) return <RouteFallback />;
  return (
    <SessionProvider config={setup.config} manager={setup.manager}>
      {children}
    </SessionProvider>
  );
}

function SessionProvider({
  config,
  manager,
  children,
}: {
  config: UIConfig;
  manager: UserManager;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  // undefined until the stored user has been read.
  const [user, setUserState] = useState<User | null | undefined>(undefined);
  // Mirrors `user` synchronously: the API client reads the token right after
  // a renewal resolves, before React has re-rendered.
  const userRef = useRef<User | null>(null);
  const setUser = useCallback((next: User | null) => {
    userRef.current = next;
    setUserState(next);
  }, []);
  const [sessionMessage, setSessionMessage] = useState<string | null>(null);
  const renewalInFlight = useRef<Promise<Renewal> | null>(null);

  useEffect(() => {
    let cancelled = false;
    manager
      .getUser()
      .then((stored) => {
        if (!cancelled) setUser(stored);
      })
      .catch(() => {
        if (!cancelled) setUser(null);
      });
    const onLoaded = (loaded: User) => setUser(loaded);
    const onUnloaded = () => setUser(null);
    manager.events.addUserLoaded(onLoaded);
    manager.events.addUserUnloaded(onUnloaded);
    return () => {
      cancelled = true;
      manager.events.removeUserLoaded(onLoaded);
      manager.events.removeUserUnloaded(onUnloaded);
    };
  }, [manager, setUser]);

  const endSession = useCallback(async () => {
    await manager.removeUser();
    setUser(null);
    // Cached API data belongs to the user who fetched it.
    queryClient.clear();
  }, [manager, queryClient, setUser]);

  const getToken = useCallback(() => userRef.current?.access_token ?? null, []);

  const renew = useCallback((): Promise<Renewal> => {
    renewalInFlight.current ??= (async (): Promise<Renewal> => {
      const current = await manager.getUser();
      if (!current?.refresh_token) return 'refused';
      try {
        // With a refresh token this is a plain token request, no iframe.
        const renewed = await manager.signinSilent();
        return isUsable(renewed) ? 'renewed' : 'refused';
      } catch (err) {
        // An error answer from the provider (e.g. invalid_grant) means the
        // refresh token is no good; anything else (offline, timeout) may pass.
        return err instanceof ErrorResponse ? 'refused' : 'failed';
      }
    })().finally(() => {
      renewalInFlight.current = null;
    });
    return renewalInFlight.current;
  }, [manager]);

  const refreshSession = useCallback(async () => (await renew()) === 'renewed', [renew]);

  const expireSession = useCallback(() => {
    // Late failures of requests from a session that already ended must not
    // announce an expiry that didn't happen.
    if (!userRef.current) return;
    void endSession();
    setSessionMessage(t('auth.sessionExpired'));
  }, [endSession, t]);

  // Background refresh: renew shortly before the access token expires, so
  // requests rarely hit a 401 and an idle tab keeps its session. Failed
  // attempts are retried with backoff until the provider refuses. Timers
  // stall while the device sleeps, so waking up or coming back online
  // re-checks right away. A renewal loads a new user, which reschedules.
  useEffect(() => {
    const expiresAt = user?.expires_at;
    if (!user?.refresh_token || expiresAt === undefined) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let failures = 0;
    let stopped = false;

    const untilDue = () => Math.max(0, (expiresAt - RENEW_BEFORE_EXPIRY_S) * 1000 - Date.now());
    const schedule = (ms: number) => {
      clearTimeout(timer);
      timer = setTimeout(() => void run(), Math.min(ms, MAX_TIMEOUT_MS));
    };
    const run = async () => {
      if (untilDue() > 0 && failures === 0) {
        // A clamped or early wake-up: not due yet.
        schedule(untilDue());
        return;
      }
      const result = await renew();
      if (stopped || result === 'renewed') return;
      if (result === 'refused') {
        expireSession();
        return;
      }
      failures++;
      schedule(Math.min(RETRY_BASE_MS * 2 ** (failures - 1), RETRY_MAX_MS));
    };
    const onWake = () => {
      if (document.visibilityState !== 'visible') return;
      if (failures > 0 || untilDue() === 0) schedule(0);
    };

    schedule(untilDue());
    document.addEventListener('visibilitychange', onWake);
    window.addEventListener('online', onWake);
    return () => {
      stopped = true;
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', onWake);
      window.removeEventListener('online', onWake);
    };
  }, [user, renew, expireSession]);

  const login = useCallback(
    async (from?: string | null) => {
      setSessionMessage(null);
      await manager.signinRedirect({ state: { from: from ?? null } satisfies LoginState });
    },
    [manager],
  );

  const completeLogin = useCallback(async () => {
    const signedIn = await manager.signinRedirectCallback();
    setSessionMessage(null);
    setUser(signedIn);
    return (signedIn.state as LoginState | undefined)?.from ?? null;
  }, [manager, setUser]);

  const logout = useCallback(async () => {
    const current = await manager.getUser();
    await endSession();
    setSessionMessage(null);
    let endSessionEndpoint: string | undefined;
    try {
      endSessionEndpoint = await manager.metadataService.getEndSessionEndpoint();
    } catch {
      endSessionEndpoint = undefined;
    }
    if (endSessionEndpoint) {
      await manager.signoutRedirect({ id_token_hint: current?.id_token });
    }
  }, [manager, endSession]);

  const clearSessionMessage = useCallback(() => setSessionMessage(null), []);

  // An expired token is still a session if it can be renewed: the first
  // request gets a 401 and renews it.
  const usable = isUsable(user ?? null) || !!user?.refresh_token;
  const value = useMemo<AuthState>(
    () => ({
      username: user && usable ? displayName(user) : null,
      isAuthenticated: !!user && usable,
      getToken,
      config,
      sessionMessage,
      login,
      completeLogin,
      logout,
      clearSessionMessage,
      refreshSession,
      expireSession,
    }),
    [
      user,
      usable,
      getToken,
      config,
      sessionMessage,
      login,
      completeLogin,
      logout,
      clearSessionMessage,
      refreshSession,
      expireSession,
    ],
  );

  if (user === undefined) return <RouteFallback />;
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
