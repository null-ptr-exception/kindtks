#!/usr/bin/env bats

load '../helpers/test_helper'

setup_file() {
  go build -o "${BATS_FILE_TMPDIR}/kindtks" "${BATS_TEST_DIRNAME}/../../"
}

setup() {
  export KINDTKS_BIN="${BATS_FILE_TMPDIR}/kindtks"
  export KINDTKS_PROFILE_DIR="${BATS_TEST_DIRNAME}/../../profiles/gen1-rootless"
  export KINDTKS_CHARTS_DIR="/fake/charts"
  unset KINDTKS_KIND_CONFIG GEN1_DIR

  # Set image env vars (as the Go CLI would from config.yaml)
  export IMAGE_KIND_NODE="kindest/node:v1.24.17"
  export IMAGE_ISTIO_PILOT="docker.io/istio/pilot:1.16.7"
  export IMAGE_ISTIO_PROXY="docker.io/istio/proxyv2:1.16.7"
  export IMAGE_VAULT="docker.io/hashicorp/vault:2.0.3"
  export IMAGE_VAULT_SECRETS_OPERATOR="ghcr.io/ricoberger/vault-secrets-operator:v1.26.0"

  # Mock bin directory: helm, kubectl and kind record their arguments
  export MOCK_BIN="${BATS_TEST_TMPDIR}/bin"
  export CALLS_FILE="${BATS_TEST_TMPDIR}/calls"
  mkdir -p "$MOCK_BIN"
  for cmd in helm kubectl kind; do
    cat > "${MOCK_BIN}/${cmd}" <<'MOCK'
#!/usr/bin/env bash
echo "$(basename "$0") $*" >> "$CALLS_FILE"
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

@test "cluster is named gen1-rootless" {
  assert_equal "$CLUSTER_NAME" "gen1-rootless"
}

@test "values and manifests come from the gen1 profile" {
  assert_equal "$GEN1_DIR" "$(cd "${BATS_TEST_DIRNAME}/../../profiles/gen1" && pwd)"
}

# --- No Cilium ---

@test "kind config keeps kind's default CNI" {
  run grep disableDefaultCNI "${KINDTKS_PROFILE_DIR}/kind-config.yaml"
  assert_failure
}

@test "config has no Cilium images" {
  run grep -i cilium "${KINDTKS_PROFILE_DIR}/config.yaml" "${KINDTKS_PROFILE_DIR}/config.schema.json"
  assert_failure
}

@test "create installs everything from gen1 except Cilium" {
  run create
  assert_success

  run cat "$CALLS_FILE"
  assert_line --partial "kind create cluster --name gen1-rootless --image kindest/node:v1.24.17 --config ${KINDTKS_PROFILE_DIR}/kind-config.yaml"
  refute_output --partial "cilium"
  for release in istio-base istiod istio-ingress vault vault-secrets-operator; do
    assert_line --partial "helm install ${release} "
  done
  assert_line --partial "kubectl apply -f ${GEN1_DIR}/storageclasses.yaml"
  assert_line --partial "kubectl apply -f ${GEN1_DIR}/gateway.yaml"
  assert_line --partial "kubectl apply -f ${GEN1_DIR}/vault-virtualservice.yaml"
}

@test "create reports the gen1-rootless cluster" {
  run create
  assert_output --partial "==> gen1-rootless cluster is ready!"
}

@test "delete deletes the gen1-rootless cluster" {
  run delete
  assert_success
  run cat "$CALLS_FILE"
  assert_output "kind delete cluster --name gen1-rootless"
}

# --- Same cluster layout as gen1 ---

@test "kind config is gen1's without disableDefaultCNI" {
  run diff <(grep -v disableDefaultCNI "${GEN1_DIR}/kind-config.yaml") "${KINDTKS_PROFILE_DIR}/kind-config.yaml"
  assert_success
}

@test "config is gen1's without Cilium images" {
  run diff <(grep -v '^  cilium' "${GEN1_DIR}/config.yaml") "${KINDTKS_PROFILE_DIR}/config.yaml"
  assert_success
}
