#!/bin/bash
# build-pkg.sh — assemble a macOS .pkg installer for cowork-mdm.
#
# Inputs (args):
#   $1  path to the goreleaser tar.gz (e.g. dist/cowork-mdm_0.3.1_darwin_arm64.tar.gz)
#   $2  arch tag for the output filename (arm64 | x86_64)
#   $3  pkg version — fed to `pkgbuild --version`. Must be numeric dotted
#       (e.g. 0.3.1). Strip any SemVer prerelease suffix before calling.
#   $4  release version — used only in the output filename, may contain
#       suffixes (e.g. 0.3.1-rc.1 → cowork-mdm_0.3.1-rc.1_darwin_arm64.pkg).
#   $5  output directory (created if missing)
#
# Signing / notarization are opt-in via env vars. Any missing set is skipped.
#
# Env vars (optional):
#   MACOS_APP_IDENTITY         Common Name of Developer ID Application cert
#                              (looked up in the default keychain by that name)
#   MACOS_INSTALLER_IDENTITY   Common Name of Developer ID Installer cert
#   NOTARIZE_KEYCHAIN_PROFILE  notarytool keychain profile name (set up earlier
#                              in the workflow via notarytool store-credentials).
#                              Only effective when the installer cert is also
#                              set — Apple's notary rejects unsigned pkgs.
#
# Output:
#   $OUTDIR/cowork-mdm_${RELEASE_VERSION}_darwin_${ARCH}.pkg
#
# Exit codes:
#   0  success
#   1  usage / input error
#   2  pkgbuild / productsign / notarytool failure

set -euo pipefail

if [ $# -ne 5 ]; then
  echo "usage: $0 <tarball> <arch> <pkg_version> <release_version> <outdir>" >&2
  exit 1
fi

TARBALL="$1"
ARCH="$2"
PKG_VERSION="$3"
RELEASE_VERSION="$4"
OUTDIR="$5"

if [ ! -f "$TARBALL" ]; then
  echo "build-pkg: tarball not found: $TARBALL" >&2
  exit 1
fi

mkdir -p "$OUTDIR"

BUNDLE_ID="com.krislavten.cowork-mdm"
STAGE="$(mktemp -d)"
PAYLOAD="$STAGE/payload"
SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)/scripts"

trap 'rm -rf "$STAGE"' EXIT

# ---------------------------------------------------------------------------
# 1. Unpack the goreleaser tarball into payload/opt/cowork-mdm/.
# ---------------------------------------------------------------------------
mkdir -p "$PAYLOAD/opt/cowork-mdm/bin"
# --no-same-owner avoids uid/gid mismatch when the CI runner unpacks a
# tarball owned by someone else. COPYFILE_DISABLE=1 prevents macOS tar
# from materializing AppleDouble ._* files for quarantined inputs, which
# would otherwise pollute the pkg payload (no-op on Linux / CI).
COPYFILE_DISABLE=1 tar -xzf "$TARBALL" -C "$STAGE" --no-same-owner
xattr -cr "$STAGE" 2>/dev/null || true
find "$STAGE" \( -name '._*' -o -name '.DS_Store' \) -delete
# goreleaser tar.gz layout: ./cowork-mdm ./LICENSE ./README.md  (no top-level dir)
find "$STAGE" -maxdepth 2 -name cowork-mdm -type f -perm -u+x -exec \
  cp {} "$PAYLOAD/opt/cowork-mdm/bin/cowork-mdm" \;
for extra in LICENSE README.md; do
  found="$(find "$STAGE" -maxdepth 2 -name "$extra" -type f | head -n1 || true)"
  if [ -n "$found" ]; then
    cp "$found" "$PAYLOAD/opt/cowork-mdm/$extra"
  fi
done

if [ ! -x "$PAYLOAD/opt/cowork-mdm/bin/cowork-mdm" ]; then
  echo "build-pkg: binary missing after unpacking $TARBALL" >&2
  exit 2
fi

chmod 0755 "$PAYLOAD/opt/cowork-mdm/bin/cowork-mdm"

# ---------------------------------------------------------------------------
# 2. Optionally sign the Mach-O binary (Developer ID Application).
# ---------------------------------------------------------------------------
if [ -n "${MACOS_APP_IDENTITY:-}" ]; then
  echo "build-pkg: signing binary with Developer ID Application ($MACOS_APP_IDENTITY)"
  codesign --force --sign "$MACOS_APP_IDENTITY" \
    --options runtime --timestamp \
    "$PAYLOAD/opt/cowork-mdm/bin/cowork-mdm"
  codesign --verify --strict --verbose=2 "$PAYLOAD/opt/cowork-mdm/bin/cowork-mdm"
else
  echo "build-pkg: MACOS_APP_IDENTITY unset — skipping binary signing"
fi

# ---------------------------------------------------------------------------
# 3. Build the unsigned component pkg.
# ---------------------------------------------------------------------------
UNSIGNED_PKG="$STAGE/cowork-mdm-unsigned.pkg"
pkgbuild \
  --root "$PAYLOAD" \
  --identifier "$BUNDLE_ID" \
  --version "$PKG_VERSION" \
  --scripts "$SCRIPTS_DIR" \
  --install-location "/" \
  "$UNSIGNED_PKG"

OUT_PKG="$OUTDIR/cowork-mdm_${RELEASE_VERSION}_darwin_${ARCH}.pkg"

# ---------------------------------------------------------------------------
# 4. Optionally productsign with Developer ID Installer.
# ---------------------------------------------------------------------------
if [ -n "${MACOS_INSTALLER_IDENTITY:-}" ]; then
  echo "build-pkg: productsign with Developer ID Installer ($MACOS_INSTALLER_IDENTITY)"
  productsign --sign "$MACOS_INSTALLER_IDENTITY" \
    "$UNSIGNED_PKG" "$OUT_PKG"
  pkgutil --check-signature "$OUT_PKG"
else
  echo "build-pkg: MACOS_INSTALLER_IDENTITY unset — emitting unsigned pkg"
  cp "$UNSIGNED_PKG" "$OUT_PKG"
fi

# ---------------------------------------------------------------------------
# 5. Optionally notarize + staple.
# ---------------------------------------------------------------------------
if [ -n "${NOTARIZE_KEYCHAIN_PROFILE:-}" ]; then
  # Apple's notary service rejects unsigned pkgs — skip rather than submit
  # and get a flakey failure.
  if [ -z "${MACOS_INSTALLER_IDENTITY:-}" ]; then
    echo "build-pkg: NOTARIZE_KEYCHAIN_PROFILE set but no installer identity — refusing to submit unsigned pkg"
  else
    echo "build-pkg: submitting to notarytool (profile: $NOTARIZE_KEYCHAIN_PROFILE)"
    xcrun notarytool submit "$OUT_PKG" \
      --keychain-profile "$NOTARIZE_KEYCHAIN_PROFILE" \
      --wait
    xcrun stapler staple "$OUT_PKG"
    echo "build-pkg: notarized + stapled"
  fi
else
  echo "build-pkg: NOTARIZE_KEYCHAIN_PROFILE unset — skipping notarization"
fi

echo "build-pkg: wrote $OUT_PKG"
