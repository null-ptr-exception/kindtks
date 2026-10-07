# Prompted and checked registry credentials

Let users keep registry passwords out of config files (`auth: prompt`), and catch wrong credentials before a cluster is created by checking them against the registry the way `docker login` does.

## Problem

`registries.<reg>.auth` takes a username and password written in the config file. Keeping them out of the file today means templating it (`--config <(kindtks envsubst < file)`), which is not intuitive. Wrong credentials surface only as image pull errors minutes into `kindtks create`.

## Config

```yaml
registries:
  registry.example.com:
    auth: prompt                      # ask on the terminal at create time
  mirror.example.com:5000:
    auth:                             # as today
      username: svc-account
      password: secret-token
```

- `auth` is either `{username, password}` (unchanged) or the exact string `prompt`. Any other string fails schema validation.
- Merging is unchanged: objects merge key by key, scalars replace, so `prompt` in the user file replaces a lower layer's object and vice versa.
- `kindtks config <profile>` and `delete` never prompt (delete does not read `--config`).

## `kindtks create` flow

All of this happens after the config is resolved and before the kind config is written or any cluster is created.

1. **Credentials from the file** are checked (see Checking). Rejected → `create` stops with the registry's answer. Could not check → print `warning: could not check the credentials for <reg>: <err>; continuing (use --skip-auth-check to skip checking)` and continue. An empty username or password in the file stops `create` with `registries.<reg>.auth has an empty username or password`, before any check and also with `--skip-auth-check`.
2. **Registries with `auth: prompt`**, in sorted order:
   - With no terminal (`/dev/tty` cannot be opened): stop with `registries.<reg>.auth is "prompt", but there is no terminal to ask on`.
   - From the second prompted registry on, when earlier credentials were entered: `Use the same as <previous registry> (<username>)? [Y/n]` (default yes). The offer is the most recently accepted credentials.
   - Otherwise (or on "n"): `  Username for <reg>: ` (echoed) and `  Password for <reg>: ` (not echoed; only trailing CR/LF is trimmed). Empty username or password → ask again (counts as an attempt).
   - Each entry is checked right away:
     - `ok` → next registry;
     - rejected → `rejected by <reg> (<status>)`, ask again; at most 3 attempts per registry, then `create` stops. Rejected reused credentials fall back to asking.
     - could not check → print the error and ask `Continue without checking? [y/N]`; no → stop.
     - registry needs no login → print `<reg> does not require a login; credentials not checked` and keep the credentials.
3. **`--skip-auth-check`** (flag on `create`) skips all checking; prompting still happens.

Prompts are written to and read from `/dev/tty` (so they work when stdin or stdout is redirected); status lines (accepted, rejected, notes) go to stderr. Credentials live in memory and end up only in the kind config patch (existing behaviour) and so in each node's containerd config.

## Checking

Same handshake as `docker login`:

1. `GET <endpoint>/v2/` without credentials.
   - `200` → the registry needs no login.
   - A redirect is followed only on this unauthenticated probe, once, from `http` to `https` on the same hostname (port may differ); the rest of the check then uses the redirected `https://host[:port]`. Any other redirect → could not check. The credentialed request never follows redirects.
   - `401` with `WWW-Authenticate: Basic …` → repeat with HTTP basic auth (always sent, even when empty): `200` ok, `401`/`403` rejected.
   - `401` with `WWW-Authenticate: Bearer realm=…,service=…` → `GET <realm>?service=<service>&scope=<scope>…` with HTTP basic auth: `200` ok, `401`/`403` rejected.
   - anything else, or a network/DNS/TLS error → could not check.
2. Scope (bearer only): one `scope=repository:<repo>:pull` per profile image on that registry (`docker.io` images without a namespace get `library/`). No images on that registry → no scope (login only).

Endpoint (scheme, host, TLS verification) follows what containerd will use for that host name:

- The registry's own hosts file is checked first, then the other registries' in sorted order. If a hosts file kindtks writes (explicit `hosts`, or the generated auto-trust file) names a URL whose host:port is this registry name — as `server` or a `[host."…"]` key — use that URL's scheme and host, and skip TLS verification when that section sets `skip_verify = true`.
- Else `docker.io` → `https://registry-1.docker.io`.
- Else → `https://<name>`, verifying TLS.

Requests use a 10-second timeout and honour `HTTPS_PROXY`/`NO_PROXY` (Go's default transport).

Spike (2026-10-08), standard library only: `registry:2` with htpasswd → right password `ok`, wrong `rejected (401)`; Docker Hub token login with a wrong password → `rejected (401)`; a zot pull-through mirror → `no-auth`; unknown host → could not check (DNS); HTTPS against a plain-HTTP port → could not check. Not exercised: correct credentials through token login.

## Code layout

- `internal/config`: `RegistryAuth` gains `Prompt bool`; custom JSON unmarshalling accepts `"prompt"`. `AuthPatch` errors (or is never called) while any `Prompt` is unresolved. Schema: `auth` is `oneOf` the object and `{"const": "prompt"}`. A function returning the check endpoint for a registry name from the config (parsing the hosts TOML), and the scopes for a registry from the profile images.
- `internal/regauth` (new): `Check(ctx, endpoint, username, password, scopes) (Result, error)` with `Result` one of `OK`, `Rejected(status)`, `NoAuth`; an error means could not check.
- `internal/cmd`: the prompt flow, taking an injected terminal (reader/writer, password reader) and checker so it is unit-testable; `create` wires `/dev/tty`, `golang.org/x/term` for hidden input, and `--skip-auth-check`.
- New dependency: `golang.org/x/term` v0.40.0 (last release with `go 1.24`), with `golang.org/x/sys` v0.41.0.

## Testing

- `internal/regauth`: `httptest` servers for basic ok/rejected, bearer ok/rejected (realm on the same test server, scope passed through), no auth, unexpected status, connection refused, TLS to plain HTTP.
- `internal/config`: schema accepts `prompt`, rejects other strings; `prompt` survives merge and typed decoding; endpoint selection (explicit hosts, generated auto-trust, docker.io, default https); scopes from images.
- `internal/cmd` prompt flow with a fake terminal and fake checker: single registry ok; rejected then ok; three rejections → error; reuse yes/no; reused rejected → asks; empty input; could-not-check continue/stop (prompted) and warning (file); empty file credentials; no terminal; no-auth note; `--skip-auth-check`; file credentials rejected → error before prompting.
- E2e (`test/e2e/registry-auth.bats`): a local `registry:2` with htpasswd on a random port and a test profile whose `create()` only prints the generated kind config (no cluster is created). Cases: wrong credentials in the file → `create` fails before the profile runs; `auth: prompt` answered through a pseudo-terminal (`script -qec … /dev/null` fed on stdin) with a wrong then a right password → one rejection message, and the printed kind config contains the auth patch.
- README: `auth: prompt`, checking, `--skip-auth-check`.

## Out of scope

- Cloud-registry credential helpers (ECR, GCR) and `docker login` credential stores.
- Checking registries that have no `auth`.
- Keyring or caching of prompted credentials between runs.
