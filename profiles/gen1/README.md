# gen1

Single-node cluster with Cilium CNI, Istio service mesh, Vault (dev mode), and Vault Secrets Operator.

| Component | Version |
|---|---|
| Kubernetes | 1.24.17 |
| Cilium | 1.13.10 |
| Istio | 1.16.7 |
| Vault | 0.34.0 (chart) |
| Vault Secrets Operator | 2.7.0 (chart) / 1.26.0 (app) |

## Config

```bash
kindtks config gen1
```

```yaml
images:
    cilium: quay.io/cilium/cilium:v1.13.10
    cilium-operator: quay.io/cilium/operator:v1.13.10
    istio-pilot: docker.io/istio/pilot:1.16.7
    istio-proxy: docker.io/istio/proxyv2:1.16.7
    vault: docker.io/hashicorp/vault:2.0.3
    vault-secrets-operator: ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
```

Override any image by passing `--config`:

```bash
kindtks create gen1 --config my-config.yaml
```

Note: `cilium-operator` must use the base image name (`quay.io/cilium/operator`) without `-generic` — the Helm chart appends `-generic` automatically.

## Istio Ingress

A wildcard Gateway `istio-ingress/kindtks` is created for `*.kindtks.localhost`. Services are exposed via VirtualService on port 30080.

To expose a service:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: my-app
spec:
  hosts:
    - my-app.kindtks.localhost
  gateways:
    - istio-ingress/kindtks
  http:
    - route:
        - destination:
            host: my-app-svc
            port:
              number: 80
```

Access it at `http://my-app.kindtks.localhost:30080`.

## Vault

Vault runs in dev mode with the root token `root`. The UI is exposed at `http://vault.kindtks.localhost:30080`.

Vault uses KV v1 engine. To store and sync a secret:

```bash
kubectl exec -n vault vault-0 -- vault kv put secret/my-app key=value

kubectl apply -f - <<EOF
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: my-app
spec:
  path: secret/my-app
  type: Opaque
EOF
```

The Vault Secrets Operator will create a corresponding Kubernetes Secret.

## Resource Sizing

Istio resource requests are nulled out so the profile runs on a single-CPU node. Istiod falls back to `global.defaultResources` (10m CPU).
