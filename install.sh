#!/bin/sh
# Install srcpack from an official GitHub release.
# Usage: curl -fsSL URL/install.sh | sh -s -- [--version VERSION] [--install-dir DIR]
set -eu

REPO='nexssp/srcpack'
API='https://api.github.com/repos/nexssp/srcpack'
VERSION=${SRCPACK_VERSION:-latest}
INSTALL_DIR=${SRCPACK_INSTALL_DIR:-}

usage() {
  cat <<'EOF'
Install srcpack from an official precompiled release.

Options:
  --version VERSION    Release version (for example 0.2.0 or v0.2.0); default: latest
  --install-dir DIR    Install directory; default: ~/.local/bin
  -h, --help           Show this help

Environment:
  SRCPACK_VERSION      Same as --version
  SRCPACK_INSTALL_DIR  Same as --install-dir
EOF
}

fail() {
  printf 'srcpack installer: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail '--version requires a value'
      VERSION=$2
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail '--install-dir requires a value'
      INSTALL_DIR=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1 (use --help)"
      ;;
  esac
done

command -v curl >/dev/null 2>&1 || fail 'curl is required'
command -v tar >/dev/null 2>&1 || fail 'tar is required'
command -v awk >/dev/null 2>&1 || fail 'awk is required'
command -v sed >/dev/null 2>&1 || fail 'sed is required'

if [ -z "$INSTALL_DIR" ]; then
  [ -n "${HOME:-}" ] || fail 'HOME is not set; provide --install-dir'
  INSTALL_DIR=$HOME/.local/bin
fi
case "$INSTALL_DIR" in
  /*) ;;
  *) INSTALL_DIR=$PWD/$INSTALL_DIR ;;
esac

os=$(uname -s 2>/dev/null || true)
arch=$(uname -m 2>/dev/null || true)
case "$os" in
  Linux) goos=linux ;;
  Darwin) goos=darwin ;;
  *) fail "unsupported operating system '$os'; supported systems are Linux and macOS" ;;
esac
case "$arch" in
  x86_64|amd64|AMD64) goarch=amd64 ;;
  aarch64|arm64|ARM64) goarch=arm64 ;;
  *) fail "unsupported architecture '$arch'; supported architectures are amd64 and arm64" ;;
esac

work=$(mktemp -d "${TMPDIR:-/tmp}/srcpack-install.XXXXXX") || fail 'could not create a temporary directory'
trap 'rm -rf "$work"' EXIT HUP INT TERM

if [ "$VERSION" = latest ]; then
  if ! curl -fLsS --retry 2 "$API/releases/latest" -o "$work/release.json"; then
    fail 'could not query the latest GitHub release'
  fi
  tag=$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$work/release.json" | sed -n '1p')
else
  case "$VERSION" in
    *[!A-Za-z0-9._-]*) fail "invalid version '$VERSION'" ;;
  esac
  case "$VERSION" in
    v*) tag=$VERSION ;;
    *) tag=v$VERSION ;;
  esac
fi
[ -n "${tag:-}" ] || fail 'could not determine a release tag'
case "$tag" in
  v*) release_version=${tag#v} ;;
  *) fail "unexpected release tag '$tag'" ;;
esac
case "$release_version" in
  ''|*[!A-Za-z0-9._-]*) fail "invalid release version '$release_version'" ;;
esac

archive="srcpack_${release_version}_${goos}_${goarch}.tar.gz"
checksums="srcpack_${release_version}_checksums.txt"
base="https://github.com/${REPO}/releases/download/${tag}"
archive_path="$work/$archive"
checksums_path="$work/$checksums"

if ! curl -fLsS --retry 2 "$base/$archive" -o "$archive_path"; then
  fail "release $tag has no downloadable $goos/$goarch archive ($archive); check the published release assets"
fi
if ! curl -fLsS --retry 2 "$base/$checksums" -o "$checksums_path"; then
  fail "could not download $checksums from release $tag"
fi

expected=$(awk -v name="$archive" '$2 == name { print $1; exit }' "$checksums_path")
[ -n "$expected" ] || fail "$checksums does not contain an entry for $archive"
case "$expected" in
  *[!0123456789abcdefABCDEF]*) fail "invalid SHA-256 entry for $archive" ;;
esac
[ "${#expected}" -eq 64 ] || fail "invalid SHA-256 length for $archive"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$archive_path" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$archive_path" | awk '{print $1}')
else
  fail 'sha256sum or shasum is required to verify the release archive'
fi
[ "$actual" = "$expected" ] || fail "SHA-256 verification failed for $archive"

if ! tar -xzf "$archive_path" -C "$work" srcpack; then
  fail "could not extract srcpack from $archive"
fi
[ -f "$work/srcpack" ] && [ ! -L "$work/srcpack" ] || fail 'the release archive does not contain a regular srcpack executable'
chmod 0755 "$work/srcpack"
if ! "$work/srcpack" --help >/dev/null 2>&1; then
  fail 'the downloaded srcpack executable did not pass its --help smoke test'
fi

mkdir -p "$INSTALL_DIR" || fail "could not create install directory: $INSTALL_DIR"
staged="$INSTALL_DIR/.srcpack.$$"
if ! cp "$work/srcpack" "$staged" || ! chmod 0755 "$staged" || ! mv -f "$staged" "$INSTALL_DIR/srcpack"; then
  rm -f "$staged"
  fail "could not install srcpack into $INSTALL_DIR"
fi

printf 'Installed srcpack %s to %s/srcpack\n' "$tag" "$INSTALL_DIR"
case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf 'Add it to PATH for future shells with:\n  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
    ;;
esac
printf 'Try: %s/srcpack --help\n' "$INSTALL_DIR"
