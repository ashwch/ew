#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: ./scripts/preflight.sh <version>" >&2
  exit 1
fi

version="${1}"

if [[ "${version}" != v* ]]; then
  version="v${version}"
fi

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "version must look like vX.Y.Z or vX.Y.Z-beta.N (got ${version})" >&2
  exit 1
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd go
need_cmd make

echo "==> preflight: ${version}"

echo "==> checking required release files"
required=(
  "README.md"
  "LICENSE"
  "CHANGELOG.md"
  "CONTRIBUTING.md"
  "SECURITY.md"
  "CODE_OF_CONDUCT.md"
  "docs/RELEASING.md"
  "docs/RELEASE-CHECKLIST.md"
  "docs/HOMEBREW.md"
  ".github/workflows/ci.yml"
  ".github/workflows/release.yml"
  "scripts/install.sh"
  "scripts/render_formula.sh"
  "scripts/update_tap_formula.sh"
  "Formula/ew.rb"
)
for file in "${required[@]}"; do
  if [[ ! -f "${file}" ]]; then
    echo "missing required file: ${file}" >&2
    exit 1
  fi
done

echo "==> checking changelog has ${version}"
if ! grep -q "^## \[${version#v}\] " CHANGELOG.md; then
  echo "CHANGELOG.md missing entry for ${version}" >&2
  exit 1
fi

echo "==> checking executable scripts"
for script in scripts/install.sh scripts/render_formula.sh scripts/update_tap_formula.sh; do
  if [[ ! -x "${script}" ]]; then
    echo "script is not executable: ${script}" >&2
    exit 1
  fi
done

echo "==> running format/test/vet/build"
make fmt
make test
make vet
make build VERSION="${version}"

echo "==> running race tests"
go test -race ./...
go test -race ./cmd/_ew

echo "==> checking version/help behavior"
actual_version="$(./bin/ew --version | tr -d '\r\n')"
if [[ "${actual_version}" != "${version}" ]]; then
  echo "ew --version mismatch: expected ${version}, got ${actual_version}" >&2
  exit 1
fi

if ./bin/ew --help >/tmp/ew-preflight-help.out 2>&1; then
  :
else
  echo "ew --help returned non-zero exit code" >&2
  exit 1
fi

echo "==> preflight passed for ${version}"
