# Profile-specific Config with Two-layer JSON Schema

Let each profile declare and validate its own options, alongside kindtks' common options, and expose them to profile scripts without extra tools. First consumer: `gateway.extraHosts` for gen1's Istio Gateway.

## Problem

The config schema is a fixed Go struct (`internal/config.Config`: `images`, `registries`) shared by every profile, with strict decoding. A profile cannot add its own options: anything profile-specific (e.g. extra Istio Gateway hosts for gen1) would have to become a global Go field that other profiles silently ignore. `images` is already de facto profile-specific: its keys (`cilium`, `istio-pilot`, ...) are defined by gen1's script.

## Config file layout

```yaml
registries:                     # common section
  quay.io:
    hosts: |
      server = "https://quay.io"
profiles:
  gen1:                         # profile section
    images:                     # reserved key
      kind-node: kindest/node:v1.24.17
      cilium: quay.io/cilium/cilium:v1.13.10
    gateway:
      extraHosts: ["*.example.net"]
```

- Top level (common): `registries` (unchanged from 0.4.0) and `profiles`. Nothing else.
- `profiles.<name>`: that profile's options. One user file can hold sections for several profiles.
- `images` is a reserved key inside every profile section: a map of string image references. The common layer owns its shape and uses it for private-registry auto-trust and `IMAGE_*` exports; the profile schema declares which image keys exist.
- `kind-node` stays an image key of the profile (it is the profile's Kubernetes version).

## Schemas

Two layers of JSON Schema (draft 2020-12), validated with `github.com/santhosh-tekuri/jsonschema/v6`:

1. **Common schema**, embedded in the binary (`internal/config/schema/common.json`, `go:embed`): top-level object with `registries` and `profiles`, `additionalProperties: false`. `registries` entries: `{hosts: string, auth: {username: string, password: string}}`, no extra keys. `profiles` values: objects whose `images`, if present, is an object of strings.
2. **Profile schema**, `profiles/<name>/config.schema.json`, applied to `profiles.<name>`. It must describe `images` (the allowed image keys) and every profile option. A profile without a schema file accepts only `images` (any keys).

YAML is decoded into generic values (`map[string]any`, `[]any`, scalars) before validation. Go-side checks that JSON Schema can't express stay in Go: `hosts` must be valid TOML; registry names must be safe directory names (existing rules).

Validation errors report the instance path, e.g. `profiles.gen1.gateway.extraHosts[0]: does not match pattern ...`, and which schema failed (common or `<name>`).

## Loading and merging

Layers, lowest to highest precedence:

1. Common defaults (built in; empty today).
2. Profile defaults: `profiles/<name>/config.yaml`. This file contains the **body** of the profile section (e.g. `images: ...`, `gateway: ...`), not `profiles: <name>: ...`.
3. User file from `--config` (full layout).

Merge rules: objects merge key by key, recursively; arrays and scalars from a higher layer replace lower ones. `registries.<reg>` merges per field as in 0.4.0 (falls out of the object rule).

Validation on `create <profile>`:

- The merged document is validated against the common schema.
- `profiles.<profile>` (merged) is validated against that profile's schema.
- Sections for other installed profiles are validated against their own schemas; sections for profiles not installed locally produce a warning (`profiles.foo: profile not installed, ignoring`) so one user file can be shared across machines.

## Exposing options to profile scripts

Before running a profile function, kindtks writes the merged `profiles.<profile>` section as JSON to a temp file (removed afterwards) and exports:

| Variable | Value |
|---|---|
| `KINDTKS_BIN` | absolute path of the running kindtks binary |
| `KINDTKS_PROFILE_CONFIG` | path to the JSON file |
| `IMAGE_<KEY>` | one per `images` entry (as today) |

plus the existing variables (`KINDTKS_KIND_CONFIG`, `KINDTKS_PROFILE_DIR`, ...).

**`kindtks value <dot.path> [--required]`** (hidden command): reads `KINDTKS_PROFILE_CONFIG`, resolves a dot path (object keys only; e.g. `gateway.extraHosts`), and prints:

- scalar → the value on one line (booleans/numbers in JSON form);
- array of scalars → one element per line;
- missing path → nothing, exit 0; with `--required` → error, non-zero exit;
- object or array containing objects → error (scripts query leaves).

No `jq` or other new host dependency.

Private-registry auto-trust reads `profiles.<profile>.images`. `registries` handling is otherwise unchanged from 0.4.0.

## Commands

- `kindtks config <profile>`: prints the merged defaults in the new layout (`profiles: <profile>: ...`).
- `kindtks config <profile> --schema`: prints a combined JSON Schema (common schema with the profile schema placed at `profiles.<profile>`), usable with `# yaml-language-server: $schema=...`.

## gen1

- `profiles/gen1/config.yaml`: current image defaults under `images:`; `gateway.extraHosts: []`.
- `profiles/gen1/config.schema.json`: `images` with exactly the gen1 image keys (all strings, `additionalProperties: false`); `gateway.extraHosts`: array of unique strings matching a hostname or `*.`-prefixed wildcard hostname; no other keys.
- `install.sh`: after applying `gateway.yaml`, for each host from `"$KINDTKS_BIN" value gateway.extraHosts`, add it to the HTTP (port 80) server of Gateway `istio-ingress/kindtks` (`kubectl patch --type=json`, append to `/spec/servers/0/hosts`). HTTPS is unchanged (the `kindtks-tls` certificate covers only the built-in domains).
- `REQUIRES` unchanged.

## Migration

Clean break (internal alpha). A top-level `images` key (or any unknown top-level key) fails with an error; for `images` the message says it moved to `profiles.<name>.images`. The 0.4.0 `registryAuth` hint stays. Update README, gen1 README, and e2e configs (`airgap-gen1.bats`, `registry-hosts.bats`).

## Testing

Unit (Go):

- merge: nested objects, array replacement, three layers.
- common schema: unknown top-level key, top-level `images` hint, bad `registries` entry, non-string image value.
- profile schema: valid config, unknown option, bad `extraHosts` entry, missing schema file (images only).
- other-profile sections: installed → validated; not installed → warning.
- `value`: scalar, array, missing, `--required`, object error.
- env: `KINDTKS_BIN`, `KINDTKS_PROFILE_CONFIG` content, `IMAGE_*` from the profile section.
- `config --schema` output is a valid schema and validates the profile's own defaults.

Bats:

- unit: gen1's `config.yaml` validates against its schema; profile contract tests still pass.
- e2e `gen1.bats`: create with `profiles.gen1.gateway.extraHosts: ["*.example.net"]`; assert the host is in the Gateway's HTTP server and a request with `Host: echo.example.net` reaches the echo service.
- e2e `registry-hosts.bats` and `airgap-gen1.bats`: updated to the new layout, still pass.

All docs, tests, and examples use `example.com` / `example.net` only.

## Out of scope

- Template rendering of profile manifests by kindtks.
- Adding extra hosts to the HTTPS server / certificate.
- JSON Schema for the kind config itself.
