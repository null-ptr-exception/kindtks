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

  # Use high ports to avoid conflicts
  sed -i 's/hostPort: 80$/hostPort: 18080/' \
    "${E2E_HOME}/.local/share/kindtks/profiles/gen1/kind-config.yaml"
  sed -i 's/hostPort: 443$/hostPort: 18443/' \
    "${E2E_HOME}/.local/share/kindtks/profiles/gen1/kind-config.yaml"

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

# --- Vault ---

@test "vault server pod is running" {
  run kube -n vault get pod vault-0 -o jsonpath='{.status.phase}'
  assert_success
  assert_output "Running"
}

# --- Vault Secrets Operator ---

@test "vault-secrets-operator deployment is available" {
  run kube -n vault-secrets-operator get deployment -l app.kubernetes.io/name=vault-secrets-operator \
    -o jsonpath='{.items[0].status.availableReplicas}'
  assert_success
  assert_output "1"
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
