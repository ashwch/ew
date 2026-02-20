#!/usr/bin/env bash
set -euo pipefail

REPO="${EW_REPO:-ashwch/ew}"
VERSION="${VERSION:-${1:-latest}}"
CHANNEL="${EW_CHANNEL:-}"
REQUESTED_VERSION="${VERSION}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd tar

api_get() {
  local url="$1"
  curl -fsSL "$url" 2>/dev/null || true
}

extract_first_tag() {
  sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1
}

resolve_latest_stable() {
  api_get "https://api.github.com/repos/${REPO}/releases/latest" | extract_first_tag
}

resolve_latest_any() {
  api_get "https://api.github.com/repos/${REPO}/releases?per_page=20" | extract_first_tag
}

resolve_latest_beta() {
  api_get "https://api.github.com/repos/${REPO}/releases?per_page=50" | awk '
    /"tag_name":[[:space:]]*"/ {
      tag = $0
      sub(/^.*"tag_name":[[:space:]]*"/, "", tag)
      sub(/".*$/, "", tag)
    }
    /"prerelease":[[:space:]]*true/ {
      if (tag != "") {
        print tag
        exit
      }
    }
  '
}

case "${VERSION}" in
  latest|latest-stable)
    if [[ "${CHANNEL}" == "beta" ]]; then
      VERSION="$(resolve_latest_beta)"
    else
      VERSION="$(resolve_latest_stable)"
      if [[ -z "${VERSION}" ]]; then
        VERSION="$(resolve_latest_any)"
      fi
    fi
    ;;
  latest-beta|beta)
    VERSION="$(resolve_latest_beta)"
    ;;
esac

if [[ -z "${VERSION}" ]]; then
  echo "could not resolve release tag for ${REPO} (requested VERSION=${REQUESTED_VERSION}, EW_CHANNEL=${CHANNEL:-unset})" >&2
  exit 1
fi

if [[ "$VERSION" != v* ]]; then
  VERSION="v${VERSION}"
fi

uname_s="$(uname -s)"
uname_m="$(uname -m)"

case "${uname_s}" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "unsupported OS: ${uname_s}" >&2
    exit 1
    ;;
esac

case "${uname_m}" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "unsupported architecture: ${uname_m}" >&2
    exit 1
    ;;
esac

asset="ew_${VERSION}_${OS}_${ARCH}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${VERSION}"
asset_url="${base_url}/${asset}"
checksums_url="${base_url}/checksums.txt"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

archive="${tmpdir}/${asset}"
checksums="${tmpdir}/checksums.txt"

echo "Downloading ${asset_url}"
curl -fsSL "${asset_url}" -o "${archive}"
curl -fsSL "${checksums_url}" -o "${checksums}"

expected="$(grep " ${asset}\$" "${checksums}" | awk '{print $1}')"
if [[ -z "${expected}" ]]; then
  echo "checksum entry not found for ${asset}" >&2
  exit 1
fi

actual=""
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${archive}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "${archive}" | awk '{print $1}')"
else
  echo "missing checksum tool: sha256sum or shasum" >&2
  exit 1
fi

if [[ "${actual}" != "${expected}" ]]; then
  echo "checksum mismatch for ${asset}" >&2
  echo "expected: ${expected}" >&2
  echo "actual:   ${actual}" >&2
  exit 1
fi

tar -xzf "${archive}" -C "${tmpdir}"

install_dir="${EW_INSTALL_DIR:-}"
if [[ -z "${install_dir}" ]]; then
  if [[ -d "/opt/homebrew/bin" && -w "/opt/homebrew/bin" ]]; then
    install_dir="/opt/homebrew/bin"
  elif [[ -d "/usr/local/bin" && -w "/usr/local/bin" ]]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
  fi
fi

mkdir -p "${install_dir}"
install -m 0755 "${tmpdir}/ew" "${install_dir}/ew"
install -m 0755 "${tmpdir}/_ew" "${install_dir}/_ew"

echo "Installed ew and _ew to ${install_dir}"
echo "Run: ew --version"
