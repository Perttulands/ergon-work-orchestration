# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added
- Added `work spawn <citizen>` command to launch ready worker sessions in tmux.
- Added config-driven runtime profiles with a default profile file at `internal/worker/worker_profiles.default.json`.
- Added support for central runtime configuration loading from:
  - `WORK_RUNTIME_CONFIG`
  - `~/.work/worker_profiles.json`
  - embedded default profile
- Added agent-to-runtime mapping support in runtime profiles.
- Added tests for runtime profile resolution and spawn command behavior.

### Changed
- Updated `work run` to resolve runtime from central profiles (with optional `--runtime` override).
- Updated run record model labeling to come from runtime profile configuration.
- Updated README with runtime profile source-of-truth documentation.

### Fixed
- Fixed Codex launch alias collision by using `command <binary> ...` in launch commands.
- Fixed relay startup reliability by registering agents before heartbeat in `work run` and `work spawn`.
- Fixed error propagation in learning-loop integration (`SelectTemplate`, `QueryLearningLoop`) and aligned tests.
