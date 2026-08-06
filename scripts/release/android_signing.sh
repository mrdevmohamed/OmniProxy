#!/usr/bin/env bash
#
# Configure Android release signing from CI secrets.
#
# Reads (env): KEYSTORE_BASE64, KEYSTORE_PASSWORD, KEY_ALIAS, KEY_PASSWORD
# Writes:     app/android/upload-keystore.jks, app/android/key.properties
#
# If KEYSTORE_BASE64 is empty/missing (local dev, forks without signing
# secrets), nothing is written and the build falls back to the debug key,
# so builds never break.
#
# app/android/app/build.gradle.kts reads key.properties when present and
# resolves storeFile relative to the Gradle root project (app/android).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

if [[ -z "${KEYSTORE_BASE64:-}" ]]; then
    echo "::warning::No Android signing secrets; building with the debug key."
    exit 0
fi

: "${KEYSTORE_PASSWORD:?KEYSTORE_PASSWORD is required when KEYSTORE_BASE64 is set}"
: "${KEY_ALIAS:?KEY_ALIAS is required when KEYSTORE_BASE64 is set}"
: "${KEY_PASSWORD:?KEY_PASSWORD is required when KEYSTORE_BASE64 is set}"

ANDROID_DIR="$ROOT/app/android"
mkdir -p "$ANDROID_DIR"

# Decode the keystore. Never print secrets to logs.
echo "$KEYSTORE_BASE64" | base64 --decode > "$ANDROID_DIR/upload-keystore.jks"
chmod 600 "$ANDROID_DIR/upload-keystore.jks"

# Values are written raw (unquoted): java.util.Properties in build.gradle.kts
# reads them verbatim, so literal quotes would corrupt alias/password. printf
# passes the env values as arguments, so shell expansion/globbing of special
# characters in secrets is avoided.
KEYSTORE_FILE="$ANDROID_DIR/key.properties"
printf 'storeFile=%s\n' 'upload-keystore.jks' > "$KEYSTORE_FILE"
printf 'storePassword=%s\n' "$KEYSTORE_PASSWORD" >> "$KEYSTORE_FILE"
printf 'keyAlias=%s\n' "$KEY_ALIAS" >> "$KEYSTORE_FILE"
printf 'keyPassword=%s\n' "$KEY_PASSWORD" >> "$KEYSTORE_FILE"
chmod 600 "$KEYSTORE_FILE"

echo "::notice::Android release signing configured (upload-keystore.jks + key.properties)."
