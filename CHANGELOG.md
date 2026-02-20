# Changelog

All notable changes to this project will be documented in this file.

## [0.0.1-beta.2] - 2026-02-20

### Fixed
- Release workflow YAML parsing issue that prevented GitHub Release jobs from running.

### Changed
- README logo rendering is now fixed-size for cleaner GitHub display.

## [0.0.1-beta.1] - 2026-02-20

### Added
- Single-command `ew` intent router for fix/find/run/config/show/doctor/setup-hooks flows.
- Internal `_ew` helper for shell hook snippets, event capture, history search, and config primitives.
- Provider registry architecture with command adapters and configurable provider/model catalog.
- Confidence and execution policy gates for provider responses.
- Hook/event hardening, shell history loaders, and recency-weighted reverse search.
- CI checks for format, tests, vet, plus explicit `_ew` test/vet coverage.

### Security
- Private permissions for config/state directories and config/events files.
- Command normalization and high-risk execution gating.
- Optional prompt-level secret redaction before provider calls.
