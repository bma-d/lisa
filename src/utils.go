package app

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func runCmd(name string, args ...string) (string, error) {
	return runCmdInternal("", name, args...)
}

func runCmdInput(input, name string, args ...string) (string, error) {
	return runCmdInternal(input, name, args...)
}

func runCmdInternal(input, name string, args ...string) (string, error) {
	timeout := time.Duration(getIntEnv("LISA_CMD_TIMEOUT_SECONDS", defaultCmdTimeoutSeconds)) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(defaultCmdTimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = commandExecEnv(name)
	var out bytes.Buffer
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out.String(), fmt.Errorf("command timed out after %s: %s %s", timeout, name, strings.Join(args, " "))
	}
	return out.String(), err
}

func commandExecEnv(name string) []string {
	env := os.Environ()
	if filepath.Base(name) != "tmux" {
		return env
	}
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

func getPWD() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func md5Hex8(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])[:8]
}

func writeJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Println("{}")
		return
	}
	fmt.Println(string(b))
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, fmt.Sprintf(".%s.tmp-*", filepath.Base(path)))
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmp)
		}
	}()

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanupTmp = false
	return nil
}

func trimLines(input string) []string {
	raw := strings.Split(input, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		out = append(out, strings.TrimRight(line, "\r"))
	}
	return out
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func filterInputBox(input string) string {
	lines := strings.Split(input, "\n")
	var out []string
	inBox := false
	startRe := regexp.MustCompile(`^\s*[╭┌]`)
	endRe := regexp.MustCompile(`^\s*[╰└]`)
	boxLineRe := regexp.MustCompile(`^\s*[│|]`)
	separatorRe := regexp.MustCompile(`^\s*─+\s*$`)
	modeIndicatorRe := regexp.MustCompile(`^\s*--\s*(INSERT|NORMAL)\s*--`)
	statusBarRe := regexp.MustCompile(`\|\s*ctx\(\d+%\)\s*\|`)

	for _, line := range lines {
		if startRe.MatchString(line) {
			inBox = true
			continue
		}
		if endRe.MatchString(line) {
			inBox = false
			continue
		}
		if inBox && boxLineRe.MatchString(line) {
			continue
		}
		if separatorRe.MatchString(line) {
			continue
		}
		if modeIndicatorRe.MatchString(line) {
			continue
		}
		if statusBarRe.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func filterCaptureNoise(input string) string {
	lines := trimLines(input)
	out := make([]string, 0, len(lines))
	skipIndentedNoiseContinuation := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			skipIndentedNoiseContinuation = false
			out = append(out, line)
			continue
		}
		if skipIndentedNoiseContinuation {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				continue
			}
			skipIndentedNoiseContinuation = false
		}
		if isCaptureNoiseLine(trimmed) {
			if strings.HasPrefix(trimmed, "⚠ MCP client for ") {
				skipIndentedNoiseContinuation = true
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func isCaptureNoiseLine(trimmed string) bool {
	switch {
	case strings.HasPrefix(trimmed, "mcp: "):
		return true
	case strings.HasPrefix(trimmed, "mcp startup: "):
		return true
	case strings.HasPrefix(trimmed, "⚠ MCP client for "):
		return true
	case strings.HasPrefix(trimmed, "⚠ MCP startup incomplete"):
		return true
	case strings.HasPrefix(trimmed, "⚠ Under-development features enabled:"):
		return true
	case strings.Contains(trimmed, "codex_state::runtime: failed to open state db"):
		return true
	case strings.Contains(trimmed, "codex_core::rollout::list: state db missing rollout path"):
		return true
	case strings.Contains(trimmed, "codex_core::state_db: state db record_discrepancy"):
		return true
	default:
		return false
	}
}

func containsAnyPrefix(line string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func isShellCommand(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return false
	}
	command = filepath.Base(command)
	switch command {
	case "zsh", "bash", "sh", "dash", "ash", "ksh", "mksh", "pdksh", "yash",
		"fish", "tcsh", "csh", "nu", "pwsh", "powershell", "xonsh", "elvish",
		"ion", "tmux":
		return true
	}
	return strings.HasSuffix(command, "sh")
}

func flagValueError(flag string) int {
	fmt.Fprintf(os.Stderr, "missing value for %s\n", flag)
	return 1
}

func unknownFlagError(flag string) int {
	fmt.Fprintf(os.Stderr, "unknown flag: %s\n", flag)
	return 1
}

func getIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func boolExit(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func parseBoolFlag(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean: %s", raw)
	}
}

func writeCSVRecord(fields ...string) error {
	writer := csv.NewWriter(os.Stdout)
	if err := writer.Write(fields); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}
