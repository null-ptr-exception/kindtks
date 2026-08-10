#!/usr/bin/env bats

load '../helpers/test_helper'

CLUSTER_NAME="gen1-e2e"

setup_file() {
  export E2E_HOME="${BATS_FILE_TMPDIR}/home"
  mkdir -p "${E2E_HOME}/.local/bin" "${E2E_HOME}/.local/share/kindtks"

  # Build kindtks binary
  go build -o "${E2E_HOME}/.local/bin/kindtks" "${BATS_TEST_DIRNAME}/../../"

  # Copy profiles
  cp -r "${BATS_TEST_DIRNAME}/../../profiles" "${E2E_HOME}/.local/share/kindtks/profiles"

  # Pull charts if not cached
  local charts_cache="${BATS_TEST_DIRNAME}/../../.cache/charts"
  if [ ! -d "$charts_cache" ] || [ -z "$(ls "$charts_cache"/*.tgz 2>/dev/null)" ]; then
    mkdir -p "$charts_cache"
    "${BATS_TEST_DIRNAME}/../../scripts/pull-charts.sh" "$charts_cache"
  fi
  cp -r "$charts_cache" "${E2E_HOME}/.local/share/kindtks/charts"

  # Use a unique cluster name to avoid collisions
  sed -i "s/CLUSTER_NAME=\"gen1\"/CLUSTER_NAME=\"${CLUSTER_NAME}\"/" \
    "${E2E_HOME}/.local/share/kindtks/profiles/gen1/install.sh"

  # Create the cluster
  HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" create gen1

  # Export KUBECONFIG so kubectl can find the cluster
  export KUBECONFIG="${E2E_HOME}/.kube/config"
}

teardown_file() {
  kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
}

setup() {
  export KUBECONFIG="${E2E_HOME}/.kube/config"
}

kube() {
  kubectl --context "kind-${CLUSTER_NAME}" "$@"
}

kindtks() {
  HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" "$@"
}

# --- Cluster state ---

@test "kind cluster exists" {
  run kind get clusters
  assert_output --partial "$CLUSTER_NAME"
}

@test "node is Ready" {
  run kube get nodes -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}'
  assert_success
  assert_output "True"
}

# --- Cilium ---

@test "cilium daemonset is running" {
  run kube -n kube-system get daemonset cilium -o jsonpath='{.status.numberReady}'
  assert_success
  refute_output "0"
}

@test "cilium-operator deployment is available" {
  run kube -n kube-system get deployment cilium-operator -o jsonpath='{.status.availableReplicas}'
  assert_success
  assert_output "1"
}

# --- Istio ---

@test "istiod deployment is available" {
  run kube -n istio-system get deployment istiod -o jsonpath='{.status.availableReplicas}'
  assert_success
  assert_output "1"
}

@test "istio-ingress deployment is available" {
  run kube -n istio-ingress get deployment istio-ingress -o jsonpath='{.status.availableReplicas}'
  assert_success
  assert_output "1"
}

@test "istio-ingress service is NodePort on correct ports" {
  run kube -n istio-ingress get svc istio-ingress -o jsonpath='{.spec.type}'
  assert_success
  assert_output "NodePort"

  run kube -n istio-ingress get svc istio-ingress -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}'
  assert_success
  assert_output "30080"

  run kube -n istio-ingress get svc istio-ingress -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}'
  assert_success
  assert_output "30443"
}

@test "wildcard gateway exists for *.kindtks.localhost" {
  run kube -n istio-ingress get gateway kindtks -o jsonpath='{.spec.servers[0].hosts[0]}'
  assert_success
  assert_output "*.kindtks.localhost"
}

# --- Vault ---

@test "vault server pod is running" {
  run kube -n vault get pod vault-0 -o jsonpath='{.status.phase}'
  assert_success
  assert_output "Running"
}

# --- Vault UI ---

@test "vault UI is reachable via gateway" {
  local retries=10
  for i in $(seq 1 $retries); do
    local code
    code=$(curl -s -o /dev/null -w '%{http_code}' -L --resolve vault.kindtks.localhost:30080:127.0.0.1 http://vault.kindtks.localhost:30080)
    if [ "$code" = "200" ]; then
      break
    fi
    sleep 2
  done

  run curl -s -o /dev/null -w '%{http_code}' -L --resolve vault.kindtks.localhost:30080:127.0.0.1 http://vault.kindtks.localhost:30080
  assert_success
  assert_output "200"
}

# --- Vault Secrets Operator (ricoberger) ---

@test "vault-secrets-operator deployment is available" {
  run kube -n vault-secrets-operator get deployment vault-secrets-operator \
    -o jsonpath='{.status.availableReplicas}'
  assert_success
  assert_output "1"
}

@test "vault-secrets-operator syncs a secret from vault" {
  # Write a secret to Vault via the vault pod
  kube -n vault exec vault-0 -- vault kv put secret/e2e-test username=admin password=s3cret

  # Create a VaultSecret CR in default namespace
  kube apply -f - <<'YAML'
apiVersion: ricoberger.de/v1alpha1
kind: VaultSecret
metadata:
  name: e2e-test
  namespace: default
spec:
  path: secret/e2e-test
  type: Opaque
YAML

  # Wait for the operator to sync it into a Kubernetes Secret
  local retries=15
  local synced=false
  for i in $(seq 1 $retries); do
    if kube -n default get secret e2e-test &>/dev/null; then
      synced=true
      break
    fi
    sleep 2
  done
  assert [ "$synced" = "true" ]

  # Verify the secret data matches what was written to Vault
  run kube -n default get secret e2e-test -o jsonpath='{.data.username}'
  assert_success
  run bash -c "echo '$output' | base64 -d"
  assert_output "admin"

  run kube -n default get secret e2e-test -o jsonpath='{.data.password}'
  assert_success
  run bash -c "echo '$output' | base64 -d"
  assert_output "s3cret"

  # Cleanup
  kube delete vaultsecret e2e-test -n default
  kube -n vault exec vault-0 -- vault kv delete secret/e2e-test
}

# --- Echo service smoke test ---

@test "deploy echo service and reach it via gateway" {
  # Deploy echo server (no sidecar — testing ingress routing only)
  kube create namespace echo-test
  kube label namespace echo-test istio-injection=disabled
  kube -n echo-test apply -f - <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: echo-html
data:
  index.html: "kindtks-ok"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
  template:
    metadata:
      labels:
        app: echo
    spec:
      containers:
        - name: nginx
          image: nginx:1.24-alpine
          ports:
            - containerPort: 80
          volumeMounts:
            - name: html
              mountPath: /usr/share/nginx/html
      volumes:
        - name: html
          configMap:
            name: echo-html
---
apiVersion: v1
kind: Service
metadata:
  name: echo
spec:
  selector:
    app: echo
  ports:
    - port: 80
      targetPort: 80
YAML

  # Create VirtualService to route echo.kindtks.localhost to the echo service
  kube apply -f - <<'YAML'
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: echo
  namespace: echo-test
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
YAML

  # Wait for echo pod to be ready
  kube -n echo-test rollout status deployment/echo --timeout=300s

  # Curl through the gateway
  local retries=10
  local success=false
  for i in $(seq 1 $retries); do
    if curl -s -o /dev/null -w '%{http_code}' --resolve echo.kindtks.localhost:30080:127.0.0.1 http://echo.kindtks.localhost:30080 | grep -q 200; then
      success=true
      break
    fi
    sleep 2
  done

  run curl -s --resolve echo.kindtks.localhost:30080:127.0.0.1 http://echo.kindtks.localhost:30080
  assert_success
  assert_output --partial "kindtks-ok"

  # Cleanup
  kube delete namespace echo-test
}

# --- Delete ---

@test "kindtks delete gen1 succeeds" {
  run kindtks delete gen1
  assert_success
  assert_output --partial "Cluster '${CLUSTER_NAME}' deleted"
}

@test "cluster is gone after delete" {
  run kind get clusters
  refute_output --partial "$CLUSTER_NAME"
}
