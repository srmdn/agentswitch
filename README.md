# agentswitch

`agentswitch` is a small CLI for keeping agent skill sets easy to inspect,
enable, and disable.

Agent tools such as Codex load skill metadata when a session starts. That is
useful when the active skills match the work in front of you, but noisy when old
or unrelated skills stay enabled forever. `agentswitch` gives you a simple way
to see what is active, move skills in and out of the active set, and check for
broken skill links.

The current MVP is Codex-first and focused on local filesystem skill folders.

## Why Use This?

Use `agentswitch` when you want to:

- keep only the skills you need for the current kind of work active
- switch between small skill sets without remembering local paths
- disable a skill without deleting it
- spot stale or broken symlinks in your skill folders
- preview changes before touching the filesystem

This is especially helpful if you collect many skills over time. A lean active
set can make startup context smaller and skill routing easier to reason about.

## What It Manages

`agentswitch` looks for skills in the locations Codex documents for local skill
discovery. A skill is a directory or symlink containing a `SKILL.md` file.

Discovered skill roots:

```text
$CWD/.agents/skills
$CWD/../.agents/skills
...
$REPO_ROOT/.agents/skills
$HOME/.agents/skills
/etc/codex/skills
```

For repository skills, Codex scans `.agents/skills` from the directory where you
launch Codex up to the repository root. That lets a repo define skills for a
whole project or for a nested module.

`agentswitch` currently switches only user-level skills:

```text
$HOME/.agents/skills
$HOME/.agents/skills.disabled
$HOME/.codex/skills
$HOME/.codex/skills.disabled
```

Enabling a user skill moves it from `$HOME/.agents/skills.disabled` to
`$HOME/.agents/skills`. Disabling it moves it back. The tool does not edit skill
contents.

For compatibility with early personal setups, `agentswitch` also understands
nested pack layouts under `.agents/skills.disabled` and
`.codex/skills.disabled`. Repo and admin skills are included in status and
doctor output, but they are not modified by enable, disable, or preset commands
yet.

## Install

From a local checkout:

```bash
go build -o agentswitch .
```

Then run:

```bash
./agentswitch status
```

Or install with Go:

```bash
go install github.com/srmdn/agentswitch@latest
```

The `go install` command is intended for published releases. During early
development, building from a checkout is the most reliable option.

## Commands

Create a user config:

```bash
agentswitch init
```

Show active and disabled skills:

```bash
agentswitch status
```

Enable or disable one user skill:

```bash
agentswitch enable <skill>
agentswitch disable <skill>
```

List, enable, or disable a pack:

```bash
agentswitch pack list
agentswitch pack enable <pack>
agentswitch pack disable <pack>
```

List or apply a preset:

```bash
agentswitch preset list
agentswitch preset apply <preset>
```

Check for broken skill symlinks:

```bash
agentswitch doctor
```

Preview changes without moving files:

```bash
agentswitch --dry-run disable <skill>
agentswitch -n preset apply <preset>
```

After enabling, disabling, or applying a preset, restart Codex so it can reload
the active skill metadata.

The older `agentswitch skills ...` command shape is still accepted as a
compatibility alias while the CLI settles. For example,
`agentswitch skills doctor` is the same as `agentswitch doctor`.

## Configuration

Packs and presets are user-owned configuration, not hardcoded product defaults.
Run `agentswitch init` to create:

```text
~/.config/agentswitch/config.toml
```

The generated config includes roots, pack layouts, and presets:

```toml
[packs.go]
type = "directory"
active = "$HOME/.agents/skills/cc-skills-golang"
disabled = "$HOME/.agents/skills.disabled/cc-skills-golang"
skills_subdir = "skills"

[presets]
lean = []
go = ["go"]
```

Use `AGENTSWITCH_CONFIG` to point at another config file:

```bash
AGENTSWITCH_CONFIG=./config.toml agentswitch preset list
```

## Concepts

**Skill:** A directory or symlink with a `SKILL.md` file.

**Discovered skill:** A skill found in a Codex-supported repo, user, admin, or
system location.

**Active skill:** A skill in a discovered active root. Codex can load it when a
new session starts.

**Disabled skill:** A user skill parked in `$HOME/.agents/skills.disabled`. It
remains on disk but is not in the active user skill directory.

**Pack:** A named group of skills. Packs let one command enable or disable a
related set. Example: `agentswitch pack enable wordpress`.

**Preset:** A target active set. Applying a preset enables the skills included in
that preset and disables other managed skills. Example:
`agentswitch preset apply lean`.

## Current Generated Packs And Presets

The initial generated config includes compatibility packs for the original local
shell script:

```text
packs:   go, wordpress, translation
presets: lean, web, go, wordpress
```

The compatibility layouts are:

```text
go:          $HOME/.agents/skills.disabled/cc-skills-golang/skills/*
translation: $HOME/.agents/skills.disabled/translation/*
wordpress:   $HOME/.codex/skills.disabled/wordpress-pack/*
```

WordPress compatibility also preserves the original behavior of keeping real
skill directories under `$HOME/.codex/skills` and symlinks under
`$HOME/.agents/skills`.

These generated entries are migration defaults, not a statement that every user
should organize their skills around those stacks. Edit
`~/.config/agentswitch/config.toml` for your own workflow.

## Safety Model

`agentswitch` is intentionally conservative:

- it moves skill directories or symlinks; it does not rewrite `SKILL.md`
- it only switches user-level and compatibility pack skills in this skeleton
- it creates the relevant active or disabled root when needed
- it supports `--dry-run` for previews
- it reports broken symlinks instead of silently ignoring them
- it prints a restart reminder after real changes

Before broad use, run status and dry-run first:

```bash
agentswitch status
agentswitch --dry-run preset apply lean
```

## Status

This project is in the initial CLI skeleton stage.

Implemented:

- active and disabled skill inventory
- repo-local `.agents/skills` discovery from CWD to repo root
- user skill discovery in `$HOME/.agents/skills`
- compatibility discovery in `$HOME/.codex/skills`
- admin skill discovery in `/etc/codex/skills`
- enable and disable user skills by name or built-in pack
- config file support at `~/.config/agentswitch/config.toml`
- user-defined roots, packs, and presets
- generated compatibility packs for Go, WordPress, and translation
- generated presets: `lean`, `web`, `go`, `wordpress`
- dry-run mode
- broken symlink detection
- Codex-oriented restart reminder

Planned:

- support for Codex's official `~/.codex/config.toml` skill enablement entries
- JSON output
- richer `doctor` checks
- inventory for other agent tools
- plugin and MCP inventory before any plugin or MCP switching

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build ./...
```
