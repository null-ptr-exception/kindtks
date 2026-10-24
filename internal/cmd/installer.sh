#!/usr/bin/env bash
# kindtks installer for one image build. Printed by
#   docker run --rm <kindtks image> install-script | bash -s -- [--yes] [--no-tools] [--dest DIR]
# It finds the image on this host by its build label, copies kindtks and the
# bundled kind, kubectl and helm for this host (Linux, or macOS on Apple
# Silicon) out of it, and offers the bundled tools where the host lacks them
# or has older ones.
set -euo pipefail

BUILD_ID="__KINDTKS_BUILD_ID__"
DATA_DIR="$HOME/.local/share/kindtks"
DEST="$HOME/.local/bin"
TTY="${KINDTKS_INSTALLER_TTY:-/dev/tty}"
YES=0
TOOLS=1

usage() {
  echo "usage: docker run --rm <kindtks image> install-script | bash -s -- [--yes] [--no-tools] [--dest DIR]"
}

die() {
  echo "error: $*" >&2
  exit 1
}

copy_out() { # copy_out PATH_IN_IMAGE HOST_PATH
  local out
  out=$(docker cp "$cid:$1" "$2" </dev/null 2>&1) || die "copying $1 from the image: $out"
}

version_of() { # version_of TOOL PATH: prints vX.Y.Z, or nothing
  local args
  case "$1" in
    kind) args=(version) ;;
    kubectl) args=(version --client) ;;
    helm) args=(version --short) ;;
  esac
  "$2" "${args[@]}" </dev/null 2>/dev/null | grep -o 'v[0-9]\+\.[0-9]\+\.[0-9]\+' | head -n 1 || true
}

older() { # older A B: version A is older than version B
  [ "${1#v}" != "${2#v}" ] &&
    [ "$(printf '%s\n%s\n' "${1#v}" "${2#v}" | sort -V | head -n 1)" = "${1#v}" ]
}

path_index() { # path_index DIR: position of DIR on PATH, or -1
  local i=0 entry IFS=:
  for entry in $PATH; do
    if [ "${entry%/}" = "$1" ]; then
      echo "$i"
      return
    fi
    i=$((i + 1))
  done
  echo -1
}

tty_state=unknown
confirm() { # confirm QUESTION: succeeds when the answer is yes
  local answer
  if [ "$YES" = 1 ]; then
    echo "$1 [Y/n] yes (--yes)"
    return 0
  fi
  if [ "$tty_state" = unknown ]; then
    if { exec 3<"$TTY"; } 2>/dev/null; then tty_state=open; else tty_state=none; fi
  fi
  if [ "$tty_state" = none ]; then
    echo "$1 [Y/n] skipped: no terminal"
    need_yes=1
    return 1
  fi
  printf '%s [Y/n] ' "$1"
  read -r answer <&3 || answer=n
  [ -t 3 ] || echo "$answer"
  case "$answer" in
    "" | y | Y | yes | Yes | YES) return 0 ;;
    *) return 1 ;;
  esac
}

main() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --yes) YES=1 ;;
      --no-tools) TOOLS=0 ;;
      --dest)
        [ $# -ge 2 ] || { usage >&2; exit 2; }
        DEST="$2"
        shift
        ;;
      -h | --help) usage; exit 0 ;;
      *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
    shift
  done
  DEST="${DEST%/}"

  # 0. The binaries for this host. Linux hosts run the image's own
  # architecture, so its /usr/local/bin fits.
  case "$(uname -s)/$(uname -m)" in
    Linux/*) SRC=/usr/local/bin ;;
    Darwin/arm64) SRC=/dist/darwin-arm64 ;;
    Darwin/*) die "Intel Macs are not supported (on Apple Silicon, run the installer outside Rosetta)" ;;
    *) die "unsupported system: $(uname -s) $(uname -m)" ;;
  esac

  # 1. Docker must work for this user.
  if ! err=$(docker info 2>&1 >/dev/null </dev/null); then
    die "docker is not usable as $(id -un): $err"
  fi

  image=$(docker images -q --filter "label=io.kindtks.build=$BUILD_ID" </dev/null | head -n 1)
  [ -n "$image" ] || die "kindtks image with build $BUILD_ID not found locally"
  cid=$(docker create "$image" </dev/null)
  tmp_files=()
  cleanup() {
    docker rm "$cid" </dev/null >/dev/null 2>&1 || true
    if [ ${#tmp_files[@]} -gt 0 ]; then rm -f "${tmp_files[@]}"; fi
  }
  trap cleanup EXIT

  # 2. kindtks, and the private tools, profiles and charts it runs with.
  mkdir -p "$DATA_DIR" "$DEST"
  rm -rf "$DATA_DIR/bin.new" "$DATA_DIR/profiles.new" "$DATA_DIR/charts.new"
  mkdir "$DATA_DIR/bin.new"
  for tool in kind kubectl helm; do
    copy_out "$SRC/$tool" "$DATA_DIR/bin.new/$tool"
  done
  copy_out /profiles "$DATA_DIR/profiles.new"
  copy_out /charts "$DATA_DIR/charts.new"
  for dir in bin profiles charts; do
    rm -rf "${DATA_DIR:?}/$dir"
    mv "$DATA_DIR/$dir.new" "$DATA_DIR/$dir"
  done
  # Write via a temp file in DEST and rename, so a symlink at the final name is
  # replaced instead of its target being overwritten.
  tmp="$DEST/.kindtks-install.kindtks.$$"
  tmp_files+=("$tmp")
  copy_out "$SRC/kindtks" "$tmp"
  mv -f "$tmp" "$DEST/kindtks"

  installed=("kindtks -> $DEST/kindtks")
  skipped=()
  need_yes=0

  # 3. kind, kubectl and helm for using clusters yourself.
  if [ "$TOOLS" = 1 ]; then
    dest_index=$(path_index "$DEST")
    for tool in kind kubectl helm; do
      bundled=$(version_of "$tool" "$DATA_DIR/bin/$tool")
      current=$(command -v "$tool" || true)
      if [ -z "$current" ]; then
        question="Install $tool $bundled to $DEST?"
      else
        have=$(version_of "$tool" "$current")
        if [ -z "$have" ]; then
          echo "$tool found at $current, version unknown (bundled $bundled)"
          continue
        fi
        if ! older "$have" "$bundled"; then
          echo "$tool $have found at $current (bundled $bundled)"
          continue
        fi
        question="$tool $have at $current is older than bundled $bundled. Install $bundled to $DEST"
        current_dir=$(dirname "$current")
        if [ "$current_dir" = "$DEST" ]; then
          question="$question?"
        elif [ "$dest_index" = -1 ]; then
          question="$question ($DEST is not on PATH yet)?"
        elif [ "$dest_index" -lt "$(path_index "$current_dir")" ]; then
          question="$question ($DEST comes first on PATH)?"
        else
          question="$question ($DEST comes after $current_dir on PATH; the new copy will not take effect until PATH is changed)?"
        fi
      fi
      if confirm "$question"; then
        tmp="$DEST/.kindtks-install.$tool.$$"
        tmp_files+=("$tmp")
        cp "$DATA_DIR/bin/$tool" "$tmp"
        mv -f "$tmp" "$DEST/$tool"
        installed+=("$tool $bundled -> $DEST/$tool")
      else
        skipped+=("$tool")
      fi
    done
  fi

  # 4 and 5. PATH and summary.
  echo
  echo "Installed:"
  printf '  %s\n' "${installed[@]}"
  echo "  kind, kubectl, helm, profiles, charts -> $DATA_DIR (used by kindtks)"
  if [ ${#skipped[@]} -gt 0 ]; then
    echo "Skipped: ${skipped[*]}"
    if [ "$need_yes" = 1 ]; then
      echo "  No terminal for prompts; to accept them, rerun with: ... install-script | bash -s -- --yes"
    fi
  fi
  if [ "$(path_index "$DEST")" = -1 ]; then
    echo
    echo "$DEST is not on PATH. Add this line to your shell profile:"
    printf '  export PATH="%s:$PATH"\n' "$DEST"
  fi
  echo
  echo "Get started: kindtks list"
}

main "$@"
