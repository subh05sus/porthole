#!/usr/bin/env bash
# Install porthole for Linux/macOS: detects OS/arch and downloads the
# matching release asset from GitHub Releases.
#
# NOT tested end-to-end — there is no published release to download from
# yet (this session never pushes or publishes anything). The OS/arch
# detection and archive-extraction logic below is straightforward and
# has been read through carefully, but "curl a real URL and it works"
# has not actually been exercised, honestly, since no such URL exists yet.
set -euo pipefail

REPO="subh05sus/porthole"
BINARY="porthole"

main() {
  local os arch archive_ext url tmp_dir

  os="$(detect_os)"
  arch="$(detect_arch)"
  archive_ext="tar.gz"
  if [ "$os" = "windows" ]; then
    archive_ext="zip"
  fi

  local version="${PORTHOLE_VERSION:-latest}"
  local version_path="latest/download"
  if [ "$version" != "latest" ]; then
    version_path="download/${version}"
  fi

  local asset="${BINARY}_${version#v}_${os}_${arch}.${archive_ext}"
  url="https://github.com/${REPO}/releases/${version_path}/${asset}"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  echo "Downloading ${url}"
  if ! curl -fsSL "$url" -o "${tmp_dir}/${asset}"; then
    echo "error: failed to download ${url}" >&2
    echo "Check https://github.com/${REPO}/releases for available versions/platforms." >&2
    exit 1
  fi

  if [ "$archive_ext" = "zip" ]; then
    unzip -q "${tmp_dir}/${asset}" -d "$tmp_dir"
  else
    tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"
  fi

  local install_dir="${PORTHOLE_INSTALL_DIR:-$HOME/.local/bin}"
  mkdir -p "$install_dir"
  install -m 755 "${tmp_dir}/${BINARY}" "${install_dir}/${BINARY}"

  echo "Installed ${BINARY} to ${install_dir}/${BINARY}"
  case ":$PATH:" in
    *":${install_dir}:"*) ;;
    *) echo "note: ${install_dir} is not on your PATH — add it, e.g. export PATH=\"${install_dir}:\$PATH\"" ;;
  esac
}

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *)
      echo "error: unsupported OS $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "error: unsupported architecture $(uname -m)" >&2
      exit 1
      ;;
  esac
}

main "$@"
