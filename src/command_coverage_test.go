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
	cmd, err := buildAgentCommand("claude", "interactive", "hello world", "--model haiku")
	if err != nil {
		t.Fatalf("buildAgentCommand interactive claude failed: %v", err)
	}
	if cmd != "claude --dangerously-skip-permissions --model haiku 'hello world'" {
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
		t.Fatalf("expected codex exec command in payload (codex should not have --dangerously-skip-permissions), got %q", stdout)
	}
	if !strings.Contains(stdout, `--skip-git-repo-check`) {
		t.Fatalf("expected codex exec command to include --skip-git-repo-check, got %q", stdout)
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

func TestCmdAgentBuildCmdExecPathWithModel(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdAgentBuildCmd([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--model", "GPT-5.3-Codex-Spark",
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
	if !strings.Contains(stdout, `--model 'GPT-5.3-Codex-Spark'`) {
		t.Fatalf("expected codex exec command to include model flag, got %q", stdout)
	}
}

func TestCmdAgentBuildCmdModelRejectsClaude(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdAgentBuildCmd([]string{
			"--agent", "claude",
			"--mode", "interactive",
			"--model", "GPT-5.3-Codex-Spark",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected model/agent validation failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"invalid_model_configuration"`) {
		t.Fatalf("expected invalid_model_configuration, got %q", stdout)
	}
}

func TestCmdAgentBuildCmdExecPathAutoBypassesNestedCodexPrompt(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdAgentBuildCmd([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "Use ./lisa session spawn for nested workers",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected exec build-cmd success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `--dangerously-bypass-approvals-and-sandbox`) {
		t.Fatalf("expected nested codex exec build-cmd to include bypass sandbox, got %q", stdout)
	}
	if strings.Contains(stdout, `--full-auto`) {
		t.Fatalf("expected nested codex exec build-cmd to omit --full-auto, got %q", stdout)
	}
	if !strings.Contains(stdout, `--skip-git-repo-check`) {
		t.Fatalf("expected codex exec command to include --skip-git-repo-check, got %q", stdout)
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

func TestCmdSessionMonitorStopsOnWaitingInput(t *testing.T) {
	origCompute := computeSessionStatusFn
	origAppend := appendSessionEventFn
	origWaitingTurnComplete := monitorWaitingTurnCompleteFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		appendSessionEventFn = origAppend
		monitorWaitingTurnCompleteFn = origWaitingTurnComplete
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "waiting_input",
		}, nil
	}

	var observedReason string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		observedReason = event.Reason
		return nil
	}
	monitorWaitingTurnCompleteFn = func(session, projectRoot string, status sessionStatus) waitingTurnCompleteResult {
		return waitingTurnCompleteResult{Ready: false}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-waiting",
			"--poll-interval", "1",
			"--max-polls", "3",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected waiting-input monitor exit 0, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"finalState":"waiting_input"`) || !strings.Contains(stdout, `"exitReason":"waiting_input"`) {
		t.Fatalf("expected waiting_input monitor payload, got %q", stdout)
	}
	if observedReason != "monitor_waiting_input" {
		t.Fatalf("expected lifecycle reason monitor_waiting_input, got %q", observedReason)
	}
}

func TestCmdSessionMonitorDoesNotStopOnWaitingInputWhenDisabled(t *testing.T) {
	origCompute := computeSessionStatusFn
	origWaitingTurnComplete := monitorWaitingTurnCompleteFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		monitorWaitingTurnCompleteFn = origWaitingTurnComplete
	})

	calls := 0
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		calls++
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "waiting_input",
		}, nil
	}
	monitorWaitingTurnCompleteFn = func(session, projectRoot string, status sessionStatus) waitingTurnCompleteResult {
		return waitingTurnCompleteResult{Ready: true}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-waiting-disabled",
			"--project-root", t.TempDir(),
			"--poll-interval", "1",
			"--max-polls", "1",
			"--stop-on-waiting", "false",
			"--json",
		})
		if code != 2 {
			t.Fatalf("expected timeout monitor exit 2 when waiting stop disabled, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if calls != 1 {
		t.Fatalf("expected one monitor poll, got %d", calls)
	}
	if !strings.Contains(stdout, `"finalState":"timeout"`) {
		t.Fatalf("expected timeout final state when waiting stop disabled, got %q", stdout)
	}
	if !strings.Contains(stdout, `"exitReason":"max_polls_exceeded"`) {
		t.Fatalf("expected max_polls_exceeded reason when waiting stop disabled, got %q", stdout)
	}
}

func TestCmdSessionMonitorVerboseWritesProgressLine(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "completed",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-verbose",
			"--poll-interval", "1",
			"--max-polls", "1",
			"--verbose",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected completed monitor exit 0, got %d", code)
		}
	})
	if !strings.Contains(stderr, "poll=1 state=completed status=completed") {
		t.Fatalf("expected verbose monitor progress line in stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, `"finalState":"completed"`) {
		t.Fatalf("expected completed monitor payload, got %q", stdout)
	}
}

func TestCmdSessionExplainTextOutputWithTmuxReadError(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origReadTail := readSessionEventTailFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		readSessionEventTailFn = origReadTail
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxDisplayFn = func(session, format string) (string, error) {
		return "0\t\tbash\t12345", nil
	}
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
		"state: just_started (idle)",
		"reason: grace_period_just_started",
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
		`if [ "$1" = "-S" ]; then shift 2; fi`,
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
