#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  uptimyctl Uninstaller
#
#  Usage:
#    sudo bash uninstall.sh
#
#  Environment variables:
#    UPTIMYCTL_INSTALL     - binary directory  (default: /usr/local/bin)
#    UPTIMYCTL_KEEP_CONFIG - keep config dir   (set to 1 to preserve)
# ──────────────────────────────────────────────────────────────────────
set -euo pipefail

BINARY="uptimyctl"
INSTALL_DIR="${UPTIMYCTL_INSTALL:-/usr/local/bin}"
KEEP_CONFIG="${UPTIMYCTL_KEEP_CONFIG:-0}"
CONFIG_DIR="${HOME}/.config/uptimyctl"

info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
warn()  { printf "\033[1;33mWARN:\033[0m %s\n" "$*"; }

# ── Remove binary ───────────────────────────────────────────────────
if [ -f "${INSTALL_DIR}/${BINARY}" ]; then
  info "Removing binary: ${INSTALL_DIR}/${BINARY}"
  rm -f "${INSTALL_DIR}/${BINARY}"
else
  warn "Binary not found at ${INSTALL_DIR}/${BINARY}"
fi

# ── Remove config ───────────────────────────────────────────────────
if [ "$KEEP_CONFIG" = "1" ]; then
  warn "Keeping config directory: ${CONFIG_DIR}"
else
  if [ -d "$CONFIG_DIR" ]; then
    info "Removing config directory: ${CONFIG_DIR}"
    rm -rf "$CONFIG_DIR"
  fi
fi

echo ""
info "uptimyctl has been uninstalled."
if [ "$KEEP_CONFIG" = "1" ]; then
  info "Config preserved at: ${CONFIG_DIR}"
fi
echo ""
