#!/usr/bin/env bats
# The installer against a fake docker whose "image" is a directory, and fake
# kind/kubectl/helm that print set versions.

load '../helpers/test_helper'

fake_tool() { # fake_tool DIR NAME VERSION [PLATFORM]
  local out
  case "$2" in
    kind) out="kind $3 go1.26.7 ${4:-linux/amd64}" ;;
    kubectl) out="Client Version: $3" ;;
    helm) out="$3+gabcdef0" ;;
  esac
  mkdir -p "$1"
  printf '#!/usr/bin/env bash\necho "%s"\n' "$out" > "$1/$2"
  chmod +x "$1/$2"
}

setup() {
  for t in kind kubectl helm; do
    if PATH=/usr/bin:/bin command -v "$t" >/dev/null; then
      skip "a real $t in /usr/bin or /bin would interfere"
    fi
  done

  export HOME="${BATS_TEST_TMPDIR}/home"
  mkdir -p "$HOME"
  DEST="$HOME/.local/bin"
  DATA="$HOME/.local/share/kindtks"

  export FAKE_IMAGE_ROOT="${BATS_TEST_TMPDIR}/image"
  fake_tool "$FAKE_IMAGE_ROOT/usr/local/bin" kind v0.33.0
  fake_tool "$FAKE_IMAGE_ROOT/usr/local/bin" kubectl v1.33.1
  fake_tool "$FAKE_IMAGE_ROOT/usr/local/bin" helm v3.21.0
  printf '#!/usr/bin/env bash\necho kindtks linux\n' > "$FAKE_IMAGE_ROOT/usr/local/bin/kindtks"
  chmod +x "$FAKE_IMAGE_ROOT/usr/local/bin/kindtks"
  DARWIN="$FAKE_IMAGE_ROOT/dist/darwin-arm64"
  fake_tool "$DARWIN" kind v0.33.0 darwin/arm64
  fake_tool "$DARWIN" kubectl v1.33.1
  fake_tool "$DARWIN" helm v3.21.0
  printf '#!/usr/bin/env bash\necho kindtks darwin\n' > "$DARWIN/kindtks"
  chmod +x "$DARWIN/kindtks"
  mkdir -p "$FAKE_IMAGE_ROOT/profiles/gen1" "$FAKE_IMAGE_ROOT/charts"
  echo 'REQUIRES=""' > "$FAKE_IMAGE_ROOT/profiles/gen1/install.sh"
  touch "$FAKE_IMAGE_ROOT/charts/c-1.0.0.tgz"

  MOCK_BIN="${BATS_TEST_TMPDIR}/mock"
  mkdir -p "$MOCK_BIN"
  export DOCKER_LOG="${BATS_TEST_TMPDIR}/docker.log"
  cat > "$MOCK_BIN/docker" <<'MOCK'
#!/usr/bin/env bash
echo "$*" >> "$DOCKER_LOG"
case "$1" in
  info)
    if [ -n "${FAKE_DOCKER_DOWN:-}" ]; then echo "Cannot connect to the Docker daemon" >&2; exit 1; fi ;;
  images) [ -n "${FAKE_IMAGE_MISSING:-}" ] || echo "fakeimage123" ;;
  create) echo "fakecid" ;;
  cp)
    src="${FAKE_IMAGE_ROOT}${2#fakecid:}"
    if [ -d "$src" ]; then rm -rf "$3"; cp -r "$src" "$3"; else cp "$src" "$3"; fi ;;
  rm) ;;
esac
MOCK
  chmod +x "$MOCK_BIN/docker"

  cat > "$MOCK_BIN/uname" <<'MOCK'
#!/usr/bin/env bash
case "$1" in
  -s) echo "${FAKE_UNAME_S:-Linux}" ;;
  -m) echo "${FAKE_UNAME_M:-x86_64}" ;;
  *) /usr/bin/uname "$@" ;;
esac
MOCK
  chmod +x "$MOCK_BIN/uname"

  HOSTBIN="${BATS_TEST_TMPDIR}/hostbin"
  mkdir -p "$HOSTBIN"
  export PATH="$MOCK_BIN:/usr/bin:/bin"
  export KINDTKS_INSTALLER_TTY="${BATS_TEST_TMPDIR}/no-such-tty"

  INSTALLER="${BATS_TEST_TMPDIR}/install.sh"
  sed 's/__KINDTKS_BUILD_ID__/test-build-1/' "${BATS_TEST_DIRNAME}/../../internal/cmd/installer.sh" > "$INSTALLER"
}

@test "installs kindtks and the private copies" {
  run bash "$INSTALLER" --no-tools
  assert_success
  run "$DEST/kindtks"
  assert_output "kindtks linux"
  [ -x "$DEST/kindtks" ]
  for t in kind kubectl helm; do [ -x "$DATA/bin/$t" ]; done
  [ -f "$DATA/profiles/gen1/install.sh" ]
  [ -f "$DATA/charts/c-1.0.0.tgz" ]
  run cat "$DOCKER_LOG"
  assert_output --partial "images -q --filter label=io.kindtks.build=test-build-1"
  assert_output --partial "rm fakecid"
  assert_output --partial "cp fakecid:/usr/local/bin/kindtks"
}

@test "upgrade replaces the private directories whole" {
  mkdir -p "$DATA/profiles/removed-profile"
  run bash "$INSTALLER" --no-tools
  assert_success
  [ ! -e "$DATA/profiles/removed-profile" ]
}

@test "missing tools are installed with --yes" {
  run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "Install kubectl v1.33.1 to $DEST? [Y/n] yes (--yes)"
  for t in kind kubectl helm; do [ -x "$DEST/$t" ]; done
}

@test "missing tools are skipped without a terminal" {
  run bash "$INSTALLER"
  assert_success
  assert_output --partial "Install kubectl v1.33.1 to $DEST? [Y/n] skipped: no terminal"
  assert_output --partial "--yes"
  [ ! -e "$DEST/kubectl" ]
}

@test "answers are read from the terminal" {
  printf 'y\nn\n\n' > "${BATS_TEST_TMPDIR}/tty"
  KINDTKS_INSTALLER_TTY="${BATS_TEST_TMPDIR}/tty" run bash "$INSTALLER"
  assert_success
  [ -x "$DEST/kind" ]
  [ ! -e "$DEST/kubectl" ]
  [ -x "$DEST/helm" ]
}

@test "older tool is offered when the destination comes first on PATH" {
  fake_tool "$HOSTBIN" kubectl v1.29.3
  PATH="$MOCK_BIN:$DEST:$HOSTBIN:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "kubectl v1.29.3 at $HOSTBIN/kubectl is older than bundled v1.33.1. Install v1.33.1 to $DEST ($DEST comes first on PATH)?"
  [ -x "$DEST/kubectl" ]
  run "$HOSTBIN/kubectl"
  assert_output "Client Version: v1.29.3"
}

@test "older tool warns when the destination comes later on PATH" {
  fake_tool "$HOSTBIN" kubectl v1.29.3
  PATH="$MOCK_BIN:$HOSTBIN:$DEST:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "($DEST comes after $HOSTBIN on PATH; the new copy will not take effect until PATH is changed)?"
}

@test "newer tool is only reported" {
  fake_tool "$HOSTBIN" kubectl v1.34.0
  PATH="$MOCK_BIN:$HOSTBIN:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "kubectl v1.34.0 found at $HOSTBIN/kubectl (bundled v1.33.1)"
  [ ! -e "$DEST/kubectl" ]
}

@test "--dest installs elsewhere and prints the PATH line" {
  run bash "$INSTALLER" --no-tools --dest "${BATS_TEST_TMPDIR}/custom"
  assert_success
  [ -x "${BATS_TEST_TMPDIR}/custom/kindtks" ]
  assert_output --partial "${BATS_TEST_TMPDIR}/custom is not on PATH"
  assert_output --partial "export PATH=\"${BATS_TEST_TMPDIR}/custom:\$PATH\""
}

@test "image not found" {
  FAKE_IMAGE_MISSING=1 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "kindtks image with build test-build-1 not found locally"
}

@test "docker not usable" {
  FAKE_DOCKER_DOWN=1 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "docker is not usable"
  assert_output --partial "Cannot connect to the Docker daemon"
}

@test "unknown option" {
  run bash "$INSTALLER" --bogus
  assert_failure 2
  assert_output --partial "unknown option: --bogus"
}

@test "symlinks in the destination are replaced, not written through" {
  mkdir -p "$DEST" "${BATS_TEST_TMPDIR}/elsewhere"
  fake_tool "${BATS_TEST_TMPDIR}/elsewhere" helm v3.10.0
  printf '#!/usr/bin/env bash\necho old\n' > "${BATS_TEST_TMPDIR}/elsewhere/kindtks"
  chmod +x "${BATS_TEST_TMPDIR}/elsewhere/kindtks"
  ln -s "${BATS_TEST_TMPDIR}/elsewhere/helm" "$DEST/helm"
  ln -s "${BATS_TEST_TMPDIR}/elsewhere/kindtks" "$DEST/kindtks"
  PATH="$MOCK_BIN:$DEST:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  run "${BATS_TEST_TMPDIR}/elsewhere/helm"
  assert_output "v3.10.0+gabcdef0"
  run "${BATS_TEST_TMPDIR}/elsewhere/kindtks"
  assert_output "old"
  [ ! -L "$DEST/helm" ]
  [ ! -L "$DEST/kindtks" ]
  [ -x "$DEST/helm" ]
  [ -x "$DEST/kindtks" ]
}

@test "a truncated installer runs nothing" {
  sed '$d' "$INSTALLER" > "${BATS_TEST_TMPDIR}/truncated.sh"
  run bash "${BATS_TEST_TMPDIR}/truncated.sh" --yes
  assert_success
  [ ! -e "$HOME/.local" ]
}

@test "macOS on Apple Silicon gets the darwin binaries" {
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 run bash "$INSTALLER" --yes
  assert_success
  run "$DEST/kindtks"
  assert_output "kindtks darwin"
  run "$DATA/bin/kind"
  assert_output --partial "darwin/arm64"
  run "$DEST/kind"
  assert_output --partial "darwin/arm64"
}

@test "Intel Mac is refused before anything is installed" {
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=x86_64 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "error: Intel Macs are not supported"
  [ ! -e "$HOME/.local" ]
}

@test "other systems are refused" {
  FAKE_UNAME_S=FreeBSD FAKE_UNAME_M=amd64 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "error: unsupported system: FreeBSD amd64"
}
