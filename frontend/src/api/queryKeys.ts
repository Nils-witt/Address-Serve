import type { GeoObjectFilter, HouseNumberFilter, StreetFilter } from './types';

// Every query key in one place, so what is cached (and cleared on logout) is
// visible at a glance and two hooks can't collide on a key by accident.
export const queryKeys = {
  streets: (filter: StreetFilter) => ['streets', 'list', filter] as const,
  street: (streetId: string) => ['streets', streetId] as const,
  cities: ['cities'] as const,
  districts: (city: string) => ['districts', city] as const,
  houseNumbers: (streetId: string) => ['house-numbers', streetId] as const,
  allHouseNumbers: (filter: HouseNumberFilter) => ['house-numbers', 'list', filter] as const,
  geoObjects: (filter: GeoObjectFilter) => ['geo-objects', 'list', filter] as const,
  apiTokens: ['api-tokens'] as const,

  // Prefixes covering many keys at once, for invalidating a whole family
  // after a mutation.
  under: {
    // A street change can add or remove a city or district, too.
    streets: [['streets'], ['cities'], ['districts']] as const,
    // Deleting a house number detaches its geo objects; deleting a street
    // (which callers follow with this family, too) deletes them.
    houseNumbers: [['house-numbers'], ['geo-objects']] as const,
    geoObjects: [['geo-objects']] as const,
    apiTokens: [['api-tokens']] as const,
  },
};
