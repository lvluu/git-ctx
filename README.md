# git-ctx

Manage git identity and local worktree context from the command line.

## Install

```bash
go install github.com/lvluu/git-ctx@latest
```

Or download a binary from [Releases](https://github.com/lvluu/git-ctx/releases).

## Quick start

```bash
# 1. Create config file
git ctx init

# 2. Add a shell hook (auto-apply profile on cd)
echo 'eval "$(git ctx shell-init)"' >> ~/.bashrc
source ~/.bashrc

# 3. Add profiles
git ctx profile add   # interactive

# 4. Validate everything
git ctx doctor
```

## Commands

```
git ctx init                        Create ~/.git-ctx.yaml config
git ctx shell-init                  Print shell hook snippet
git ctx doctor                      Validate config and environment

git ctx profile ls                  List profiles (marks active)
git ctx profile add                 Add profile (interactive)
git ctx profile edit                Edit profile (interactive)
git ctx profile rm                  Remove profile (interactive)
git ctx profile apply               Apply profile (interactive)
git ctx profile auto [--force]      Auto-apply from .gitprofilerc or directory rules
git ctx profile export [file]       Export profiles to JSON
git ctx profile import <file>       Import profiles from JSON

git ctx worktree ls                 List git worktrees
git ctx worktree add <path>         Add worktree and sync files
git ctx worktree sync [<path>]      Sync files into one or all worktrees
```

**`gc`** is a short alias for `git-ctx` (provided by `shell-init`).

## Directory-based auto profiles

Edit `~/.git-ctx.yaml` to map directories to profiles:

```yaml
directory_rules:
  - pattern: "~/work"
    profile: work
  - pattern: "~/personal"
    profile: personal
```

When you `cd` into `~/work/myrepo`, the **work** profile is applied automatically.

## Worktree file sync

Create `.git-ctx-sync.yaml` in your repo root (gitignored):

```yaml
mode: symlink   # or: copy
files:
  - app/.env
  - .vscode/settings.json
```

Then `git ctx worktree add ../my-feature` creates the worktree and symlinks each file.
