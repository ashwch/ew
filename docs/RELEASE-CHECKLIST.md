# Release Checklist

Run this checklist for every release.
Reference runbook: `docs/RELEASING.md`.

## 1) Select Version and Channel

- [ ] Pick version format:
  - beta: `vX.Y.Z-beta.N`
  - stable: `vX.Y.Z`
- [ ] Open a release tracking issue from `.github/ISSUE_TEMPLATE/release.yml`.
- [ ] Confirm `CHANGELOG.md` contains an entry for the exact version.
- [ ] Confirm release docs still match reality:
  - `docs/RELEASING.md`
  - `docs/HOMEBREW.md`

## 2) Local Validation

- [ ] Run preflight:

```bash
./scripts/preflight.sh vX.Y.Z
```

or:

```bash
make preflight VERSION=vX.Y.Z
```

## 3) Tag and Publish

- [ ] Create and push tag:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

- [ ] Wait for `.github/workflows/release.yml` to succeed.
- [ ] Confirm release assets exist:
  - `ew_*` archives
  - `checksums.txt`
  - `ew.rb`
  - `ew-beta.rb` (beta only)

## 4) Homebrew Update

- [ ] Publish tap formula with one command (update + commit + push + remote verify):

```bash
./scripts/publish_tap_formula.sh vX.Y.Z /path/to/homebrew-tap
```

- [ ] If the one-command publish flow is unavailable, fallback manually:
  - `./scripts/update_tap_formula.sh vX.Y.Z /path/to/homebrew-tap`
  - commit and push in tap repo:
  - stable: `Formula/ew.rb`
  - beta: `Formula/ew-beta.rb`
- [ ] Keep stable `Formula/ew.rb` on the latest stable tag only.
- [ ] Confirm remote tap has the expected channel formula:
  - stable: `gh api repos/ashwch/homebrew-tap/contents/Formula/ew.rb?ref=main --jq '.download_url'`
  - beta: `gh api repos/ashwch/homebrew-tap/contents/Formula/ew-beta.rb?ref=main --jq '.download_url'`

## 5) Post-Publish Smoke Test

- [ ] Verify curl install:

```bash
curl -fsSL https://raw.githubusercontent.com/ashwch/ew/main/scripts/install.sh | VERSION=vX.Y.Z bash
```

- [ ] Verify Homebrew install:
  - stable: `brew install <tap>/ew`
  - beta: `brew install <tap>/ew-beta`
- [ ] Verify runtime:

```bash
ew --version
ew --setup-hooks
ew --doctor
```
