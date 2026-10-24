#!/usr/bin/env bats
# scripts/release-notes.sh: the release notes in CHANGELOG/<version>.md.

load '../helpers/test_helper'

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../../scripts/release-notes.sh"
  D="${BATS_TEST_TMPDIR}/CHANGELOG"
  mkdir -p "$D"
  printf '### New\n\n- feature one\n' > "$D/0.7.0.md"
}

@test "prints the notes of the version" {
  run "$SCRIPT" 0.7.0 "$D"
  assert_success
  assert_output "### New

- feature one"
}

@test "a pre-release uses the notes of its final version" {
  run "$SCRIPT" 0.7.0-rc1 "$D"
  assert_success
  assert_output --partial "- feature one"
}

@test "a missing file is an error" {
  run "$SCRIPT" 0.7.1 "$D"
  assert_failure 1
  assert_output --partial "release notes: $D/0.7.1.md not found"
}

@test "an empty file is an error" {
  printf '\n  \n' > "$D/0.7.2.md"
  run "$SCRIPT" 0.7.2 "$D"
  assert_failure 1
  assert_output --partial "release notes: $D/0.7.2.md is empty"
}
