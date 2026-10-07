# Bundled tools and a one-line installer

Install kindtks and the exact tools its profiles use (kind, kubectl, helm) from the kindtks image with one command, on any host that has Docker and bash. Supersedes `2026-10-07-docker-run-design.md` (run every command through `docker run`), which is merged on `main` but not released.

## Problem

0.5.x needs `kind`, `kubectl`, `helm` and `openssl` on the host at versions nobody checks, and installs through a root-run container that copies into `~/.local`. The docker-run design removed the host tools but made every command a long `docker run` with same-path mounts, and the user still needs kubectl and helm on the host to use the cluster.

## Install

```bash
docker run --rm <image> install-script | bash
docker run --rm <image> install-script | bash -s -- --yes   # no prompts
```

`<image>` is any reference to the kindtks image: `ghcr.io/null-ptr-exception/kindtks:<version>` or a copy in a private registry. Upgrading is the same command with a newer image.

### Finding the image without its name

A container cannot see the reference it was started with unless the Docker socket is mounted. The installer does not need it:

- Every build stamps the image with a unique build ID, as `LABEL io.kindtks.build=<id>` and `ENV KINDTKS_BUILD_ID=<id>`. The ID is `<version>-<git sha>-<UTC build time>`.
- `kindtks install-script` prints the installer with that ID filled in.
- The installer finds the image on the host with `docker images -q --filter label=io.kindtks.build=<id>` (Docker just pulled it to run `install-script`), then `docker create <image id>` (the image's entrypoint is `kindtks`) and `docker cp` the files out, and removes the container.

Spike (2026-10-07): an image pushed to a local registry under another name, with all local copies removed, was found by label after `docker run … install-script | bash`; the copied binary was owned by the user and ran. Labels are part of the image config, so they survive retags, push/pull and registry mirroring. If the image is missing when the script runs, it fails with `kindtks image with build <id> not found locally`.

### Installer steps

1. **Docker**: `docker info` must succeed as the current user; otherwise exit with the reason (daemon not running, or user not allowed to use the socket).
2. **kindtks and its private copies**: replace, as a whole, `~/.local/share/kindtks/{bin,profiles,charts}` with the image's copies (pinned `kind`, `kubectl`, `helm`; profiles; charts), and copy `kindtks` to the destination dir (default `~/.local/bin`). Files are owned by the user (`docker cp` writes as the calling user).
3. **Host tools**: for each of `kind`, `kubectl`, `helm` (versions read from the private copies just installed):
   - not on `PATH`: prompt `Install kubectl v1.33.1 to <dest>? [Y/n]`;
   - on `PATH` and older than bundled (`sort -V`): prompt `kubectl v1.29.3 at /usr/bin/kubectl is older than bundled v1.33.1. Install v1.33.1 to <dest>? [Y/n]`, adding `(<dest> comes first on PATH)` or `(<dest> comes after /usr/bin on PATH; the new copy will not take effect until PATH is changed)`;
   - on `PATH` and same or newer: report only (`kubectl v1.34.0 found (bundled v1.33.1)`).
   Installing writes only into `<dest>`; files elsewhere are never touched.
4. **PATH**: if `<dest>` is not on `PATH`, print the line to add.
5. **Summary**: what was installed, skipped, and how to start (`kindtks list`).

Prompts read from `/dev/tty` (stdin is the script). With no terminal and no `--yes`, optional installs are skipped and the summary names `--yes`.

Flags: `--yes` (answer yes to every prompt), `--no-tools` (skip step 3), `--dest DIR` (default `~/.local/bin`).

## kindtks changes

Relative to `main` (docker-run design):

- **Data dir** `~/.local/share/kindtks` again holds `bin/`, `profiles/`, `charts/`. Profiles dir = `$KINDTKS_PROFILES_DIR` if set (tests, development), else `<data dir>/profiles`. `KINDTKS_CHARTS_DIR` exported to scripts = `$KINDTKS_CHARTS_DIR` if set, else `<data dir>/charts`.
- **Private tools first**: at startup kindtks prepends `<data dir>/bin` to its own `PATH`, so the `REQUIRES` check and profile scripts use the bundled tools.
- **`kindtks tls-cert <dir> <dns-name>...`** (hidden): writes `tls.crt` and `tls.key` (ECDSA P-256, self-signed, 10 years, CN = first name, SANs = all names). gen1 uses it instead of `openssl`; `openssl` was missing from gen1's `REQUIRES`.
- **`kindtks install-script`**: prints the installer for this build. Fails if `KINDTKS_BUILD_ID` is unset (a binary not running from a release image).
- **No arguments**: prints the README (no image substitution).
- **Removed** (from the docker-run design): mount checks (`checkMounts`, `checkConfigVisible`, `dockerReachable`), `KINDTKS_IMAGE` and README substitution, the `docker run` usage section, `test/e2e/docker-run.bats`.
- Unchanged: config format, schemas, `config --schema`, `registries` handling, state dir `~/.local/state/kindtks`, `kindtks value`, profile script interface.

## Image

- Build stages keep the pinned versions and SHA-256 checks of the docker-run design (Go 1.24.13 builder, kind v0.33.0, kubectl v1.33.1, helm v3.21.0; charts pulled with the pinned helm). The docker CLI, bash and gettext are dropped: nothing runs in the image except `kindtks` printing text.
- Final stage `FROM scratch`: `/usr/local/bin/{kindtks,kind,kubectl,helm}`, `/profiles`, `/charts`, the build-ID label and env, `ENTRYPOINT ["kindtks"]`. All binaries are static.
- The build ID is a build arg; CI passes `<tag>-<sha>-<time>`. It defaults to `dev`, and `install-script` refuses `dev` (two local builds would share the label), telling the user to build with `--build-arg BUILD_ID=<unique>`. Tests pass a unique ID.
- linux/amd64 only.

## Compatibility

- One bundled kubectl (v1.33.1) for every profile. gen1 runs Kubernetes 1.24, outside kubectl's one-minor skew policy; this is accepted as tested (all e2e runs pass) and documented.
- Supported hosts: Linux and WSL2 with Docker. macOS: not built or tested.

## Docs and release

- README: Install section = the one-liner, flags, what goes where, upgrade, air-gapped use (push the image to the internal registry; same command with that reference). Prerequisites: Docker and bash.
- Release notes: the one-liner with the release tag.
- 0.6.0, breaking for 0.5.x users: reinstall with the one-liner; `kindtks install` is gone.

## Testing

Unit (Go):
- `tls-cert`: certificate parses, is self-signed, SANs match, key matches.
- data/profiles/charts dirs with and without overrides; tools dir prepended to `PATH` once.
- `install-script` output contains the build ID; errors without it.

Bats unit (`test/unit/installer.bats`), running the installer against fake `docker`, `kind`, `kubectl`, `helm` scripts on `PATH`:
- missing tool + `--yes` installs it; missing tool without terminal and without `--yes` skips it;
- older tool offers replacement and reports PATH order; newer tool is reported only;
- `--no-tools`, `--dest`;
- image not found → error, exit 1.

E2e (`test/e2e/installer.bats`): build the image, push it to a temporary local `registry:2` under another name, remove local copies, run the one-liner with `--yes --dest` into a temporary `HOME`; assert the files exist, are owned by the user, and `kindtks list` shows gen1. Then `gen1.bats` and `registry-hosts.bats` run with the installed data dir layout. `airgap-gen1.bats` and `test/llm/airgap-deploy.bats` switch to the one-liner from the lab registry.

## Out of scope

- macOS and Windows-native builds.
- Per-profile kubectl versions.
- Uninstall command.
- Rewriting profiles in Go.
