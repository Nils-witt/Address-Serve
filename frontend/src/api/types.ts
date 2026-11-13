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

export interface HouseNumberFilter {
  streetId?: string;
  /** Exact number, without addition. */
  number?: string;
  /** Postcode prefix. */
  postcode?: string;
  /** Substring of the street name. */
  street?: string;
  city?: string;
  district?: string;
  country?: string;
}

export interface GeoObjectInput {
  name: string;
  group: string;
  streetId: string;
  /** A house number on `streetId`; null or omitted for none. */
  houseNumberId?: string | null;
  /** The object's own position, independent of its street and house number. */
  latitude: number;
  longitude: number;
  /** ID in another system that keeps this object in sync, e.g. tileserve-go.
   * Omitted on update to keep the stored one. */
  externalId?: string | null;
}

export interface GeoObject extends GeoObjectInput {
  id: string;
}

export interface GeoObjectFilter {
  /** Substring of the name. */
  name?: string;
  /** Exact group, case-insensitive. */
  group?: string;
  streetId?: string;
  houseNumberId?: string;
  /** Exact external ID. */
  externalId?: string;
  /** Prefix of the external ID. */
  externalIdPrefix?: string;
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
  /** Git branch of the build; empty when unknown. */
  branch: string;
}
