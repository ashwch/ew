#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "usage: $0 <version> <checksums.txt> [output-file]" >&2
  exit 1
fi

version="$1"
checksums_file="$2"
output_file="${3:-}"

if [[ "${version}" != v* ]]; then
  version="v${version}"
fi

need_checksum() {
  local os="$1"
  local arch="$2"
  local ext="$3"
  local asset="ew_${version}_${os}_${arch}.${ext}"
  local sum
  sum="$(grep " ${asset}\$" "${checksums_file}" | awk '{print $1}' | head -n1 || true)"
  if [[ -z "${sum}" ]]; then
    echo "missing checksum for ${asset}" >&2
    exit 1
  fi
  printf '%s' "${sum}"
}

darwin_arm64="$(need_checksum darwin arm64 tar.gz)"
darwin_amd64="$(need_checksum darwin amd64 tar.gz)"
linux_arm64="$(need_checksum linux arm64 tar.gz)"
linux_amd64="$(need_checksum linux amd64 tar.gz)"

formula=$(
  cat <<EOF
class Ew < Formula
  desc "Fix failed shell commands and search command history using natural language"
  homepage "https://github.com/ashwch/ew"
  version "${version#v}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/ashwch/ew/releases/download/${version}/ew_${version}_darwin_arm64.tar.gz"
      sha256 "${darwin_arm64}"
    else
      url "https://github.com/ashwch/ew/releases/download/${version}/ew_${version}_darwin_amd64.tar.gz"
      sha256 "${darwin_amd64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/ashwch/ew/releases/download/${version}/ew_${version}_linux_arm64.tar.gz"
      sha256 "${linux_arm64}"
    else
      url "https://github.com/ashwch/ew/releases/download/${version}/ew_${version}_linux_amd64.tar.gz"
      sha256 "${linux_amd64}"
    end
  end

  def install
    bin.install "ew"
    bin.install "_ew"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ew --version").strip
  end
end
EOF
)

if [[ -n "${output_file}" ]]; then
  printf '%s\n' "${formula}" > "${output_file}"
else
  printf '%s\n' "${formula}"
fi
