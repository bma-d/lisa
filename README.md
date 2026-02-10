# lisa

Standalone Go CLI for orchestrating Claude/Codex agent sessions in tmux.

## Requirements

- Go 1.21+
- tmux on `PATH`
- `claude` and/or `codex` on `PATH`
- Supported OS: macOS or Linux

## Run From Source

```bash
go run .
```

## Install

### Homebrew (macOS/Linux)

```bash
brew install bma-d/tap/lisa
```

### Debian/Ubuntu (.deb)

```bash
sudo dpkg -i lisa_*.deb
```

### Fedora/RHEL (.rpm)

```bash
sudo rpm -i lisa_*.rpm
```

### Alpine (.apk)

```bash
sudo apk add --allow-untrusted lisa_*.apk
```

### Go install

```bash
go install github.com/bma-d/lisa@latest
```

### Manual download

Download the archive for your OS/arch from the Releases page and extract `lisa` to a directory on your `PATH`.

## Quick Check

```bash
lisa version
lisa doctor
```
