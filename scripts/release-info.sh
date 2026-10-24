#!/bin/sh
# release-info.sh [VERSION_FILE]: check the version and print, as
# key=value lines for $GITHUB_OUTPUT:
#   version=X.Y.Z[-rcN|-dev]
#   release=true|false       (false for -dev: development versions never release)
#   prerelease=true|false    (true for -rcN: no "latest" image tag)
set -eu
file="${1:-VERSION}"
v=$(tr -d '[:space:]' < "$file")
if ! echo "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-dev|-rc[0-9]+)?$'; then
  echo "invalid version in $file: '$v' (want X.Y.Z, X.Y.Z-rcN or X.Y.Z-dev)" >&2
  exit 1
fi
release=true
prerelease=false
case "$v" in
  *-dev) release=false ;;
  *-rc*) prerelease=true ;;
esac
echo "version=$v"
echo "release=$release"
echo "prerelease=$prerelease"
