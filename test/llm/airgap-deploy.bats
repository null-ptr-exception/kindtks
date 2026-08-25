#!/usr/bin/env bats

# LLM-driven air-gap deployment and doc-completeness test.
#
# An AI agent is given only the container image and must figure out
# how to install, deploy a gen1 cluster, and use its features
# (per-DC StorageClasses, Istio ingress with *.kindtks.local)
# from the help/doc output alone.
# The test independently verifies the cluster and workload state —
# it never trusts the AI's self-report.
#
# Requires:
#   - Air-gap lab running (~/projects/airgap-lab, make status)
#   - claude CLI with API key
#   - Docker, skopeo
#   - All gen1 images already in the airgap registry
#
# Run: bats test/llm/airgap-deploy.bats

load '../helpers/test_helper'

AIRGAP_VM_IP="${AIRGAP_VM_IP:?Set AIRGAP_VM_IP to the VM address}"
AIRGAP_VM_USER="${AIRGAP_VM_USER:-ubuntu}"
VM_REGISTRY="${VM_REGISTRY:-registry.airgap:5000}"
AIRGAP_REGISTRY_USER="${AIRGAP_REGISTRY_USER:?Set AIRGAP_REGISTRY_USER}"
AIRGAP_REGISTRY_PASS="${AIRGAP_REGISTRY_PASS:?Set AIRGAP_REGISTRY_PASS}"

ssh_vm() {
  ssh -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR ${AIRGAP_VM_USER}@"$AIRGAP_VM_IP" "$@"
}

setup_file() {
  export IMAGE="${IMAGE:-${VM_REGISTRY}/kindtks:airgap-test}"
  export LOG_DIR="${BATS_TEST_DIRNAME}/logs"
  mkdir -p "$LOG_DIR"

  local TIMESTAMP
  TIMESTAMP=$(date +%Y%m%d-%H%M%S)
  export GAPS_FILE="${LOG_DIR}/gaps-${TIMESTAMP}.txt"
  export CLAUDE_LOG="${LOG_DIR}/claude-${TIMESTAMP}.log"

  # Verify prerequisites
  command -v claude >/dev/null 2>&1 || skip "claude CLI not found"

  run ssh_vm 'echo ok'
  if [ "$status" -ne 0 ]; then
    skip "Air-gap VM not reachable at ${AIRGAP_VM_IP}"
  fi

  # Verify image exists in registry
  run ssh_vm "docker pull ${IMAGE} 2>&1"
  if [ "$status" -ne 0 ]; then
    skip "Image ${IMAGE} not in registry — run 'bats test/e2e/airgap-gen1.bats' first to push images"
  fi

  # Clean VM state
  ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kind delete cluster --name gen1 2>/dev/null; true'
  ssh_vm 'sudo rm -rf ~/.local/bin/kindtks ~/.local/share/kindtks'
}

teardown_file() {
  ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kind delete cluster --name gen1 2>/dev/null' || true
}

@test "AI deploys gen1 from container image alone" {
  local PROMPT
  PROMPT="$(cat <<PROMPT_EOF
You have a container image: ${IMAGE}

You are working inside an air-gapped VM accessible via: ssh -o StrictHostKeyChecking=accept-new ${AIRGAP_VM_USER}@${AIRGAP_VM_IP}

The VM has docker, kind (v0.17.0), kubectl, and helm installed at ~/.local/bin.
The VM has a private Docker registry at ${VM_REGISTRY} with credentials: username=${AIRGAP_REGISTRY_USER}, password=${AIRGAP_REGISTRY_PASS}.
All required container images are already in the registry.

Your task:
1. Run the container image to discover what it does and how to use it (read its help output)
2. Install the tool from the container image
3. Deploy a gen1 cluster using images from ${VM_REGISTRY} with registry authentication
4. Verify all pods are Running
5. Deploy a busybox pod named "app-dc2" in namespace "demo" that uses a PersistentVolumeClaim
   named "data-dc2" bound to the dc2 datacenter StorageClass. The pod should mount
   the PVC at /data and run "sleep 3600".
6. Expose an nginx service named "hello" in namespace "demo" via the Istio ingress gateway
   at hostname hello.kindtks.local, serving a page that says "hello-from-dc2".

You must figure out how to do steps 5 and 6 from the tool's documentation alone.

Rules:
- Run ALL commands via SSH into the VM: ssh -o StrictHostKeyChecking=accept-new ${AIRGAP_VM_USER}@${AIRGAP_VM_IP} '<commands>'
- You must figure out the tool usage from its help output alone — do NOT read source code
- If you encounter documentation gaps or unclear instructions, note them in ${GAPS_FILE}
- The VM has no internet access — all images must come from ${VM_REGISTRY}
- For step 5, use the busybox image from ${VM_REGISTRY} (e.g. ${VM_REGISTRY}/busybox:latest)
- For step 6, use the nginx image from ${VM_REGISTRY} (e.g. ${VM_REGISTRY}/nginx:1.24-alpine)
PROMPT_EOF
)"

  claude -p "$PROMPT" \
    --dangerously-skip-permissions \
    --max-budget-usd 5 \
    --allowedTools "Bash Read Write" \
    2>&1 | tee "${CLAUDE_LOG}"

  # We don't assert anything here — the AI's self-report is not trusted.
  # The following tests independently verify the actual cluster state.
}

# --- Independent verification ---
# These tests verify the cluster state directly via kubectl.
# They don't rely on anything the AI claimed.

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

# --- Doc-completeness: AI-deployed workload verification ---
# These verify whether the AI could figure out per-DC storage and
# ingress routing from the documentation alone.

@test "app-dc2 pod is running in demo namespace" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n demo get pod app-dc2 -o jsonpath="{.status.phase}"'
  assert_success
  assert_output "Running"
}

@test "data-dc2 PVC is bound" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n demo get pvc data-dc2 -o jsonpath="{.status.phase}"'
  assert_success
  assert_output "Bound"
}

@test "app-dc2 pod landed on a dc2 node" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; kubectl --context kind-gen1 -n demo get pod app-dc2 -o jsonpath="{.spec.nodeName}"'
  assert_success
  local node="$output"

  run ssh_vm "export PATH=\"\$HOME/.local/bin:\$PATH\"; kubectl --context kind-gen1 get node ${node} -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}'"
  assert_success
  assert_output "dc2"
}

@test "hello service is routable via hello.kindtks.local" {
  run ssh_vm 'export PATH="$HOME/.local/bin:$PATH"; curl -s --resolve hello.kindtks.local:30080:127.0.0.1 http://hello.kindtks.local:30080'
  assert_success
  assert_output --partial "hello-from-dc2"
}
