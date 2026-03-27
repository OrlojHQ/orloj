#!/usr/bin/env sh
# Orloj install script
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | sh
#
# Options (env vars):
#   ORLOJ_VERSION   - version to install (default: latest)
#   ORLOJ_BINARIES  - space-separated list of binaries to install (default: "orlojd orlojworker orlojctl")
#   ORLOJ_INSTALL_DIR - install directory (default: /usr/local/bin, or ~/.local/bin if no sudo)
#
# Examples:
#   Install latest:
#     curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | sh
#
#   Install specific version:
#     curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | ORLOJ_VERSION=v0.2.0 sh
#
#   Install all binaries including worker:
#     curl -sSfL https://raw.githubusercontent.com/OrlojHQ/orloj/main/scripts/install.sh | ORLOJ_BINARIES="orlojd orlojworker orlojctl" sh

set -e

GITHUB_REPO="OrlojHQ/orloj"
RELEASES_URL="https://github.com/${GITHUB_REPO}/releases"
API_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

# ── Helpers ────────────────────────────────────────────────────────────────────

say() {
  printf '\033[1m%s\033[0m\n' "$1"
}

say_ok() {
  printf '\033[32m✓\033[0m %s\n' "$1"
}

say_err() {
  printf '\033[31merror:\033[0m %s\n' "$1" >&2
}

need() {
  if ! command -v "$1" > /dev/null 2>&1; then
    say_err "required tool not found: $1"
    exit 1
  fi
}

# ── Detect OS and arch ─────────────────────────────────────────────────────────

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux"  ;;
    *)
      say_err "unsupported operating system: $(uname -s)"
      say_err "please download manually from ${RELEASES_URL}"
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      say_err "unsupported architecture: $(uname -m)"
      say_err "please download manually from ${RELEASES_URL}"
      exit 1
      ;;
  esac
}

# ── Resolve latest version ─────────────────────────────────────────────────────

latest_version() {
  if command -v curl > /dev/null 2>&1; then
    curl -sSfL "$API_URL" \
      -H "Accept: application/vnd.github+json" \
      | grep '"tag_name"' \
      | head -1 \
      | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
  elif command -v wget > /dev/null 2>&1; then
    wget -qO- "$API_URL" \
      | grep '"tag_name"' \
      | head -1 \
      | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
  else
    say_err "curl or wget is required"
    exit 1
  fi
}

# ── Download helper ────────────────────────────────────────────────────────────

download() {
  url="$1"
  dest="$2"
  if command -v curl > /dev/null 2>&1; then
    curl -sSfL "$url" -o "$dest"
  else
    wget -qO "$dest" "$url"
  fi
}

# ── Verify checksum ────────────────────────────────────────────────────────────

verify_checksum() {
  file="$1"
  checksums_file="$2"
  filename="$(basename "$file")"

  expected="$(grep " ${filename}$" "$checksums_file" | awk '{print $1}')"
  if [ -z "$expected" ]; then
    say_err "checksum not found for ${filename}"
    exit 1
  fi

  if command -v sha256sum > /dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum > /dev/null 2>&1; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    say_err "sha256sum or shasum is required for checksum verification"
    exit 1
  fi

  if [ "$actual" != "$expected" ]; then
    say_err "checksum mismatch for ${filename}"
    say_err "  expected: ${expected}"
    say_err "  actual:   ${actual}"
    exit 1
  fi
}

# ── Determine install directory ────────────────────────────────────────────────

resolve_install_dir() {
  if [ -n "$ORLOJ_INSTALL_DIR" ]; then
    echo "$ORLOJ_INSTALL_DIR"
    return
  fi

  # Try /usr/local/bin first; fall back to ~/.local/bin if we can't write there
  if [ -w "/usr/local/bin" ]; then
    echo "/usr/local/bin"
  elif [ "$(id -u)" = "0" ]; then
    echo "/usr/local/bin"
  else
    echo "${HOME}/.local/bin"
  fi
}

# ── Main ───────────────────────────────────────────────────────────────────────

main() {
  need tar

  OS="$(detect_os)"
  ARCH="$(detect_arch)"

  VERSION="${ORLOJ_VERSION:-}"
  if [ -z "$VERSION" ]; then
    printf 'Fetching latest version... '
    VERSION="$(latest_version)"
    if [ -z "$VERSION" ]; then
      say_err "could not determine latest version"
      say_err "set ORLOJ_VERSION=vX.Y.Z to install a specific version"
      exit 1
    fi
    printf '%s\n' "$VERSION"
  fi

  BINARIES="${ORLOJ_BINARIES:-orlojd orlojworker orlojctl}"
  INSTALL_DIR="$(resolve_install_dir)"

  say ""
  say "Installing Orloj ${VERSION}"
  printf '  OS:          %s\n' "$OS"
  printf '  Arch:        %s\n' "$ARCH"
  printf '  Binaries:    %s\n' "$BINARIES"
  printf '  Install dir: %s\n' "$INSTALL_DIR"
  say ""

  # Create install dir if needed
  if [ ! -d "$INSTALL_DIR" ]; then
    mkdir -p "$INSTALL_DIR"
  fi

  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT

  # Download and verify checksums file once
  CHECKSUMS_URL="${RELEASES_URL}/download/${VERSION}/checksums.txt"
  CHECKSUMS_FILE="${TMP}/checksums.txt"
  printf 'Downloading checksums... '
  download "$CHECKSUMS_URL" "$CHECKSUMS_FILE"
  printf 'done\n'

  # Install each binary
  for BINARY in $BINARIES; do
    ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
    ARCHIVE_URL="${RELEASES_URL}/download/${VERSION}/${ARCHIVE}"
    ARCHIVE_PATH="${TMP}/${ARCHIVE}"

    printf 'Downloading %s... ' "$BINARY"
    download "$ARCHIVE_URL" "$ARCHIVE_PATH"
    printf 'done\n'

    printf 'Verifying checksum for %s... ' "$BINARY"
    verify_checksum "$ARCHIVE_PATH" "$CHECKSUMS_FILE"
    printf 'ok\n'

    printf 'Installing %s to %s... ' "$BINARY" "$INSTALL_DIR"
    tar -xzf "$ARCHIVE_PATH" -C "$TMP" "$BINARY"
    mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
    printf 'done\n'

    say_ok "$BINARY installed"
  done

  say ""
  say "Orloj ${VERSION} installed successfully!"

  # Warn if install dir is not on PATH
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
      printf '\n\033[33mNote:\033[0m %s is not on your PATH.\n' "$INSTALL_DIR"
      printf 'Add the following to your shell profile:\n\n'
      printf '  export PATH="%s:$PATH"\n\n' "$INSTALL_DIR"
      ;;
  esac
}

main "$@"
