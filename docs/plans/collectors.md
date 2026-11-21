# Plan: Collectors (pluggable address sources)

Status: **implemented** on branch `feature/collector-plugins` (see §11 for deviations).

A *collector* is an executable in a collectors folder. The app runs it on
demand. It returns a list of addresses (and geo objects), and the app
**syncs** them into the database: it creates, updates and deletes the rows that
the collector owns.

---

## 1. Decisions taken

| # | Topic | Decision |
|---|---|---|
| D1 | Plugin form | **Executables** (subprocess), any language. They are not Go `.so` plugins, which were rejected because of cgo, the lack of Windows support and toolchain lockstep. |
| D2 | Trigger | **Manual only**, via the API or UI. No schedule, and nothing runs at startup. |
| D3 | Result use | **Sync per collector**. The output is the full truth for the rows the collector owns: add, update, and delete what's missing. |
| D4 | Output content | **Addresses + geo objects**. |
| D5 | Address shape | **Flat**: one row per address. The app groups rows into streets. |
| D6 | Street coordinates | **Medoid**: the house number of the street that lies closest to the centroid of all its house numbers. |
| D7 | Foreign rows | A row that exists but isn't owned by this collector is **skipped and reported**. It is never updated or deleted. The one exception is D35. |
| D8 | Foreign street | When the street is foreign, the app skips the **street and all its house numbers** in the output and reports one conflict. |
| D9 | Geo object linking | Use the address **reference** if it's given and resolves. Otherwise fall back to the **nearest house number within a max distance** (env). If nothing is in range, skip and report. |
| D10 | Geo objects on foreign rows | **Allowed.** The geo object is owned by the collector, and only its street/house-number link points to a foreign row. |
| D11 | Geo object identity | Match on `externalId` when the collector gives one, otherwise on `group + name`. |
| D12 | Invalid rows | **Skip the row, apply the rest**, and report the skipped rows. ⚠️ A skipped row that already exists in the DB is **deleted** by the sync, because it counts as "not returned". This is accepted as decided. |
| D13 | Delete guard | **None.** Exit code 0 means the output is trusted. An empty output deletes all owned rows. |
| D14 | Execution | **Async.** `POST` starts a run and returns `202` + the run. Runs are persisted in the DB with status, counts, skipped rows, conflicts and the stderr log. |
| D15 | Concurrency | **One active run per collector.** A second start returns `409`. |
| D16 | Timeout | `COLLECTOR_TIMEOUT`, default **`10m`**. The process is killed when the timeout is reached. |
| D17 | Cancel | `POST /api/collector-runs/{id}/cancel` kills the process or rolls back the apply transaction. The run ends as `cancelled` and nothing is applied. |
| D18 | Process env | **Minimal**: no inherited app env, only the allowlist in §2.2 plus the collector's settings. |
| D19 | Settings | Stored **in the DB** and edited in the UI. Each collector declares its **settings schema** via `describe`. |
| D20 | Secrets | Schema fields of type `secret` are **write-only**: the API only reports `set: true/false`. They are stored in plain text in the DB. |
| D21 | Settings transport | **Env vars** (`COLLECTOR_SETTING_<KEY>`). The result goes to **stdout** and the log to **stderr**. |
| D22 | Metadata | `<exe> describe` prints JSON (name, version, description, settings schema). |
| D23 | Discovery | The app scans **at startup** and when `POST /api/collectors/reload` is called. |
| D24 | Identity | The **filename without extension** is the collector ID. A collector whose executable disappears is shown as `missing`, and its settings and owned rows are kept. |
| D25 | Folder | `COLLECTORS_DIR`, which defaults to `collectors` **next to the binary** (`filepath.Dir(os.Executable())`). For `go run`, `COLLECTORS_DIR` must be set. |
| D26 | Permissions | Membership in the group `COLLECTOR_ADMIN_GROUP`, read from the OIDC token's `groups` claim. **All** collector routes, including reads, require it. If the var is unset, the collector routes return `403`. **API tokens can never pass the check.** (This replaced an earlier generic `COLLECTOR_ADMIN_CLAIM` + `COLLECTOR_ADMIN_VALUE` rule.) |
| D27 | Manual edits | Collector-owned rows are **read-only** outside the collector. A mutating API route that would change or delete an owned row, directly or through an FK cascade or `SET NULL`, returns `409`. |
| D28 | Stale owned row with foreign dependants | The app does **not** delete such a row. It **releases ownership** (sets it to manual) and reports it as `released`. This applies to streets with foreign house numbers or geo objects, and to house numbers that foreign geo objects point to (D36). |
| D29 | House number key | Add a **unique index** on `(street_id, number, coalesce(number_addition, ''))`. Existing duplicates are reported at startup, following the `migrateGeoObjectGroupNameKey` pattern. |
| D30 | Reference code | A **separate example collector**, written in **Go**. `import/bonn.py` stays as it is. |
| D31 | Duplicates in output (Q1) | **The first occurrence wins.** Later ones go to `skipped` with the reason `duplicate`. |
| D32 | Manual children on owned streets (Q2) | **Allowed.** Users may add manual house numbers and geo objects to a collector-owned street. |
| D33 | Env allowlist (Q3) | `PATH HOME TMPDIR LANG TZ HTTP_PROXY HTTPS_PROXY NO_PROXY SSL_CERT_FILE SSL_CERT_DIR` |
| D34 | Translations (Q4) | `label` and `description` are either a string or a `{ "en": …, "de": … }` map. The UI uses the current locale, then `en`, then any available language. |
| D35 | Same-key manual house number (Q16) | If the collector returns a house number whose key matches a **manual** house number on one of **its own** streets, it **adopts** it (`collector_id` is set and the row is updated). Reported as `adopted`. House numbers owned by another collector are still conflicts. |
| D36 | Stale house number with foreign geo object (Q6) | It is **kept and released** (D28), not detached. |
| D37 | Deployment (Q7) | **Single instance.** The advisory lock and the partial unique index stay as safety nets. |
| D38 | Run storage (Q5) | Keep the last 1 MiB of stderr and at most 1000 entries per issue list, with full counts always stored. Keep only the **last N runs per collector** (`COLLECTOR_RUN_HISTORY`, default 50), pruned when a run finishes. |
| D39 | Deleting a collector (Q8) | Only allowed while it is `missing` or `invalid`. Its rows are **kept as manual** (`collector_id` is set to NULL), and `?deleteData=true` deletes them instead. |
| D40 | Deleting a manual parent of owned rows (Q9, Q15) | Deleting a manual street or house number that collector-owned rows depend on (cascade delete, or `SET NULL` on `house_number_id`) returns **409**, naming the collector(s). |
| D41 | Docker (Q11) | Keep `distroless/static`. `docker-compose.yml` mounts `./collectors:/collectors:ro`. Collectors must be static binaries, as documented. |
| D42 | Windows (Q13) | **Drop Windows** from `.goreleaser.yml`. The collector exec code is `//go:build unix`. A non-Unix build compiles with a stub that disables the feature, so `go build ./...` still works everywhere. |
| D43 | Stable IDs (Q14, Q19) | Collectors may send an optional **`streetExternalId`** and **`houseNumberExternalId`** per address. These are matched before the natural keys, so a rename or renumbering updates the row in place. This needs new `external_id` columns on `streets` and `house_numbers`, each with a partial unique index. |
| D44 | Inconsistent street IDs (Q17) | If the same name key carries different `streetExternalId`s, or the same `streetExternalId` carries different name keys, the app skips **all rows of that street** as `inconsistent street`. Under D12, existing rows are then deleted. |
| D45 | Backfill (Q18) | An owned row without an `external_id` that later arrives with one is matched by its natural key, and the new `external_id` is stored. This applies to streets and house numbers alike. |
| D46 | Who sets external IDs (Q20) | **Only collectors** set them. `externalId` on streets and house numbers is read-only in the API and in `/api/bulk`. |
| D47 | Geo reference by ID (Q21) | A geo object's `address` may carry `streetExternalId`, which is matched before the street name key. |
| D48 | Geo adoption (Q22) | **None.** A matching manual geo object stays a conflict (D7). |
| D49 | Geo reference by house number ID (Q23) | A geo object's `address` may carry `houseNumberExternalId`, which is matched before number + addition. |

---

## 2. Collector contract (v1)

This section becomes `docs/collectors/contract.md` during implementation,
alongside a JSON Schema for the `describe` and `collect` outputs.

### 2.1 Discovery

- The app scans `COLLECTORS_DIR`, non-recursively.
- It considers every **regular file** that is **executable** for the app user
  and doesn't start with `.`.
- Collector ID = filename up to the first `.` (`bonn.py` → `bonn`). It must match
  `^[a-z0-9][a-z0-9_-]{0,63}$`. Files whose ID doesn't match are listed as
  `invalid`.
- If two files share an ID (`bonn.py` and `bonn.sh`), **both** are marked
  `invalid` with an error naming the clash.
- For each file, the app runs `describe` (§2.3). If that fails, the collector is
  stored as `invalid` with the error, and it can't be run.

### 2.2 Process environment (both commands)

The working directory is a fresh temp dir per invocation, removed afterwards.
The environment holds **only** these variables:

| Variable | Value |
|---|---|
| `PATH`, `HOME`, `TMPDIR`, `LANG`, `TZ`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `SSL_CERT_FILE`, `SSL_CERT_DIR` | Copied from the app when set |
| `ADDRESS_SERV_CONTRACT_VERSION` | `1` |
| `ADDRESS_SERV_COLLECTOR_ID` | The collector ID |
| `ADDRESS_SERV_RUN_ID` | The run UUID (only for `collect`) |
| `COLLECTOR_SETTING_<KEY>` | One per configured setting (only for `collect`). `<KEY>` is the setting key in upper case. Booleans are `true`/`false`, numbers are in decimal notation, and unset optional settings are absent. |

stdin is empty (`/dev/null`).

### 2.3 `describe`

```
<exe> describe
```

- Must finish within 10 s and exit with 0.
- It prints one JSON document to stdout:

```json
{
  "contractVersion": 1,
  "name": "Bonn open data",
  "version": "1.0.0",
  "description": { "en": "House numbers and hydrants from stadtplan.bonn.de", "de": "Hausnummern und Hydranten von stadtplan.bonn.de" },
  "settings": [
    {
      "key": "source_url",
      "label": { "en": "Source URL", "de": "Quell-URL" },
      "type": "string",
      "required": true,
      "default": "https://stadtplan.bonn.de/geojson?OD=171"
    },
    { "key": "api_key", "label": "API key", "type": "secret", "required": false }
  ]
}
```

| Field | Rules |
|---|---|
| `contractVersion` | Required. The app accepts `1`. Anything else makes the collector `invalid`. |
| `name` | Required, 1–100 characters (plain string). |
| `version` | Required, free-form, at most 50 characters. |
| `description` | Optional. A localized text: at most 1000 characters per language. |
| `settings[].key` | `^[a-z][a-z0-9_]{0,63}$`, unique within the collector. |
| `settings[].label` | Required, a localized text. |
| `settings[].description` | Optional, a localized text. |
| `settings[].type` | `string` \| `secret` \| `integer` \| `number` \| `boolean` |
| `settings[].required` | Defaults to `false`. A run won't start while a required setting has no value and no default. |
| `settings[].default` | Optional, must match `type`. Not allowed for `secret`. |

A *localized text* is either a string or an object that maps a language code
(`^[a-z]{2}$`) to a string, with at least one entry (D34).

### 2.4 `collect`

```
<exe> collect
```

- The exit code is `0` on success. Any other exit code fails the run, and
  **nothing is applied**.
- **stdout** holds exactly one JSON document, read fully after the process exits:

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

Address fields: the same rules as the existing `Street.Validate` and
`HouseNumber.Validate`. `street`, `district`, `city`, `country` and `postcode`
are non-empty, `number` > 0, `numberAddition` is optional (`null`, absent, or a
non-empty string), and lat/lon are in range. `streetExternalId` and
`houseNumberExternalId` are optional (1–255 characters each). All rows of one
street must carry the same `streetExternalId` (§3 step 2).

Geo object fields: `group` and `name` are required. `externalId` is optional
(1–255 characters). Lat/lon are required. `address` is optional, but when it's
given, `street/district/city/country` are required and `streetExternalId`,
`houseNumberExternalId`, `number` and `numberAddition` are optional.

- Unknown fields are ignored, which keeps the contract forward-compatible.
- **Duplicates:** the first occurrence of a key wins, and later ones are skipped
  as `duplicate` (D31).
- **stderr** is the log. The app keeps its last 1 MiB in the run.
- **Timeout:** after `COLLECTOR_TIMEOUT` (default 10m), the app kills the whole
  process group (`Setpgid`), and the run fails with `timeout`.
- **Cancel:** the app sends the same kill, and the run is marked `cancelled`.
- **Renames:** without `streetExternalId`, changing a street's name, district,
  city or country is a delete + create, which gives the street a new UUID. With
  it, the street is updated in place. `houseNumberExternalId` does the same for
  a house number's number or addition. A house number whose ID is given can also
  move to another street owned by the same collector.

### 2.5 Versioning

`contractVersion` is part of both outputs. Breaking changes raise it, and
the app may support several versions side by side. Adding optional fields
doesn't change it.

---

## 3. Sync semantics

All writes of one run happen in **one `db.Transaction`**, because of
`SkipDefaultTransaction`. Either the whole sync is committed, or none of it is.

**Keys**

- Street: `streetExternalId` when given (new partial unique index), otherwise
  `(name, district, city, country)` (the existing unique index).
- House number: `houseNumberExternalId` when given (new partial unique index),
  otherwise `(street, number, numberAddition)`. Its unique index is new (D29).
- Geo object: `externalId`, falling back to `(group, name)`.

**Ownership**: a nullable `collector_id` on `streets`, `house_numbers` and
`geo_objects`. `NULL` marks a manual row (created via the API, `/api/bulk` or the UI).

**Steps** (row-level problems are recorded; nothing aborts the run):

1. **Parse and validate.** Invalid rows go to `skipped` with their index and
   reason (D12). Duplicate keys are skipped as `duplicate` (D31).
2. **Group addresses** by street, using `streetExternalId` when it's given and
   the name key otherwise. If rows that share a name key carry different
   `streetExternalId`s, or rows that share a `streetExternalId` carry different
   name keys, the app skips all rows of that street as `inconsistent street` (D44).
3. **Streets.** For each street in the output:
   - Look it up by `external_id` first (when given), then by name key. A match
     by name key on an owned street without an `external_id` stores the new
     one (backfill, D45). A match by name key on a street that has a
     **different** `external_id` is a conflict.
   - Exists and is foreign (`collector_id` is NULL or another collector) →
     **conflict**. The street and all its addresses are skipped (D8).
   - Exists and is owned → update its name key fields (rename), its
     `external_id` and its lat/lon (the medoid, D6). If a rename would clash with
     another street's name key, the street is reported as a conflict and skipped.
   - Doesn't exist → insert it with `collector_id = this`.
4. **House numbers** on streets that are owned or new. Look each one up by
   `external_id` first (when given, across all of this collector's streets),
   then by `(street, number, addition)`, with the same backfill and conflict
   rules as for streets:
   - Matches a house number owned by this collector → update its street,
     number, addition, `postcode`, lat/lon and `external_id`. A renumbering that
     would clash with another house number's key is a conflict.
   - Matches a **manual** house number → **adopt** it: set `collector_id`,
     update it, and report it as `adopted` (D35).
   - Matches a house number owned by **another** collector → **conflict**, skipped.
   - New → insert it as owned.
5. **Geo objects.** Resolve the link:
   1. `address` given → look up the street by `streetExternalId` (when given),
      then by name key (**any owner**, D10). Then look up the house number on
      that street by `houseNumberExternalId` (D49), or by number + addition
      if a number is given.
   2. No reference, or the reference doesn't resolve → nearest house number
      within `COLLECTOR_GEO_MAX_DISTANCE_M`, using the existing earthdistance
      index, and take its street.
   3. Nothing found → skipped.

   Then match by externalId, falling back to `(group, name)`:
   - Matches a foreign row → **conflict**.
   - Matches an owned row → update it.
   - No match → insert it.

   A `(group, name)` or `externalId` clash with a foreign row is also a conflict.
6. **Delete stale rows**, in this order:
   1. Owned geo objects that weren't returned → delete them.
   2. Owned house numbers that weren't returned → delete them, unless a
      **foreign** geo object points to one. That house number is **released**
      instead: `collector_id` is set to NULL (D36).
   3. Owned streets that weren't returned → delete them, unless foreign house
      numbers (manual ones, including those just released) or foreign geo
      objects remain on them. Such a street is **released** instead (D28).
7. **Record the result**: counts per entity type (`created`, `updated`,
   `adopted`, `deleted`, `unchanged`, `skipped`, `conflicts`, `released`) plus
   the first 1000 entries of each list. Then prune runs beyond
   `COLLECTOR_RUN_HISTORY` (D38).

**Locking:** the in-process "one active run" check, the partial unique index
on active runs, and `pg_advisory_xact_lock(hashtext('collector:' || id))`
inside the transaction.

---

## 4. Data model (Migrate, idempotent)

```text
collectors
  id              text primary key           -- filename-derived ID
  path            text not null              -- last seen absolute path
  status          text not null              -- ok | invalid | missing
  error           text                       -- describe/validation error
  name, version   text
  description     jsonb                      -- localized text
  settings_schema jsonb not null default '[]'
  settings        jsonb not null default '{}' -- values incl. secrets (D20)
  discovered_at, updated_at timestamptz

collector_runs
  id              uuid pk default gen_random_uuid()
  collector_id    text not null references collectors on delete cascade
  status          text not null              -- running | succeeded | failed | cancelled
  phase           text                       -- collecting | applying
  triggered_by    text not null              -- principal subject
  started_at, finished_at timestamptz
  exit_code       int
  error           text
  stats           jsonb                      -- counts (step 7)
  issues          jsonb                      -- skipped/conflicts/adopted/released (capped)
  log             text                       -- stderr tail
  unique index collector_runs_active_key on (collector_id) where status = 'running'
  index on (collector_id, started_at desc)   -- history + pruning

streets.collector_id, house_numbers.collector_id, geo_objects.collector_id
  text null references collectors(id) on delete set null, indexed   (D39)

streets.external_id, house_numbers.external_id
  text null, unique index <table>_external_id_key where external_id is not null   (D43)

house_numbers unique index on (street_id, number, coalesce(number_addition, ''))
  -- pre-check reports duplicates like migrateGeoObjectGroupNameKey   (D29)
```

At startup, runs left in `running` state are set to `failed` with the error
`interrupted by restart`. The app has no graceful shutdown today
(`ListenAndServe` without `Shutdown`), so running collector processes die with it.

---

## 5. HTTP API (`internal/api/openapi.yaml`)

Every route needs auth **plus** membership in the admin group (D26). Otherwise it returns `403`
with a clear message, including "API tokens cannot manage collectors".

| Method & path | Purpose | Notable responses |
|---|---|---|
| `GET /api/collectors` | List collectors: metadata, status, schema, settings (secrets as `{set: bool}`), last run summary | 200 |
| `GET /api/collectors/{id}` | One collector | 404 |
| `PUT /api/collectors/{id}/settings` | Replace the settings. For a secret, omitting it keeps the current value and `null` clears it. Validated against the schema. | 400, 404 |
| `POST /api/collectors/reload` | Rescan the folder and re-run `describe` | 200 + list |
| `DELETE /api/collectors/{id}[?deleteData=true]` | Only while the collector is `missing` or `invalid`. Its rows become manual, or are deleted with `deleteData` (D39). | 204, 404, 409 (collector `ok` or running) |
| `POST /api/collectors/{id}/runs` | Start a run | 202 + run; 400 missing required settings; 409 already running, or collector missing/invalid |
| `GET /api/collectors/{id}/runs` | Run history, newest first, with `limit` and `offset` | 200 |
| `GET /api/collector-runs/{runId}` | Run detail, including issues and log | 404 |
| `POST /api/collector-runs/{runId}/cancel` | Cancel a run | 202; 409 if the run isn't running |

Run routes live under `/api/collector-runs/…` rather than
`/api/collectors/runs/…`. With the latter, `ServeMux` would panic: it would
see `GET /api/collectors/{id}/runs` and `GET /api/collectors/runs/{runId}` as
conflicting patterns.

**Changes to existing routes**

- `Street` and `HouseNumber` get the read-only fields `collectorId` and
  `externalId`, which are ignored on input, including `/api/bulk` (D46).
  `GeoObject` gets `collectorId`. Its existing `externalId` stays writable.
- **409 `ErrOwnedByCollector`** (D27, D40), naming the collector:
  - PUT/DELETE on an owned street, house number or geo object; geo object bulk
    delete and group delete/rename when they touch owned rows.
  - DELETE on a manual street that has owned house numbers or geo objects (the
    cascade).
  - DELETE on a manual house number that owned geo objects point to
    (`SET NULL`).
- Still allowed: **creating** manual house numbers and geo objects on owned
  streets (D32).

---

## 6. Configuration (`.env.example` + `docker-compose.yml`)

| Variable | Default | Meaning |
|---|---|---|
| `COLLECTORS_DIR` | `<dir of binary>/collectors` | Folder with the collector executables. If it doesn't exist, the app logs that and the list stays empty. |
| `COLLECTOR_TIMEOUT` | `10m` | Go duration. The process is killed when it's reached. |
| `COLLECTOR_GEO_MAX_DISTANCE_M` | `100` | Max distance in metres for the nearest-house-number fallback. |
| `COLLECTOR_RUN_HISTORY` | `50` | Runs kept per collector. |
| `COLLECTOR_ADMIN_GROUP` | unset | Group whose members may manage collectors, matched exactly against the token's `groups` claim. |

Docker (D41): `docker-compose.yml` mounts `./collectors:/collectors:ro`, which
is the default folder because the binary is at `/address-serv`. Collectors must
be static binaries. `.goreleaser.yml` drops `windows` (D42).

---

## 7. Code layout

```text
internal/collector/          new package
  contract/                  leaf package: describe/collect DTOs, localized text, validation (contract v1)
  discovery.go               folder scan, ID derivation, describe invocation
  exec_unix.go               //go:build unix: minimal env, temp dir, process group, timeout, cancel, stderr tail
  exec_other.go              //go:build !unix: stub that reports "collectors unsupported on this OS"
  runner.go                  async run lifecycle; one active run per collector; cancel registry
  medoid.go                  street coordinate derivation (D6)
internal/store/
  collector.go               CollectorStore: CRUD for collectors and runs, pruning
  collector_sync.go          Sync(ctx, collectorID, input) → stats/issues, in one transaction (§3)
  migrate.go                 new tables and columns, streets.external_id, house-number unique key + duplicate report
  street.go / house_number.go / geo_object.go   collectorId/externalId fields + ErrOwnedByCollector guards
internal/api/
  collector.go               registerCollectorRoutes
  auth.go                    keep claims on the principal; requireCollectorAdmin middleware
  openapi.yaml
cmd/address-serv/main.go     env wiring; discovery at startup; mark interrupted runs
examples/collectors/example/ Go example collector (static binary, fixed sample data) + README with build line
docs/collectors/contract.md  the contract from §2 + JSON Schema
frontend/src/
  api/ApiClient.ts, api/types.ts        collector + run endpoints and types; collectorId/externalId on entities
  hooks/useCollectors.ts, useCollectorRuns.ts   TanStack Query; poll while a run is active
  pages/CollectorsPage.tsx               list, status, reload, run/cancel, history, delete (missing/invalid)
  components/CollectorSettingsDialog.tsx form generated from the schema; localized labels; secret = "set / replace / clear"
  components/CollectorRunDialog.tsx      stats, issues, log
  existing street/house-number/geo pages: "managed by <collector>" badge, edit/delete disabled; surface 409 messages
  locales/en + locales/de                all new strings
```

The nav entry is hidden when `GET /api/collectors` returns 403.

---

## 8. Tests

Following CLAUDE.md: table-driven, `t.Parallel()`, standard `testing` only.

- **Unit (always run):** contract validation (describe and collect, every rule
  in §2, localized text), ID derivation and clashes, medoid, claim path matching
  (string, array, nested, missing), settings validation against the schema, the
  env builder (it must not leak the app env), duplicate handling, street grouping
  consistency.
- **Exec (always run, Unix):** test collectors written as small shell scripts
  in `t.TempDir()`. Covered: success, non-zero exit, invalid JSON, timeout
  kills child processes, cancel, stderr tail truncation.
- **Store (needs `ADDRESS_SERV_TEST_DSN`):** every sync rule in §3: create,
  update, unchanged, delete, foreign street conflict, adopt a manual house
  number, conflict with another collector's house number, rename via
  `streetExternalId` (also when it clashes), geo ref resolution and nearest
  fallback (in and out of range), externalId vs group+name, release of a
  stale house number or street that has foreign dependants, a second
  collector, an invalid row being deleted (D12), rollback on error, the
  house-number unique migration with duplicate reporting, run pruning, and
  interrupted-run recovery.
- **API (needs the DSN):** 403 without the claim and for API tokens, settings
  round-trip with secrets hidden, 409 for a concurrent run, every 409 case
  from §5, collector deletion with and without `deleteData`, the run lifecycle
  up to `succeeded`.
- **Frontend:** `npm run build`, `npm run lint` and `npm run format:check`.

---

## 9. Implementation order

1. Contract doc + DTOs + validation (`internal/collector/contract`, a leaf package so `store` and the runner can both import it) + unit tests.
2. Migration: tables, `collector_id`, `streets.external_id`, house-number unique key + duplicate report.
3. `CollectorStore` + `Sync` + store tests.
4. Exec + discovery + runner + tests.
5. Auth claim check + API routes + `openapi.yaml` + API tests.
6. Ownership guards (409) on existing routes + tests + `openapi.yaml`.
7. Wiring in `main.go`, `.env.example`, `docker-compose.yml`, and dropping Windows from `.goreleaser.yml`.
8. Example collector (Go).
9. Frontend: client, types, hooks, page, dialogs, badges, en + de.
10. Run `golangci-lint`, `govulncheck`, `go mod tidy -diff`, the full test run with
    the DSN, and the frontend checks.

---

## 10. Open questions

None. All questions (Q1–Q23) are answered and recorded as D31–D49.

---

## 11. Implementation notes

Where the implementation departs from, or adds to, the plan above:

- **Contract package.** The contract types live in the leaf package
  `internal/collector/contract`, because both `store` (the sync) and the
  runner (`internal/collector`, which imports `store`) need them.
- **Conflicting entries keep their row.** An output entry that matches one of
  the collector's own rows but can't be applied (a rename onto a taken name, a
  different external ID) is a conflict, and the row stays unchanged instead
  of being deleted as stale. For a street, that includes its house numbers.
  This is documented in the contract.
- **Released rows lose their external ID.** Only collectors' rows carry one, so
  a released row can be adopted again later.
- **Renames within one run.** Swapping two names, or chaining renames, in a
  single run reports conflicts. Every rename is checked against the names as
  they are when it is applied.
- **Settings in the API.** A collector's `settings` holds the values that are
  set, except secrets; `secretsSet` lists the secrets that have a value.
- **Migration lock.** `Migrate` holds a Postgres advisory lock, and its index
  checks look only in the current schema. Concurrent migrations (several
  instances, or parallel tests) raced to create the same extension or index.
- **UI access.** The Collectors tab is hidden when `GET /api/collectors`
  answers 403 (not in the admin group) or 404 (collectors are unsupported on the
  server's OS).
- **Live log.** While a run is collecting, the runner saves the stderr tail to
  the run every 2 s (`SetRunLog`, only while the run is `running`), so
  `GET /api/collector-runs/{runId}` and the UI, which polls running runs, show
  the log before the run ends. `FinishRun` still writes the final log.
- **Test helper.** `internal/store/storetest.OpenIsolated` gives tests a
  migrated database in a schema of their own. Tests that reload collectors
  need it, because a reload marks every collector it doesn't find as missing.
