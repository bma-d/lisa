# lisa

<p align="center">
  <img src="docs/lisa.webp" alt="Lisa" width="320" />
</p>

Standalone Go CLI for orchestrating Claude/Codex agent sessions in tmux.

Like her namesake, Lisa is the smartest one in the room. She keeps your AI agents organized so you don't have to — spawning parallel workers, tracking their progress, nudging them when they stall, and capturing their output. Zero dependencies, zero chaos.

## Install

### Homebrew (macOS/Linux)

```bash
brew install bma-d/tap/lisa
```

### Go install

```bash
go install github.com/bma-d/lisa@latest
```

### Manual download

Download the archive for your OS/arch from the [Releases](https://github.com/bma-d/lisa/releases) page and extract `lisa` to a directory on your `PATH`.

<details>
<summary>Linux packages</summary>

```bash
# Debian/Ubuntu
sudo dpkg -i lisa_*.deb

# Fedora/RHEL
sudo rpm -i lisa_*.rpm

# Alpine
sudo apk add --allow-untrusted lisa_*.apk
```

</details>

## Requirements

- tmux on `PATH`
- `claude` and/or `codex` on `PATH`
- macOS or Linux

## Quick start

```bash
lisa doctor                # verify setup
lisa version               # print version
```

### Spawn a session

```bash
# Interactive Claude session
lisa session spawn --agent claude --mode interactive --prompt "Review current repo status" --json

# Non-interactive execution (claude -p / codex exec)
lisa session spawn --agent claude --mode exec --prompt "Summarize uncommitted changes" --json
```

`--mode exec` maps to `claude -p` or `codex exec --full-auto`. Use `--command` to fully override the startup command.

### Track progress

```bash
# One-shot status check
lisa session status --session <SESSION> --json

# Continuous polling
lisa session monitor --session <SESSION> --json --poll-interval 20 --max-polls 120
```

Session states: `just_started` | `in_progress` | `waiting_input` | `completed` | `stuck` | `crashed`

### Send follow-up input

```bash
lisa session send --session <SESSION> --text "Continue and apply all safe fixes" --enter

# Or send raw tmux keys
lisa session send --session <SESSION> --keys "C-c" --enter
```

### Capture output

```bash
lisa session capture --session <SESSION> --lines 300
```

### Manage sessions

```bash
lisa session list                          # list all sessions
lisa session list --project-only           # current project only
lisa session exists --session <SESSION>    # check if exists
lisa session kill --session <SESSION>      # kill one
lisa session kill-all                      # kill all lisa sessions
```

## Integration pattern

1. Spawn one session per task (`session spawn --json`), store the returned session name.
2. Poll with `session monitor` or `session status`.
3. On `waiting_input` or `stuck`, send next instruction with `session send --enter`.
4. Fetch output with `session capture`.
5. Clean up with `session kill` when done.

## Exit codes

- `session monitor`: `0` on `completed`/`waiting_input`, `2` on `crashed`/`stuck`/`not_found`/timeout.
- `session status`: always returns a status payload unless argument parsing fails.

## Build from source

```bash
go build -o lisa .
```
