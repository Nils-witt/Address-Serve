import { createContext } from 'react';
import type { UIConfig } from '../api/types';

export interface AuthState {
  /** A display name for the signed-in user, or null with no session. */
  username: string | null;
  isAuthenticated: boolean;
  /** The current access token sent to the API, or null with no session.
   * Stable across renders and always current, even right after a renewal. */
  getToken: () => string | null;
  /** The server's public config (OIDC client, version). */
  config: UIConfig;
  /** Set when the session ended on its own (expired and could not be
   * renewed), so the login page can say why. Cleared by `logout` and
   * `clearSessionMessage`. */
  sessionMessage: string | null;
  /** Sends the browser to the OpenID provider; after signing in the user
   * comes back to `from` (an app path) via the callback route. */
  login: (from?: string | null) => Promise<void>;
  /** Finishes the sign-in on the callback route; resolves the app path the
   * user started from. */
  completeLogin: () => Promise<string | null>;
  /** Ends the session on the user's request (and at the provider, if it
   * supports that). */
  logout: () => Promise<void>;
  clearSessionMessage: () => void;
  /** Renews the access token with the refresh token. Concurrent calls share
   * one renewal. Resolves false if there is nothing to renew with or the
   * provider refused. Stable across renders. */
  refreshSession: () => Promise<boolean>;
  /** Ends a session the server no longer accepts and remembers why in
   * `sessionMessage`. Does nothing if there is no session. Stable across
   * renders. */
  expireSession: () => void;
}

export const AuthContext = createContext<AuthState | null>(null);
