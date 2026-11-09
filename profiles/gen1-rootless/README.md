# gen1-rootless

gen1 for [rootless Docker](https://docs.docker.com/engine/security/rootless/): the same cluster as gen1, without Cilium. Pod networking comes from kind's default CNI (kindnet). Istio, Vault, Vault Secrets Operator, the per-DC StorageClasses, the Gateway and the ports are the same as in gen1; see `kindtks help gen1` for how to use them. Replace `gen1` with `gen1-rootless` in commands, node names and config keys (`profiles.gen1-rootless`).

| Component | Version |
|---|---|
| Kubernetes | 1.24.17 |
| CNI | kindnet (from the kind node image) |
| Istio | 1.16.7 |
| Vault | 0.34.0 (chart) / 2.0.3 (image) |
| Vault Secrets Operator | 2.7.0 (chart) / 1.26.0 (app) |

## Differences from gen1

- **No NetworkPolicy enforcement.** kindnet in this node image does not enforce NetworkPolicy; the API accepts them, but they have no effect. CiliumNetworkPolicy and other Cilium resources do not exist.
- **Client source IP.** With rootless Docker's default port driver, pods see the client IP as a gateway address, not the real client.
- gen1 and gen1-rootless use the same host ports (30080, 30443), so only one of them can run at a time.

## Requirements

Rootless Docker 20.10 or later, and on the host (from [kind's rootless guide](https://kind.sigs.k8s.io/docs/user/rootless/)):

- cgroup v2, with the `cpu` controller delegated to user services (systemd 252 and later do this by default);
- the kernel modules `ip_tables`, `iptable_nat`, `ip6_tables` and `ip6table_nat` loaded.

Minimum 6 GB RAM recommended (3 nodes).

## Quick Start

```bash
kindtks create gen1-rootless
```

Afterwards the kubectl context is `kind-gen1-rootless`.

## Config

```bash
kindtks config gen1-rootless
```

```yaml
profiles:
    gen1-rootless:
        images:
            kind-node: kindest/node:v1.24.17
            istio-pilot: docker.io/istio/pilot:1.16.7
            istio-proxy: docker.io/istio/proxyv2:1.16.7
            vault: docker.io/hashicorp/vault:2.0.3
            vault-secrets-operator: ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
        gateway:
            extraHosts: []
```
