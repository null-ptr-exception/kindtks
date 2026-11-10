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
  export IMAGE_KUBE_NETWORK_POLICIES="registry.k8s.io/networking/kube-network-policies:v1.1.1"
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

@test "create installs kube-network-policies and everything from gen1 except Cilium" {
  run create
  assert_success

  run cat "$CALLS_FILE"
  assert_line --partial "kind create cluster --name gen1-rootless --image kindest/node:v1.24.17 --config ${KINDTKS_PROFILE_DIR}/kind-config.yaml"
  refute_output --partial "cilium"
  assert_line --regexp "^helm install kube-network-policies /fake/charts/kube-network-policies-1\.1\.2\.tgz --namespace kube-system --create-namespace --values "
  assert_line "kubectl -n kube-system rollout status daemonset/kube-network-policies --timeout=600s"
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

@test "config is gen1's without Cilium images, with kube-network-policies" {
  run diff <(grep -v '^  cilium' "${GEN1_DIR}/config.yaml") <(grep -v '^  kube-network-policies:' "${KINDTKS_PROFILE_DIR}/config.yaml")
  assert_success
  run grep -c '^  kube-network-policies: ' "${KINDTKS_PROFILE_DIR}/config.yaml"
  assert_output "1"
}

# --- kube-network-policies ---

@test "kube-network-policies values take the image from config" {
  split_image_refs
  run "$KINDTKS_BIN" envsubst < "${KINDTKS_PROFILE_DIR}/values-kube-network-policies.yaml"
  assert_output --partial "repository: registry.k8s.io/networking/kube-network-policies"
  assert_output --partial "tag: v1.1.1"
  refute_output --partial '${IMAGE_'
}

@test "kube-network-policies does not need AdminNetworkPolicy CRDs" {
  run grep -x "adminNetworkPolicy: false" "${KINDTKS_PROFILE_DIR}/values-kube-network-policies.yaml"
  assert_success
}

@test "helm_install prefers the profile's own values file" {
  split_image_refs
  helm_install kube-network-policies /fake/charts/knp.tgz kube-system
  run grep -c -- "--values" "$CALLS_FILE"
  assert_output "1"
}

@test "helm_install falls back to gen1's values file" {
  split_image_refs
  helm_install vault /fake/charts/vault.tgz vault
  run grep -c -- "--values" "$CALLS_FILE"
  assert_output "1"
}

@test "pull-charts.sh packages the pinned kube-network-policies chart" {
  run grep -E '^KNP_(VERSION|SHA256)=' "${BATS_TEST_DIRNAME}/../../scripts/pull-charts.sh"
  assert_line "KNP_VERSION=1.1.2"
  assert_line --regexp '^KNP_SHA256=[0-9a-f]{64}$'
}
