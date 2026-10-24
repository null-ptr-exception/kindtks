#!/bin/sh
# release-notes.sh VERSION [DIR]: print the release notes of VERSION, the
# file DIR/X.Y.Z.md (DIR defaults to CHANGELOG). A pre-release (X.Y.Z-rcN)
# uses the file of X.Y.Z. Fails when the file is missing or blank.
set -eu
version="$1"
dir="${2:-CHANGELOG}"
file="$dir/${version%%-*}.md"
if [ ! -f "$file" ]; then
  echo "release notes: $file not found" >&2
  exit 1
fi
if ! grep -q '[^[:space:]]' "$file"; then
  echo "release notes: $file is empty" >&2
  exit 1
fi
cat "$file"
