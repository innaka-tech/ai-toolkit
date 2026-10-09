#!/bin/sh
# Install aitk: download a release, verify its SHA-256 checksum (and cosign signature
# when cosign is installed), and place the binary in ~/.local/bin (no sudo).
#
#   curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh | sh
#
# Environment: AITK_VERSION (e.g. v2.0.0; default: latest), AITK_INSTALL_DIR (default: ~/.local/bin).
# The v1 bash toolkit is still available: git clone --branch v1.0.0 https://github.com/innaka-tech/ai-toolkit.git
set -eu

REPO="innaka-tech/ai-toolkit"
BASE="${AITK_DOWNLOAD_URL:-https://github.com/$REPO/releases/download}"
DEST="${AITK_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf 'aitk-install: %s\n' "$*" >&2; }
die() { say "error: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

need curl
need tar
need uname

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  mingw*|msys*|cygwin*) die "on Windows, download the .zip from https://github.com/$REPO/releases or run: go install github.com/$REPO/cmd/aitk@latest" ;;
  *) die "unsupported OS: $os" ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

version="${AITK_VERSION:-}"
if [ -z "$version" ]; then
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") || die "cannot reach GitHub"
  version=${url##*/}
fi
case "$version" in v*) ;; *) version="v$version" ;; esac
num=${version#v}

archive="aitk_${num}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading aitk $version for $os/$arch"
curl -fsSL "$BASE/$version/$archive" -o "$tmp/$archive" || die "download failed: $BASE/$version/$archive"
curl -fsSL "$BASE/$version/checksums.txt" -o "$tmp/checksums.txt" || die "checksums.txt not found for $version"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$expected" ] || die "$archive is not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
else
  need shasum
  actual=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
fi
[ "$expected" = "$actual" ] || die "checksum mismatch for $archive (expected $expected, got $actual)"
say "checksum verified"

if command -v cosign >/dev/null 2>&1; then
  if curl -fsSL "$BASE/$version/checksums.txt.sig" -o "$tmp/checksums.txt.sig" 2>/dev/null &&
     curl -fsSL "$BASE/$version/checksums.txt.pem" -o "$tmp/checksums.txt.pem" 2>/dev/null; then
    cosign verify-blob --certificate "$tmp/checksums.txt.pem" --signature "$tmp/checksums.txt.sig" \
      --certificate-identity-regexp "^https://github.com/$REPO/" \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com "$tmp/checksums.txt" >/dev/null 2>&1 ||
      die "cosign signature verification failed"
    say "signature verified (cosign)"
  fi
fi

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$DEST"
mv "$tmp/aitk" "$DEST/aitk"
chmod 755 "$DEST/aitk"
say "installed $("$DEST/aitk" version 2>/dev/null || echo aitk) to $DEST/aitk"

case ":$PATH:" in
  *":$DEST:"*) ;;
  *) say "add $DEST to your PATH, e.g.: echo 'export PATH=\"$DEST:\$PATH\"' >> ~/.profile" ;;
esac
say "next: cd <your repo> && aitk init (or aitk migrate for a v1 project) && aitk adapters sync"
