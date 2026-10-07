#!/usr/bin/env bats
# Registry credentials end to end against a local registry:2 with htpasswd:
# checked before anything is created, and asked for on a terminal with
# `auth: prompt`. The test profile only prints its kind config; no cluster
# is created.

load '../helpers/test_helper'

REG_NAME="kindtks-auth-e2e-reg"
PROFILE="auth-e2e"

setup_file() {
  export E2E_HOME="${BATS_FILE_TMPDIR}/home"
  export KINDTKS_PROFILES_DIR="${BATS_FILE_TMPDIR}/profiles"
  mkdir -p "$E2E_HOME" "$KINDTKS_PROFILES_DIR/$PROFILE" "${BATS_FILE_TMPDIR}/auth"
  export KT="${BATS_FILE_TMPDIR}/kindtks"
  go build -o "$KT" "${BATS_TEST_DIRNAME}/../../"

  docker run --rm --entrypoint htpasswd httpd:2 -Bbn e2euser e2epass > "${BATS_FILE_TMPDIR}/auth/htpasswd"
  docker rm -f "$REG_NAME" >/dev/null 2>&1 || true
  docker run -d --name "$REG_NAME" -p 127.0.0.1::5000 \
    -v "${BATS_FILE_TMPDIR}/auth:/auth:ro" \
    -e REGISTRY_AUTH=htpasswd -e REGISTRY_AUTH_HTPASSWD_REALM=e2e \
    -e REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd registry:2 >/dev/null
  local port
  port=$(docker port "$REG_NAME" 5000/tcp | head -n 1 | cut -d: -f2)
  export REG="127.0.0.1:${port}"
  for _ in $(seq 1 30); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "http://${REG}/v2/")" = 401 ] && break
    sleep 1
  done

  cat > "$KINDTKS_PROFILES_DIR/$PROFILE/install.sh" <<'INNER'
REQUIRES=""
create() { echo "--- kind config"; cat "$KINDTKS_KIND_CONFIG"; }
delete() { :; }
INNER
  printf 'kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n' \
    > "$KINDTKS_PROFILES_DIR/$PROFILE/kind-config.yaml"
  printf 'images:\n  probe: %s/probe:v1\n' "$REG" > "$KINDTKS_PROFILES_DIR/$PROFILE/config.yaml"
}

teardown_file() {
  docker rm -f "$REG_NAME" >/dev/null 2>&1 || true
}

kt() {
  HOME="$E2E_HOME" "$KT" "$@"
}

write_config() { # write_config AUTH_YAML
  printf 'registries:\n  "%s":\n    auth: %s\n' "$REG" "$1" > "${BATS_TEST_TMPDIR}/config.yaml"
}

@test "wrong credentials in the file stop create before the profile runs" {
  write_config '{username: e2euser, password: wrong}'
  run kt create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml"
  assert_failure
  assert_output --partial "rejected by the registry"
  refute_output --partial "--- kind config"
}

@test "right credentials in the file are checked and applied" {
  write_config '{username: e2euser, password: e2epass}'
  run kt create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml"
  assert_success
  assert_output --partial "configs.\"${REG}\".auth"
  assert_output --partial 'username = "e2euser"'
}

@test "--skip-auth-check creates with unchecked credentials" {
  write_config '{username: e2euser, password: wrong}'
  run kt create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml" --skip-auth-check
  assert_success
  assert_output --partial "--- kind config"
}

@test "auth: prompt without a terminal fails" {
  write_config prompt
  run setsid -w env HOME="$E2E_HOME" "$KT" create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml" </dev/null
  assert_failure
  assert_output --partial 'is "prompt", but there is no terminal to ask on'
}

@test "auth: prompt asks on the terminal and asks again after a rejection" {
  write_config prompt
  local cmd="HOME='$E2E_HOME' '$KT' create '$PROFILE' --config '${BATS_TEST_TMPDIR}/config.yaml'"
  run bash -c "{ sleep 1; printf 'e2euser\n'; sleep 1; printf 'wrong\n'; sleep 2; printf 'e2euser\n'; sleep 1; printf 'e2epass\n'; sleep 3; } | script -qec \"$cmd\" /dev/null"
  assert_success
  assert_output --partial "Rejected by ${REG}"
  assert_output --partial 'username = "e2euser"'
  assert_output --partial 'password = "e2epass"'
}
