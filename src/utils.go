package app

import (
	"bytes"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func runCmdInput(input, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
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
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp", filepath.Base(path)))
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
		out = append(out, line)
	}
	return strings.Join(out, "\n")
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
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "zsh", "bash", "sh", "fish", "tmux":
		return true
	default:
		return false
	}
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
