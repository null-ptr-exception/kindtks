# kindtks

An opinionated customization of kind kubernetes with many thanks.

Bootstraps fully configured clusters from pre-defined profiles.

## Install

```bash
docker run --rm \
  -v "$HOME/.local/bin:/.local/bin" \
  -v "$HOME/.local/share:/.local/share" \
  ghcr.io/null-ptr-exception/kindtks:latest install
```

This copies the `kindtks` binary, profiles, and bundled Helm charts to `~/.local`. Make sure `~/.local/bin` is on your `$PATH`.

### Prerequisites

- Docker
- [kind](https://kind.sigs.k8s.io/) (v0.17+)
- kubectl
- helm (for profiles that use Helm charts)

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

After creation, the kubectl context is set to `kind-<profile>` (e.g. `kind-gen1`).

## Profiles

Each profile has its own documentation accessible via `kindtks help <profile>`.

- **gen1** — Cilium, Istio, Vault, Vault Secrets Operator (`kindtks help gen1`)

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

Profile scripts read their options with `"$KINDTKS_BIN" value <dot.path>` (lists print one item per line).

## Custom Image Registry

Override image references with a config file to pull from a private or air-gapped registry:

```bash
kindtks config gen1 > my-config.yaml
# Edit profiles.gen1.images to point at your registry
kindtks create gen1 --config my-config.yaml
```

Example config for an air-gapped registry:

```yaml
profiles:
    gen1:
        images:
            kind-node: registry.internal:5000/kindest/node:v1.24.17
            cilium: registry.internal:5000/quay.io/cilium/cilium:v1.13.10
            cilium-operator: registry.internal:5000/quay.io/cilium/operator-generic:v1.13.10
            istio-pilot: registry.internal:5000/docker.io/istio/pilot:1.16.7
            istio-proxy: registry.internal:5000/docker.io/istio/proxyv2:1.16.7
            vault: registry.internal:5000/hashicorp/vault:2.0.3
            vault-secrets-operator: registry.internal:5000/ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
```

Private registries are automatically trusted as insecure (HTTP): kindtks writes a containerd `hosts.toml` for each one and mounts it into every Kind node. The following well-known registries are excluded from this auto-trust: docker.io, quay.io, ghcr.io, gcr.io, registry.k8s.io, k8s.gcr.io, mcr.microsoft.com, public.ecr.aws. Everything else is treated as private.

### Per-registry settings

The `registries` section configures containerd per registry:

- `hosts`: raw content of containerd's [`hosts.toml`](https://github.com/containerd/containerd/blob/main/docs/hosts.md) for that registry. Replaces the auto-trust config for a private registry.
- `auth`: username and password, for registries that need authentication (HTTP or HTTPS).

Migrating from 0.3.0: top-level `registryAuth` was replaced by `registries.<registry>.auth`.
Migrating from 0.4.0: top-level `images` moved to `profiles.<profile>.images`.

Changing `registries` requires re-creating the cluster: the host files are only written and mounted at `create` time. `auth` applies to the host containerd actually connects to, so for a mirror needing credentials, put `auth` under the mirror's host, not the upstream registry.

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

Host files are written to `~/.local/state/kindtks/<profile>/certs.d/` (`$XDG_STATE_HOME` overrides `~/.local/state`) and removed by `kindtks delete`. The mirror must be reachable from the Kind nodes (e.g. a container on the `kind` Docker network).

Note: `kind` pulls the node image from the local Docker daemon, not from inside the cluster. In an air-gapped environment, pre-pull it so it's available locally:

```bash
docker pull registry.internal:5000/kindest/node:v1.24.17
```
