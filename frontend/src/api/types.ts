// Mirrors the schemas in internal/api/openapi.yaml.

export interface StreetInput {
  city: string;
  district: string;
  name: string;
  country: string;
  latitude: number;
  longitude: number;
}

export interface Street extends StreetInput {
  id: string;
}

export interface StreetFilter {
  city?: string;
  district?: string;
  name?: string;
  country?: string;
}

export interface District {
  name: string;
  city: string;
}

export interface HouseNumberInput {
  streetId: string;
  number: number;
  numberAddition?: string | null;
  postcode: string;
  latitude: number;
  longitude: number;
}

export interface HouseNumber extends HouseNumberInput {
  id: string;
}

export interface APITokenInput {
  name: string;
  /** RFC 3339; null for a token that never expires. */
  expiresAt: string | null;
}

export interface APIToken {
  id: string;
  name: string;
  /** The first characters of the secret, to tell tokens apart. */
  prefix: string;
  createdAt: string;
  expiresAt: string | null;
  /** Approximate: recorded at most once a minute. */
  lastUsedAt: string | null;
}

/** POST /api/tokens: the only response that carries the secret. */
export interface CreatedAPIToken extends APIToken {
  token: string;
}

/** GET /ui-config: served before sign-in. */
export interface UIConfig {
  oidc: {
    issuer: string;
    clientId: string;
    scope: string;
  };
  version: string;
  commit: string;
}
