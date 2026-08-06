#!/usr/bin/env bash
# Package the Linux Flutter release bundle into a relocatable tarball.
# The bundle dir (app/build/linux/x64/release/bundle) is self-contained and
# relocatable, so it is archived as-is under the `bundle/` prefix.
#
# Env required:
#   VERSION            release version without a leading "v", e.g. 1.0.0
#   GITHUB_WORKSPACE   repo root (set by GitHub Actions)
#
# Produces: dist/omniproxy-${VERSION}-linux-x64.tar.gz
set -euo pipefail

VERSION="${VERSION:?VERSION is required (e.g. 1.0.0)}"
WORKSPACE="${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"

BUNDLE_PARENT="${WORKSPACE}/app/build/linux/x64/release"
BUNDLE_DIR="${BUNDLE_PARENT}/bundle"
OUT="${WORKSPACE}/dist/omniproxy-${VERSION}-linux-x64.tar.gz"

if [ ! -d "${BUNDLE_DIR}" ]; then
    echo "error: Linux bundle not found at ${BUNDLE_DIR}" >&2
    exit 1
fi

cd "${WORKSPACE}"
rm -rf dist
mkdir -p dist

tar -czf "${OUT}" -C "${BUNDLE_PARENT}" bundle

# Fail the build if the archive was not produced (guards against a silent
# tar error, e.g. a race with the build step).
test -f "${OUT}"

echo "==> packaged ${OUT}"
ls -lh dist
