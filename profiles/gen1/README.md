# gen1

3-node cluster (1 control-plane + 2 workers) with Cilium CNI, Istio service mesh, Vault (dev mode), Vault Secrets Operator, and per-DC StorageClasses.

| Component | Version |
|---|---|
| Kubernetes | 1.24.17 |
| Cilium | 1.13.10 |
| Istio | 1.16.7 |
| Vault | 0.34.0 (chart) / 2.0.3 (image) |
| Vault Secrets Operator | 2.7.0 (chart) / 1.26.0 (app) |

## Requirements

Minimum 6 GB RAM recommended (3 nodes). Istio resource requests are nulled out; istiod falls back to 10m CPU.

## Quick Start

```bash
kindtks create gen1
kubectl config use-context kind-gen1
```

After creation, services are accessible at `http://<app>.kindtks.localhost:30080`. Browsers (Chrome, Edge) resolve `*.localhost` to 127.0.0.1 automatically. For curl or other tools, add entries to `/etc/hosts`:

```
127.0.0.1 vault.kindtks.localhost my-app.kindtks.localhost
```

## Deploying a Service

Full example deploying an echo server with Istio ingress:

```bash
kubectl create namespace echo

kubectl -n echo create deployment echo --image=hashicorp/http-echo -- -text="hello"
kubectl -n echo expose deployment echo --port=80 --target-port=5678

kubectl apply -f - <<EOF
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: echo
  namespace: echo
spec:
  hosts:
    - echo.kindtks.localhost
  gateways:
    - istio-ingress/kindtks
  http:
    - route:
        - destination:
            host: echo
            port:
              number: 80
EOF

curl -s http://echo.kindtks.localhost:30080
# → hello
```

The gateway `istio-ingress/kindtks` is a wildcard for `*.kindtks.localhost` and works across namespaces.

Istio sidecar injection is not enabled by default. To enable it for a namespace: `kubectl label namespace <ns> istio-injection=enabled`.

## Vault

Vault runs in dev mode with root token `root`. The UI is at `http://vault.kindtks.localhost:30080`.

Vault uses KV v1 engine (no `data/` path prefix, no versioning). To store and sync a secret:

```bash
# Store a secret in Vault
kubectl exec -n vault vault-0 -- vault kv put secret/my-app key=value

# Create a VaultSecret to sync it to Kubernetes
kubectl apply -f - <<EOF
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: my-app
  namespace: default
spec:
  path: secret/my-app
  type: Opaque
EOF

# Verify the secret was synced (takes a few seconds)
kubectl get secret my-app -o jsonpath='{.data.key}' | base64 -d
# → value
```

The VaultSecret CR creates a Kubernetes Secret in the same namespace. Sync typically completes within 10 seconds.

## Nodes and Topology

The cluster has 3 nodes labeled by datacenter:

| Node | Role | Zone Label |
|---|---|---|
| gen1-control-plane | control-plane | dc1 |
| gen1-worker | worker | dc2 |
| gen1-worker2 | worker | dc3 |

## StorageClasses

Per-DC StorageClasses simulate topology-aware storage (e.g. NetApp per-datacenter). PVCs using a DC-specific class will only bind on nodes in that zone.

| StorageClass | Zone | Binding Mode |
|---|---|---|
| netapp-dc1 | dc1 | WaitForFirstConsumer |
| netapp-dc2 | dc2 | WaitForFirstConsumer |
| netapp-dc3 | dc3 | WaitForFirstConsumer |

All use the `rancher.io/local-path` provisioner (Kind's built-in). The `standard` StorageClass remains available and binds to any node.

```bash
# Verify StorageClasses
kubectl get sc

# Test: PVC bound to dc2
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-dc2
spec:
  storageClassName: netapp-dc2
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Mi
EOF

# Pod must land on a dc2 node for the PVC to bind
kubectl run test-dc2 --image=busybox --restart=Never \
  --overrides='{"spec":{"volumes":[{"name":"v","persistentVolumeClaim":{"claimName":"test-dc2"}}],"containers":[{"name":"c","image":"busybox","command":["sleep","10"],"volumeMounts":[{"name":"v","mountPath":"/data"}]}]}}'
kubectl get pvc test-dc2
# STATUS should be Bound, node should be in dc2
```

## Config

```bash
kindtks config gen1
```

```yaml
images:
    cilium: quay.io/cilium/cilium:v1.13.10
    cilium-operator: quay.io/cilium/operator-generic:v1.13.10
    istio-pilot: docker.io/istio/pilot:1.16.7
    istio-proxy: docker.io/istio/proxyv2:1.16.7
    vault: docker.io/hashicorp/vault:2.0.3
    vault-secrets-operator: ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
```

Override any image by passing `--config`:

```bash
kindtks create gen1 --config my-config.yaml
```

## Cluster Lifecycle

If a cluster named `gen1` already exists, `kindtks create gen1` will fail. Delete first with `kindtks delete gen1`, which removes the Kind cluster and all associated containers.
