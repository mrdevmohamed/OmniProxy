#!/usr/bin/env bash
# Generate release notes (Markdown) from commits since the previous release tag.
# Uses the gh CLI (preinstalled on GitHub-hosted runners) with GH_TOKEN /
# GITHUB_TOKEN for auth.
#
# Env required:
#   GITHUB_REPOSITORY   owner/repo, e.g. mrdevmohamed/OmniProxy
#   TAG                 current release tag, e.g. v1.0.0
# Arg: $1 = output file path (e.g. release-assets/CHANGELOG.md)
#
# Logic: find the most recent release tag before TAG (gh api releases, which
# lists newest first; skip TAG; take the first). If a previous tag exists, list
# commit subjects via the compare API (PREV...TAG); otherwise fall back to
# `git log` over the full history (works for the very first release, which is
# why the caller checks out with fetch-depth: 0). Robust when there is no
# previous tag or no commits.
set -euo pipefail

OUTPUT="${1:?usage: changelog.sh <output-file>}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY env is required}"
TAG="${TAG:?TAG env is required}"

REPO="${GITHUB_REPOSITORY}"

command -v gh >/dev/null 2>&1 || { echo "error: gh CLI not found" >&2; exit 1; }

# Most recent release tag before TAG (newest-first, skipping the current one).
# `gh api` emits `null` when there are no releases yet. Auth/API errors exit
# non-zero and fail the release loudly rather than emitting an empty changelog.
PREV="$(gh api "repos/${REPO}/releases" --paginate \
    --jq "[.[].tag_name] | map(select(. != \"${TAG}\"))[0]" 2>/dev/null)"

if [ -z "${PREV}" ] || [ "${PREV}" = "null" ]; then
    PREV=""
fi

# Commit subjects as a bulleted list. `sed 's/^/- /'` also prefixes every line
# of a multi-line message, so we extract the first line (subject) in jq first.
if [ -n "${PREV}" ]; then
    COMMITS="$(gh api "repos/${REPO}/compare/${PREV}...${TAG}" \
        --jq '.commits[].commit.message | split("\n")[0] | select(length > 0)' \
        2>/dev/null | sed 's/^/- /')"
else
    COMMITS="$(git log --format='%s' 2>/dev/null | sed 's/^/- /')"
fi

{
    echo "# Release ${TAG}"
    echo
    echo "Released: $(date -u +%Y-%m-%d)"
    echo
    if [ -n "${PREV}" ]; then
        echo "Commits since ${PREV}:"
    else
        echo "All commits:"
    fi
    echo
    if [ -n "${COMMITS}" ]; then
        printf '%s\n' "${COMMITS}"
    else
        echo "- No commit messages available for this release."
    fi
} > "${OUTPUT}"

echo "==> changelog written to ${OUTPUT}"
