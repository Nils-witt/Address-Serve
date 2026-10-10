import type { HouseNumber } from '../api/types';

export function formatCoordinates(latitude: number, longitude: number): string {
  return `${latitude.toFixed(6)}, ${longitude.toFixed(6)}`;
}

export function osmUrl(latitude: number, longitude: number): string {
  return `https://www.openstreetmap.org/?mlat=${latitude}&mlon=${longitude}#map=18/${latitude}/${longitude}`;
}

export function formatHouseNumber(h: Pick<HouseNumber, 'number' | 'numberAddition'>): string {
  return `${h.number}${h.numberAddition ?? ''}`;
}

/** A date and time in the UI language, e.g. "10 Oct 2026, 14:05". */
export function formatDateTime(iso: string, locale: string): string {
  return new Date(iso).toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'short' });
}

/** Orders house numbers the way they appear on a street: 1, 2, 2a, 2b, 10. */
export function compareHouseNumbers(a: HouseNumber, b: HouseNumber): number {
  return a.number - b.number || (a.numberAddition ?? '').localeCompare(b.numberAddition ?? '');
}
