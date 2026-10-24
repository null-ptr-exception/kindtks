#!/usr/bin/env bats
# The documented one-liner against a real image pulled under another name
# from a temporary registry, installing into a temporary HOME.

load '../helpers/test_helper'

REGISTRY_NAME="kindtks-installer-e2e-reg"

setup_file() {
  export BUILD_ID="e2e-$(date +%s)-$$"
  export SRC_IMAGE="kindtks:installer-e2e"
  docker build --build-arg BUILD_ID="$BUILD_ID" -t "$SRC_IMAGE" "${BATS_TEST_DIRNAME}/../../"

  docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
  docker run -d --name "$REGISTRY_NAME" -p 127.0.0.1::5000 registry:2 >/dev/null
  local port
  port=$(docker port "$REGISTRY_NAME" 5000/tcp | head -n 1 | cut -d: -f2)
  for _ in $(seq 1 30); do
    curl -fs "http://127.0.0.1:${port}/v2/" >/dev/null && break
    sleep 1
  done
  export CORP_IMAGE="localhost:${port}/corp/kindtks:e2e"
  docker tag "$SRC_IMAGE" "$CORP_IMAGE"
  docker push -q "$CORP_IMAGE"
  docker rmi "$SRC_IMAGE" "$CORP_IMAGE" >/dev/null

  export H="${BATS_FILE_TMPDIR}/home"
  mkdir -p "$H"
}

teardown_file() {
  docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
  docker rmi -f "$CORP_IMAGE" "$SRC_IMAGE" >/dev/null 2>&1 || true
}

@test "no local copy of the image before installing" {
  run docker images -q --filter "label=io.kindtks.build=$BUILD_ID"
  assert_output ""
}

@test "one-liner installs from the registry name" {
  run bash -c "docker run --rm '$CORP_IMAGE' install-script | HOME='$H' bash -s -- --yes"
  assert_success
  assert_output --partial "Get started: kindtks list"
}

@test "installed files belong to the user" {
  for f in "$H/.local/bin/kindtks" "$H/.local/share/kindtks/bin/kind" \
    "$H/.local/share/kindtks/bin/kubectl" "$H/.local/share/kindtks/bin/helm"; do
    [ -x "$f" ]
    [ -O "$f" ]
  done
  [ -O "$H/.local/share/kindtks/profiles/gen1/install.sh" ]
}

@test "installed binaries are built for this host" {
  local os
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  run "$H/.local/share/kindtks/bin/kind" version
  assert_success
  assert_output --partial " $os/"
  run "$H/.local/bin/kindtks" --version
  assert_success
}

@test "installed kindtks lists gen1 from the installed profiles" {
  run env -u KINDTKS_PROFILES_DIR HOME="$H" "$H/.local/bin/kindtks" list
  assert_success
  assert_output --partial "gen1"
}

@test "bundled tools are the pinned versions" {
  run "$H/.local/share/kindtks/bin/kind" version
  assert_output --partial "v0.33.0"
  run "$H/.local/share/kindtks/bin/kubectl" version --client
  assert_output --partial "v1.29.15"
  run "$H/.local/share/kindtks/bin/helm" version --short
  assert_output --partial "v3.21.0"
}

@test "installed charts are present" {
  run ls "$H/.local/share/kindtks/charts"
  assert_output --partial "cilium-1.13.10.tgz"
}
