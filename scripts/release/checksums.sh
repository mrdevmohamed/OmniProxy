#!/usr/bin/env bash
# Generate SHA256SUMS over every file in the given directory.
#
# Usage: checksums.sh <assets-dir>
# Writes <assets-dir>/SHA256SUMS and prints it. Excludes SHA256SUMS itself so
# re-runs stay idempotent (the manifest never lists itself).
set -euo pipefail

ASSETS_DIR="${1:?usage: checksums.sh <assets-dir>}"

if [ ! -d "${ASSETS_DIR}" ]; then
    echo "error: assets dir '${ASSETS_DIR}' does not exist" >&2
    exit 1
fi

cd "${ASSETS_DIR}"

rm -f SHA256SUMS
find . -maxdepth 1 -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS

if [ ! -s SHA256SUMS ]; then
    echo "error: no files to checksum in '${ASSETS_DIR}'" >&2
    exit 1
fi

echo "==> SHA256SUMS written for $(wc -l < SHA256SUMS) file(s):"
cat SHA256SUMS
