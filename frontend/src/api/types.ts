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
