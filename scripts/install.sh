#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  uptimyctl Installer
#
#  One-liner:
#    curl -sSfL https://raw.githubusercontent.com/uptimy/uptimyctl/master/scripts/install.sh | sudo bash
#
#  Environment variables:
#    UPTIMYCTL_VERSION   - version tag to install   (default: latest)
#    UPTIMYCTL_INSTALL   - install directory         (default: /usr/local/bin)
#    UPTIMYCTL_NO_VERIFY - skip checksum verification (set to 1)
# ──────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────────
REPO="uptimy/uptimyctl"
BINARY="uptimyctl"
VERSION="${UPTIMYCTL_VERSION:-latest}"
INSTALL_DIR="${UPTIMYCTL_INSTALL:-/usr/local/bin}"
NO_VERIFY="${UPTIMYCTL_NO_VERIFY:-0}"

# ── Pretty output ───────────────────────────────────────────────────
info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
warn()  { printf "\033[1;33mWARN:\033[0m %s\n" "$*"; }
error() { printf "\033[1;31mERROR:\033[0m %s\n" "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "Required command not found: $1"
}

# ── Platform detection ───────────────────────────────────────────────
detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux)  echo "linux"  ;;
    darwin) echo "darwin" ;;
    *)      error "Unsupported OS: $os. Only Linux and macOS are supported." ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)             error "Unsupported architecture: $arch. Only amd64 and arm64 are supported." ;;
  esac
}

# ── Version resolution ───────────────────────────────────────────────
resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    info "Resolving latest release..."
    need_cmd curl
    local api_response
    api_response="$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null)" \
      || error "Could not reach GitHub API. Check your internet connection."
    VERSION="$(echo "$api_response" | grep '"tag_name"' | head -1 | sed -E 's/.*"v?([^"]+)".*/\1/')"
    [ -n "$VERSION" ] || error "Could not determine latest version. Specify UPTIMYCTL_VERSION manually."
    info "Latest version: v${VERSION}"
  fi
  VERSION="${VERSION#v}"
}

# ── Checksum verification ────────────────────────────────────────────
verify_checksum() {
  local file="$1" checksums_file="$2"

  if [ "$NO_VERIFY" = "1" ]; then
    warn "Skipping checksum verification (UPTIMYCTL_NO_VERIFY=1)"
    return 0
  fi

  local filename
  filename="$(basename "$file")"

  if [ ! -f "$checksums_file" ]; then
    warn "Checksums file not found - skipping verification"
    return 0
  fi

  local expected
  expected="$(grep "$filename" "$checksums_file" | awk '{print $1}')"
  if [ -z "$expected" ]; then
    warn "No checksum entry for $filename - skipping verification"
    return 0
  fi

  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    warn "Neither sha256sum nor shasum found - skipping verification"
    return 0
  fi

  if [ "$actual" != "$expected" ]; then
    error "Checksum mismatch for ${filename}!
  Expected: ${expected}
  Actual:   ${actual}
This may indicate a corrupted or tampered download. Aborting."
  fi

  info "Checksum verified: ${filename}"
}

# ══════════════════════════════════════════════════════════════════════
#  MAIN
# ══════════════════════════════════════════════════════════════════════

need_cmd uname
need_cmd curl
need_cmd tar

OS="$(detect_os)"
ARCH="$(detect_arch)"

echo ""
info "uptimyctl Installer"
info "Platform: ${OS}/${ARCH}"
echo ""

resolve_version

# ── Download & verify ────────────────────────────────────────────────
TARBALL="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_BASE="https://github.com/${REPO}/releases/download/v${VERSION}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading ${TARBALL}..."
curl -sSfL -o "${TMPDIR}/${TARBALL}" "${DOWNLOAD_BASE}/${TARBALL}" \
  || error "Download failed. Check the version (v${VERSION}) and that a release exists for ${OS}/${ARCH}.
  URL: ${DOWNLOAD_BASE}/${TARBALL}"

curl -sSfL -o "${TMPDIR}/checksums.txt" "${DOWNLOAD_BASE}/checksums.txt" 2>/dev/null || true
verify_checksum "${TMPDIR}/${TARBALL}" "${TMPDIR}/checksums.txt"

info "Extracting..."
tar -xzf "${TMPDIR}/${TARBALL}" -C "$TMPDIR"

# ── Install binary ───────────────────────────────────────────────────
info "Installing binary → ${INSTALL_DIR}/${BINARY}"
install -d "$INSTALL_DIR"
install -m 0755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

if "${INSTALL_DIR}/${BINARY}" version >/dev/null 2>&1; then
  INSTALLED_VER="$("${INSTALL_DIR}/${BINARY}" version 2>&1 | head -1)"
  info "Installed: ${INSTALLED_VER}"
else
  warn "Binary installed but could not run 'version' command"
fi

# ── Summary ──────────────────────────────────────────────────────────
echo ""
info "────────────────────────────────────────────"
info "  uptimyctl v${VERSION} installed!"
info "────────────────────────────────────────────"
echo ""
info "Get started:"
info "  uptimyctl auth login"
info "  uptimyctl applications list"
echo ""
info "Docs: https://github.com/uptimy/uptimyctl"
echo ""
