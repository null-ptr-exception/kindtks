# kindtks

An opinionated customization of kind kubernetes with many thanks.

Bootstraps fully configured clusters from pre-defined profiles.

## Install

```bash
docker run --rm \
  -v "$HOME/.local/bin:/.local/bin" \
  -v "$HOME/.local/share:/.local/share" \
  ghcr.io/rophy/kindtks:latest install
```

This copies the `kindtks` binary, profiles, and bundled Helm charts to `~/.local`.

### Prerequisites

- Docker
- [kind](https://kind.sigs.k8s.io/)
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
```

## Profiles

Each profile has its own README with component details and usage instructions.

- **[gen1](profiles/gen1/README.md)** — Cilium, Istio, Vault, Vault Secrets Operator

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
    cilium: registry.internal:5000/quay.io/cilium/cilium:v1.13.10
    cilium-operator: registry.internal:5000/quay.io/cilium/operator:v1.13.10
    istio-pilot: registry.internal:5000/docker.io/istio/pilot:1.16.7
    istio-proxy: registry.internal:5000/docker.io/istio/proxyv2:1.16.7
    vault: registry.internal:5000/hashicorp/vault:2.0.3
    vault-secrets-operator: registry.internal:5000/ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
```

Private registries (anything not docker.io, quay.io, ghcr.io, etc.) are automatically trusted in the Kind node's containerd config, so HTTP registries work without additional setup.
