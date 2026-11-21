# Example collector

A reference implementation of the [collector contract](../../../docs/collectors/contract.md).
It returns fixed sample data: two streets with house numbers, and
optionally two hydrants. One hydrant is placed at an address, and the other
is attached to the nearest house number by address-serv.

It uses only the Go standard library, so you can copy it as a starting
point for a collector that lives outside this repository.

## Try it

```sh
CGO_ENABLED=0 go build -o collectors/example ./examples/collectors/example
COLLECTORS_DIR=./collectors go run ./cmd/address-serv
```

Then open *Collectors* in the web UI, configure the settings, and run it.
Opening the page requires the claim configured in `COLLECTOR_ADMIN_CLAIM`
and `COLLECTOR_ADMIN_VALUE`.

You can also run the collector by hand:

```sh
./collectors/example describe
COLLECTOR_SETTING_COUNT=5 ./collectors/example collect
```

## Settings

| Key | Type | Default | Meaning |
|---|---|---|---|
| `city` | string | `Exampletown` | City of the sample addresses |
| `count` | integer | `3` | House numbers per street, 1–50 |
| `with_hydrants` | boolean | `true` | Whether to return the sample hydrants |

In the Docker image, which has no shell or interpreters, a collector has
to be a static binary like this one. Build it with `GOOS=linux` and the
image's architecture, then put it in the folder mounted at `/collectors`.
