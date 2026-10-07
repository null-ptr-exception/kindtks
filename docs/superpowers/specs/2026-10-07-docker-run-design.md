# Run kindtks from its Docker image

Drop the host install step. kindtks, its profiles, charts, and every tool the profiles need ship in the image; the host needs only Docker. Users run each command with `docker run`.

## Problem

Today `docker run … kindtks install` copies the binary to `~/.local/bin` and profiles/charts to `~/.local/share/kindtks`. The host then also needs `kind`, `kubectl`, `helm`, and `envsubst` (gen1's `REQUIRES` and `install.sh`). Upgrading means re-running install; the copied profiles can drift from the binary; `install` runs as root and leaves root-owned files.

## Spike result (2026-10-07)

A throwaway image (alpine, docker-cli, kind v0.33.0, kubectl, helm, gettext) created and deleted a two-node kind cluster through the command below, with a `--config` file routing pulls through a local pull-through mirror:

- kubeconfig and `~/.local/state/kindtks/<profile>/certs.d` were owned by the host user;
- host `kubectl` used the written kubeconfig;
- the certs.d mount reached both nodes and pulls went through the mirror;
- `delete` removed the cluster, state dir, and kubeconfig entry.

Found: `-v` creates a missing host directory as root, which kindtks then cannot write. `--mount type=bind` fails instead with "bind source path does not exist".

## How users run it

```bash
mkdir -p ~/.kube ~/.local/state/kindtks
docker run --rm --network host \
  --user "$(id -u):$(id -g)" --group-add "$(stat -c %g /var/run/docker.sock)" \
  --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
  -e HOME="$HOME" \
  --mount type=bind,source="$HOME/.kube",target="$HOME/.kube" \
  --mount type=bind,source="$HOME/.local/state/kindtks",target="$HOME/.local/state/kindtks" \
  --mount type=bind,source="$PWD",target="$PWD",readonly -w "$PWD" \
  ghcr.io/null-ptr-exception/kindtks:<version> create gen1 --config my-config.yaml
```

Why each part is there:

| Part | Reason |
|---|---|
| `--network host` | profile scripts run kubectl/helm against the API server on `127.0.0.1` |
| docker socket | kind creates node containers on the host daemon |
| `--user`, `--group-add` | written files belong to the user; the user can use the socket |
| `-e HOME="$HOME"` + same-path mounts | kind mounts the state dir into nodes by its **host** path, so the path must be identical inside and outside; kubeconfig lands in the real `~/.kube` |
| `$PWD` mount | relative `--config` paths work; a config elsewhere needs its own mount. Read-only is safe even when `$PWD` is `$HOME`: Docker mounts the deeper `~/.kube` and state targets over it |
| `--mount` instead of `-v` | a missing host dir fails loudly instead of being created as root |

No wrapper script or alias is shipped. Scope is Linux with the default socket at `/var/run/docker.sock`.

## Image

Every tool and base image is pinned to an exact version; downloads are checked against a SHA-256 written in the Dockerfile; versions live in `ARG`s at the top of the Dockerfile.

| Item | Pin | Source |
|---|---|---|
| builder base | `golang:1.24.13@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804` | matches `go 1.24.13` in go.mod |
| runtime and charts-stage base | `alpine:3.20.10@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc` | |
| bash | apk `bash=5.2.26-r0` | alpine 3.20 |
| envsubst | apk `gettext=0.22.5-r0` | alpine 3.20 |
| docker CLI | 29.2.1 static, `995b1d0b51e96d551a3b49c552c0170bc6ce9f8b9e0866b8c15bbc67d1cf93a3` | download.docker.com; same version as the dev host daemon |
| kind | v0.33.0, `aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d` | spike version |
| kubectl | v1.33.1, `5de4e9f2266738fd112b721265a0c1cd7f4e5208b670f811861f699474a100a3` | version gen1 e2e passes with on the dev host |
| helm | v3.21.0, `0093eb572e3d2380f094df162ddb525e219249de88957afe24cfbb19632acd36` | same; also used by the charts stage instead of apk `helm` |

Alpine keeps only the latest revision of a package per branch, so a pinned apk version can disappear and break the build. That failure is the intended signal to bump the pin; it is not worked around.

- Only linux/amd64 is built (as today); the checksums are for amd64.
- Profiles at `/profiles`, charts at `/charts` (as today).
- `ENV KINDTKS_PROFILES_DIR=/profiles KINDTKS_CHARTS_DIR=/charts KINDTKS_IMAGE=ghcr.io/null-ptr-exception/kindtks:<version>`. `KINDTKS_IMAGE` comes from the `VERSION` build arg and marks "running in the image".

## kindtks changes

- **Directories.** Profiles dir = `$KINDTKS_PROFILES_DIR`; if unset, kindtks errors and says to run it via the image (`docker run --rm <image>`). `KINDTKS_CHARTS_DIR` is not read by kindtks; profile scripts inherit it from the environment. `~/.local/share/kindtks` and `KINDTKS_DATA_DIR` go away. State dir unchanged (`$XDG_STATE_HOME/kindtks`, default `~/.local/state/kindtks`).
- **No arguments.** `kindtks` with no arguments prints the project README (already embedded via `SetProjectReadme`), whose first section is the run command above with the tag of this build in place of `<version>` (taken from `KINDTKS_IMAGE`; outside the image, `latest`).
- **Mount checks.** When `KINDTKS_IMAGE` is set, `create` and `delete` first check, and on failure print what is missing plus the run command:
  - `HOME` is absolute and not `/`;
  - the docker socket is reachable (`docker version` succeeds);
  - `$HOME/.kube` and the state dir exist and are writable.
  A missing `--config` file inside the image gets a hint that its directory must be mounted.
- **`install` removed**, with its copy helpers and the root command's install hint.
- **REQUIRES check** stays (it now runs against the image's tools, and still guards direct runs of a host-built binary).
- Running a host-built binary directly (development, bats tests) still works when the two env vars are set.

## Air-gapped hosts

Push the kindtks image to the internal registry and use that reference in the run command. The kind-node image is pulled by the host daemon through the socket, as today. No install step.

## Docs and release

- README: replace Install/Prerequisites with the run command and the table above, and state that later examples write `kindtks …` as short for that command. gen1 README keeps the short form and links to the top-level README.
- Release workflow notes: replace the `install` snippet with the run command.
- 0.6.0, breaking: `install` gone; `~/.local/bin/kindtks` and `~/.local/share/kindtks` are no longer used (users may delete them).

## Testing

Unit (Go):
- profiles/charts dirs from env; error when unset.
- no-arg output contains the run command with the version tag.
- mount checks: each failure (HOME `/`, unwritable kube dir, unwritable state dir) produces its message; checks skipped when `KINDTKS_IMAGE` unset.
- `install` command absent.

Bats:
- `gen1.bats`, `registry-hosts.bats`: set `KINDTKS_PROFILES_DIR`/`KINDTKS_CHARTS_DIR` instead of copying into `~/.local/share` (still a host-built binary, keeping them fast to iterate).
- New `test/e2e/docker-run.bats`: build the image, run `create`/`delete` of the `example` profile with the documented command, assert host ownership of kubeconfig and state dir, and that a missing `~/.kube` fails with the mount hint.
- `airgap-gen1.bats`: replace install with the run command using the image from the air-gap registry.

## Out of scope

- macOS / Docker Desktop / rootless Docker socket paths.
- Custom `KUBECONFIG` locations.
- User-supplied profiles from the host (possible later via mounting a dir and setting `KINDTKS_PROFILES_DIR`).
- Multi-arch images.
