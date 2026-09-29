/** Parses a latitude/longitude pair typed into a form; null if either is not
 * a number in range. Accepts a decimal comma. */
export function parseCoordinates(
  latitude: string,
  longitude: string,
): { latitude: number; longitude: number } | null {
  const lat = Number(latitude.trim().replace(',', '.'));
  const lon = Number(longitude.trim().replace(',', '.'));
  if (!latitude.trim() || !longitude.trim() || !Number.isFinite(lat) || !Number.isFinite(lon)) {
    return null;
  }
  if (lat < -90 || lat > 90 || lon < -180 || lon > 180) return null;
  return { latitude: lat, longitude: lon };
}
