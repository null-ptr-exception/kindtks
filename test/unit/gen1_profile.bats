#!/usr/bin/env bats

load '../helpers/test_helper'

setup() {
  export KINDTKS_PROFILE_DIR="${BATS_TEST_DIRNAME}/../../profiles/gen1"
  export KINDTKS_CHARTS_DIR="/fake/charts"
  export KINDTKS_REGISTRY=""

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

# --- helm_install values file lookup ---

@test "helm_install uses values file matching release name" {
  helm_install cilium /fake/charts/cilium.tgz kube-system

  run cat "$HELM_ARGS_FILE"
  assert_output --partial "--values"
  assert_output --partial "install cilium /fake/charts/cilium.tgz --namespace kube-system --create-namespace"
}

@test "helm_install skips values when no matching file exists" {
  helm_install no-such-release /fake/charts/foo.tgz default

  run cat "$HELM_ARGS_FILE"
  refute_output --partial "--values"
  assert_output --partial "install no-such-release /fake/charts/foo.tgz --namespace default --create-namespace"
}

@test "helm_install forwards extra arguments" {
  helm_install cilium /fake/charts/cilium.tgz kube-system --wait --timeout 300s

  run cat "$HELM_ARGS_FILE"
  assert_output --partial "--wait --timeout 300s"
}

# --- envsubst with empty registry ---

@test "envsubst leaves default repos when KINDTKS_REGISTRY is empty" {
  export KINDTKS_REGISTRY=""

  helm_install cilium /fake/charts/cilium.tgz kube-system

  # Find the values file that was passed to helm
  local values_arg
  values_arg=$(grep -oP '(?<=--values )\S+' "$HELM_ARGS_FILE")
  # The tmp file is cleaned up, so we envsubst ourselves to verify
  run envsubst < "${KINDTKS_PROFILE_DIR}/values-cilium.yaml"
  assert_output --partial "repository: quay.io/cilium/cilium"
  refute_output --partial '${KINDTKS_REGISTRY}'
}

# --- envsubst with registry prefix ---

@test "envsubst prepends registry prefix to image repos" {
  export KINDTKS_REGISTRY="registry.corp.com/"

  run envsubst < "${KINDTKS_PROFILE_DIR}/values-cilium.yaml"
  assert_output --partial "repository: registry.corp.com/quay.io/cilium/cilium"
  assert_output --partial "repository: registry.corp.com/quay.io/cilium/operator"
}

@test "envsubst applies registry to istio hub" {
  export KINDTKS_REGISTRY="registry.corp.com/"

  run envsubst < "${KINDTKS_PROFILE_DIR}/values-istiod.yaml"
  assert_output --partial "hub: registry.corp.com/docker.io/istio"
}

@test "envsubst applies registry to vault image" {
  export KINDTKS_REGISTRY="registry.corp.com/"

  run envsubst < "${KINDTKS_PROFILE_DIR}/values-vault.yaml"
  assert_output --partial "repository: registry.corp.com/hashicorp/vault"
}

@test "envsubst applies registry to vault-secrets-operator image" {
  export KINDTKS_REGISTRY="registry.corp.com/"

  run envsubst < "${KINDTKS_PROFILE_DIR}/values-vault-secrets-operator.yaml"
  assert_output --partial "repository: registry.corp.com/ghcr.io/ricoberger/vault-secrets-operator"
}

@test "vault-secrets-operator is pre-configured for in-cluster vault" {
  run cat "${KINDTKS_PROFILE_DIR}/values-vault-secrets-operator.yaml"
  assert_output --partial "address: http://vault.vault.svc:8200"
  assert_output --partial "authMethod: token"
  assert_output --partial "value: root"
}

# --- gateway service config ---

@test "gateway values set NodePort with correct ports" {
  run cat "${KINDTKS_PROFILE_DIR}/values-istio-ingress.yaml"
  assert_output --partial "type: NodePort"
  assert_output --partial "nodePort: 30080"
  assert_output --partial "nodePort: 30443"
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
