#!/usr/bin/env bats
# scripts/release-info.sh: which versions release, and how.

load '../helpers/test_helper'

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../../scripts/release-info.sh"
  F="${BATS_TEST_TMPDIR}/VERSION"
}

@test "a plain version is a release" {
  echo "0.7.0" > "$F"
  run "$SCRIPT" "$F"
  assert_success
  assert_output "version=0.7.0
release=true
prerelease=false"
}

@test "an rc version is a pre-release" {
  echo "0.7.0-rc1" > "$F"
  run "$SCRIPT" "$F"
  assert_success
  assert_line "release=true"
  assert_line "prerelease=true"
}

@test "a dev version never releases" {
  echo "0.8.0-dev" > "$F"
  run "$SCRIPT" "$F"
  assert_success
  assert_line "version=0.8.0-dev"
  assert_line "release=false"
}

@test "surrounding whitespace is ignored" {
  printf '  0.7.0\n\n' > "$F"
  run "$SCRIPT" "$F"
  assert_success
  assert_line "version=0.7.0"
}

@test "invalid versions are refused" {
  for v in v0.7.0 0.7 0.7.0-beta 0.7.0-rc 0.7.0-dev1 ""; do
    echo "$v" > "$F"
    run "$SCRIPT" "$F"
    assert_failure 1
    assert_output --partial "invalid version"
  done
}

@test "the repository VERSION is valid" {
  run "$SCRIPT" "${BATS_TEST_DIRNAME}/../../VERSION"
  assert_success
}
