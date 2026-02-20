# Releasing `ew`

This is the maintainer runbook for beta and stable releases.

## Current Policy (Beta-First)

As of February 20, 2026, `ew` ships as **beta-first**:

- Default public channel: `beta`
- Stable channel: only after beta quality gates pass
- Homebrew stable formula `ew.rb` must never point at a beta tag

## Channels and Tags

| Channel | Tag pattern | GitHub release type | Homebrew formula |
| --- | --- | --- | --- |
| beta | `vX.Y.Z-beta.N` | prerelease | `ew-beta.rb` |
| stable | `vX.Y.Z` | stable | `ew.rb` |

Examples:

- `v0.1.0-beta.1`
- `v0.1.0-beta.2`
- `v0.1.0`

## Release Cadence

- Beta: cut frequently as changes accumulate (at least weekly while active).
- Stable: promote only after:
  - at least one beta has passed smoke tests,
  - no open P0/P1 release blockers,
  - install paths (curl + Homebrew) are verified.

## Source-of-Truth Files

Keep these in sync before tagging:

- `CHANGELOG.md`
- `docs/RELEASE-CHECKLIST.md`
- `docs/HOMEBREW.md`
- `.github/workflows/release.yml`
- `scripts/preflight.sh`
- `scripts/render_formula.sh`
- `scripts/update_tap_formula.sh`
- `scripts/publish_tap_formula.sh`

## End-to-End Flow

### 1) Prepare a version

1. Pick the channel and version:
   - beta: `vX.Y.Z-beta.N`
   - stable: `vX.Y.Z`
2. Open a GitHub issue using `.github/ISSUE_TEMPLATE/release.yml`.
3. Link that issue in any release prep PR.
4. Add a matching entry in `CHANGELOG.md`.
5. Ensure release docs are accurate for any process changes.

### 2) Run local gates

```bash
./scripts/preflight.sh v0.1.0-beta.1
```

or:

```bash
make preflight VERSION=v0.1.0-beta.1
```

### 3) Tag and push

```bash
git tag v0.1.0-beta.1
git push origin v0.1.0-beta.1
```

### 4) Verify release workflow output

`.github/workflows/release.yml` should publish:

- platform archives:
  - `ew_<version>_linux_amd64.tar.gz`
  - `ew_<version>_linux_arm64.tar.gz`
  - `ew_<version>_darwin_amd64.tar.gz`
  - `ew_<version>_darwin_arm64.tar.gz`
  - `ew_<version>_windows_amd64.zip`
- `checksums.txt`
- `ew.rb`
- `ew-beta.rb` (beta tags only)

### 5) Update Homebrew tap

Preferred one-command publish flow:

```bash
./scripts/publish_tap_formula.sh v0.1.0-beta.1 /path/to/homebrew-tap beta
```

This command updates the formula, commits, pushes, and verifies the remote tap file.

Manual fallback:

```bash
./scripts/update_tap_formula.sh v0.1.0-beta.1 /path/to/homebrew-tap beta
cd /path/to/homebrew-tap
git add Formula/ew-beta.rb
git commit -m "ew: beta v0.1.0-beta.1"
git push
```

For stable tags, use `Formula/ew.rb` instead.

Full details: `docs/HOMEBREW.md`.

### 6) Post-release smoke test

Curl installer:

```bash
curl -fsSL https://raw.githubusercontent.com/ashwch/ew/main/scripts/install.sh | VERSION=v0.1.0-beta.1 bash
```

Homebrew:

- beta: `brew install <tap>/ew-beta`
- stable: `brew install <tap>/ew`

Runtime checks:

```bash
ew --version
ew --doctor
ew --setup-hooks
```

## Promoting Beta to Stable

Promotion is a **new stable tag**, not a UI toggle:

1. Start from the commit you want to promote.
2. Ensure changelog includes stable version notes.
3. Run preflight with stable tag.
4. Tag and publish `vX.Y.Z`.
5. Update tap stable formula `ew.rb`.

## Rollback / Hotfix

If a bad release ships:

1. Mark the bad release clearly in GitHub notes.
2. Publish a follow-up patch tag quickly.
3. Move tap formula(s) to known-good version.
4. Document the issue in `CHANGELOG.md`.

## Operational Checklist

Use `docs/RELEASE-CHECKLIST.md` for a step-by-step execution checklist.
