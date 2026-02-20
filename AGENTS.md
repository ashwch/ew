# AGENTS.md - The ew Story

*A practical guide for agents and contributors working on the `ew` CLI.*

---

## What This Repo Is

`ew` is a Go CLI that turns plain-language shell intent into safe command suggestions or execution.

Core product rule:
- Public UX is a single command: `ew <english sentence>`.
- Internal helper logic lives in `_ew` and is not the user-facing API.

Think of it as a shell copilot with three lanes:
- `fix`: repair the latest failed command.
- `find`: suggest the best command.
- `run`: execute the best command under policy gates.

```
+--------------------------------------------------------------------------+
|                                  ew CLI                                  |
+--------------------------------------------------------------------------+
|                                                                          |
|  shell input ----> cmd/ew ----> intent/router ----> find/fix/run paths  |
|                                 |                                        |
|                                 +--> history + memory search             |
|                                 +--> provider fallback (claude/codex/ew) |
|                                 +--> safety + execution policy           |
|                                 +--> output (plain/json/tui)             |
|                                                                          |
|  shell hooks ----> _ew hook-record ----> state/events.jsonl ----> ew fix |
|                                                                          |
+--------------------------------------------------------------------------+
```

---

## Architecture Diagrams

### Runtime Command Path

```
+-------------------+      +--------------------+      +---------------------+
| User Prompt       |----->| cmd/ew/main.go     |----->| router.Intent       |
| (ew <text>)       |      | parse flags/prompt |      | fix/find/run        |
+-------------------+      +--------------------+      +---------------------+
                                                           |
                                                           v
                         +--------------------+      +---------------------+
                         | memory + history   |----->| candidate commands  |
                         | internal/memory    |      | local-first         |
                         +--------------------+      +---------------------+
                                                           |
                                      no good local match  |
                                                           v
                         +--------------------+      +---------------------+
                         | provider service   |----->| ai resolution       |
                         | internal/provider  |      | action/conf/risk    |
                         +--------------------+      +---------------------+
                                                           |
                                                           v
                         +--------------------+      +---------------------+
                         | ai_policy + safety |----->| runtime execution   |
                         | cmd/ew/ai_policy   |      | internal/runtime    |
                         +--------------------+      +---------------------+
```

### Hook Capture and Fix Loop

```
+-------------------+      +-----------------------+      +------------------+
| Shell preexec     |----->| _ew hook-record       |----->| events.jsonl     |
| zsh/bash/fish     |      | internal/hook/events  |      | latest failures  |
+-------------------+      +-----------------------+      +------------------+
                                                            |
                                                            v
                                                     +--------------+
                                                     | ew (fix)     |
                                                     | latestFailure |
                                                     +--------------+
```

### UI Backends

```
+-------------------+      +-------------------+      +-------------------+
| auto/bubbletea    |      | huh               |      | tview/plain       |
| selector/confirm  |      | prompt UX         |      | fallback UX       |
+-------------------+      +-------------------+      +-------------------+
             \                     |                          /
              \                    |                         /
               +-------------------+------------------------+
                                   |
                                   v
                           final command decision
```

---

## Folder Structure

```
ew/
|
+-- cmd/
|   +-- ew/                 # Public CLI entrypoint and runtime orchestration
|   +-- _ew/                # Internal helper CLI (hooks/config/history tools)
|
+-- internal/
|   +-- appdirs/            # OS-specific config/state paths
|   +-- config/             # Config schema + load/save + key set/get
|   +-- history/            # Shell history loaders + ranking/filtering
|   +-- hook/               # Failure event capture and retrieval
|   +-- i18n/               # Locale catalogs (en/hi + community packs)
|   +-- knowledge/          # Self-knowledge prompt payload
|   +-- memory/             # Learned query->command mapping
|   +-- provider/           # Provider adapters and resolver service
|   +-- router/             # Intent detection
|   +-- runtime/            # Execute/normalize command policy and shell runner
|   +-- safety/             # Redaction helpers
|   +-- systemprofile/      # First-run machine profile context
|   +-- ui/                 # Bubble Tea / Huh / TView interactions
|
+-- scripts/
|   +-- install.sh          # Curl installer
|   +-- preflight.sh        # Release preflight gates
|   +-- render_formula.sh   # Homebrew formula rendering
|
+-- .github/workflows/
|   +-- ci.yml              # Build/test/vet matrix
|   +-- release.yml         # Tagged release artifacts + checksums + formula
|
+-- docs/
|   +-- RELEASE-CHECKLIST.md
|   +-- locales/community-locale.example.json
```

---

## Technologies

| Category | Technology | Purpose |
|----------|------------|---------|
| Language | Go 1.25+ | Core CLI implementation |
| CLI UX | Bubble Tea, Huh, TView | Interactive picker/confirm/onboarding |
| Config | TOML (`go-toml/v2`) | Persistent user/provider settings |
| AI Integration | External command adapters | Claude/Codex/provider-agnostic resolution |
| Distribution | GitHub Releases, Homebrew formula, curl installer | OSS install paths |
| CI/CD | GitHub Actions | Build/test/vet and release packaging |

---

## Do

- Keep public UX as `ew` plus flags; avoid introducing user-facing subcommands.
- Preserve local-first behavior: memory/history before provider fallback.
- Enforce execution safety gates before running provider commands.
- Add tests for any router/policy/memory/safety change.
- Keep docs and behavior aligned (especially execution policy and flags).

## Don't

- Do not auto-execute provider output when policy says suggest/ask.
- Do not bypass safety checks for destructive/high-risk commands.
- Do not leak secrets into persisted hook events or prompts.
- Do not add stale internal review/planning docs to public-facing release branch.

---

## Common Commands

```bash
# Build binaries
make build

# Run all core checks
make fmt
make test
make vet
go test -race ./...
go test -race ./cmd/_ew

# Local run
go run ./cmd/ew --version
go run ./cmd/ew "logout from aws sso"
go run ./cmd/ew --execute "find which process is using port 8000"

# Internal helper diagnostics
go run ./cmd/_ew config-path
go run ./cmd/_ew state-path
go run ./cmd/_ew doctor

# Release preflight
./scripts/preflight.sh v0.0.1
```

---

## The Hard Lessons

### Lesson 1: "Suggest" is not "Run"

Provider output may return a valid command but still declare `action=suggest` or `action=ask`.

The cause:
- Trusting command presence instead of action policy.

The fix:
- Gate execution through `evaluateAIResolution` and `ai.allow_suggest_execution`.

### Lesson 2: Memory must respect hard constraints

Queries that differ only by critical numbers (for example, port `3000` vs `8000`) cannot be treated as equivalent.

The cause:
- Token overlap scoring without strict numeric compatibility.

The fix:
- Numeric token set checks in memory compatibility logic and tests for port mismatch cases.

### Lesson 3: Hook data goes stale quickly

Fix mode should not blindly trust old failure captures.

The cause:
- Shell sessions and state files can include stale events.

The fix:
- Staleness checks and fallback to recent timestamped history inference.

---

## Quick Reference

**Core UX**
```bash
ew
ew <plain english query>
ew --execute <plain english query>
```

**Headless/automation**
```bash
ew --execute --yes --ui plain "<query>"
ew --mode yolo --save
ew "allow suggest execution for ew and save"
```

**Config/state paths**
```bash
_ew config-path
_ew state-path
```

**When in doubt:** prefer policy-safe behavior over convenience, then add explicit config/flags.
