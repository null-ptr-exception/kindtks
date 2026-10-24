# kindtks

An opinionated customization of kind kubernetes with many thanks.

Bootstraps fully configured clusters from pre-defined profiles.

## Install

Requires Docker and bash: Linux or WSL2, or macOS on Apple Silicon with Colima (see [macOS](#macos)).

```bash
docker run --rm ghcr.io/null-ptr-exception/kindtks:latest install-script | bash
```

The installer copies `kindtks` to `~/.local/bin`, and the pinned `kind`, `kubectl` and `helm`, the profiles and the Helm charts to `~/.local/share/kindtks`; kindtks always runs profiles with these. It then offers the bundled `kind`, `kubectl` and `helm` for `~/.local/bin` where yours are missing or older. It writes nowhere else (a symlink there is replaced, not written through).

Options, passed as `| bash -s -- <options>`:

| Option | Effect |
|---|---|
| `--yes` | answer yes to every prompt; also works without a terminal (without `--yes`, prompts are skipped there) |
| `--no-tools` | install only kindtks and its private copies |
| `--dest DIR` | install kindtks, and any tools you accept, to `DIR` instead of `~/.local/bin` |

Air-gapped hosts: push the image to your registry and run the same command with that reference. The installer finds the pulled image by its build label, whatever its name.

Bundled versions (Linux amd64, macOS arm64): kind v0.33.0, kubectl v1.33.1, helm v3.21.0. kubectl 1.33 is outside Kubernetes' version-skew policy for gen1's Kubernetes 1.24; this combination is tested with gen1.

### macOS

Apple Silicon only, with [Colima](https://github.com/abiosoft/colima) as the Docker runtime:

```bash
brew install colima docker
colima start --cpu 4 --memory 4   # gen1 uses about 2.2 GiB; Colima's default 2 GiB is too small
```

Then run the installer above. With zsh, put the PATH line it prints in `~/.zprofile`.

`*.kindtks.localhost` names resolve in browsers and curl only; other programs (for example Python, Go and Java clients, `ping`, `ssh`) report an unknown host, because macOS resolves only `localhost` itself. Add one `/etc/hosts` line per name you need:

```bash
echo '127.0.0.1 vault.kindtks.localhost' | sudo tee -a /etc/hosts
```

For all names at once (not tested with kindtks), run dnsmasq (from brew) answering `*.localhost` with `127.0.0.1`, and point `/etc/resolver/localhost` at it.

### Upgrading

- Run the installer again with a newer image. It replaces `~/.local/share/kindtks` as a whole; keep your own profiles elsewhere and point `KINDTKS_PROFILES_DIR` at them.
- From 0.5.x: `kindtks install` is gone; use the installer above.
- From 0.4.x: top-level `images` moved to `profiles.<profile>.images`.
- From 0.3.x: top-level `registryAuth` was replaced by `registries.<registry>.auth`.

## Usage

```bash
# List available profiles
kindtks list

# Show a profile's default config
kindtks config gen1

# Create a cluster
kindtks create gen1

# Delete a cluster
kindtks delete gen1

# View full docs for a profile
kindtks help gen1
```

After creation, the kubectl context is set to `kind-<cluster>` (e.g. `kind-gen1`).

## Profiles

Each profile has its own documentation accessible via `kindtks help <profile>`.

- **gen1** — Cilium, Istio, Vault, Vault Secrets Operator (`kindtks help gen1`)

A profile is a directory with an `install.sh` defining `create` and `delete`; it reads its options with `"$KINDTKS_BIN" value <dot.path>` (lists print one item per line).

## Configuration

`kindtks create <profile> --config <file>` merges `<file>` over the profile's defaults (`kindtks config <profile>` prints them). Objects merge key by key; lists and scalars replace.

`kindtks delete <profile>` also validates the profile's shipped defaults, but does not take `--config`.

```yaml
registries:            # common to all profiles (see Custom Image Registry)
  ...
profiles:
  gen1:                # options of the gen1 profile
    images:
      cilium: quay.io/cilium/cilium:v1.13.10
    gateway:
      extraHosts: ["*.example.net"]
```

One file can hold sections for several profiles; sections for profiles not installed are ignored with a warning. Each profile validates its section with its own JSON Schema (`profiles/<name>/config.schema.json`), written against JSON Schema 2020-12 with format assertion enabled (e.g. `"format": "hostname"`). For editor completion:

```bash
kindtks config gen1 --schema > kindtks.schema.json
# then in your config file:
# yaml-language-server: $schema=./kindtks.schema.json
```

## Custom Image Registry

Override image references with a config file to pull from a private or air-gapped registry:

```bash
kindtks config gen1 > my-config.yaml
# Edit profiles.gen1.images to point at your registry
kindtks create gen1 --config my-config.yaml
```

Example config for an air-gapped registry (use whatever paths your mirror has):

```yaml
profiles:
    gen1:
        images:
            kind-node: registry.internal:5000/kindest/node:v1.24.17
            cilium: registry.internal:5000/quay.io/cilium/cilium:v1.13.10
            cilium-operator: registry.internal:5000/quay.io/cilium/operator-generic:v1.13.10
            istio-pilot: registry.internal:5000/docker.io/istio/pilot:1.16.7
            istio-proxy: registry.internal:5000/docker.io/istio/proxyv2:1.16.7
            vault: registry.internal:5000/docker.io/hashicorp/vault:2.0.3
            vault-secrets-operator: registry.internal:5000/ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
```

Private registries used by the profile's images are trusted as insecure (HTTP): kindtks writes a containerd `hosts.toml` for each one and mounts it into every Kind node. A registry counts as private unless it is one of docker.io (also index.docker.io, registry-1.docker.io), quay.io, ghcr.io, gcr.io, registry.k8s.io, k8s.gcr.io, mcr.microsoft.com, public.ecr.aws.

Your own workloads' images need the same: point them at your registry (`registries` settings, including `auth`, apply to them too).

The `kind-node` image is different: kind pulls it with the host's Docker, so `registries` settings do not apply to it. The host needs access to that registry itself (`docker login`, and `insecure-registries` in Docker's `daemon.json` for plain HTTP), or the image pulled beforehand.

### Per-registry settings

The `registries` section configures containerd in the Kind nodes, per registry:

- `hosts`: raw content of containerd's [`hosts.toml`](https://github.com/containerd/containerd/blob/main/docs/hosts.md) for that registry. Replaces the auto-trust config for a private registry.
- `auth`: `{username, password}`, or `prompt` to be asked at `kindtks create`. Applies to the host containerd actually connects to, so for a mirror needing credentials, put `auth` under the mirror's host, not the upstream registry.

Changing `registries` requires re-creating the cluster: the settings are only applied at `create`. Host files are written to `~/.local/state/kindtks/<profile>/certs.d/` and removed by `kindtks delete`.

Authentication:

```yaml
profiles:
    gen1:
        images:
            cilium: registry.example.com/cilium/cilium:v1.13.10
registries:
    registry.example.com:
        auth:
            username: svc-account
            password: secret-token
```

To keep the password out of the file, use `auth: prompt`. `kindtks create` then asks for the username and password on the terminal (from the second such registry on, it offers to reuse the previous one) and fails when there is no terminal:

```yaml
registries:
    registry.example.com:
        auth: prompt
```

Before creating anything, `kindtks create` checks every registry's credentials the way `docker login` does:

- prompted credentials that are rejected are asked for again, up to three times;
- rejected credentials from the file stop `create`;
- credentials from the file that cannot be checked (for example, the registry is unreachable from the host) only print a warning.

`--skip-auth-check` turns the check off.

Pull-through mirror: images keep their normal names and containerd pulls them through the mirror, falling back to the upstream registry if the mirror is down. An override config only needs the `registries` section:

```yaml
registries:
    quay.io:
        hosts: |
            server = "https://quay.io"
            [host."http://mirror:5000/v2/quay.io"]
              capabilities = ["pull", "resolve"]
              override_path = true
```

The mirror must be reachable from the Kind nodes (e.g. a container on the `kind` Docker network).

## Releasing

`VERSION` holds the version; `kindtks --version` prints it. On `main` it is the next version with `-dev` (e.g. `0.8.0-dev`), which never releases.

1. Commit `VERSION` set to the release (`0.7.0`, or `0.7.0-rc1` for a pre-release) together with its notes in `CHANGELOG/0.7.0.md` (pre-releases use the notes of their final version), and push it to `main`. The Release workflow tests, publishes the image (`latest` too, except for `-rcN`), then creates the tag and the GitHub release with those notes. A version that already has a tag is skipped; a release without notes fails before anything is built.
2. Commit `VERSION` set to the next `-dev` version (`0.8.0-dev`).
