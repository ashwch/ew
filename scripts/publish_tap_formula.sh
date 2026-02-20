#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  cat >&2 <<'EOF'
usage: ./scripts/publish_tap_formula.sh <version> <tap-dir> [channel]

examples:
  ./scripts/publish_tap_formula.sh v0.1.0-beta.1 ../homebrew-tap
  ./scripts/publish_tap_formula.sh v0.1.0 ../homebrew-tap stable
EOF
  exit 1
fi

version="$1"
tap_dir="$2"
channel="${3:-}"

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

need_cmd git
need_cmd gh
need_cmd base64

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
    ;;
  beta)
    formula_file="ew-beta.rb"
    ;;
  *)
    echo "invalid channel: ${channel} (expected stable|beta)" >&2
    exit 1
    ;;
esac

if [[ ! -d "${tap_dir}/.git" ]]; then
  echo "tap directory is not a git repo: ${tap_dir}" >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${script_dir}/update_tap_formula.sh" "${version}" "${tap_dir}" "${channel}"

formula_path="${tap_dir}/Formula/${formula_file}"
expected_formula_version="${version#v}"

if ! grep -Fq "version \"${expected_formula_version}\"" "${formula_path}"; then
  echo "formula version check failed for ${formula_path}" >&2
  exit 1
fi

if ! grep -Fq "/releases/download/${version}/" "${formula_path}"; then
  echo "formula release URL check failed for ${formula_path}" >&2
  exit 1
fi

git -C "${tap_dir}" add "Formula/${formula_file}"
if git -C "${tap_dir}" diff --cached --quiet; then
  echo "No tap formula changes to commit for ${formula_file}"
else
  git -C "${tap_dir}" commit -m "ew: ${channel} ${version}"
fi

branch="$(git -C "${tap_dir}" rev-parse --abbrev-ref HEAD)"
git -C "${tap_dir}" push origin "${branch}"

origin_url="$(git -C "${tap_dir}" remote get-url origin)"
tap_repo="${origin_url}"
tap_repo="${tap_repo#git@github.com:}"
tap_repo="${tap_repo#https://github.com/}"
tap_repo="${tap_repo#ssh://git@github.com/}"
tap_repo="${tap_repo%.git}"

if [[ "${tap_repo}" == "${origin_url}" ]]; then
  echo "failed to parse GitHub repo from tap origin URL: ${origin_url}" >&2
  exit 1
fi

remote_formula="$(
  gh api "repos/${tap_repo}/contents/Formula/${formula_file}?ref=${branch}" --jq '.content' \
    | tr -d '\n' \
    | base64 --decode
)"

if ! grep -Fq "version \"${expected_formula_version}\"" <<<"${remote_formula}"; then
  echo "remote tap formula version mismatch for ${tap_repo}/Formula/${formula_file}" >&2
  exit 1
fi

if ! grep -Fq "/releases/download/${version}/" <<<"${remote_formula}"; then
  echo "remote tap formula release URL mismatch for ${tap_repo}/Formula/${formula_file}" >&2
  exit 1
fi

echo "Tap sync verified: ${tap_repo}/Formula/${formula_file} -> ${version}"
