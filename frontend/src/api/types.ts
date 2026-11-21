// Mirrors the schemas in internal/api/openapi.yaml.

export interface StreetInput {
  city: string;
  district: string;
  name: string;
  country: string;
  latitude: number;
  longitude: number;
}

/** Set on rows a collector owns; only its runs may change them, so the API
 * refuses edits and deletes with a 409. */
export interface CollectorOwned {
  collectorId?: string;
}

export interface Street extends StreetInput, CollectorOwned {
  id: string;
  /** The street's ID in its collector's source. */
  externalId?: string;
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

export interface HouseNumber extends HouseNumberInput, CollectorOwned {
  id: string;
  /** The house number's ID in its collector's source. */
  externalId?: string;
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

export interface GeoObject extends GeoObjectInput, CollectorOwned {
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

/** A plain string for every language, or one string per language code. */
export type LocalizedText = string | Record<string, string>;

export type SettingType = 'string' | 'secret' | 'integer' | 'number' | 'boolean';

export type SettingValue = string | number | boolean;

export interface CollectorSetting {
  key: string;
  label: LocalizedText;
  description?: LocalizedText;
  type: SettingType;
  required?: boolean;
  /** Used when no value is set; never present for secrets. */
  default?: SettingValue;
}

export type CollectorStatus = 'ok' | 'invalid' | 'missing';

export interface Collector {
  /** The executable's filename up to its first dot. */
  id: string;
  path: string;
  status: CollectorStatus;
  /** Why an invalid collector was rejected. */
  error?: string;
  name: string;
  version: string;
  description?: LocalizedText;
  settingsSchema: CollectorSetting[];
  /** Values of the settings that are set, except secrets. */
  settings: Record<string, SettingValue>;
  /** Keys of the secrets that have a value. */
  secretsSet: string[];
  discoveredAt: string;
  updatedAt: string;
  lastRun?: CollectorRun;
}

/** PUT /api/collectors/{id}/settings: absent settings are unset, except
 * secrets, which are kept; null unsets a secret. */
export type CollectorSettingsInput = Record<string, SettingValue | null>;

export type RunStatus = 'running' | 'succeeded' | 'failed' | 'cancelled';

export interface EntityStats {
  created: number;
  updated: number;
  unchanged: number;
  adopted: number;
  deleted: number;
  released: number;
  skipped: number;
  conflicts: number;
}

export interface SyncStats {
  streets: EntityStats;
  houseNumbers: EntityStats;
  geoObjects: EntityStats;
}

export type IssueKind = 'skipped' | 'conflict' | 'adopted' | 'released';

export interface SyncIssue {
  kind: IssueKind;
  entity: 'address' | 'geoObject' | 'street' | 'houseNumber';
  /** Position of the entry in the collector's addresses or geoObjects. */
  index?: number;
  key: string;
  reason: string;
  rowId?: string;
}

export interface CollectorRun {
  id: string;
  collectorId: string;
  status: RunStatus;
  phase?: 'collecting' | 'applying';
  triggeredBy: string;
  startedAt: string;
  finishedAt?: string;
  exitCode?: number;
  error?: string;
  stats?: SyncStats;
  /** Only on a single run: the first 1000 of each kind. */
  issues?: SyncIssue[];
  /** Only on a single run: the tail of the collector's stderr. */
  log?: string;
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
