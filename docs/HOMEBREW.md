# Homebrew Release Guide

This guide documents how `ew` is distributed through Homebrew.

## Formula Files and Channels

- Stable formula: `Formula/ew.rb` (class `Ew`)
- Beta formula: `Formula/ew@beta.rb` (class `EwATBeta`)

Channel policy:

- Stable tags (`vX.Y.Z`) update `ew.rb`.
- Beta tags (`vX.Y.Z-beta.N`) update `ew@beta.rb`.
- Stable `ew.rb` must not point to beta tags.

## User Install Paths

Stable from tap:

```bash
brew tap ashwch/homebrew-tap
brew install ashwch/homebrew-tap/ew
```

Beta from tap:

```bash
brew tap ashwch/homebrew-tap
brew install ashwch/homebrew-tap/ew@beta
```

Direct formula install from a GitHub release asset:

```bash
brew install https://github.com/ashwch/ew/releases/download/vX.Y.Z/ew.rb
brew install https://github.com/ashwch/ew/releases/download/vX.Y.Z-beta.N/ew@beta.rb
```

## Maintainer Workflow

### 1) Download formula from release

Use the helper script from this repo:

```bash
./scripts/update_tap_formula.sh vX.Y.Z /path/to/homebrew-tap
./scripts/update_tap_formula.sh vX.Y.Z-beta.N /path/to/homebrew-tap beta
```

This script:

- detects channel from tag (or explicit channel arg),
- downloads the correct formula asset from GitHub release,
- writes it into `<tap>/Formula/`.

### 2) Validate in tap repo

```bash
cd /path/to/homebrew-tap
brew install --formula ./Formula/ew.rb
brew install --formula ./Formula/ew@beta.rb
```

You usually run one of those depending on channel.

### 3) Commit and publish tap update

Stable:

```bash
git add Formula/ew.rb
git commit -m "ew: stable vX.Y.Z"
git push
```

Beta:

```bash
git add Formula/ew@beta.rb
git commit -m "ew: beta vX.Y.Z-beta.N"
git push
```

## Troubleshooting

`brew` still installs old version:

```bash
brew update
brew upgrade ew
brew upgrade ew@beta
```

Check formula version:

```bash
brew info ew
brew info ew@beta
```

Verify local binary:

```bash
ew --version
```
