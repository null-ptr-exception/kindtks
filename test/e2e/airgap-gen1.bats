#!/usr/bin/env bats

# Air-gap e2e test for kindtks gen1 profile.
#
# Builds the image, pushes it and all gen1 component images to an
# air-gapped registry, deploys the cluster, and verifies all
# components are healthy.
#
# Requires:
#   - Air-gap lab running (~/projects/airgap-lab, make status)
#   - Docker, skopeo
#
# Run: bats test/e2e/airgap-gen1.bats

load '../helpers/test_helper'

AIRGAP_LAB_DIR="${AIRGAP_LAB_DIR:?Set AIRGAP_LAB_DIR to your airgap-lab checkout}"
AIRGAP_VM_IP="${AIRGAP_VM_IP:?Set AIRGAP_VM_IP to the VM address}"
AIRGAP_VM_USER="${AIRGAP_VM_USER:-ubuntu}"
AIRGAP_REGISTRY="${AIRGAP_REGISTRY:-localhost:5000}"
AIRGAP_REGISTRY_USER="${AIRGAP_REGISTRY_USER:?Set AIRGAP_REGISTRY_USER}"
AIRGAP_REGISTRY_PASS="${AIRGAP_REGISTRY_PASS:?Set AIRGAP_REGISTRY_PASS}"
VM_REGISTRY="${VM_REGISTRY:-registry.airgap:5000}"

ssh_vm() {
  ssh -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR ${AIRGAP_VM_USER}@"$AIRGAP_VM_IP" "$@"
}

push_image() {
  "${AIRGAP_LAB_DIR}/scripts/push-image.sh" "$1"
}

push_image_skopeo() {
  skopeo copy --override-arch amd64 --override-os linux \
    "docker://$1" "docker://${AIRGAP_REGISTRY}/${2:-$1}" \
    --dest-tls-verify=false --dest-creds "${AIRGAP_REGISTRY_USER}:${AIRGAP_REGISTRY_PASS}"
}

registry_catalog() {
  curl -s -u "${AIRGAP_REGISTRY_USER}:${AIRGAP_REGISTRY_PASS}" \
    "http://${AIRGAP_REGISTRY}/v2/_catalog"
}

setup_file() {
  # Verify airgap lab is running
  run ssh_vm 'echo ok'
  if [ "$status" -ne 0 ]; then
    skip "Air-gap VM not reachable at ${AIRGAP_VM_IP}"
  fi
}

teardown_file() {
  ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kind delete cluster --name gen1 2>/dev/null' || true
}

# --- Phase 1: Build and push ---

@test "build kindtks image" {
  run docker build -t kindtks:airgap-test "${BATS_TEST_DIRNAME}/../../"
  assert_success
}

@test "wipe airgap registry" {
  run bash -c "cd '${AIRGAP_LAB_DIR}' && docker compose down registry && docker volume rm airgap-lab_registry-data && docker compose up -d registry"
  assert_success

  local retries=10
  for i in $(seq 1 $retries); do
    if curl -s -u "${AIRGAP_REGISTRY_USER}:${AIRGAP_REGISTRY_PASS}" "http://${AIRGAP_REGISTRY}/v2/" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  run registry_catalog
  assert_success
  assert_output '{"repositories":[]}'
}

@test "push kindtks image to airgap registry" {
  docker tag kindtks:airgap-test "${AIRGAP_REGISTRY}/kindtks:airgap-test"
  run docker push "${AIRGAP_REGISTRY}/kindtks:airgap-test"
  assert_success
}

@test "push gen1 component images to airgap registry" {
  run push_image "quay.io/cilium/cilium:v1.13.10"
  assert_success

  run push_image "quay.io/cilium/operator-generic:v1.13.10"
  assert_success

  run push_image "docker.io/istio/pilot:1.16.7"
  assert_success

  run push_image "docker.io/istio/proxyv2:1.16.7"
  assert_success

  run push_image_skopeo "docker.io/hashicorp/vault:2.0.3" "hashicorp/vault:2.0.3"
  assert_success

  run push_image "ghcr.io/ricoberger/vault-secrets-operator:v1.26.0"
  assert_success

  run push_image "kindest/node:v1.24.17"
  assert_success
}

# --- Phase 2: Deploy ---

@test "clean VM state" {
  ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kind delete cluster --name gen1 2>/dev/null; true'
  ssh_vm 'sudo rm -rf ~/.local/bin/kindtks ~/.local/share/kindtks'
  ssh_vm 'docker system prune -af' || true
  assert true
}

@test "install kindtks in VM" {
  run ssh_vm bash -e <<SSH
docker pull ${VM_REGISTRY}/kindtks:airgap-test
docker run --rm \
  -v "\$HOME/.local/bin:/.local/bin" \
  -v "\$HOME/.local/share:/.local/share" \
  ${VM_REGISTRY}/kindtks:airgap-test install
SSH
  assert_success
}

@test "create gen1 cluster with registry auth" {
  run ssh_vm bash -e <<'SSH'
export PATH="$HOME/.local/bin:$PATH"

docker pull registry.airgap:5000/kindest/node:v1.24.17
docker tag registry.airgap:5000/kindest/node:v1.24.17 kindest/node:v1.24.17

cat > ~/airgap-config.yaml <<'EOF'
images:
    cilium: registry.airgap:5000/quay.io/cilium/cilium:v1.13.10
    cilium-operator: registry.airgap:5000/quay.io/cilium/operator-generic:v1.13.10
    istio-pilot: registry.airgap:5000/docker.io/istio/pilot:1.16.7
    istio-proxy: registry.airgap:5000/docker.io/istio/proxyv2:1.16.7
    vault: registry.airgap:5000/hashicorp/vault:2.0.3
    vault-secrets-operator: registry.airgap:5000/ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
registries:
    registry.airgap:5000:
        auth:
            username: airgap
            password: airgap
EOF

kindtks create gen1 --config ~/airgap-config.yaml
SSH
  assert_success
}

# --- Phase 3: Verification ---

@test "kind cluster exists in VM" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kind get clusters'
  assert_success
  assert_output --partial "gen1"
}

@test "all pods are Running" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 get pods -A --no-headers'
  assert_success

  local non_running
  non_running=$(echo "$output" | grep -v Running || true)
  [ -z "$non_running" ]
}

@test "cilium is healthy" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n kube-system get daemonset cilium -o jsonpath="{.status.numberReady}"'
  assert_success
  refute_output "0"
}

@test "cilium-operator is available" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n kube-system get deployment cilium-operator -o jsonpath="{.status.availableReplicas}"'
  assert_success
  assert_output "1"
}

@test "istiod is available" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n istio-system get deployment istiod -o jsonpath="{.status.availableReplicas}"'
  assert_success
  assert_output "1"
}

@test "istio-ingress is available" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n istio-ingress get deployment istio-ingress -o jsonpath="{.status.availableReplicas}"'
  assert_success
  assert_output "1"
}

@test "vault is running" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n vault get pod vault-0 -o jsonpath="{.status.phase}"'
  assert_success
  assert_output "Running"
}

@test "vault-secrets-operator is available" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n vault-secrets-operator get deployment vault-secrets-operator -o jsonpath="{.status.availableReplicas}"'
  assert_success
  assert_output "1"
}
