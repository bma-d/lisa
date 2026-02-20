package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunRoutesHelpAndUnknown(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		if code := Run([]string{"help"}); code != 0 {
			t.Fatalf("expected help exit 0, got %d", code)
		}
	})
	if stdout != "" {
		t.Fatalf("expected no stdout for help, got %q", stdout)
	}
	if !strings.Contains(stderr, "lisa <command> [args]") {
		t.Fatalf("expected usage text on stderr, got %q", stderr)
	}

	_, stderr = captureOutput(t, func() {
		if code := Run([]string{"nope"}); code == 0 {
			t.Fatalf("expected unknown command to fail")
		}
	})
	if !strings.Contains(stderr, "unknown command: nope") {
		t.Fatalf("expected unknown command error, got %q", stderr)
	}
}

func TestUsageIncludesCoreCommands(t *testing.T) {
	_, stderr := captureOutput(t, func() { usage() })
	for _, token := range []string{
		"doctor",
		"cleanup",
		"session spawn",
		"session explain",
		"agent build-cmd",
	} {
		if !strings.Contains(stderr, token) {
			t.Fatalf("usage missing token %q in %q", token, stderr)
		}
	}
}

func TestCmdAgentRoutesAndBuildCmd(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		if code := cmdAgent(nil); code == 0 {
			t.Fatalf("expected cmdAgent without subcommand to fail")
		}
	})
	if !strings.Contains(stderr, "usage: lisa agent <subcommand>") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	_, stderr = captureOutput(t, func() {
		if code := cmdAgent([]string{"unknown"}); code == 0 {
			t.Fatalf("expected unknown agent subcommand to fail")
		}
	})
	if !strings.Contains(stderr, "unknown agent subcommand: unknown") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdAgentBuildCmd([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "ship it",
			"--json",
		}); code != 0 {
			t.Fatalf("expected build-cmd success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse JSON: %v (%q)", err, stdout)
	}
	if payload["agent"] != "codex" || payload["mode"] != "exec" {
		t.Fatalf("unexpected payload: %v", payload)
	}
	command, _ := payload["command"].(string)
	if !strings.Contains(command, "codex exec 'ship it' --full-auto") {
		t.Fatalf("unexpected command: %q", command)
	}
	if !strings.Contains(command, "--skip-git-repo-check") {
		t.Fatalf("expected codex exec command to include --skip-git-repo-check, got %q", command)
	}
}

func TestCmdSessionRouterAndName(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		if code := cmdSession(nil); code == 0 {
			t.Fatalf("expected cmdSession without subcommand to fail")
		}
	})
	if !strings.Contains(stderr, "usage: lisa session <subcommand>") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	_, stderr = captureOutput(t, func() {
		if code := cmdSession([]string{"missing"}); code == 0 {
			t.Fatalf("expected unknown session subcommand to fail")
		}
	})
	if !strings.Contains(stderr, "unknown session subcommand: missing") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	projectRoot := t.TempDir()
	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionName([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--project-root", projectRoot,
			"--tag", "Feature Alpha",
		}); code != 0 {
			t.Fatalf("expected cmdSessionName success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.HasPrefix(stdout, "lisa-") || !strings.Contains(stdout, "-codex-exec-featurealpha") {
		t.Fatalf("unexpected session name: %q", stdout)
	}
}

func TestWriteSessionOutputFilePersistsTail(t *testing.T) {
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() { tmuxCapturePaneFn = origCapture })

	lines := make([]string, 0, 300)
	for i := 1; i <= 300; i++ {
		lines = append(lines, "line-"+strconv.Itoa(i))
	}
	tmuxCapturePaneFn = func(session string, max int) (string, error) {
		return strings.Join(lines, "\n"), nil
	}

	projectRoot := t.TempDir()
	session := "lisa-output-tail"
	path, err := writeSessionOutputFile(projectRoot, session)
	if err != nil {
		t.Fatalf("writeSessionOutputFile failed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outLines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(outLines) != 260 {
		t.Fatalf("expected 260 lines, got %d", len(outLines))
	}
	if outLines[0] != "line-41" || outLines[len(outLines)-1] != "line-300" {
		t.Fatalf("unexpected tail bounds: first=%q last=%q", outLines[0], outLines[len(outLines)-1])
	}
}

func TestRunCmdInputRoundTripAndTimeout(t *testing.T) {
	out, err := runCmdInput("hello\n", "sh", "-c", "cat")
	if err != nil {
		t.Fatalf("runCmdInput should succeed: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("unexpected cat output: %q", out)
	}

	origTimeout := os.Getenv("LISA_CMD_TIMEOUT_SECONDS")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_CMD_TIMEOUT_SECONDS", origTimeout)
	})
	if err := os.Setenv("LISA_CMD_TIMEOUT_SECONDS", "1"); err != nil {
		t.Fatalf("failed to set timeout env: %v", err)
	}

	start := time.Now()
	_, err = runCmd("sh", "-c", "sleep 2")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "command timed out") {
		t.Fatalf("expected timeout error message, got %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Fatalf("timeout path took too long: %s", time.Since(start))
	}
}

func TestWrapTmuxCommandErrorIncludesOutputWhenPresent(t *testing.T) {
	base := errors.New("exit status 1")
	err := wrapTmuxCommandError(base, "failed to create socket: operation not permitted\n")
	if err == nil {
		t.Fatalf("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("expected base error in wrapped error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "failed to create socket: operation not permitted") {
		t.Fatalf("expected tmux output in wrapped error, got %q", err.Error())
	}
}

func TestCommandEnvStripsTMUXOnlyForTmuxCommands(t *testing.T) {
	origTMUX := os.Getenv("TMUX")
	origLog := os.Getenv("TMUX_LOG_FILE")
	t.Cleanup(func() {
		_ = os.Setenv("TMUX", origTMUX)
		_ = os.Setenv("TMUX_LOG_FILE", origLog)
	})

	if err := os.Setenv("TMUX", "/tmp/parent,1,1"); err != nil {
		t.Fatalf("failed to set TMUX: %v", err)
	}
	if err := os.Setenv("TMUX_LOG_FILE", "/tmp/tmux.log"); err != nil {
		t.Fatalf("failed to set TMUX_LOG_FILE: %v", err)
	}

	hasKey := func(env []string, key string) bool {
		prefix := key + "="
		for _, kv := range env {
			if strings.HasPrefix(kv, prefix) {
				return true
			}
		}
		return false
	}

	tmuxEnv := commandExecEnv("tmux")
	if hasKey(tmuxEnv, "TMUX") {
		t.Fatalf("expected TMUX to be stripped for tmux commands")
	}
	if !hasKey(tmuxEnv, "TMUX_LOG_FILE") {
		t.Fatalf("expected non-TMUX_* variables to remain for tmux commands")
	}

	otherEnv := commandExecEnv("sh")
	if !hasKey(otherEnv, "TMUX") {
		t.Fatalf("expected TMUX to be preserved for non-tmux commands")
	}
}

func TestWithProjectRuntimeEnvSetsAndRestores(t *testing.T) {
	origRoot, hadRoot := os.LookupEnv(lisaProjectRootEnv)
	origSocket, hadSocket := os.LookupEnv(lisaTmuxSocketEnv)
	t.Cleanup(func() {
		if hadRoot {
			_ = os.Setenv(lisaProjectRootEnv, origRoot)
		} else {
			_ = os.Unsetenv(lisaProjectRootEnv)
		}
		if hadSocket {
			_ = os.Setenv(lisaTmuxSocketEnv, origSocket)
		} else {
			_ = os.Unsetenv(lisaTmuxSocketEnv)
		}
	})

	_ = os.Setenv(lisaProjectRootEnv, "old-root")
	_ = os.Setenv(lisaTmuxSocketEnv, "old-socket")

	projectRoot := t.TempDir()
	restore := withProjectRuntimeEnv(projectRoot)

	if got := os.Getenv(lisaProjectRootEnv); got != canonicalProjectRoot(projectRoot) {
		t.Fatalf("expected %s=%q, got %q", lisaProjectRootEnv, canonicalProjectRoot(projectRoot), got)
	}
	if got := os.Getenv(lisaTmuxSocketEnv); got != tmuxSocketPathForProjectRoot(projectRoot) {
		t.Fatalf("expected %s=%q, got %q", lisaTmuxSocketEnv, tmuxSocketPathForProjectRoot(projectRoot), got)
	}

	restore()
	if got := os.Getenv(lisaProjectRootEnv); got != "old-root" {
		t.Fatalf("expected %s restored, got %q", lisaProjectRootEnv, got)
	}
	if got := os.Getenv(lisaTmuxSocketEnv); got != "old-socket" {
		t.Fatalf("expected %s restored, got %q", lisaTmuxSocketEnv, got)
	}
}

func TestCurrentTmuxSocketPathPrefersExplicitEnv(t *testing.T) {
	origRoot, hadRoot := os.LookupEnv(lisaProjectRootEnv)
	origSocket, hadSocket := os.LookupEnv(lisaTmuxSocketEnv)
	t.Cleanup(func() {
		if hadRoot {
			_ = os.Setenv(lisaProjectRootEnv, origRoot)
		} else {
			_ = os.Unsetenv(lisaProjectRootEnv)
		}
		if hadSocket {
			_ = os.Setenv(lisaTmuxSocketEnv, origSocket)
		} else {
			_ = os.Unsetenv(lisaTmuxSocketEnv)
		}
	})

	_ = os.Setenv(lisaProjectRootEnv, t.TempDir())
	_ = os.Setenv(lisaTmuxSocketEnv, "/tmp/custom.sock")
	if got := currentTmuxSocketPath(); got != "/tmp/custom.sock" {
		t.Fatalf("expected explicit socket path, got %q", got)
	}
}

func TestCmdSessionExistsUsesProjectRuntimeSocket(t *testing.T) {
	projectRoot := t.TempDir()
	expectedSocket := tmuxSocketPathForProjectRoot(projectRoot)
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`sock=""`,
		`if [ "$1" = "-S" ]; then`,
		`  sock="$2"`,
		`  shift 2`,
		`fi`,
		`if [ "$1" = "has-session" ] && [ "$2" = "-t" ] && [ "$3" = "lisa-exists-socket" ] && [ "$sock" = "` + expectedSocket + `" ]; then`,
		`  exit 0`,
		`fi`,
		`exit 1`,
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux: %v", err)
	}

	origPath := os.Getenv("PATH")
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	origRoot, hadRoot := os.LookupEnv(lisaProjectRootEnv)
	origSocket, hadSocket := os.LookupEnv(lisaTmuxSocketEnv)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Chdir(origWD)
		if hadRoot {
			_ = os.Setenv(lisaProjectRootEnv, origRoot)
		} else {
			_ = os.Unsetenv(lisaProjectRootEnv)
		}
		if hadSocket {
			_ = os.Setenv(lisaTmuxSocketEnv, origSocket)
		} else {
			_ = os.Unsetenv(lisaTmuxSocketEnv)
		}
	})
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to chdir project root: %v", err)
	}
	_ = os.Setenv(lisaProjectRootEnv, "/tmp/wrong-root")
	_ = os.Setenv(lisaTmuxSocketEnv, "/tmp/wrong.sock")

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionExists([]string{"--session", "lisa-exists-socket"}); code != 0 {
			t.Fatalf("expected exists success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.TrimSpace(stdout) != "true" {
		t.Fatalf("expected true output, got %q", stdout)
	}
}

func TestCmdSessionExistsSupportsProjectRootFlag(t *testing.T) {
	projectRoot := t.TempDir()
	otherRoot := t.TempDir()
	expectedSocket := tmuxSocketPathForProjectRoot(projectRoot)
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`sock=""`,
		`if [ "$1" = "-S" ]; then`,
		`  sock="$2"`,
		`  shift 2`,
		`fi`,
		`if [ "$1" = "has-session" ] && [ "$2" = "-t" ] && [ "$3" = "lisa-exists-project-root" ] && [ "$sock" = "` + expectedSocket + `" ]; then`,
		`  exit 0`,
		`fi`,
		`exit 1`,
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux: %v", err)
	}

	origPath := os.Getenv("PATH")
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Chdir(origWD)
	})
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Chdir(otherRoot); err != nil {
		t.Fatalf("failed to chdir other root: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionExists([]string{"--session", "lisa-exists-project-root", "--project-root", projectRoot}); code != 0 {
			t.Fatalf("expected exists success with --project-root, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.TrimSpace(stdout) != "true" {
		t.Fatalf("expected true output, got %q", stdout)
	}
}

func TestTrimSessionEventFileEnforcesLineCapEvenWhenBytesSmall(t *testing.T) {
	origMaxBytes := os.Getenv("LISA_EVENTS_MAX_BYTES")
	origMaxLines := os.Getenv("LISA_EVENTS_MAX_LINES")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_EVENTS_MAX_BYTES", origMaxBytes)
		_ = os.Setenv("LISA_EVENTS_MAX_LINES", origMaxLines)
	})
	_ = os.Setenv("LISA_EVENTS_MAX_BYTES", "100000")
	_ = os.Setenv("LISA_EVENTS_MAX_LINES", "3")

	path := filepath.Join(t.TempDir(), "events.jsonl")
	lines := []string{
		`{"poll":1}`,
		`{"poll":2}`,
		`{"poll":3}`,
		`{"poll":4}`,
		`{"poll":5}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to seed event file: %v", err)
	}

	if err := trimSessionEventFile(path); err != nil {
		t.Fatalf("trimSessionEventFile failed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read trimmed file: %v", err)
	}
	trimmed := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(trimmed) != 3 {
		t.Fatalf("expected line cap 3, got %d", len(trimmed))
	}
	if trimmed[0] != `{"poll":3}` || trimmed[2] != `{"poll":5}` {
		t.Fatalf("unexpected retained lines: %v", trimmed)
	}
}

func TestAppendSessionEventConcurrentWritersRemainValidJSONL(t *testing.T) {
	origMaxBytes := os.Getenv("LISA_EVENTS_MAX_BYTES")
	origMaxLines := os.Getenv("LISA_EVENTS_MAX_LINES")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_EVENTS_MAX_BYTES", origMaxBytes)
		_ = os.Setenv("LISA_EVENTS_MAX_LINES", origMaxLines)
	})
	_ = os.Setenv("LISA_EVENTS_MAX_BYTES", "1000000")
	_ = os.Setenv("LISA_EVENTS_MAX_LINES", "20")

	projectRoot := t.TempDir()
	session := "lisa-events-concurrent"
	var wg sync.WaitGroup
	errCh := make(chan error, 64)

	for i := 0; i < 64; i++ {
		wg.Add(1)
		poll := i + 1
		go func() {
			defer wg.Done()
			errCh <- appendSessionEvent(projectRoot, session, sessionEvent{
				At:      time.Now().UTC().Format(time.RFC3339Nano),
				Type:    "snapshot",
				Session: session,
				State:   "in_progress",
				Status:  "active",
				Reason:  "parallel",
				Poll:    poll,
				Signals: statusSignals{},
			})
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("appendSessionEvent failed: %v", err)
		}
	}

	tail, err := readSessionEventTail(projectRoot, session, 50)
	if err != nil {
		t.Fatalf("readSessionEventTail failed: %v", err)
	}
	if tail.DroppedLines != 0 {
		t.Fatalf("expected no malformed lines after concurrent writes, dropped=%d", tail.DroppedLines)
	}
	if len(tail.Events) == 0 || len(tail.Events) > 20 {
		t.Fatalf("expected bounded retained events (1..20), got %d", len(tail.Events))
	}
}

func TestComputeSessionStatusAgentScanErrorDegradesWhenNoOtherSignals(t *testing.T) {
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
	capture := "idle output"
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return capture, nil }
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "zsh", nil
		case "#{pane_pid}":
			return "123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, errors.New("ps failed")
	}

	projectRoot := t.TempDir()
	session := "lisa-scan-error"
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastOutputHash: md5Hex8(capture),
		LastOutputAt:   time.Now().Add(-20 * time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("failed to seed stale state: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "interactive", false, 4)
	if err != nil {
		t.Fatalf("computeSessionStatus failed: %v", err)
	}
	if status.SessionState != "degraded" || status.ClassificationReason != "agent_scan_error" {
		t.Fatalf("expected degraded agent_scan_error, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if status.Signals.AgentScanError == "" {
		t.Fatalf("expected agent scan error signal")
	}
}

func TestListProcessesParsesOutput(t *testing.T) {
	binDir := t.TempDir()
	psPath := filepath.Join(binDir, "ps")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		"cat <<'EOF'",
		"123 1 0.0 /usr/bin/claude exec",
		"bad line",
		"456 123 3.5 codex exec --full-auto",
		"EOF",
	}, "\n")
	if err := os.WriteFile(psPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake ps: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}

	procs, err := listProcesses()
	if err != nil {
		t.Fatalf("listProcesses failed: %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("expected 2 parsed processes, got %d (%v)", len(procs), procs)
	}
	if procs[1].PID != 456 || procs[1].PPID != 123 {
		t.Fatalf("unexpected parsed process: %+v", procs[1])
	}
	if !strings.Contains(procs[1].Command, "codex exec --full-auto") {
		t.Fatalf("unexpected command parse: %q", procs[1].Command)
	}
}

func TestNormalizeHelpersAndExitHelpers(t *testing.T) {
	if got := normalizeAgent("codex"); got != "codex" {
		t.Fatalf("expected codex normalization, got %q", got)
	}
	if got := normalizeAgent("invalid"); got != "claude" {
		t.Fatalf("expected fallback to claude, got %q", got)
	}
	if got := normalizeMode("exec"); got != "exec" {
		t.Fatalf("expected exec normalization, got %q", got)
	}
	if got := normalizeMode("invalid"); got != "interactive" {
		t.Fatalf("expected fallback to interactive, got %q", got)
	}
	if boolExit(true) != 0 || boolExit(false) != 1 {
		t.Fatalf("unexpected boolExit mapping")
	}
	_, stderr := captureOutput(t, func() { _ = flagValueError("--demo") })
	if !strings.Contains(stderr, "missing value for --demo") {
		t.Fatalf("expected flag value error output, got %q", stderr)
	}
}

func TestTmuxWrappersWithFakeBinary(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "tmux.log")
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`log="${TMUX_LOG_FILE:-/tmp/tmux.log}"`,
		`echo "$@" >> "$log"`,
		`if [ -n "${TMUX:-}" ]; then echo "TMUX_ENV_SET" >> "$log"; else echo "TMUX_ENV_UNSET" >> "$log"; fi`,
		`sock=""`,
		`if [ "$1" = "-S" ]; then`,
		`  sock="$2"`,
		`  echo "TMUX_SOCKET:$sock" >> "$log"`,
		`  shift 2`,
		`fi`,
		`cmd="$1"`,
		`case "$cmd" in`,
		`  has-session)`,
		`    if [ "${TMUX_HAS_SESSION:-1}" = "1" ]; then exit 0; fi`,
		`    echo "missing" >&2; exit 1 ;;`,
		`  kill-session) exit 0 ;;`,
		`  new-session) exit 0 ;;`,
		`  send-keys) exit 0 ;;`,
		`  capture-pane) echo "captured-line"; exit 0 ;;`,
		`  show-environment) echo "LISA_MODE=exec"; exit 0 ;;`,
		`  load-buffer) cat >/dev/null; exit 0 ;;`,
		`  paste-buffer) exit 0 ;;`,
		`  delete-buffer) exit 0 ;;`,
		`  display-message)`,
		`    last=""`,
		`    for arg in "$@"; do last="$arg"; done`,
		`    if [ "$last" = "#{pane_dead}" ]; then echo "0"; exit 0; fi`,
		`    if [ "$last" = "#{pane_dead_status}" ]; then echo "0"; exit 0; fi`,
		`    echo ""; exit 0 ;;`,
		`  *) exit 0 ;;`,
		`esac`,
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux: %v", err)
	}

	origPath := os.Getenv("PATH")
	origLog := os.Getenv("TMUX_LOG_FILE")
	origHas := os.Getenv("TMUX_HAS_SESSION")
	origTMUX := os.Getenv("TMUX")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Setenv("TMUX_LOG_FILE", origLog)
		_ = os.Setenv("TMUX_HAS_SESSION", origHas)
		_ = os.Setenv("TMUX", origTMUX)
	})
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	_ = os.Setenv("TMUX_LOG_FILE", logPath)
	_ = os.Setenv("TMUX_HAS_SESSION", "1")
	_ = os.Setenv("TMUX", "/tmp/parent,123,1")

	projectRoot := t.TempDir()
	expectedSocket := tmuxSocketPathForProjectRoot(projectRoot)
	if err := tmuxNewSession("lisa-tmux-test", projectRoot, "claude", "interactive", 120, 50); err != nil {
		t.Fatalf("tmuxNewSession failed: %v", err)
	}
	if err := tmuxSendCommandWithFallback(projectRoot, "lisa-tmux-test", strings.Repeat("x", 550), true); err != nil {
		t.Fatalf("tmuxSendCommandWithFallback failed: %v", err)
	}
	if err := tmuxSendText("lisa-tmux-test", "hello", true); err != nil {
		t.Fatalf("tmuxSendText failed: %v", err)
	}
	if err := tmuxSendKeys("lisa-tmux-test", []string{"C-c"}, false); err != nil {
		t.Fatalf("tmuxSendKeys failed: %v", err)
	}
	if !tmuxHasSession("lisa-tmux-test") {
		t.Fatalf("expected tmuxHasSession true")
	}
	_ = os.Setenv("TMUX_HAS_SESSION", "0")
	if tmuxHasSession("lisa-tmux-test") {
		t.Fatalf("expected tmuxHasSession false when fake tmux returns error")
	}
	_ = os.Setenv("TMUX_HAS_SESSION", "1")
	if err := tmuxKillSession("lisa-tmux-test"); err != nil {
		t.Fatalf("tmuxKillSession failed: %v", err)
	}
	capture, err := tmuxCapturePane("lisa-tmux-test", 25)
	if err != nil {
		t.Fatalf("tmuxCapturePane failed: %v", err)
	}
	if strings.TrimSpace(capture) != "captured-line" {
		t.Fatalf("unexpected capture output: %q", capture)
	}
	envVal, err := tmuxShowEnvironment("lisa-tmux-test", "LISA_MODE")
	if err != nil {
		t.Fatalf("tmuxShowEnvironment failed: %v", err)
	}
	if envVal != "exec" {
		t.Fatalf("unexpected env value: %q", envVal)
	}
	paneStatus, err := tmuxPaneStatus("lisa-tmux-test")
	if err != nil {
		t.Fatalf("tmuxPaneStatus failed: %v", err)
	}
	if paneStatus != "alive" {
		t.Fatalf("unexpected pane status: %q", paneStatus)
	}

	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux log: %v", err)
	}
	logText := string(logRaw)
	for _, cmd := range []string{"new-session", "send-keys", "load-buffer", "paste-buffer", "capture-pane"} {
		if !strings.Contains(logText, cmd) {
			t.Fatalf("expected %q in tmux invocation log: %s", cmd, logText)
		}
	}
	if strings.Contains(logText, "TMUX_ENV_SET") {
		t.Fatalf("expected tmux commands to run with TMUX unset, log: %s", logText)
	}
	if !strings.Contains(logText, "TMUX_ENV_UNSET") {
		t.Fatalf("expected tmux log marker for TMUX unset, log: %s", logText)
	}
	if !strings.Contains(logText, "TMUX_SOCKET:"+expectedSocket) {
		t.Fatalf("expected tmux socket path marker in log: %s", logText)
	}
	if !strings.Contains(logText, "LISA_TMUX_SOCKET="+expectedSocket) {
		t.Fatalf("expected spawned pane env to include LISA_TMUX_SOCKET, log: %s", logText)
	}
	if !strings.Contains(logText, "LISA_PROJECT_ROOT="+projectRoot) {
		t.Fatalf("expected spawned pane env to include LISA_PROJECT_ROOT, log: %s", logText)
	}
}

func TestTmuxSendCommandWithFallbackLongCommandUsesTempScript(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "tmux.log")
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`log="${TMUX_LOG_FILE:-/tmp/tmux.log}"`,
		`echo "$@" >> "$log"`,
		"exit 0",
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux: %v", err)
	}

	origPath := os.Getenv("PATH")
	origLog := os.Getenv("TMUX_LOG_FILE")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Setenv("TMUX_LOG_FILE", origLog)
	})
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG_FILE", logPath); err != nil {
		t.Fatalf("failed to set TMUX_LOG_FILE: %v", err)
	}

	projectRoot := t.TempDir()
	session := "lisa-long-command-fallback"
	longCommand := "echo LONG_COMMAND_START " + strings.Repeat("x", 560)
	if len(longCommand) <= maxInlineSendLength {
		t.Fatalf("expected command length > %d, got %d", maxInlineSendLength, len(longCommand))
	}

	if err := tmuxSendCommandWithFallback(projectRoot, session, longCommand, true); err != nil {
		t.Fatalf("tmuxSendCommandWithFallback failed: %v", err)
	}

	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux log: %v", err)
	}
	logLines := trimLines(string(logRaw))
	respawnLine := ""
	for _, line := range logLines {
		if strings.Contains(strings.TrimSpace(line), "respawn-pane ") {
			respawnLine = line
			break
		}
	}
	if respawnLine == "" {
		t.Fatalf("expected respawn-pane invocation in log, got %q", string(logRaw))
	}

	fields := strings.Fields(respawnLine)
	if len(fields) < 2 {
		t.Fatalf("unexpected respawn-pane invocation: %q", respawnLine)
	}
	scriptPath := fields[len(fields)-1]
	if !strings.HasPrefix(scriptPath, "/tmp/lisa-cmd-") {
		t.Fatalf("expected temp script path in respawn command, got %q", scriptPath)
	}
	if !strings.Contains(scriptPath, projectHash(projectRoot)) {
		t.Fatalf("expected project hash in temp script path %q", scriptPath)
	}
	if !strings.Contains(scriptPath, sessionArtifactID(session)) {
		t.Fatalf("expected session artifact id in temp script path %q", scriptPath)
	}

	scriptRaw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read fallback script %q: %v", scriptPath, err)
	}
	scriptText := string(scriptRaw)
	for _, token := range []string{
		"#!/usr/bin/env bash",
		"unset CLAUDECODE",
		longCommand,
		"exec \"${SHELL:-/bin/sh}\" -l",
	} {
		if !strings.Contains(scriptText, token) {
			t.Fatalf("expected token %q in fallback script: %q", token, scriptText)
		}
	}
}

func TestCmdDoctorJSONReadyAndNotReady(t *testing.T) {
	makeBin := func(dir, name string) {
		path := filepath.Join(dir, name)
		body := "#!/usr/bin/env sh\nexit 0\n"
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatalf("failed to create fake %s: %v", name, err)
		}
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
	})

	readyDir := t.TempDir()
	makeBin(readyDir, "tmux")
	makeBin(readyDir, "claude")
	makeBin(readyDir, "codex")
	if err := os.Setenv("PATH", readyDir); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdDoctor([]string{"--json"}); code != 0 {
			t.Fatalf("expected ready doctor exit 0, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr for ready doctor: %q", stderr)
	}
	var ready map[string]any
	if err := json.Unmarshal([]byte(stdout), &ready); err != nil {
		t.Fatalf("failed to parse ready doctor json: %v (%q)", err, stdout)
	}
	if ready["ok"] != true {
		t.Fatalf("expected ready doctor payload ok=true, got %v", ready["ok"])
	}

	notReadyDir := t.TempDir()
	makeBin(notReadyDir, "tmux")
	if err := os.Setenv("PATH", notReadyDir); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}

	stdout, stderr = captureOutput(t, func() {
		if code := cmdDoctor([]string{"--json"}); code != 1 {
			t.Fatalf("expected not-ready doctor exit 1, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr for not-ready doctor: %q", stderr)
	}
	var notReady map[string]any
	if err := json.Unmarshal([]byte(stdout), &notReady); err != nil {
		t.Fatalf("failed to parse not-ready doctor json: %v (%q)", err, stdout)
	}
	if notReady["ok"] != false {
		t.Fatalf("expected not-ready doctor payload ok=false, got %v", notReady["ok"])
	}
}
