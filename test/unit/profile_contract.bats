#!/usr/bin/env bats

load '../helpers/test_helper'

PROFILES_DIR="${BATS_TEST_DIRNAME}/../../profiles"

@test "all profiles have install.sh" {
  local missing=()
  for dir in "$PROFILES_DIR"/*/; do
    local name
    name=$(basename "$dir")
    if [ ! -f "${dir}/install.sh" ]; then
      missing+=("$name")
    fi
  done
  assert_equal "${#missing[@]}" 0 "Profiles missing install.sh: ${missing[*]}"
}

@test "all profiles define REQUIRES" {
  local missing=()
  for dir in "$PROFILES_DIR"/*/; do
    local name
    name=$(basename "$dir")
    if ! grep -q '^REQUIRES=' "${dir}/install.sh"; then
      missing+=("$name")
    fi
  done
  assert_equal "${#missing[@]}" 0 "Profiles missing REQUIRES: ${missing[*]}"
}

@test "all profiles define create function" {
  local missing=()
  for dir in "$PROFILES_DIR"/*/; do
    local name
    name=$(basename "$dir")
    if ! grep -q '^create()' "${dir}/install.sh"; then
      missing+=("$name")
    fi
  done
  assert_equal "${#missing[@]}" 0 "Profiles missing create(): ${missing[*]}"
}

@test "all profiles define delete function" {
  local missing=()
  for dir in "$PROFILES_DIR"/*/; do
    local name
    name=$(basename "$dir")
    if ! grep -q '^delete()' "${dir}/install.sh"; then
      missing+=("$name")
    fi
  done
  assert_equal "${#missing[@]}" 0 "Profiles missing delete(): ${missing[*]}"
}
