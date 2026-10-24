# macOS support (Apple Silicon, Colima)

Install and run kindtks on Apple Silicon Macs, with Colima as the Docker runtime.

## Scope

- Supported: macOS on Apple Silicon (`darwin/arm64`), Docker from Colima.
- Not supported: Intel Macs (no test machine), Docker Desktop, OrbStack. kindtks does not check which runtime gives `docker`; the docs name Colima only.
- Side effect: the image becomes multi-arch (`linux/amd64`, `linux/arm64`), because Colima on Apple Silicon runs an arm64 Linux VM and `docker run … install-script` must run natively there. Linux arm64 hosts are not tested and not documented as supported.

## Spike (2026-10-24)

MacBook Air M1, macOS 15.7.9, Colima 0.10.3 (4 CPU / 4 GiB), docker CLI 29.9.2:

- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build` runs unchanged (the Go linker signs it ad hoc).
- Upstream darwin/arm64 kind v0.33.0, kubectl v1.33.1, helm v3.21.0 run.
- `kindtks create gen1`: all 18 pods Running; the 3 nodes use about 2.2 GiB; `curl` to `vault.kindtks.localhost` on 30080 and 30443 → 200.
- `create` with a `registries` config: the generated `hosts.toml` (under `~/.local/state/kindtks`) is mounted in all 3 nodes, through Colima's home mount.
- `/bin/bash` 3.2.57, BSD `sort -V` and `grep -o 'v[0-9]\+…'` give the results the installer needs.
- `*.kindtks.localhost` does not resolve in the macOS system resolver (`ping`: unknown host). curl and browsers resolve `.localhost` themselves.

Not exercised: the installer end to end, prompted registry credentials on the Mac terminal.

## Image

- One Dockerfile, built for `linux/amd64` and `linux/arm64` with buildx. The Go, tools and charts stages run on `$BUILDPLATFORM` (Go cross-compiles with `GOOS`/`GOARCH`), so no QEMU is needed; the final stage stays `scratch`.
- `/usr/local/bin/{kindtks,kind,kubectl,helm}`: Linux binaries for the image's architecture (as today).
- `/dist/darwin-arm64/{kindtks,kind,kubectl,helm}`: macOS binaries, in both image variants.
- Every download is pinned with its SHA-256: kind, kubectl, helm for `linux-amd64`, `linux-arm64`, `darwin-arm64`. Versions stay kind v0.33.0, kubectl v1.33.1, helm v3.21.0.
- Charts are pulled once (architecture-independent) in a `$BUILDPLATFORM` stage.
- `BUILD_ID` label and env are the same in both variants.

## Installer

- Selects the source directory by `uname -s`/`uname -m`:
  - `Linux` → `/usr/local/bin` (as today).
  - `Darwin` + `arm64` → `/dist/darwin-arm64`.
  - `Darwin` + other → `error: Intel Macs are not supported`.
- Everything else (destinations, prompts, tool offers, PATH note) is unchanged.
- Test files use portable commands: no `head -n -1`, `stat -c`.

## kindtks

No code change expected. `create` output on macOS is unchanged; the README notes the resolver behaviour.

## CI and release

- `ci.yml`: add a `macos-latest` job: `go test ./...`, `bats test/unit/installer.bats` (bats-core from brew). The existing Linux job builds the image for both platforms (no push).
- `release.yml`: `docker/setup-buildx-action`, `platforms: linux/amd64,linux/arm64`. Attestation covers the index digest.
- GitHub macOS runners have no Docker, so e2e on macOS is manual on the Mac before each release: `test/e2e/installer.bats` (image built there under Colima) and `kindtks create gen1` from the installed copy. Release flow: tag an `-rc`, test on the Mac, tag the release.

## Docs

README Install section:

- Requirements: Linux or WSL2 with Docker, or macOS on Apple Silicon with Colima.
- macOS setup: `brew install colima docker`, `colima start --cpu 4 --memory 4` (gen1 needs about 2.2 GiB; Colima's default of 2 GiB is too small).
- `*.kindtks.localhost` on macOS works in browsers and curl; other tools need an `/etc/hosts` entry.

## Out of scope

- Intel Macs, Docker Desktop, OrbStack, Linux arm64 as a documented host.
- Homebrew formula, notarization.
