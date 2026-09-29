import { UserManager, WebStorageStateStore, type User } from 'oidc-client-ts';
import type { UIConfig } from './types';
import { ROUTES } from '../routes';

/** The OIDC client for the provider the server trusts. Tokens live in
 * sessionStorage, so a session ends with the browser tab. Renewal is driven
 * by the AuthProvider with the refresh token, never a hidden iframe. */
export function createUserManager(config: UIConfig): UserManager {
  const origin = window.location.origin;
  return new UserManager({
    authority: config.oidc.issuer,
    client_id: config.oidc.clientId,
    scope: config.oidc.scope,
    redirect_uri: origin + ROUTES.callback,
    post_logout_redirect_uri: origin + ROUTES.login,
    response_type: 'code',
    userStore: new WebStorageStateStore({ store: window.sessionStorage }),
    automaticSilentRenew: false,
  });
}

/** The name shown for a user: the most human-friendly claim available. */
export function displayName(user: User): string {
  const p = user.profile;
  return p.preferred_username ?? p.name ?? p.email ?? p.sub;
}

/** A user whose access token can still be sent. */
export function isUsable(user: User | null): user is User {
  return !!user && !user.expired && !!user.access_token;
}
