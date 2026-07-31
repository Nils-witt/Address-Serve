import json
import sys
import urllib.error
import urllib.request
from collections import Counter, defaultdict

GEOJSON_URL = "https://stadtplan.bonn.de/geojson?OD=171"
API_BASE_URL = "http://localhost:8080"

# The source dataset has no postal code field, so every street is uploaded
# with this placeholder. Replace it once a real postcode source is wired up.
PLACEHOLDER_POSTCODE = "00000"


def download_geojson(url):
    with urllib.request.urlopen(url) as response:
        return json.load(response)


def group_streets(features):
    """Group address points by street key into streets with their house numbers."""
    streets = {}
    districts = defaultdict(Counter)

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

        street["lat_sum"] += latitude
        street["lon_sum"] += longitude
        street["house_numbers"].append({
            "number": number,
            "numberAddition": props.get("hnr_zusatz"),
            "latitude": latitude,
            "longitude": longitude,
        })

    for street_key, street in streets.items():
        count = len(street["house_numbers"])
        street["latitude"] = street.pop("lat_sum") / count
        street["longitude"] = street.pop("lon_sum") / count
        most_common = districts[street_key].most_common(1)
        street["district"] = most_common[0][0] if most_common else ""

    return streets


def get_json(path):
    with urllib.request.urlopen(f"{API_BASE_URL}{path}") as response:
        return json.load(response)


def post_json(path, payload):
    url = f"{API_BASE_URL}{path}"
    data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}, method="POST",
    )
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
            house_number_payload = dict(house_number, streetId=street_id, postcode=PLACEHOLDER_POSTCODE)
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
    print(f"downloading {GEOJSON_URL}")
    geojson = download_geojson(GEOJSON_URL)

    streets = group_streets(geojson["features"])
    print(f"parsed {len(streets)} streets with {sum(len(s['house_numbers']) for s in streets.values())} house numbers")

    upload_streets(streets)


if __name__ == '__main__':
    main()
