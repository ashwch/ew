# ew

<img src="assets/logo/ew-mark.svg" alt="ew logo" width="160" />

`ew` is a plain-English shell copilot with one public command:

```bash
ew <what you want>
```

No user-facing subcommands. You ask in natural language and optionally add flags.

## What It Does

- Fixes the latest failed shell command.
- Finds commands from shell history with semantic matching.
- Executes the best command with safety gates.
- Learns your preferred commands over time.
- Uses local memory/history first, then LLM providers when needed.

## Why It Is Different

- `local-first`: history + learned memory before provider calls.
- `single-command UX`: no command tree to remember.
- `self-aware`: can handle requests about its own settings.
- `automation-friendly`: strict `--json` mode and non-interactive execution.
- `extensible`: provider/model registry is open and config-driven.

## Install

### Curl installer

Latest stable:

```bash
curl -fsSL https://raw.githubusercontent.com/ashwch/ew/main/scripts/install.sh | bash
```

Latest beta:

```bash
curl -fsSL https://raw.githubusercontent.com/ashwch/ew/main/scripts/install.sh | VERSION=latest-beta bash
```

Specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/ashwch/ew/main/scripts/install.sh | VERSION=v0.0.1 bash
```

Installer knobs:

- `EW_CHANNEL=beta` to prefer beta when `VERSION=latest`.
- `EW_INSTALL_DIR=/custom/bin` to override install target.
- `EW_REPO=owner/repo` to install from a fork.

### Homebrew

Stable:

```bash
brew tap ashwch/homebrew-tap
brew install ashwch/homebrew-tap/ew
```

Beta:

```bash
brew tap ashwch/homebrew-tap
brew install ashwch/homebrew-tap/ew@beta
```

### Build from source

```bash
make build
./bin/ew --version
```

## 60-Second Quickstart

1. Install shell hooks once:

```bash
ew --setup-hooks
```

2. Run something that fails:

```bash
gti status
```

3. Ask `ew` to fix it:

```bash
ew
```

4. Ask for commands in English:

```bash
ew how to push current branch
```

5. Execute directly:

```bash
ew --execute --yes "find which process is using port 3000"
```

## Core Usage

- `ew` with no prompt: fix the latest captured failure.
- `ew <text>`: find/suggest best command for the request.
- `ew --execute <text>`: run best command with policy gates.

## High-Signal Examples

```bash
# Find
ew where is go installed
ew path to .zshrc
ew show my aws profiles

# Execute
ew --execute --yes "fetch unshallow git origin"
ew --execute --dry-run "logout from aws sso"

# Machine-readable
ew --json "find my global gitignore file"
ew --json --execute --yes --ui plain "find which process is using port 8000"

# Fast local-only mode
ew --offline "path to .zshrc"

# Easy copy/paste
ew --quiet "logout from aws sso"
ew --copy "logout from aws sso"
ew --quiet --copy "logout from aws sso"

# Self-aware config by natural language
ew set ui bubbletea and save
ew set language hindi and save

# Memory controls (still through ew prompt text)
ew remember push current branch means git push origin HEAD
ew show memory for push current branch
ew prefer git push origin HEAD for push current branch
ew forget memory for push current branch
```

## Flags

Common flags:

- `--execute`: run selected command.
- `--yes`: skip confirm prompt.
- `--mode`: `suggest|confirm|yolo`.
- `--json`: JSON-only output.
- `--offline`: skip provider fallback.
- `--dry-run`: resolve command but do not execute.
- `--quiet`: command-only output.
- `--copy`: copy suggested command.
- `--provider`: provider override for this invocation.
- `--model`: model alias override for this invocation.
- `--thinking`: thinking level override.
- `--ui`: `auto|bubbletea|huh|tview|plain`.
- `--locale`: `auto|en|en-US|hi|hi-IN`.
- `--show-config`, `--doctor`, `--setup-hooks`, `--version`.

Persist any override with `--save`:

```bash
ew --provider claude --save
ew --intent fix --model sonnet --thinking medium --save
ew --intent find --model haiku --thinking minimal --save
ew --mode yolo --save
ew --ui bubbletea --save
ew --locale hi --save
```

## Safety Model

- Read-only prompts filter out mutating commands.
- Destructive/high-risk commands are blocked or downgraded to confirm.
- `yolo` respects safety policy unless explicitly configured otherwise.
- Secrets are redacted before failed commands are stored in local state.

## Automation and Agents

For CI, bots, and headless agents:

```bash
ew --execute --yes --ui plain --json "find which process is using port 8000"
```

JSON contract behavior:

- In confirm mode without `--yes`, JSON output is returned with `executed=false`.
- No interactive prompt is printed in `--json` mode.

## Learning and Memory

`ew` can learn query-to-command preferences.

- Successful `--execute` runs can reinforce memory automatically.
- Manual controls are available via natural-language memory prompts.
- Memory is local state, not cloud sync.

## First-Run System Context

On first interactive run, `ew` captures a safe local system profile (OS/shell/tools/config hints) and shows an onboarding card.

- Context improves provider grounding for machine-specific commands.
- Stored at `<state_dir>/system_profile.json` with private permissions.

Self-aware controls:

```bash
ew enable system context for ew and save
ew disable system context for ew and save
ew enable auto train for system profile and save
ew refresh every 72 for system profile and save
```

## UI Backends

- `bubbletea` (default): full interactive UX.
- `huh`: prompt-first forms.
- `tview`: classic terminal widgets.
- `plain`: no TUI.
- `auto`: best available backend.

Loader behavior:

- Loader appears in interactive terminals.
- Uses rotating `ew` motif.
- Writes to `stderr`.
- Disable with `EW_LOADER=off`.

## Localization

- Built-in locales: English (`en`) and Hindi (`hi`).
- Resolution order: `--locale`, `config.locale`, `EW_LOCALE`, `LC_ALL`, `LC_MESSAGES`, `LANG`.
- Community locale packs are supported.

Community locale path examples:

- macOS: `~/Library/Application Support/ew/locales/<locale>.json`
- Linux: `~/.config/ew/locales/<locale>.json`

Reference schema:

- `docs/locales/community-locale.example.json`

## Providers and Models

Default providers include `auto`, `codex`, `claude`, and local fallback `ew`.

Model aliases are config-driven. Example:

```toml
[providers.codex.models.gpt-5-ultra]
provider_model = "gpt-5-ultra"
thinking = "high"
speed = "quality"
description = "Deep reasoning profile"
```

Add a command-based provider:

```toml
[providers.openrouter]
type = "command"
command = "openrouter-cli"
enabled = true
model = "qwen3-coder"
thinking = "medium"
model_flag = "--model"
thinking_flag = "--reasoning {thinking}"

[providers.openrouter.models.qwen3-coder]
provider_model = "qwen3-coder"
thinking = "medium"
speed = "balanced"
```

Then:

```bash
ew --provider openrouter --save
```

## Config and State Paths

Config file:

- macOS: `~/Library/Application Support/ew/config.toml`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/ew/config.toml`
- Windows: `%APPDATA%\\ew\\config.toml`

State directory:

- macOS: `~/Library/Application Support/ew/state`
- Linux: `${XDG_STATE_HOME:-~/.local/state}/ew/state`
- Windows: `%LOCALAPPDATA%\\ew\\state`

## Troubleshooting

No failure detected:

- Run `ew --setup-hooks`.
- Open a new shell.
- Re-run a failing command, then run `ew`.

Provider issues:

```bash
ew --doctor
```

Non-interactive failure in confirm mode:

- Add `--yes`, or use `--mode yolo` if your policy allows it.

## Development

Requirements:

- Go `1.25` or `1.26`
- `make`

Commands:

```bash
make fmt
make test
make vet
make build
go test -race ./...
```

## Internal Helper

`_ew` is an internal binary used for hooks/config/history plumbing.

- Public interface remains `ew`.
- `_ew` subcommands are implementation detail and may change.

## Docs

- `CONTRIBUTING.md`
- `SECURITY.md`
- `CODE_OF_CONDUCT.md`
- `docs/RELEASING.md`
- `docs/RELEASE-CHECKLIST.md`
- `docs/HOMEBREW.md`
