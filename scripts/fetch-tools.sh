#!/bin/sh
# fetch-tools.sh LOCK OUT: download every tool in LOCK, check its SHA-256,
# and install it as OUT/<platform>/<tool>. Helm tarballs hold <platform>/helm.
set -eu
lock="$1"
out="$2"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
grep -v '^#' "$lock" | while read -r platform tool sha url; do
  [ -n "$platform" ] || continue
  mkdir -p "$out/$platform"
  wget -qO "$work/dl" "$url"
  if ! echo "$sha  $work/dl" | sha256sum -c - >/dev/null 2>&1; then
    echo "checksum mismatch: $url" >&2
    exit 1
  fi
  case "$url" in
    *.tar.gz)
      tar xzf "$work/dl" -C "$work" "$platform/$tool"
      install -m 0755 "$work/$platform/$tool" "$out/$platform/$tool"
      ;;
    *) install -m 0755 "$work/dl" "$out/$platform/$tool" ;;
  esac
done
for p in linux-amd64 linux-arm64 darwin-arm64; do
  for t in kind kubectl helm; do
    [ -x "$out/$p/$t" ] || { echo "missing $p/$t in $lock" >&2; exit 1; }
  done
done
