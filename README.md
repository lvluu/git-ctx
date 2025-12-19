# Git Profile CLI

## Overview

`git-profile` is a powerful command-line tool that simplifies managing multiple Git profiles across different projects and environments.

## Features

- 🔄 Easily switch between Git profiles
- ➕ Interactively add new profiles
- ✏️ Edit existing profiles
- 🗑️ Remove profiles
- 📦 Export and import profile configurations
- 🖥️ Simple, intuitive CLI interface

## Installation

### Go Install (Recommended)

```bash
go install github.com/lvluu/git-profile@latest
```

### Manual Installation

Download the appropriate binary for your platform from the [Releases](https://github.com/lvluu/git-profile/releases) page.

## Usage

### Listing Profiles

```bash
git profile ls
```

### Adding a Profile

```bash
git profile add
```

- Interactively enter profile name, username, and email
- Optionally add a signing key

### Editing a Profile

```bash
git profile edit
```

- Select a profile to modify
- Update details interactively

### Removing a Profile

```bash
git profile rm
```

- Select a profile to remove
- Confirm deletion

### Applying a Profile

```bash
git profile apply
```

- Select a profile to apply globally

### Auto-applying per repo via `.gitprofilerc`

You can automatically apply a profile based on a `.gitprofilerc` file.

- If `<repo-root>/.gitprofilerc` exists, it applies **local** config (`git config --local ...`).
- Otherwise, if `~/.gitprofilerc` exists, it applies **global** config (`git config --global ...`).

Create a `.gitprofilerc` file containing the profile key you saved (the map key in `~/.git-profiles.json`):

```text
work
```

Then run:

```bash
git profile auto
```

By default, `auto` **respects existing** `user.name` / `user.email` already set in that scope.
Use `--force` to overwrite:

```bash
git profile auto --force
```

### Exporting Profiles

```bash
git profile export [output-file]
```

- Export all profiles to a JSON file
- If no file specified, exports to `~/git-profiles-export.json`

### Importing Profiles

```bash
git profile import <input-file>
```

- Import profiles from a JSON file
- Choose to merge or replace existing profiles

### Checking Version

```bash
git profile -v
```

## Configuration

Profiles are stored in `~/.git-profiles.json`

## Contributing

All the contributions are welcome

## Support

If you encounter any issues or have suggestions, please file an issue on GitHub.
