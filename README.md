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

## Custom Image Registry

Override image references with a config file to pull from a private or air-gapped registry:

```bash
kindtks config gen1 > my-config.yaml
# Edit image refs to point at your registry
kindtks create gen1 --config my-config.yaml
```

Example config for an air-gapped registry:

```yaml
images:
    kind-node: registry.internal:5000/kindest/node:v1.24.17
    cilium: registry.internal:5000/quay.io/cilium/cilium:v1.13.10
    cilium-operator: registry.internal:5000/quay.io/cilium/operator-generic:v1.13.10
    istio-pilot: registry.internal:5000/docker.io/istio/pilot:1.16.7
    istio-proxy: registry.internal:5000/docker.io/istio/proxyv2:1.16.7
    vault: registry.internal:5000/hashicorp/vault:2.0.3
    vault-secrets-operator: registry.internal:5000/ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
```

Private registries are automatically trusted as insecure (HTTP) in the Kind node's containerd config. The following well-known registries are excluded from this auto-trust: docker.io, quay.io, ghcr.io, gcr.io, registry.k8s.io, k8s.gcr.io, mcr.microsoft.com, public.ecr.aws. Everything else is treated as private.

### Registry Authentication

If your registry requires authentication, add a `registryAuth` section to the config:

```yaml
images:
    cilium: registry.corp.com/cilium/cilium:v1.13.10
registryAuth:
    registry.corp.com:
        username: svc-account
        password: secret-token
```

Credentials are injected into the Kind node's containerd config. This works independently of insecure registry auto-trust — authenticated HTTPS registries are supported.

Note: `kind` pulls the node image from the local Docker daemon, not from inside the cluster. In an air-gapped environment, pre-pull it so it's available locally:

```bash
docker pull registry.internal:5000/kindest/node:v1.24.17
```
