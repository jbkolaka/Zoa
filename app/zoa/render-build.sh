#!/usr/bin/env bash
#
# Render static-site build for the Zoa Flutter web app.
#
# Render's build image ships Node and Python, not Flutter, so the SDK is fetched
# here. Pinned to an exact tag rather than tracking the `stable` branch: an
# unattended `stable` would move under the deploy, turning an unrelated push
# into a Flutter upgrade and a build failure nobody changed anything to cause.
set -euo pipefail

FLUTTER_VERSION=3.47.0
FLUTTER_HOME="${FLUTTER_HOME:-${HOME}/flutter}"

# Guard on the binary, not the directory: a build cache that restored a partial
# clone would otherwise skip the fetch and fail later with "flutter: not found".
if [ ! -x "${FLUTTER_HOME}/bin/flutter" ]; then
  echo "==> fetching Flutter ${FLUTTER_VERSION}"
  rm -rf "${FLUTTER_HOME}"
  git clone --depth 1 --branch "${FLUTTER_VERSION}" \
    https://github.com/flutter/flutter.git "${FLUTTER_HOME}"
else
  echo "==> reusing cached Flutter at ${FLUTTER_HOME}"
fi

export PATH="${FLUTTER_HOME}/bin:${PATH}"

# Render's builder checks out as a different user than the one that cached the
# SDK; without this, git refuses to run inside the clone and every flutter
# command dies on "detected dubious ownership".
git config --global --add safe.directory "${FLUTTER_HOME}" || true

flutter --version
flutter pub get

# ZOA_API_BASE_URL must be an absolute URL pointing at the deployed API.
# ZoaConfig.apiBaseUrl defaults to http://localhost:8080 on web, which in a
# browser means the *visitor's* own machine — so a missing value here does not
# fail the build, it ships a site where every request quietly fails. Refuse to
# build instead.
: "${ZOA_API_BASE_URL:?must be set (see render.yaml) — refusing to ship a build pointed at localhost}"

echo "==> building web against ${ZOA_API_BASE_URL}"
flutter build web --release \
  --dart-define=ZOA_API_BASE_URL="${ZOA_API_BASE_URL}"
