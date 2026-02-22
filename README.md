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

## Usage

Canonical CLI usage reference lives in [`USAGE.md`](./USAGE.md).

## Quick start

```bash
lisa doctor                # verify setup
lisa session preflight --json  # verify env + core command contracts
lisa cleanup --dry-run     # inspect stale socket residue
lisa oauth add --stdin     # store Claude OAuth token in local pool (paste token via stdin)
lisa skills sync --from codex   # sync ~/.codex/skills/lisa into repo skills/lisa
lisa version               # print version
```

## Build from source

```bash
go build -o lisa .
```

## Nested smoke test

Run deterministic 3-level nested tmux orchestration smoke test (interactive mode):

```bash
./smoke-nested
# or built-in command (supports 1-4 levels)
./lisa session smoke --levels 4 --json
# include nested wording probe:
./lisa session smoke --levels 4 --prompt-style dot-slash --json
```
