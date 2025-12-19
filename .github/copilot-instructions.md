# Copilot instructions for git-profile

## Big picture
- This repo is a single-module Go CLI (`git-profile`) built with Cobra and promptui. All commands are wired in `main.go`.
- The tool manages a map of profile keys (e.g. `work`, `personal`) to `Profile` values and applies them via `git config`.

## Key files / components
- `main.go`: CLI entrypoint, command definitions (`ls`, `add`, `edit`, `rm`, `apply`, `auto`), and profile persistence via `ConfigManager`.
- `git_runner.go`: `GitRunner` interface + `ExecGitRunner` implementation. Normalizes common git failures into sentinel errors (`ErrNotGitRepo`, `ErrGitConfigKeyNotFound`) so higher-level logic is testable.
- `git_config.go`: git-config helpers (`gitConfigGet`, `gitConfigSet`) and the core behavior `applyProfileInScope(...)`.
- `gitprofilerc.go`: `.gitprofilerc` resolution (`AutoResolver`) and parsing (`parseGitProfileRC`).
- `main_test.go`: unit tests, including a `fakeGitRunner` and tests for `.gitprofilerc` + `applyProfileInScope` behavior.

## Project-specific behavior to preserve
- Profile storage: `ConfigManager` reads/writes `~/.git-profiles.json` as `map[string]Profile` (keys are the profile names users type/select).
- `auto` command scope resolution:
  - If `<repoRoot>/.gitprofilerc` exists, apply `--local` in the repo root directory.
  - Else if `~/.gitprofilerc` exists, apply `--global`.
- `.gitprofilerc` parsing rules (see `parseGitProfileRC`):
  - First non-empty, non-comment line wins; comments start with `#` or `;`.
  - Accepts `work` or `profile=work` or `profile: work`.
- Applying profiles:
  - `applyProfileInScope` sets `user.name` and `user.email`.
  - Unless `force=true`, it does NOT overwrite keys already set in that scope (it uses `git config --get` to detect set/unset).
  - `apply` currently calls `applyProfileInScope` with `force=true` and no explicit scope flag (so it relies on git’s default scope behavior).

## Conventions for new code
- Prefer calling git via `GitRunner` (and extending `ExecGitRunner` normalization if you need new failure modes) instead of sprinkling `os/exec` calls.
- Keep “decision logic” testable by injecting dependencies/functions (pattern used by `AutoResolver` and `fakeGitRunner`).

## Dev workflow
- Run tests: `go test ./...`
- Run locally: `go run . --help`
- Run a focused test: `go test -run TestAutoResolver ./...`
