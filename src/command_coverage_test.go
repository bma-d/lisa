package app

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildAgentCommandInteractiveVariants(t *testing.T) {
	cmd, err := buildAgentCommand("claude", "interactive", "hello world", "--model sonnet")
	if err != nil {
		t.Fatalf("buildAgentCommand interactive claude failed: %v", err)
	}
	if cmd != "claude --model sonnet 'hello world'" {
		t.Fatalf("unexpected claude interactive command: %q", cmd)
	}

	cmd, err = buildAgentCommand("codex", "interactive", "", "--search")
	if err != nil {
		t.Fatalf("buildAgentCommand interactive codex failed: %v", err)
	}
	if cmd != "codex --search" {
		t.Fatalf("unexpected codex interactive command: %q", cmd)
	}
}

func TestCmdAgentBuildCmdExecPath(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdAgentBuildCmd([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "ship release",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected exec build-cmd success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"agent":"codex"`) || !strings.Contains(stdout, `"mode":"exec"`) {
		t.Fatalf("expected codex exec JSON payload, got %q", stdout)
	}
	if !strings.Contains(stdout, `codex exec 'ship release' --full-auto`) {
		t.Fatalf("expected codex exec command in payload, got %q", stdout)
	}

	_, stderr = captureOutput(t, func() {
		code := cmdAgentBuildCmd([]string{"--agent", "codex", "--mode", "exec"})
		if code == 0 {
			t.Fatalf("expected exec build-cmd without prompt to fail")
		}
	})
	if !strings.Contains(stderr, "exec mode requires --prompt") {
		t.Fatalf("expected missing prompt error, got %q", stderr)
	}
}

func TestAgentDisplayNameFormatting(t *testing.T) {
	if got := agentDisplayName("claude"); got != "Claude" {
		t.Fatalf("expected Claude display name, got %q", got)
	}
	if got := agentDisplayName(" CODEX "); got != "Codex" {
		t.Fatalf("expected Codex display name, got %q", got)
	}
	if got := agentDisplayName(""); got != "Agent" {
		t.Fatalf("expected default Agent display name, got %q", got)
	}
}

func TestCmdSessionMonitorTimeoutCSVOutput(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origPaneStatus := tmuxPaneStatusFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxPaneStatusFn = origPaneStatus
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "still working", nil }
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\t123", nil
		case "#{pane_current_command}":
			return "zsh", nil
		case "#{pane_pid}":
			return "123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 1001, 2.4, nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-timeout",
			"--project-root", t.TempDir(),
			"--max-polls", "1",
			"--poll-interval", "1",
		})
		if code != 2 {
			t.Fatalf("expected timeout monitor exit 2, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	record, err := csv.NewReader(strings.NewReader(stdout)).Read()
	if err != nil {
		t.Fatalf("failed to parse monitor csv output: %v (%q)", err, stdout)
	}
	if len(record) != 7 {
		t.Fatalf("expected 7 monitor fields, got %d (%q)", len(record), stdout)
	}
	if record[0] != "timeout" {
		t.Fatalf("expected timeout final state, got %q", record[0])
	}
	if record[4] != "max_polls_exceeded" {
		t.Fatalf("expected max_polls_exceeded reason, got %q", record[4])
	}
}

func TestCmdSessionMonitorRetriesDegradedUntilCompletion(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if pollCount == 1 {
			return sessionStatus{
				Session:      session,
				Status:       "idle",
				SessionState: "degraded",
			}, nil
		}
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "completed",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-retry",
			"--max-polls", "3",
			"--poll-interval", "1",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected monitor success after degraded retry, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"finalState":"completed"`) || !strings.Contains(stdout, `"polls":2`) {
		t.Fatalf("expected completed result after second poll, got %q", stdout)
	}
}

func TestCmdSessionExplainTextOutputWithTmuxReadError(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origReadTail := readSessionEventTailFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		readSessionEventTailFn = origReadTail
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "", errors.New("tmux server busy")
	}
	readSessionEventTailFn = func(projectRoot, session string, max int) (sessionEventTail, error) {
		return sessionEventTail{}, os.ErrNotExist
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionExplain([]string{
			"--session", "lisa-explain-text",
			"--project-root", t.TempDir(),
			"--events", "5",
		})
		if code != 0 {
			t.Fatalf("expected explain text output to succeed, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	for _, token := range []string{
		"session: lisa-explain-text",
		"state: degraded (idle)",
		"reason: tmux_capture_error",
		"tmux_read_error: tmux server busy",
		"events: none",
	} {
		if !strings.Contains(stdout, token) {
			t.Fatalf("expected token %q in explain output: %q", token, stdout)
		}
	}
}

func TestTmuxListSessionsProjectOnlyFiltersByProjectHash(t *testing.T) {
	projectRoot := t.TempDir()
	expectedHash := projectHash(projectRoot)

	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`case "$1" in`,
		`  list-sessions)`,
		`    echo "lisa-proj"`,
		`    echo "lisa-other"`,
		`    echo "not-lisa"`,
		`    exit 0 ;;`,
		`  show-environment)`,
		`    session=""`,
		`    key=""`,
		`    shift`,
		`    while [ "$#" -gt 0 ]; do`,
		`      case "$1" in`,
		`        -t) session="$2"; shift 2 ;;`,
		`        *) key="$1"; shift ;;`,
		`      esac`,
		`    done`,
		`    if [ "$key" = "LISA_PROJECT_HASH" ] && [ "$session" = "lisa-proj" ]; then`,
		`      echo "LISA_PROJECT_HASH=` + expectedHash + `"`,
		`      exit 0`,
		`    fi`,
		`    if [ "$key" = "LISA_PROJECT_HASH" ] && [ "$session" = "lisa-other" ]; then`,
		`      echo "LISA_PROJECT_HASH=deadbeef"`,
		`      exit 0`,
		`    fi`,
		`    exit 1 ;;`,
		`  *)`,
		`    exit 0 ;;`,
		`esac`,
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux binary: %v", err)
	}

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})

	projectOnly, err := tmuxListSessions(true, projectRoot)
	if err != nil {
		t.Fatalf("tmuxListSessions project-only failed: %v", err)
	}
	if !reflect.DeepEqual(projectOnly, []string{"lisa-proj"}) {
		t.Fatalf("unexpected project-only sessions: %v", projectOnly)
	}

	all, err := tmuxListSessions(false, projectRoot)
	if err != nil {
		t.Fatalf("tmuxListSessions all failed: %v", err)
	}
	if !reflect.DeepEqual(all, []string{"lisa-other", "lisa-proj"}) {
		t.Fatalf("unexpected all sessions list: %v", all)
	}
}
