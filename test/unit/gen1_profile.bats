#!/usr/bin/env bats

load '../helpers/test_helper'

setup() {
  export KINDTKS_PROFILE_DIR="${BATS_TEST_DIRNAME}/../../profiles/gen1"
  export KINDTKS_CHARTS_DIR="/fake/charts"

  # Set image env vars (as the Go CLI would from config.yaml)
  export IMAGE_CILIUM="quay.io/cilium/cilium:v1.13.10"
  export IMAGE_CILIUM_OPERATOR="quay.io/cilium/operator-generic:v1.13.10"
  export IMAGE_ISTIO_PILOT="docker.io/istio/pilot:1.16.7"
  export IMAGE_ISTIO_PROXY="docker.io/istio/proxyv2:1.16.7"
  export IMAGE_VAULT="docker.io/hashicorp/vault:2.0.3"
  export IMAGE_VAULT_SECRETS_OPERATOR="ghcr.io/ricoberger/vault-secrets-operator:v1.26.0"

  # Create a mock bin directory and prepend to PATH
  export MOCK_BIN="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "$MOCK_BIN"

  # Mock helm: record arguments to a file
  export HELM_ARGS_FILE="${BATS_TEST_TMPDIR}/helm_args"
  cat > "${MOCK_BIN}/helm" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$HELM_ARGS_FILE"
MOCK
  chmod +x "${MOCK_BIN}/helm"

  # Mock kubectl and kind as no-ops
  for cmd in kubectl kind; do
    cat > "${MOCK_BIN}/${cmd}" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
    chmod +x "${MOCK_BIN}/${cmd}"
  done

  export PATH="${MOCK_BIN}:${PATH}"

  source "${KINDTKS_PROFILE_DIR}/install.sh"
}

# --- Profile contract ---

@test "REQUIRES lists kind helm kubectl" {
  assert_equal "$REQUIRES" "kind helm kubectl"
}

@test "create function is defined" {
  run type -t create
  assert_output "function"
}

@test "delete function is defined" {
  run type -t delete
  assert_output "function"
}

# --- split_image_refs ---

@test "split_image_refs creates REPO and TAG vars" {
  split_image_refs
  assert_equal "$IMAGE_CILIUM_REPO" "quay.io/cilium/cilium"
  assert_equal "$IMAGE_CILIUM_TAG" "v1.13.10"
}

@test "split_image_refs derives ISTIO_HUB from pilot image" {
  split_image_refs
  assert_equal "$ISTIO_HUB" "docker.io/istio"
}

@test "split_image_refs works with corporate registry" {
  export IMAGE_CILIUM="corp.example.com/cilium/cilium:v1.13.10"
  export IMAGE_ISTIO_PILOT="corp.example.com/istio/pilot:1.16.7"
  split_image_refs
  assert_equal "$IMAGE_CILIUM_REPO" "corp.example.com/cilium/cilium"
  assert_equal "$IMAGE_CILIUM_TAG" "v1.13.10"
  assert_equal "$ISTIO_HUB" "corp.example.com/istio"
}

# --- helm_install values file lookup ---

@test "helm_install uses values file matching release name" {
  split_image_refs
  helm_install cilium /fake/charts/cilium.tgz kube-system

  run cat "$HELM_ARGS_FILE"
  assert_output --partial "--values"
  assert_output --partial "install cilium /fake/charts/cilium.tgz --namespace kube-system --create-namespace"
}

@test "helm_install skips values when no matching file exists" {
  helm_install no-such-release /fake/charts/foo.tgz default

  run cat "$HELM_ARGS_FILE"
  refute_output --partial "--values"
}

@test "helm_install forwards extra arguments" {
  split_image_refs
  helm_install cilium /fake/charts/cilium.tgz kube-system --wait --timeout 300s

  run cat "$HELM_ARGS_FILE"
  assert_output --partial "--wait --timeout 300s"
}

# --- envsubst with default images ---

@test "envsubst resolves cilium image refs" {
  split_image_refs
  run envsubst < "${KINDTKS_PROFILE_DIR}/values-cilium.yaml"
  assert_output --partial "repository: quay.io/cilium/cilium"
  assert_output --partial "tag: v1.13.10"
  refute_output --partial '${IMAGE_'
}

@test "envsubst resolves istio hub and tag" {
  split_image_refs
  run envsubst < "${KINDTKS_PROFILE_DIR}/values-istiod.yaml"
  assert_output --partial "hub: docker.io/istio"
  assert_output --partial "tag: 1.16.7"
}

@test "envsubst resolves vault image" {
  split_image_refs
  run envsubst < "${KINDTKS_PROFILE_DIR}/values-vault.yaml"
  assert_output --partial "repository: docker.io/hashicorp/vault"
  assert_output --partial "tag: 2.0.3"
}

@test "envsubst resolves vault-secrets-operator image" {
  split_image_refs
  run envsubst < "${KINDTKS_PROFILE_DIR}/values-vault-secrets-operator.yaml"
  assert_output --partial "repository: ghcr.io/ricoberger/vault-secrets-operator"
  assert_output --partial "tag: v1.26.0"
}

# --- envsubst with corporate registry ---

@test "envsubst with corporate registry overrides all image repos" {
  export IMAGE_CILIUM="corp.example.com/cilium:v1.13.10"
  export IMAGE_VAULT="corp.example.com/vault:2.0.3"
  split_image_refs

  run envsubst < "${KINDTKS_PROFILE_DIR}/values-cilium.yaml"
  assert_output --partial "repository: corp.example.com/cilium"

  run envsubst < "${KINDTKS_PROFILE_DIR}/values-vault.yaml"
  assert_output --partial "repository: corp.example.com/vault"
}

# --- gateway service config ---

@test "gateway values set NodePort with correct ports" {
  run cat "${KINDTKS_PROFILE_DIR}/values-istio-ingress.yaml"
  assert_output --partial "type: NodePort"
  assert_output --partial "nodePort: 30080"
  assert_output --partial "nodePort: 30443"
}

# --- vault-secrets-operator config ---

@test "vault-secrets-operator is pre-configured for in-cluster vault" {
  run cat "${KINDTKS_PROFILE_DIR}/values-vault-secrets-operator.yaml"
  assert_output --partial "address: http://vault.vault.svc:8200"
  assert_output --partial "authMethod: token"
  assert_output --partial "value: root"
}

# --- values file completeness ---

@test "every helm_install call has a matching values file" {
  local missing=()
  for name in cilium istio-base istiod istio-ingress vault vault-secrets-operator; do
    if [ ! -f "${KINDTKS_PROFILE_DIR}/values-${name}.yaml" ]; then
      missing+=("$name")
    fi
  done
  assert_equal "${#missing[@]}" 0 "Missing values files: ${missing[*]}"
}

# --- profile config ---

@test "config.yaml lists all images" {
  local keys
  keys=$(grep -oP '^\s+\K[a-z-]+(?=:)' "${KINDTKS_PROFILE_DIR}/config.yaml")
  for expected in cilium cilium-operator istio-pilot istio-proxy vault vault-secrets-operator; do
    echo "$keys" | grep -q "^${expected}$" || fail "Missing image key: $expected"
  done
}
