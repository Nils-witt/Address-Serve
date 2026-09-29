// The one place that talks to the address-serv HTTP API. It owns:
//   - request plumbing (auth header, JSON bodies, error mapping). On a 401 it
//     first asks for the session to be renewed (see
//     ApiClientOptions.refreshSession) and retries the request once; only if
//     that fails is the session reported as expired,
//   - one typed method per endpoint, so callers never build URLs or
//     serialize bodies themselves.
// The session itself (the OIDC user and its tokens) belongs to the
// AuthProvider; the client only reads it through the options below.

import type {
  District,
  HouseNumber,
  HouseNumberInput,
  Street,
  StreetFilter,
  StreetInput,
  UIConfig,
} from './types';

export class ApiError extends Error {
  /** HTTP status of the failed response; 0 if there was none. */
  readonly status: number;

  constructor(message: string, status = 0) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export interface ApiClientOptions {
  /** The current bearer token. Read on every request, so a renewed token is
   * picked up without recreating the client. */
  getToken: () => string | null;
  /** Renews the session after a 401. Resolves true if a new token is in
   * place, false if the session can't be renewed. */
  refreshSession?: () => Promise<boolean>;
  /** Called when the server rejects the session and it can't be renewed. */
  onSessionExpired?: () => void;
}

/** The API answers errors as `{"error": "..."}`; fall back to the raw text. */
async function errorText(res: Response): Promise<string> {
  const text = await res.text();
  try {
    const body = JSON.parse(text) as { error?: unknown };
    if (typeof body.error === 'string' && body.error) return body.error;
  } catch {
    // Not JSON; use the text as is.
  }
  return text || `request failed with status ${res.status}`;
}

function withQuery(path: string, params: Record<string, string | undefined>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value?.trim()) query.set(key, value.trim());
  }
  const qs = query.toString();
  return qs ? `${path}?${qs}` : path;
}

/** GET /ui-config, the only call made before an ApiClient (and a session) exists. */
export async function fetchUIConfig(): Promise<UIConfig> {
  const res = await fetch('/ui-config');
  if (!res.ok) throw new ApiError(await errorText(res), res.status);
  return (await res.json()) as UIConfig;
}

export class ApiClient {
  private options: ApiClientOptions;

  constructor(options: ApiClientOptions) {
    this.options = options;
  }

  // ---- request plumbing --------------------------------------------------

  private fetchAuthorized(path: string, options: RequestInit, token: string | null) {
    const headers = new Headers(options.headers);
    if (token) headers.set('Authorization', 'Bearer ' + token);
    return fetch(path, { ...options, headers });
  }

  /** After a 401 for a request sent with `sentToken`: whether it is worth
   * retrying, i.e. the token was already renewed meanwhile by another
   * request, or renewing it now succeeds. */
  private async tokenRenewedSince(sentToken: string | null): Promise<boolean> {
    if (this.options.getToken() !== sentToken) return true;
    return (await this.options.refreshSession?.()) ?? false;
  }

  /** Attaches the bearer token; on a 401 renews the session and retries once,
   * and if that isn't possible reports the session as expired. Throws an
   * ApiError carrying the server's message and status on any other non-OK. */
  private async request(path: string, options: RequestInit = {}): Promise<Response> {
    const sentToken = this.options.getToken();
    let res = await this.fetchAuthorized(path, options, sentToken);

    if (res.status === 401 && (await this.tokenRenewedSince(sentToken))) {
      res = await this.fetchAuthorized(path, options, this.options.getToken());
    }

    if (res.status === 401) {
      this.options.onSessionExpired?.();
      throw new ApiError('unauthorized', 401);
    }

    if (!res.ok) {
      throw new ApiError(await errorText(res), res.status);
    }

    return res;
  }

  private async getJson<T>(path: string): Promise<T> {
    const res = await this.request(path);
    return (await res.json()) as T;
  }

  private async sendJsonForJson<T>(path: string, method: string, body: unknown): Promise<T> {
    const res = await this.request(path, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    return (await res.json()) as T;
  }

  private async del(path: string): Promise<void> {
    await this.request(path, { method: 'DELETE' });
  }

  // ---- streets -----------------------------------------------------------

  listStreets(filter: StreetFilter = {}): Promise<Street[]> {
    return this.getJson(withQuery('/api/streets', { ...filter }));
  }

  getStreet(streetId: string): Promise<Street> {
    return this.getJson(`/api/streets/${encodeURIComponent(streetId)}`);
  }

  createStreet(input: StreetInput): Promise<Street> {
    return this.sendJsonForJson('/api/streets', 'POST', input);
  }

  updateStreet(streetId: string, input: StreetInput): Promise<Street> {
    return this.sendJsonForJson(`/api/streets/${encodeURIComponent(streetId)}`, 'PUT', input);
  }

  /** Also deletes the street's house numbers (the database cascades). */
  deleteStreet(streetId: string): Promise<void> {
    return this.del(`/api/streets/${encodeURIComponent(streetId)}`);
  }

  listCities(): Promise<string[]> {
    return this.getJson('/api/cities');
  }

  listDistricts(city: string): Promise<District[]> {
    return this.getJson(withQuery('/api/districts', { city }));
  }

  // ---- house numbers -----------------------------------------------------

  listHouseNumbers(streetId: string): Promise<HouseNumber[]> {
    return this.getJson(withQuery('/api/house-numbers', { streetId }));
  }

  createHouseNumber(input: HouseNumberInput): Promise<HouseNumber> {
    return this.sendJsonForJson('/api/house-numbers', 'POST', input);
  }

  updateHouseNumber(id: string, input: HouseNumberInput): Promise<HouseNumber> {
    return this.sendJsonForJson(`/api/house-numbers/${encodeURIComponent(id)}`, 'PUT', input);
  }

  deleteHouseNumber(id: string): Promise<void> {
    return this.del(`/api/house-numbers/${encodeURIComponent(id)}`);
  }
}
