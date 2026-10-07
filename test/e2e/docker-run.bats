#!/usr/bin/env bats
# Runs kindtks the documented way: only `docker run` of the image. Checks the
# cluster comes up, files written on the host belong to the user, the state
# dir reaches the nodes by its host path, and missing mounts get a hint.

load '../helpers/test_helper'

IMAGE="kindtks:docker-run-e2e"
CLUSTER_NAME="kindtks-dr-e2e"
PROFILE="dr-e2e"

setup_file() {
  docker build -t "$IMAGE" "${BATS_TEST_DIRNAME}/../../"

  export H="${BATS_FILE_TMPDIR}/home"
  export TP="${BATS_FILE_TMPDIR}/test-profiles"
  mkdir -p "$H/.kube" "$H/.local/state/kindtks" "$H/work" "$TP/$PROFILE"

  cat > "$TP/$PROFILE/install.sh" <<EOF
REQUIRES="kind kubectl helm envsubst"

create() {
  kind create cluster --name ${CLUSTER_NAME} --image kindest/node:v1.24.17 --config "\$KINDTKS_KIND_CONFIG"
  kubectl --context kind-${CLUSTER_NAME} get nodes
  echo 'ns: \${CHART_NS}' | CHART_NS=istio-system envsubst
  helm --kube-context kind-${CLUSTER_NAME} install istio-base "\$KINDTKS_CHARTS_DIR/base-1.16.7.tgz" --namespace istio-system --create-namespace
}

delete() {
  kind delete cluster --name ${CLUSTER_NAME}
}
EOF
  cat > "$TP/$PROFILE/kind-config.yaml" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
EOF
  : > "$TP/$PROFILE/config.yaml"

  cat > "$H/work/my.yaml" <<'EOF'
registries:
  docker.io:
    hosts: |
      server = "https://registry-1.docker.io"
EOF
}

teardown_file() {
  kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
  docker rmi "$IMAGE" >/dev/null 2>&1 || true
}

# The documented command, with HOME pointed at the test home and the test
# profile mounted in place of the image's profiles.
kt() {
  (cd "$H/work" && docker run --rm --network host \
    --user "$(id -u):$(id -g)" --group-add "$(stat -c %g /var/run/docker.sock)" \
    --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
    -e HOME="$H" \
    --mount type=bind,source="$H/.kube",target="$H/.kube" \
    --mount type=bind,source="$H/.local/state/kindtks",target="$H/.local/state/kindtks" \
    --mount type=bind,source="$PWD",target="$PWD",readonly -w "$PWD" \
    --mount type=bind,source="$TP",target=/test-profiles,readonly -e KINDTKS_PROFILES_DIR=/test-profiles \
    "$IMAGE" "$@")
}

@test "no arguments prints the README with this image" {
  run docker run --rm "$IMAGE"
  assert_success
  assert_output --partial "docker run --rm --network host"
  # KINDTKS_IMAGE is the published reference with this build's VERSION (dev).
  assert_output --partial "ghcr.io/null-ptr-exception/kindtks:dev create gen1"
}

@test "image's own profiles list and config" {
  run docker run --rm "$IMAGE" list
  assert_success
  assert_output --partial "gen1"
  run docker run --rm "$IMAGE" config gen1
  assert_success
  assert_output --partial "extraHosts"
}

@test "missing kube mount is reported with a hint" {
  run docker run --rm --user "$(id -u):$(id -g)" \
    --group-add "$(stat -c %g /var/run/docker.sock)" \
    --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
    -e HOME="$H" \
    --mount type=bind,source="$H/.local/state/kindtks",target="$H/.local/state/kindtks" \
    --mount type=bind,source="$TP",target=/test-profiles,readonly -e KINDTKS_PROFILES_DIR=/test-profiles \
    "$IMAGE" create "$PROFILE"
  assert_failure
  assert_output --partial "$H/.kube"
  assert_output --partial "docker run --rm ghcr.io/null-ptr-exception/kindtks:dev"
}

@test "create through docker run" {
  run kt create "$PROFILE" --config my.yaml
  assert_success
  assert_output --partial "${CLUSTER_NAME}-control-plane"
  assert_output --partial "ns: istio-system"
  assert_output --partial "STATUS: deployed"
}

@test "host files belong to the user" {
  [ "$(stat -c %u "$H/.kube/config")" = "$(id -u)" ]
  [ "$(stat -c %u "$H/.local/state/kindtks/$PROFILE/certs.d/docker.io/hosts.toml")" = "$(id -u)" ]
}

@test "state dir reaches the node by its host path" {
  run docker exec "${CLUSTER_NAME}-control-plane" cat /etc/containerd/certs.d/docker.io/hosts.toml
  assert_success
  assert_output --partial "registry-1.docker.io"
}

@test "host kubectl uses the written kubeconfig" {
  run kubectl --kubeconfig "$H/.kube/config" --context "kind-${CLUSTER_NAME}" get nodes
  assert_success
}

@test "delete through docker run" {
  run kt delete "$PROFILE"
  assert_success
  [ ! -e "$H/.local/state/kindtks/$PROFILE" ]
  run kind get clusters
  refute_output --partial "$CLUSTER_NAME"
}
