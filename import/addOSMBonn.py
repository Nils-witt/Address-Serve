import json
import sys
from pathlib import Path

from bonn import get_json

SCRIPT_DIR = Path(__file__).resolve().parent


def main():
    existing_streets = fetch_existing_streets()

    osm_streets = fetch_osm_streets()

    compare_streets(osm_streets, existing_streets)


def compare_streets(osm_streets, existing_streets):
    osm_names = set(osm_streets.keys())
    existing_names = {street["name"] for street in existing_streets}

    only_in_osm = osm_names - existing_names
    only_in_existing = existing_names - osm_names

    print(f"Streets only in OSM ({len(only_in_osm)}):")
    for name in sorted(only_in_osm):
        print(f"  {name} => {osm_streets[name]}")

    print(f"Streets only in existing database ({len(only_in_existing)}):")
    for name in sorted(only_in_existing):
        print(f"  {name}")


def _representative_point(geometry):
    """Return a single [lon, lat] point representing a GeoJSON geometry."""
    geom_type = geometry.get("type")
    coords = geometry.get("coordinates")
    if coords is None:
        return None
    if geom_type == "Point":
        return coords
    if geom_type == "LineString":
        return coords[0]
    if geom_type == "Polygon":
        return coords[0][0]  # first vertex of the exterior ring
    return None


def fetch_osm_streets():
    """Return {name: [{"coord": ..., "type": ...}, ...]} for OSM highway features.

    A street name commonly maps to several disjoint OSM ways (e.g. split at
    intersections), so each name maps to a list of segments rather than a
    single one, to avoid silently discarding all but the last segment seen.
    """
    output = {}
    with open(SCRIPT_DIR / "export.geojson") as osm_streets_file:
        data = json.load(osm_streets_file)
        for entry in data["features"]:
            props = entry.get("properties", {})
            highway = props.get("highway")
            name = props.get("name")
            if highway not in ['primary', 'secondary', 'tertiary', 'service',
                                'residential', 'living_street', 'pedestrian']:
                continue
            if not name:
                continue
            # geometry may be absent or explicitly null (valid per the
            # GeoJSON spec for a feature with unknown location).
            coord = _representative_point(entry.get("geometry") or {})
            if coord is None:
                print(f"warning: skipping {name!r}, unsupported/missing geometry", file=sys.stderr)
                continue
            output.setdefault(name, []).append({"coord": coord, "type": highway})
    return output


def fetch_existing_streets():
    return get_json("/api/streets")

if __name__ == "__main__":
    main()