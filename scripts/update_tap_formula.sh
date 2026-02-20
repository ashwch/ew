#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  cat >&2 <<'EOF'
usage: ./scripts/update_tap_formula.sh <version> <tap-dir> [channel]

examples:
  ./scripts/update_tap_formula.sh v0.1.0-beta.1 ../homebrew-tap
  ./scripts/update_tap_formula.sh v0.1.0 ../homebrew-tap stable
EOF
  exit 1
fi

version="$1"
tap_dir="$2"
channel="${3:-}"
repo="${EW_REPO:-ashwch/ew}"

if [[ "${version}" != v* ]]; then
  version="v${version}"
fi

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid version: ${version}" >&2
  exit 1
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd gh

if [[ -z "${channel}" ]]; then
  if [[ "${version}" == *-* ]]; then
    channel="beta"
  else
    channel="stable"
  fi
fi

case "${channel}" in
  stable)
    formula_file="ew.rb"
    release_formula_candidates=("ew.rb")
    ;;
  beta)
    formula_file="ew-beta.rb"
    release_formula_candidates=("ew-beta.rb" "ew@beta.rb")
    ;;
  *)
    echo "invalid channel: ${channel} (expected stable|beta)" >&2
    exit 1
    ;;
esac

if [[ ! -d "${tap_dir}" ]]; then
  echo "tap directory not found: ${tap_dir}" >&2
  exit 1
fi

if [[ ! -d "${tap_dir}/.git" ]]; then
  echo "tap directory is not a git repo: ${tap_dir}" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "Downloading ${formula_file} from ${repo} ${version}"
downloaded_formula_file=""
for candidate in "${release_formula_candidates[@]}"; do
  if gh release download "${version}" \
    --repo "${repo}" \
    --pattern "${candidate}" \
    --dir "${tmpdir}" >/dev/null 2>&1; then
    downloaded_formula_file="${candidate}"
    break
  fi
done

if [[ -z "${downloaded_formula_file}" ]]; then
  echo "failed to download formula for ${version}; tried: ${release_formula_candidates[*]}" >&2
  exit 1
fi

mkdir -p "${tap_dir}/Formula"
if [[ "${channel}" == "beta" ]]; then
  sed \
    -e 's/^class EwATBeta < Formula$/class EwBeta < Formula/' \
    -e 's/^class Ew@beta < Formula$/class EwBeta < Formula/' \
    "${tmpdir}/${downloaded_formula_file}" > "${tap_dir}/Formula/${formula_file}"
  chmod 0644 "${tap_dir}/Formula/${formula_file}"
else
  install -m 0644 "${tmpdir}/${downloaded_formula_file}" "${tap_dir}/Formula/${formula_file}"
fi

echo "Updated ${tap_dir}/Formula/${formula_file}"
echo "Next steps:"
echo "  cd ${tap_dir}"
echo "  git add Formula/${formula_file}"
echo "  git commit -m \"ew: ${channel} ${version}\""
echo "  git push"
