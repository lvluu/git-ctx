# git-ctx — Developer Reference

## Module

```
github.com/lvluu/git-ctx
```

## Build & Test

```bash
go build ./...          # compile
go test ./...           # run all tests
go test ./... -v        # verbose
go run . <cmd>          # run without installing
```

## File Structure

| File | Purpose |
|------|---------|
| `main.go` | Entry point — wires commands, loads config (~60 lines) |
| `profiles.go` | `Profile`, `ConfigManager` (load/save/Export/Import), input helpers |
| `profile_commands.go` | `buildProfileCmd` — all `profile` subcommands |
| `app_config.go` | `AppConfig`, `DirectoryRule`, `loadAppConfig`, `initAppConfig`, `MatchDirectoryRule` |
| `git_config.go` | `findRepoRoot`, `gitConfigGet`, `gitConfigSet`, `applyProfileInScope` |
| `git_runner.go` | `GitRunner` interface, `ExecGitRunner`, sentinel errors |
| `gitprofilerc.go` | `AutoResolver`, `AutoResolution`, `parseGitProfileRC` |
| `shell_init.go` | `shellInitScript()` — bash/zsh hook snippet + `gc` alias |
| `worktree.go` | `buildWorktreeCmd`, `runSync`, `listWorktreePaths` |
| `worktree_config.go` | `SyncConfig`, `loadSyncConfig`, `syncFiles`, `copyFile` |
| `doctor.go` | `DoctorResult`, `runDoctorChecks`, `printDoctorResults` |

## Architecture

- `main.go` loads `AppConfig` first, then injects it into command builders (`buildProfileCmd`, `buildWorktreeCmd`).
- All cobra command builders take explicit dependencies (no globals) — makes testing straightforward.
- `GitRunner` interface allows injecting `fakeGitRunner` in tests instead of spawning a real git process.
- `AutoResolver` is fully injectable (all I/O as function fields) — tested without touching the filesystem or git.
- Config files:
  - `~/.git-ctx.yaml` — global config (profiles path, directory rules, worktree mode); created by `git ctx init`
  - `~/.git-ctx-profiles.json` — profiles store (JSON, path configurable via `~/.git-ctx.yaml`)
  - `.git-ctx-sync.yaml` — per-repo worktree file sync list (gitignored)

## Conventions

- No new top-level dependencies without discussion — current deps: `cobra`, `promptui`, `testify`, `yaml.v3`.
- Sentinel errors (`ErrNotGitRepo`, `ErrGitConfigKeyNotFound`) live in `git_runner.go`.
- Tests use `t.TempDir()` — never create test files outside temp dirs.
- `syncFiles` uses absolute symlink targets to avoid depth-dependent breakage.
- `--silent` flag on `profile auto` must exit 0 with no output when no profile matches (shell hook safety).

## Release

Binary names: `git-ctx` and `gc` (both built from the same source via `.goreleaser.yaml`).

```bash
goreleaser release --snapshot --clean   # local test build
```
