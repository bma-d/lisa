package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTmuxCmdWithSocketUsesExplicitSocket(t *testing.T) {
	expectedSocket := filepath.Join(t.TempDir(), "explicit.sock")
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "tmux.log")
	tmuxPath := filepath.Join(binDir, "tmux")

	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`log="${TMUX_LOG_FILE:-/tmp/tmux.log}"`,
		`sock=""`,
		`if [ "$1" = "-S" ]; then`,
		`  sock="$2"`,
		`  shift 2`,
		`fi`,
		`echo "sock:$sock args:$@" >> "$log"`,
		`if [ "$sock" != "` + expectedSocket + `" ]; then`,
		`  echo "bad socket: $sock" >&2`,
		`  exit 1`,
		`fi`,
		`exit 0`,
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux binary: %v", err)
	}

	origPath := os.Getenv("PATH")
	origLog := os.Getenv("TMUX_LOG_FILE")
	origSocket, hadSocket := os.LookupEnv(lisaTmuxSocketEnv)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Setenv("TMUX_LOG_FILE", origLog)
		if hadSocket {
			_ = os.Setenv(lisaTmuxSocketEnv, origSocket)
		} else {
			_ = os.Unsetenv(lisaTmuxSocketEnv)
		}
	})
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to update PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG_FILE", logPath); err != nil {
		t.Fatalf("failed to set TMUX_LOG_FILE: %v", err)
	}
	// Ensure explicit arg beats runtime env.
	if err := os.Setenv(lisaTmuxSocketEnv, "/tmp/wrong.sock"); err != nil {
		t.Fatalf("failed to set %s: %v", lisaTmuxSocketEnv, err)
	}

	if _, err := runTmuxCmdWithSocket(expectedSocket, "has-session", "-t", "lisa-socket-override"); err != nil {
		t.Fatalf("expected explicit socket command success, got err: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux log: %v", err)
	}
	logText := string(raw)
	if !strings.Contains(logText, "sock:"+expectedSocket) {
		t.Fatalf("expected explicit socket usage in log, got: %s", logText)
	}
}
