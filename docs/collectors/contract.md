# Collector contract, version 1

A **collector** is an executable that fetches addresses (and optionally geo
objects) from some source and prints them as JSON. address-serv runs it when an
admin asks, then **syncs** the result into its database. Rows created by a
collector belong to it: later runs update them, and rows the collector no
longer returns are deleted.

A collector can be written in any language. In the Docker image, which has no
shell or interpreters, it has to be a static binary. See
[`examples/collectors/example`](../../examples/collectors/example) for a
reference implementation in Go.

JSON Schemas for both outputs live next to this file:
[`describe.schema.json`](describe.schema.json) and
[`collect.schema.json`](collect.schema.json).

## Discovery

- Collectors live in `COLLECTORS_DIR`. By default that's a `collectors` folder
  next to the address-serv binary. The folder is scanned at startup and when
  an admin presses *Reload*. Subfolders are not scanned.
- Every regular file that is executable by the app user and whose name doesn't
  start with `.` counts as a collector.
- The **collector ID** is the filename up to its first `.`: `bonn.py` becomes
  `bonn`. It must match `^[a-z0-9][a-z0-9_-]{0,63}$`. If two files share an
  ID (`bonn.py` and `bonn.sh`), both are rejected.
- The ID identifies the collector: its settings and the rows it owns are
  stored under it. A collector whose file disappears is shown as *missing*,
  and its data stays until an admin deletes it.

## Process environment

Both commands run in a fresh, empty temporary working directory that is
removed afterwards. stdin is empty. The process does **not** inherit
address-serv's environment, which holds database credentials. It gets only:

| Variable | Value |
|---|---|
| `PATH`, `HOME`, `TMPDIR`, `LANG`, `TZ`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `SSL_CERT_FILE`, `SSL_CERT_DIR` | Copied from address-serv when set |
| `ADDRESS_SERV_CONTRACT_VERSION` | `1` |
| `ADDRESS_SERV_COLLECTOR_ID` | The collector ID |
| `ADDRESS_SERV_RUN_ID` | The run's UUID (`collect` only) |
| `COLLECTOR_SETTING_<KEY>` | One per setting that has a value (`collect` only). `<KEY>` is the setting key in upper case. Strings are passed as they are, numbers in decimal notation, and booleans as `true` or `false`. A setting without a value or default is absent. |

## `describe`

```
<exe> describe
```

This prints the collector's metadata to stdout. It must exit with 0 within
10 seconds. If it fails, the collector is shown as *invalid* and can't be run.

```json
{
  "contractVersion": 1,
  "name": "Bonn open data",
  "version": "1.0.0",
  "description": { "en": "House numbers from stadtplan.bonn.de", "de": "Hausnummern von stadtplan.bonn.de" },
  "settings": [
    {
      "key": "source_url",
      "label": { "en": "Source URL", "de": "Quell-URL" },
      "description": "GeoJSON endpoint",
      "type": "string",
      "required": true,
      "default": "https://stadtplan.bonn.de/geojson?OD=171"
    },
    { "key": "api_key", "label": "API key", "type": "secret" }
  ]
}
```

| Field | Rules |
|---|---|
| `contractVersion` | Required. Must be `1`. |
| `name` | Required, 1–100 characters. |
| `version` | Required, 1–50 characters, free-form. |
| `description` | Optional. A *localized text* of at most 1000 characters per language. |
| `settings` | Optional list. Admins fill in these settings in the UI, and their values are passed as environment variables. |
| `settings[].key` | `^[a-z][a-z0-9_]{0,63}$`, unique within the collector. |
| `settings[].label` | Required. A *localized text* of at most 200 characters per language. |
| `settings[].description` | Optional. A *localized text* of at most 1000 characters per language. |
| `settings[].type` | `string`, `secret`, `integer`, `number` or `boolean`. A `secret` is write-only: the API never returns its value. Strings and secrets must be non-empty when set. |
| `settings[].required` | Optional, defaults to `false`. A run won't start while a required setting has neither a value nor a default. |
| `settings[].default` | Optional. It must match `type`, and isn't allowed for `secret`. |

A **localized text** is either a plain string, used for every language, or
an object that maps two-letter language codes to strings, such as
`{"en": "…", "de": "…"}`. The UI shows the user's language, then `en`, then
any other.

## `collect`

```
<exe> collect
```

This fetches the data and prints **one** JSON document to stdout. Exit code 0
means success. With any other exit code the run fails and **nothing** is
applied. stderr is the run's log, and the last 1 MiB of it is kept. The log
shows while the run is running, saved every few seconds, so write progress
lines as you go and don't buffer stderr (in Python, `print(..., file=sys.stderr,
flush=True)`; Go's `os.Stderr` and `log` are unbuffered).

```json
{
  "contractVersion": 1,
  "addresses": [
    {
      "streetExternalId": "bonn-street-0815",
      "houseNumberExternalId": "bonn-hn-123456",
      "street": "Adenauerallee",
      "number": 12,
      "numberAddition": "a",
      "postcode": "53113",
      "district": "Bonn-Zentrum",
      "city": "Bonn",
      "country": "DE",
      "latitude": 50.7301,
      "longitude": 7.1103
    }
  ],
  "geoObjects": [
    {
      "externalId": "bonn-hydrant-4711",
      "group": "hydrant",
      "name": "H 4711",
      "latitude": 50.7302,
      "longitude": 7.1104,
      "address": {
        "streetExternalId": "bonn-street-0815",
        "street": "Adenauerallee",
        "district": "Bonn-Zentrum",
        "city": "Bonn",
        "country": "DE",
        "number": 12,
        "numberAddition": "a"
      }
    }
  ]
}
```

### Addresses

Each entry is one house number. Streets are formed from the entries:
address-serv groups them by street. A street's coordinates are those of its
house number that lies closest to the centre of all of them.

| Field | Rules |
|---|---|
| `street`, `district`, `city`, `country` | Required, non-empty. Together they identify the street. |
| `number` | Required, > 0. |
| `numberAddition` | Optional: omitted, `null` or non-empty. Together with `number` it identifies the house number on its street. |
| `postcode` | Required, non-empty. |
| `latitude`, `longitude` | Required, in range (−90…90, −180…180). |
| `streetExternalId` | Optional, 1–255 characters. This is a stable ID for the street in your source. Every entry of a street must carry the same value. |
| `houseNumberExternalId` | Optional, 1–255 characters. This is a stable ID for the house number in your source. |

The **external IDs** make renames safe. Without them, a street whose name,
district, city or country changes upstream gets deleted and created again,
so it gets a new ID in address-serv. With them, the existing row is updated
in place. A house number that has an ID can also move to another of the
collector's streets.

### Geo objects

| Field | Rules |
|---|---|
| `group`, `name` | Required, non-empty. |
| `latitude`, `longitude` | Required, in range. |
| `externalId` | Optional, 1–255 characters. It identifies the object across runs. Without it, the object is identified by `group` + `name`. |
| `address` | Optional. Names the street (`street`, `district`, `city`, `country`, and optionally `streetExternalId`) and optionally the house number (`houseNumberExternalId`, or `number` with an optional `numberAddition`). The street may belong to anyone, not just this collector. |

If `address` is missing or matches nothing, the object is attached to the
nearest house number within `COLLECTOR_GEO_MAX_DISTANCE_M` (default 100 m).
If none is that close, the object is skipped.

### What address-serv does with the output

- Unknown fields are ignored, so new optional fields can be added without
  raising the contract version.
- An invalid entry is **skipped** and listed in the run, and the rest is
  applied. Note that if the entry already exists in the database, skipping
  it means the sync **deletes** it, because it wasn't returned.
- If the same key shows up again in the output, the first occurrence wins
  and the later ones are skipped as duplicates.
- Rows that already exist but belong to someone else (created manually, or by
  another collector) are never changed. They're listed as conflicts. One
  exception: a manually created house number on one of the collector's own
  streets is **adopted** by the collector.
- An entry that matches one of the collector's own rows but can't be applied
  (for example a rename onto a name that's already taken, or a different
  external ID) is reported as a conflict, and the row stays unchanged. It
  isn't deleted. For a street, that includes its house numbers.
- A street or house number the collector no longer returns is deleted. If
  rows of others depend on it (a manual house number on the street, or
  someone else's geo object at the house number), it is handed over to manual
  ownership instead of being deleted.
- Everything is applied in one transaction, so a failure applies nothing.
- An empty output is valid, and it deletes everything the collector owns.

## Limits

- **Timeout:** a run is killed after `COLLECTOR_TIMEOUT` (default `10m`),
  including any child processes it started.
- **Cancel:** an admin can cancel a run, which kills the process in the same
  way.
- **Concurrency:** only one run per collector can be active at a time.

## Versioning

Both outputs carry `contractVersion`. Breaking changes raise it. Adding new
optional fields does not.
