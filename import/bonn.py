import json
import os
import sys
import urllib.error
import urllib.request
from collections import Counter, defaultdict
from pathlib import Path

GEOJSON_URL = "https://stadtplan.bonn.de/geojson?OD=171"
API_BASE_URL = "http://localhost:8080"
POSTCODES_PATH = Path(__file__).resolve().parent / "postcodes.geojson"

# POST /api/streets and POST /api/house-numbers require a bearer token
# issued by the API's OpenID Connect provider (see internal/api/oidc.go).
API_TOKEN = os.environ.get("API_TOKEN")

# Used for house numbers that don't fall inside any boundary in
# postcodes.geojson (e.g. points just outside its coverage area).
FALLBACK_POSTCODE = "00000"


def download_geojson(url):
    with urllib.request.urlopen(url) as response:
        return json.load(response)


def _bbox(ring):
    lons = [p[0] for p in ring]
    lats = [p[1] for p in ring]
    return min(lons), min(lats), max(lons), max(lats)


def load_postcode_boundaries(path):
    """Load postal code boundary polygons from an Overpass GeoJSON export.

    Returns a list of (postal_code, exterior_ring, interior_rings, bbox)
    tuples, one entry per polygon. A MultiPolygon feature expands into
    multiple entries sharing the same postal_code (disjoint parts, e.g.
    split by a river). interior_rings holds any holes (e.g. enclaves) to
    exclude. bbox is (min_lon, min_lat, max_lon, max_lat) of the exterior
    ring, used as a cheap pre-filter before the exact point-in-ring test.
    """
    with open(path) as f:
        data = json.load(f)

    boundaries = []
    for feature in data["features"]:
        geometry = feature["geometry"]
        geom_type = geometry["type"]
        postal_code = feature["properties"].get("postal_code")
        if not postal_code:
            continue

        if geom_type == "Polygon":
            polygons = [geometry["coordinates"]]
        elif geom_type == "MultiPolygon":
            polygons = geometry["coordinates"]
        elif geom_type == "LineString":
            # Raw member way of the boundary relation; the assembled
            # Polygon/MultiPolygon feature (handled above) carries the
            # postal_code, so these are expected and safe to skip.
            continue
        else:
            print(f"warning: skipping postcode {postal_code!r}, unsupported geometry {geom_type!r}", file=sys.stderr)
            continue

        for polygon in polygons:
            exterior_ring, *interior_rings = polygon
            boundaries.append((postal_code, exterior_ring, interior_rings, _bbox(exterior_ring)))

    return boundaries


def point_in_ring(longitude, latitude, ring):
    """PNPOLY ray-casting test for whether (longitude, latitude) lies in ring."""
    inside = False
    n = len(ring)
    j = n - 1
    for i in range(n):
        xi, yi = ring[i]
        xj, yj = ring[j]
        if (yi > latitude) != (yj > latitude) and longitude < (xj - xi) * (latitude - yi) / (yj - yi) + xi:
            inside = not inside
        j = i
    return inside


def find_postcode(boundaries, longitude, latitude):
    for postal_code, exterior_ring, interior_rings, bbox in boundaries:
        min_lon, min_lat, max_lon, max_lat = bbox
        if not (min_lon <= longitude <= max_lon and min_lat <= latitude <= max_lat):
            continue
        if not point_in_ring(longitude, latitude, exterior_ring):
            continue
        if any(point_in_ring(longitude, latitude, hole) for hole in interior_rings):
            continue
        return postal_code
    return None


def group_streets(features, postcode_boundaries):
    """Group address points by street key into streets with their house numbers."""
    streets = {}
    districts = defaultdict(Counter)
    unmatched_postcodes = 0

    for feature in features:
        props = feature["properties"]
        street_key = props.get("strassen_schluessel")
        name = props.get("stassen_langname")
        hnr = props.get("hnr_ohne_zusatz")
        if street_key is None or not name or not hnr:
            continue
        try:
            number = int(hnr)
        except ValueError:
            continue
        if number <= 0:
            continue

        longitude, latitude = feature["geometry"]["coordinates"]

        street = streets.get(street_key)
        if street is None:
            street = {
                "name": name,
                "city": props.get("gemeinde_name") or "Bonn",
                "lat_sum": 0.0,
                "lon_sum": 0.0,
                "house_numbers": [],
            }
            streets[street_key] = street

        district = props.get("ortsteil_name")
        if district:
            districts[street_key][district] += 1

        postcode = find_postcode(postcode_boundaries, longitude, latitude)
        if postcode is None:
            postcode = FALLBACK_POSTCODE
            unmatched_postcodes += 1

        street["lat_sum"] += latitude
        street["lon_sum"] += longitude
        street["house_numbers"].append({
            "number": number,
            "numberAddition": props.get("hnr_zusatz"),
            "postcode": postcode,
            "latitude": latitude,
            "longitude": longitude,
        })

    for street_key, street in streets.items():
        count = len(street["house_numbers"])
        street["latitude"] = street.pop("lat_sum") / count
        street["longitude"] = street.pop("lon_sum") / count
        most_common = districts[street_key].most_common(1)
        street["district"] = most_common[0][0] if most_common else ""

    if unmatched_postcodes:
        print(f"warning: {unmatched_postcodes} house numbers fell outside all postcode boundaries, "
              f"used fallback {FALLBACK_POSTCODE!r}", file=sys.stderr)

    return streets


def get_json(path):
    with urllib.request.urlopen(f"{API_BASE_URL}{path}") as response:
        return json.load(response)


def post_json(path, payload):
    url = f"{API_BASE_URL}{path}"
    data = json.dumps(payload).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if API_TOKEN:
        headers["Authorization"] = f"Bearer {API_TOKEN}"
    request = urllib.request.Request(url, data=data, headers=headers, method="POST")
    with urllib.request.urlopen(request) as response:
        return json.load(response)


def street_key(street):
    return (street["name"], street["district"], street["city"])


def fetch_existing_street_keys():
    existing = get_json("/api/streets")
    return {street_key(street) for street in existing}


def upload_streets(streets):
    existing_keys = fetch_existing_street_keys()
    new_streets = {
        key: street for key, street in streets.items()
        if street_key(street) not in existing_keys
    }
    already_present = len(streets) - len(new_streets)
    if already_present:
        print(f"skipping {already_present} streets that already exist")

    total = len(new_streets)
    uploaded = 0
    skipped = 0

    for index, street in enumerate(new_streets.values(), start=1):
        street_payload = {
            "city": street["city"],
            "district": street["district"],
            "name": street["name"],
            "country": "Germany",
            "latitude": street["latitude"],
            "longitude": street["longitude"],
        }
        try:
            created_street = post_json("/api/streets", street_payload)
        except urllib.error.HTTPError as err:
            print(f"[{index}/{total}] failed to create street {street['name']!r}: {err}", file=sys.stderr)
            skipped += len(street["house_numbers"])
            continue

        street_id = created_street["id"]
        for house_number in street["house_numbers"]:
            house_number_payload = dict(house_number, streetId=street_id)
            try:
                post_json("/api/house-numbers", house_number_payload)
                uploaded += 1
            except urllib.error.HTTPError as err:
                print(
                    f"[{index}/{total}] failed to create house number "
                    f"{house_number['number']}{house_number['numberAddition'] or ''} "
                    f"on {street['name']!r}: {err}",
                    file=sys.stderr,
                )
                skipped += 1

        if index % 50 == 0 or index == total:
            print(f"[{index}/{total}] streets uploaded")

    print(f"done: {uploaded} house numbers uploaded, {skipped} skipped")


def main():
    if not API_TOKEN:
        sys.exit("API_TOKEN environment variable must be set to a bearer token for the address-serv API")

    print(f"loading postcode boundaries from {POSTCODES_PATH}")
    postcode_boundaries = load_postcode_boundaries(POSTCODES_PATH)

    print(f"downloading {GEOJSON_URL}")
    geojson = download_geojson(GEOJSON_URL)

    streets = group_streets(geojson["features"], postcode_boundaries)
    print(f"parsed {len(streets)} streets with {sum(len(s['house_numbers']) for s in streets.values())} house numbers")

    upload_streets(streets)


if __name__ == '__main__':
    main()
