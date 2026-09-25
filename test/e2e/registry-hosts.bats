#!/usr/bin/env bats
# Verifies registries.<reg>.hosts end to end: host files are mounted on every
# node, containerd accepts config_path together with an auth patch, and pulls
# go through the configured mirror. Self-contained: uses its own two-node
# test profile and a registry:2 pull-through cache on the kind network.

load '../helpers/test_helper'

CLUSTER_NAME="kindtks-rh-e2e"
MIRROR_NAME="kindtks-rh-e2e-mirror"
PROFILE="rh-e2e"

setup_file() {
  export E2E_HOME="${BATS_FILE_TMPDIR}/home"
  local profile_dir="${E2E_HOME}/.local/share/kindtks/profiles/${PROFILE}"
  mkdir -p "${E2E_HOME}/.local/bin" "$profile_dir"

  go build -o "${E2E_HOME}/.local/bin/kindtks" "${BATS_TEST_DIRNAME}/../../"

  cat > "${profile_dir}/install.sh" <<EOF
REQUIRES="kind"

create() {
  kind create cluster --name ${CLUSTER_NAME} --image kindest/node:v1.24.17 --config "\$KINDTKS_KIND_CONFIG"
}

delete() {
  kind delete cluster --name ${CLUSTER_NAME}
}
EOF

  cat > "${profile_dir}/kind-config.yaml" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
EOF

  : > "${profile_dir}/config.yaml"

  cat > "${BATS_FILE_TMPDIR}/config.yaml" <<EOF
images:
  probe: registry.e2e.invalid:5000/probe:v1
registries:
  docker.io:
    hosts: |
      server = "https://registry-1.docker.io"
      [host."http://${MIRROR_NAME}:5000"]
        capabilities = ["pull", "resolve"]
  auth.e2e.invalid:
    auth:
      username: u
      password: p
EOF

  env -u XDG_STATE_HOME HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" create "$PROFILE" \
    --config "${BATS_FILE_TMPDIR}/config.yaml"

  # Started after the cluster so the kind network exists.
  docker rm -f "$MIRROR_NAME" >/dev/null 2>&1 || true
  docker run -d --name "$MIRROR_NAME" --network kind \
    -e REGISTRY_PROXY_REMOTEURL=https://registry-1.docker.io registry:2
}

teardown_file() {
  kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
  docker rm -f "$MIRROR_NAME" >/dev/null 2>&1 || true
}

nodes() {
  kind get nodes --name "$CLUSTER_NAME"
}

kindtks() {
  env -u XDG_STATE_HOME HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" "$@"
}

@test "every node has the generated host files" {
  for node in $(nodes); do
    run docker exec "$node" cat /etc/containerd/certs.d/docker.io/hosts.toml
    assert_success
    assert_output --partial "http://${MIRROR_NAME}:5000"

    run docker exec "$node" cat /etc/containerd/certs.d/registry.e2e.invalid:5000/hosts.toml
    assert_success
    assert_output --partial "skip_verify = true"
  done
}

@test "auth-only registry gets no host file" {
  run docker exec "${CLUSTER_NAME}-control-plane" test -e /etc/containerd/certs.d/auth.e2e.invalid
  assert_failure
}

@test "containerd config has config_path and the auth patch on every node" {
  for node in $(nodes); do
    run docker exec "$node" cat /etc/containerd/config.toml
    assert_success
    assert_output --partial 'config_path = "/etc/containerd/certs.d"'
    assert_output --partial 'auth.e2e.invalid'
  done
}

@test "pulls from every node go through the mirror" {
  for node in $(nodes); do
    run docker exec "$node" crictl pull docker.io/library/busybox:1.36
    assert_success
  done
  run docker logs "$MIRROR_NAME"
  assert_output --partial "library/busybox"
}

@test "kindtks delete removes generated state" {
  run kindtks delete "$PROFILE"
  assert_success
  [ ! -e "${E2E_HOME}/.local/state/kindtks/${PROFILE}" ]
}
