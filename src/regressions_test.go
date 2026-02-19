package app

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildFallbackScriptBodyPreservesExecCompletionMarkerOnFailure(t *testing.T) {
	command := wrapExecCommand("false")
	body := buildFallbackScriptBody(command)

	if !strings.Contains(body, "set +e\n") {
		t.Fatalf("expected fallback script to disable errexit for wrapped exec commands")
	}
	if !strings.Contains(body, execDonePrefix) {
		t.Fatalf("expected fallback script to include exec completion marker")
	}
}

func TestBuildFallbackScriptBodyLeavesNonExecCommandsUntouched(t *testing.T) {
	body := buildFallbackScriptBody("echo hello")
	if strings.Contains(body, "set +e\n") {
		t.Fatalf("did not expect errexit override for non-exec command")
	}
	if !strings.Contains(body, "unset CLAUDECODE\n") {
		t.Fatalf("expected fallback script to clear CLAUDECODE, got %q", body)
	}
	if !strings.Contains(body, "unset TMUX\n") {
		t.Fatalf("expected fallback script to clear TMUX, got %q", body)
	}
}

func TestWrapExecCommandTemporarilyDisablesErrexit(t *testing.T) {
	wrapped := wrapExecCommand("false")
	for _, token := range []string{
		"case $- in *e*) __lisa_had_errexit=1;; esac",
		"set +e;",
		execDonePrefix,
		"then set -e; fi",
	} {
		if !strings.Contains(wrapped, token) {
			t.Fatalf("expected wrapped exec command to contain %q, got %q", token, wrapped)
		}
	}
}

func TestWrapSessionCommandInjectsLifecycleMarkersAndHeartbeat(t *testing.T) {
	wrapped := wrapSessionCommand("echo hello", "run-test-1")
	for _, token := range []string{
		sessionStartPrefix,
		sessionDonePrefix,
		"run-test-1",
		"LISA_HEARTBEAT_FILE",
		"LISA_RUN_ID",
		"unset CLAUDECODE;",
		"unset TMUX;",
		"trap '__lisa_ec=130; exit \"$__lisa_ec\"' INT TERM HUP",
		"echo hello",
	} {
		if !strings.Contains(wrapped, token) {
			t.Fatalf("expected wrapped session command to contain %q, got %q", token, wrapped)
		}
	}
}

func TestWrapSessionCommandTracksExecFailureInSessionDoneMarker(t *testing.T) {
	runID := "run-exec-fail"
	wrapped := wrapSessionCommand(wrapExecCommand("bash -lc 'exit 7'"), runID)
	cmd := exec.Command("bash", "-lc", wrapped)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected wrapped command to fail with non-zero exit")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("expected wrapped command to exit 7, got %d; output=%q", exitErr.ExitCode(), string(out))
	}

	done, code, markerRunID, mismatch := parseSessionCompletionForRun(string(out), runID)
	if !done || mismatch || markerRunID != runID || code != 7 {
		t.Fatalf("expected run marker exit code 7, got done=%v mismatch=%v run=%q code=%d output=%q", done, mismatch, markerRunID, code, string(out))
	}
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	fn()

	_ = stdoutW.Close()
	_ = stderrW.Close()

	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("failed to read stderr: %v", err)
	}
	_ = stdoutR.Close()
	_ = stderrR.Close()

	return strings.TrimSpace(string(stdoutBytes)), strings.TrimSpace(string(stderrBytes))
}

func TestComputeSessionStatusDoesNotTreatTagLikeClaudeTailAsWaiting(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "processing output\n<results>", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
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
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 456, 3.2, nil
	}

	status, err := computeSessionStatus("lisa-test", t.TempDir(), "claude", "interactive", false, 1)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "in_progress" || status.Status != "active" {
		t.Fatalf("expected busy claude session to remain active, got state=%s status=%s", status.SessionState, status.Status)
	}
}

func TestComputeSessionStatusTreatsPwshPaneAsShell(t *testing.T) {
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
	capture := "no new output"
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return capture, nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "pwsh", nil
		case "#{pane_pid}":
			return "123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-pwsh-shell"
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastOutputHash: md5Hex8(capture),
		LastOutputAt:   time.Now().Add(-15 * time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("failed to seed stale state: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "stuck" || status.Status != "idle" {
		t.Fatalf("expected stale pwsh shell to be idle/stuck, got state=%s status=%s", status.SessionState, status.Status)
	}
}

func TestComputeSessionStatusUsesHeartbeatWhenAgentUndetected(t *testing.T) {
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
	capture := "running silently"
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return capture, nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
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
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-heartbeat-active"
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastOutputHash: md5Hex8(capture),
		LastOutputAt:   time.Now().Add(-15 * time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("failed to seed stale output state: %v", err)
	}
	if err := os.WriteFile(sessionHeartbeatFile(projectRoot, session), []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o600); err != nil {
		t.Fatalf("failed to seed heartbeat file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "in_progress" || status.Status != "active" {
		t.Fatalf("expected heartbeat-backed session to be active, got state=%s status=%s", status.SessionState, status.Status)
	}
	if status.HeartbeatAge < 0 {
		t.Fatalf("expected heartbeat age to be recorded")
	}
}

func TestComputeSessionStatusDoesNotOverwriteNewerOutputTimestamp(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origNow := nowFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		nowFn = origNow
	})

	baseNow := time.Unix(1_700_000_000, 500_000_000)
	nowFn = func() time.Time { return baseNow }

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "new-output", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\t123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 0, 0, nil }

	projectRoot := t.TempDir()
	session := "lisa-nanos-order"
	oldHash := md5Hex8("old-output")
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastOutputHash:    oldHash,
		LastOutputAt:      baseNow.Unix(),
		LastOutputAtNanos: baseNow.UnixNano() + 200,
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	if _, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4); err != nil {
		t.Fatalf("computeSessionStatus failed: %v", err)
	}

	state := loadSessionState(sessionStateFile(projectRoot, session))
	if state.LastOutputHash != oldHash {
		t.Fatalf("expected newer timestamp guard to preserve old hash, got %q", state.LastOutputHash)
	}
}

func TestComputeSessionStatusInteractiveUsesDoneFile(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "", nil }
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
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
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		if key == "LISA_MODE" {
			return "interactive", nil
		}
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}

	projectRoot := t.TempDir()
	if err := os.WriteFile(sessionDoneFile(projectRoot, "lisa-interactive-done"), []byte("run-1:0\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	status, err := computeSessionStatus("lisa-interactive-done", projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "completed" || status.Status != "idle" {
		t.Fatalf("expected done file to mark completion, got state=%s status=%s", status.SessionState, status.Status)
	}
	if status.ClassificationReason != "done_file" {
		t.Fatalf("expected done_file reason, got %s", status.ClassificationReason)
	}
}

func TestParseExecCompletionIgnoresHistoricalMarkers(t *testing.T) {
	capture := strings.Join([]string{
		"working",
		"__LISA_EXEC_DONE__:0",
		"starting next run",
		"still running",
	}, "\n")

	done, code := parseExecCompletion(capture)
	if done {
		t.Fatalf("expected stale marker to be ignored, got done=%v code=%d", done, code)
	}
}

func TestParseExecCompletionAcceptsTailMarkerWithPrompt(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_EXEC_DONE__:7",
		"user@host:~/repo$",
	}, "\n")

	done, code := parseExecCompletion(capture)
	if !done || code != 7 {
		t.Fatalf("expected tail marker to be parsed, got done=%v code=%d", done, code)
	}
}

func TestParseExecCompletionIgnoresTagLikeTrailingLine(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_EXEC_DONE__:0",
		"<results>",
	}, "\n")

	done, code := parseExecCompletion(capture)
	if done {
		t.Fatalf("expected tag-like trailing line to invalidate completion, got done=%v code=%d", done, code)
	}
}

func TestParseExecCompletionAcceptsFishStylePromptWithPath(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_EXEC_DONE__:0",
		"~/repo>",
	}, "\n")

	done, code := parseExecCompletion(capture)
	if !done || code != 0 {
		t.Fatalf("expected fish-style prompt to be accepted, got done=%v code=%d", done, code)
	}
}

func TestParseSessionCompletionAcceptsTailMarkerWithPrompt(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_SESSION_DONE__:2",
		"user@host:~/repo$",
	}, "\n")

	done, code := parseSessionCompletion(capture)
	if !done || code != 2 {
		t.Fatalf("expected session done marker to be parsed, got done=%v code=%d", done, code)
	}
}

func TestParseSessionCompletionAcceptsTailMarkerWithStarshipPrompt(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_SESSION_DONE__:run-10:0",
		"~/projects/tools/lisa main !1 ?2 ❯                                         11s 18:01:20",
	}, "\n")

	done, code, markerRunID, mismatch := parseSessionCompletionForRun(capture, "run-10")
	if !done || code != 0 || markerRunID != "run-10" || mismatch {
		t.Fatalf("expected starship prompt follow-up to keep marker valid, got done=%v code=%d runID=%q mismatch=%v", done, code, markerRunID, mismatch)
	}
}

func TestParseSessionCompletionRejectsNonPromptLineContainingPromptGlyph(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_SESSION_DONE__:run-11:0",
		"analysis note: this branch uses ❯ arrow symbol in docs",
	}, "\n")

	done, _, _, _ := parseSessionCompletionForRun(capture, "run-11")
	if done {
		t.Fatalf("expected non-prompt line with prompt glyph to invalidate completion marker")
	}
}

func TestTailLinesKeepsNewestOutput(t *testing.T) {
	values := make([]string, 300)
	for i := 0; i < 300; i++ {
		values[i] = "line-" + strconv.Itoa(i+1)
	}

	tail := tailLines(values, 260)
	if len(tail) != 260 {
		t.Fatalf("expected 260 lines, got %d", len(tail))
	}
	if tail[0] != values[40] {
		t.Fatalf("expected oldest retained line to be %q, got %q", values[40], tail[0])
	}
	if tail[len(tail)-1] != values[len(values)-1] {
		t.Fatalf("expected newest retained line to be %q, got %q", values[len(values)-1], tail[len(tail)-1])
	}
}

func TestDetectAgentProcessReturnsMatchEvenWhenCPUIsZero(t *testing.T) {
	origList := listProcessesFn
	t.Cleanup(func() {
		listProcessesFn = origList
	})

	listProcessesFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 10, PPID: 1, CPU: 0.0, Command: "codex exec 'task'"},
			{PID: 11, PPID: 1, CPU: 0.4, Command: "bash"},
		}, nil
	}

	pid, cpu, err := detectAgentProcess(1, "codex")
	if err != nil {
		t.Fatalf("unexpected process scan error: %v", err)
	}
	if pid != 10 || cpu != 0.0 {
		t.Fatalf("expected codex PID with zero CPU to be selected, got pid=%d cpu=%f", pid, cpu)
	}
}

func TestDetectAgentProcessReturnsZeroCPUWhenNoMatch(t *testing.T) {
	origList := listProcessesFn
	t.Cleanup(func() {
		listProcessesFn = origList
	})

	listProcessesFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 10, PPID: 1, CPU: 1.2, Command: "bash"},
			{PID: 11, PPID: 1, CPU: 0.4, Command: "zsh"},
		}, nil
	}

	pid, cpu, err := detectAgentProcess(1, "codex")
	if err != nil {
		t.Fatalf("unexpected process scan error: %v", err)
	}
	if pid != 0 || cpu != 0.0 {
		t.Fatalf("expected no match to return pid=0 cpu=0, got pid=%d cpu=%f", pid, cpu)
	}
}

func TestDetectAgentProcessUsesSharedProcessListCache(t *testing.T) {
	origList := listProcessesFn
	origCacheMS := os.Getenv("LISA_PROCESS_LIST_CACHE_MS")
	t.Cleanup(func() {
		listProcessesFn = origList
		_ = os.Setenv("LISA_PROCESS_LIST_CACHE_MS", origCacheMS)
	})

	if err := os.Setenv("LISA_PROCESS_LIST_CACHE_MS", "1000"); err != nil {
		t.Fatalf("failed to set process cache env: %v", err)
	}
	processCache.mu.Lock()
	processCache.fnPtr = 0
	processCache.atNanos = 0
	processCache.procs = nil
	processCache.mu.Unlock()

	listCalls := 0
	listProcessesFn = func() ([]processInfo, error) {
		listCalls++
		return []processInfo{
			{PID: 10, PPID: 1, CPU: 0.8, Command: "codex exec"},
		}, nil
	}

	for i := 0; i < 2; i++ {
		pid, _, err := detectAgentProcess(1, "codex")
		if err != nil {
			t.Fatalf("unexpected detectAgentProcess error: %v", err)
		}
		if pid != 10 {
			t.Fatalf("expected cached process lookup to find pid 10, got %d", pid)
		}
	}
	if listCalls != 1 {
		t.Fatalf("expected one process table read due to cache, got %d", listCalls)
	}
}

func TestDetectAgentProcessHonorsCustomMatchEnv(t *testing.T) {
	origList := listProcessesFn
	origMatch := os.Getenv("LISA_AGENT_PROCESS_MATCH_CODEX")
	t.Cleanup(func() {
		listProcessesFn = origList
		_ = os.Setenv("LISA_AGENT_PROCESS_MATCH_CODEX", origMatch)
	})

	if err := os.Setenv("LISA_AGENT_PROCESS_MATCH_CODEX", "wrapper-codex"); err != nil {
		t.Fatalf("failed to set custom match env: %v", err)
	}
	listProcessesFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 22, PPID: 1, CPU: 1.3, Command: "wrapper-codex --task"},
		}, nil
	}

	pid, _, err := detectAgentProcess(1, "codex")
	if err != nil {
		t.Fatalf("unexpected detectAgentProcess error: %v", err)
	}
	if pid != 22 {
		t.Fatalf("expected custom matcher to detect wrapper process, got pid=%d", pid)
	}
}

func TestDetectAgentProcessMatchesWrappedPrimaryBinary(t *testing.T) {
	origList := listProcessesFn
	t.Cleanup(func() {
		listProcessesFn = origList
	})

	listProcessesFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 31, PPID: 1, CPU: 0.3, Command: "python /usr/local/bin/codex exec task"},
			{PID: 32, PPID: 1, CPU: 0.4, Command: "bash"},
		}, nil
	}

	pid, _, err := detectAgentProcess(1, "codex")
	if err != nil {
		t.Fatalf("unexpected detectAgentProcess error: %v", err)
	}
	if pid != 31 {
		t.Fatalf("expected wrapped codex command to match pid=31, got %d", pid)
	}
}

func TestDetectAgentProcessDoesNotTreatArgumentTokenAsAgent(t *testing.T) {
	origList := listProcessesFn
	t.Cleanup(func() {
		listProcessesFn = origList
	})

	listProcessesFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 41, PPID: 1, CPU: 0.8, Command: "python worker.py --label codex"},
			{PID: 42, PPID: 1, CPU: 0.7, Command: "bash"},
		}, nil
	}

	pid, _, err := detectAgentProcess(1, "codex")
	if err != nil {
		t.Fatalf("unexpected detectAgentProcess error: %v", err)
	}
	if pid != 0 {
		t.Fatalf("expected no strict codex executable match, got pid=%d", pid)
	}
}

func TestDetectAgentProcessDoesNotCacheErrors(t *testing.T) {
	origList := listProcessesFn
	origCacheMS := os.Getenv("LISA_PROCESS_LIST_CACHE_MS")
	t.Cleanup(func() {
		listProcessesFn = origList
		_ = os.Setenv("LISA_PROCESS_LIST_CACHE_MS", origCacheMS)
	})

	if err := os.Setenv("LISA_PROCESS_LIST_CACHE_MS", "1000"); err != nil {
		t.Fatalf("failed to set process cache env: %v", err)
	}
	processCache.mu.Lock()
	processCache.fnPtr = 0
	processCache.atNanos = 0
	processCache.procs = nil
	processCache.mu.Unlock()

	listCalls := 0
	listProcessesFn = func() ([]processInfo, error) {
		listCalls++
		if listCalls == 1 {
			return nil, errors.New("ps failed")
		}
		return []processInfo{
			{PID: 10, PPID: 1, CPU: 0.8, Command: "codex exec"},
		}, nil
	}

	if _, _, err := detectAgentProcess(1, "codex"); err == nil {
		t.Fatalf("expected first process lookup to fail")
	}
	pid, _, err := detectAgentProcess(1, "codex")
	if err != nil {
		t.Fatalf("expected second process lookup to retry and succeed, got %v", err)
	}
	if pid != 10 {
		t.Fatalf("expected retry path to return pid 10, got %d", pid)
	}
	if listCalls != 2 {
		t.Fatalf("expected two list calls when first fails, got %d", listCalls)
	}
}

func TestSessionMatchesProjectRootUsesProjectHashAndLegacyMetaFallback(t *testing.T) {
	original := tmuxShowEnvironmentFn
	t.Cleanup(func() {
		tmuxShowEnvironmentFn = original
	})

	projectOne := filepath.Join(t.TempDir(), "a", "same")
	projectTwo := filepath.Join(t.TempDir(), "b", "same")
	if err := os.MkdirAll(projectOne, 0o755); err != nil {
		t.Fatalf("failed to create projectOne: %v", err)
	}
	if err := os.MkdirAll(projectTwo, 0o755); err != nil {
		t.Fatalf("failed to create projectTwo: %v", err)
	}

	session := "lisa-same-test"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectOne,
	}
	if err := saveSessionMeta(projectOne, session, meta); err != nil {
		t.Fatalf("failed to write legacy session metadata: %v", err)
	}

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	if !sessionMatchesProjectRoot(session, projectOne, "") {
		t.Fatalf("expected legacy fallback to match session in originating project")
	}
	if sessionMatchesProjectRoot(session, projectTwo, "") {
		t.Fatalf("did not expect fallback match for different root sharing basename")
	}

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return projectHash(projectOne), nil
	}
	if !sessionMatchesProjectRoot(session, projectOne, projectHash(projectOne)) {
		t.Fatalf("expected hash-based project match")
	}

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return projectHash(projectTwo), nil
	}
	if sessionMatchesProjectRoot(session, projectOne, projectHash(projectOne)) {
		t.Fatalf("did not expect hash mismatch to match")
	}
}

func TestDoctorReadyRequiresTmuxAndAtLeastOneAgent(t *testing.T) {
	cases := []struct {
		name   string
		checks []doctorCheck
		wantOK bool
	}{
		{
			name: "tmux and claude",
			checks: []doctorCheck{
				{Name: "tmux", Available: true},
				{Name: "claude", Available: true},
				{Name: "codex", Available: false},
			},
			wantOK: true,
		},
		{
			name: "tmux and codex",
			checks: []doctorCheck{
				{Name: "tmux", Available: true},
				{Name: "claude", Available: false},
				{Name: "codex", Available: true},
			},
			wantOK: true,
		},
		{
			name: "no tmux",
			checks: []doctorCheck{
				{Name: "tmux", Available: false},
				{Name: "claude", Available: true},
				{Name: "codex", Available: true},
			},
			wantOK: false,
		},
		{
			name: "no agents",
			checks: []doctorCheck{
				{Name: "tmux", Available: true},
				{Name: "claude", Available: false},
				{Name: "codex", Available: false},
			},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		if got := doctorReady(tc.checks); got != tc.wantOK {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.wantOK, got)
		}
	}
}

func TestCmdDoctorRejectsUnknownFlags(t *testing.T) {
	if code := cmdDoctor([]string{"--bogus"}); code == 0 {
		t.Fatalf("expected non-zero exit for unknown doctor flag")
	}
}

func TestRunHandlesVersionCommand(t *testing.T) {
	origVersion := BuildVersion
	origCommit := BuildCommit
	origDate := BuildDate
	t.Cleanup(func() {
		BuildVersion = origVersion
		BuildCommit = origCommit
		BuildDate = origDate
	})

	SetBuildInfo("v2.0.1", "def456", "2026-02-10T00:00:00Z")
	stdout, _ := captureOutput(t, func() {
		if code := Run([]string{"version"}); code != 0 {
			t.Fatalf("expected version command to succeed, got %d", code)
		}
	})
	if !strings.Contains(stdout, "lisa v2.0.1 (commit def456, built 2026-02-10T00:00:00Z)") {
		t.Fatalf("unexpected version output: %q", stdout)
	}
}

func TestDoctorJSONPayloadUsesBuildInfo(t *testing.T) {
	origVersion := BuildVersion
	origCommit := BuildCommit
	origDate := BuildDate
	t.Cleanup(func() {
		BuildVersion = origVersion
		BuildCommit = origCommit
		BuildDate = origDate
	})

	SetBuildInfo("v9.9.9", "abc123", "2026-02-10T00:00:00Z")
	payload := doctorJSONPayload(true, nil)

	if got := payload["version"]; got != "v9.9.9" {
		t.Fatalf("expected version from build info, got %v", got)
	}
	if got := payload["commit"]; got != "abc123" {
		t.Fatalf("expected commit from build info, got %v", got)
	}
	if got := payload["date"]; got != "2026-02-10T00:00:00Z" {
		t.Fatalf("expected date from build info, got %v", got)
	}
}

func TestBuildAgentCommandRejectsInvalidAgentAndMode(t *testing.T) {
	if _, err := buildAgentCommand("typo", "exec", "hello", ""); err == nil {
		t.Fatalf("expected invalid agent to return error")
	}
	if _, err := buildAgentCommand("claude", "typo", "hello", ""); err == nil {
		t.Fatalf("expected invalid mode to return error")
	}
}

func TestSessionMetaPathAcceptsUnsafeSessionNames(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa/review/slash*name"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectRoot,
	}

	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("expected metadata save to succeed for unsafe session name: %v", err)
	}
	path := sessionMetaFile(projectRoot, session)
	if strings.Contains(path, "lisa/review/slash") {
		t.Fatalf("session metadata path must not embed raw session path segments: %s", path)
	}
	if !fileExists(path) {
		t.Fatalf("expected session metadata file to exist at %s", path)
	}
}

func TestCleanupSessionArtifactsDoesNotExpandSessionWildcards(t *testing.T) {
	projectRoot := t.TempDir()
	sentinelOne := filepath.Join(os.TempDir(), "lisa-cmd-regression-one-111.sh")
	sentinelTwo := filepath.Join(os.TempDir(), "lisa-cmd-regression-two-222.sh")
	t.Cleanup(func() {
		_ = os.Remove(sentinelOne)
		_ = os.Remove(sentinelTwo)
	})

	if err := os.WriteFile(sentinelOne, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("failed to create sentinel file: %v", err)
	}
	if err := os.WriteFile(sentinelTwo, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("failed to create sentinel file: %v", err)
	}

	if err := cleanupSessionArtifacts(projectRoot, "*"); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}

	if !fileExists(sentinelOne) || !fileExists(sentinelTwo) {
		t.Fatalf("wildcard session cleanup removed unrelated command files")
	}
}

func TestCleanupSessionArtifactsDoesNotRemoveCrossProjectHashArtifactsByDefault(t *testing.T) {
	base := t.TempDir()
	projectOne := filepath.Join(base, "one")
	projectTwo := filepath.Join(base, "two")
	if err := os.MkdirAll(projectOne, 0o755); err != nil {
		t.Fatalf("failed to create projectOne: %v", err)
	}
	if err := os.MkdirAll(projectTwo, 0o755); err != nil {
		t.Fatalf("failed to create projectTwo: %v", err)
	}

	session := "lisa-cross-project-cleanup"
	metaPath := sessionMetaFile(projectTwo, session)
	statePath := sessionStateFile(projectTwo, session)
	outputPath := sessionOutputFile(projectTwo, session)
	heartbeatPath := sessionHeartbeatFile(projectTwo, session)
	donePath := sessionDoneFile(projectTwo, session)
	scriptPath := sessionCommandScriptPath(projectTwo, session, time.Now().UnixNano())

	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectTwo,
	}
	if err := saveSessionMeta(projectTwo, session, meta); err != nil {
		t.Fatalf("failed to save metadata: %v", err)
	}
	if err := saveSessionState(statePath, sessionState{PollCount: 2}); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("output"), 0o600); err != nil {
		t.Fatalf("failed to save output: %v", err)
	}
	if err := os.WriteFile(heartbeatPath, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o600); err != nil {
		t.Fatalf("failed to save heartbeat: %v", err)
	}
	if err := os.WriteFile(donePath, []byte("run-1:0\n"), 0o600); err != nil {
		t.Fatalf("failed to save done marker: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	if err := cleanupSessionArtifacts(projectOne, session); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}

	if !fileExists(metaPath) || !fileExists(statePath) || !fileExists(outputPath) || !fileExists(heartbeatPath) || !fileExists(donePath) {
		t.Fatalf("expected artifacts for other project hash to remain by default")
	}
	if !fileExists(scriptPath) {
		t.Fatalf("expected command script artifact for another project hash to remain by default")
	}
}

func TestCleanupSessionArtifactsRemovesCrossProjectHashArtifactsWhenEnabled(t *testing.T) {
	base := t.TempDir()
	projectOne := filepath.Join(base, "one")
	projectTwo := filepath.Join(base, "two")
	if err := os.MkdirAll(projectOne, 0o755); err != nil {
		t.Fatalf("failed to create projectOne: %v", err)
	}
	if err := os.MkdirAll(projectTwo, 0o755); err != nil {
		t.Fatalf("failed to create projectTwo: %v", err)
	}

	origEnv := os.Getenv("LISA_CLEANUP_ALL_HASHES")
	if err := os.Setenv("LISA_CLEANUP_ALL_HASHES", "true"); err != nil {
		t.Fatalf("failed to set LISA_CLEANUP_ALL_HASHES: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("LISA_CLEANUP_ALL_HASHES", origEnv)
	})

	session := "lisa-cross-project-cleanup-enabled"
	metaPath := sessionMetaFile(projectTwo, session)
	statePath := sessionStateFile(projectTwo, session)
	outputPath := sessionOutputFile(projectTwo, session)
	heartbeatPath := sessionHeartbeatFile(projectTwo, session)
	donePath := sessionDoneFile(projectTwo, session)
	scriptPath := sessionCommandScriptPath(projectTwo, session, time.Now().UnixNano())

	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectTwo,
	}
	if err := saveSessionMeta(projectTwo, session, meta); err != nil {
		t.Fatalf("failed to save metadata: %v", err)
	}
	if err := saveSessionState(statePath, sessionState{PollCount: 2}); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("output"), 0o600); err != nil {
		t.Fatalf("failed to save output: %v", err)
	}
	if err := os.WriteFile(heartbeatPath, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o600); err != nil {
		t.Fatalf("failed to save heartbeat: %v", err)
	}
	if err := os.WriteFile(donePath, []byte("run-1:0\n"), 0o600); err != nil {
		t.Fatalf("failed to save done marker: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	if err := cleanupSessionArtifacts(projectOne, session); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}

	if fileExists(metaPath) || fileExists(statePath) || fileExists(outputPath) || fileExists(heartbeatPath) || fileExists(donePath) || fileExists(scriptPath) {
		t.Fatalf("expected all artifacts to be removed across project hashes when cleanup-all-hashes is enabled")
	}
}

func TestProjectHashCanonicalizesEquivalentRoots(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("failed to chdir into temp dir: %v", err)
	}
	absTmp, err := filepath.Abs(tmp)
	if err != nil {
		t.Fatalf("failed to resolve abs path: %v", err)
	}

	hashDot := projectHash(".")
	hashAbs := projectHash(absTmp)
	if hashDot != hashAbs {
		t.Fatalf("expected canonical hash match for equivalent roots; dot=%s abs=%s", hashDot, hashAbs)
	}
}

func TestSessionArtifactsAreNotWorldReadable(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-secure-session"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectRoot,
		StartCmd:    "echo test",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to save metadata: %v", err)
	}
	statePath := sessionStateFile(projectRoot, session)
	if err := saveSessionState(statePath, sessionState{PollCount: 1}); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	metaStat, err := os.Stat(sessionMetaFile(projectRoot, session))
	if err != nil {
		t.Fatalf("failed to stat metadata file: %v", err)
	}
	stateStat, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("failed to stat state file: %v", err)
	}

	if metaStat.Mode().Perm()&0o077 != 0 {
		t.Fatalf("metadata file should not be group/world-readable: perm=%#o", metaStat.Mode().Perm())
	}
	if stateStat.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file should not be group/world-readable: perm=%#o", stateStat.Mode().Perm())
	}
}

func TestCmdSessionSpawnRejectsInvalidWidth(t *testing.T) {
	if code := cmdSessionSpawn([]string{"--width", "oops"}); code == 0 {
		t.Fatalf("expected non-zero exit for invalid width")
	}
}

func TestCmdSessionSpawnRejectsInvalidHeight(t *testing.T) {
	if code := cmdSessionSpawn([]string{"--height", "oops"}); code == 0 {
		t.Fatalf("expected non-zero exit for invalid height")
	}
}

func TestCmdSessionSpawnRejectsCustomSessionWithoutLisaPrefix(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	newCalled := false
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		newCalled = true
		return nil
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--session", "custom-name",
			"--command", "echo hello",
		}); code == 0 {
			t.Fatalf("expected non-zero exit for invalid custom session name")
		}
	})

	if newCalled {
		t.Fatalf("did not expect tmux session creation when session naming validation fails")
	}
	if !strings.Contains(stderr, `invalid --session: must start with "lisa-"`) {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSessionSpawnExecRequiresPromptOrCommand(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	newCalled := false
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		newCalled = true
		return nil
	}

	if code := cmdSessionSpawn([]string{"--mode", "exec"}); code == 0 {
		t.Fatalf("expected non-zero exit for exec mode without prompt/command")
	}
	if newCalled {
		t.Fatalf("did not expect tmux session creation when argument validation fails")
	}
}

func TestCmdSessionSpawnJSONOutputAndExecWrapping(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
	})

	tmuxHasSessionFn = func(session string) bool { return false }

	projectRoot := t.TempDir()
	sentCommand := ""
	tmuxNewSessionWithStartupFn = func(session, root, agent, mode string, width, height int, startupCommand string) error {
		if root != canonicalProjectRoot(projectRoot) {
			t.Fatalf("expected canonical project root to propagate to startup launcher; got %q want %q", root, canonicalProjectRoot(projectRoot))
		}
		sentCommand = startupCommand
		return nil
	}

	session := "lisa-spawn-json"
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--session", session,
			"--project-root", projectRoot,
			"--prompt", "hello world",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected spawn success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse spawn json: %v (%q)", err, stdout)
	}
	if payload["session"] != session {
		t.Fatalf("expected session %q, got %v", session, payload["session"])
	}
	commandText, _ := payload["command"].(string)
	if !strings.Contains(commandText, "codex exec 'hello world' --full-auto") {
		t.Fatalf("unexpected command payload: %q", commandText)
	}
	if !strings.Contains(sentCommand, execDonePrefix) {
		t.Fatalf("expected wrapped exec marker in startup command, got %q", sentCommand)
	}
	if !strings.Contains(sentCommand, sessionStartPrefix) || !strings.Contains(sentCommand, sessionDonePrefix) {
		t.Fatalf("expected wrapped session markers in startup command, got %q", sentCommand)
	}
}

func TestCmdSessionSpawnJSONOutputEscapesMultilinePromptCommand(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, root, agent, mode string, width, height int, startupCommand string) error {
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--session", "lisa-spawn-json-multiline",
			"--project-root", t.TempDir(),
			"--prompt", "line1\nline2",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected spawn success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse spawn json: %v (%q)", err, stdout)
	}
	commandText, _ := payload["command"].(string)
	if strings.Contains(commandText, "\n") {
		t.Fatalf("expected command payload to avoid literal newlines, got %q", commandText)
	}
	if !strings.Contains(commandText, "$'line1\\nline2'") {
		t.Fatalf("expected multiline prompt to use ANSI-C shell quoting, got %q", commandText)
	}
	if strings.Contains(stdout, "line1\nline2") {
		t.Fatalf("expected JSON output to avoid raw prompt newlines, got %q", stdout)
	}
}

func TestCmdSessionSpawnSendFailureCleansArtifacts(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
		tmuxKillSessionFn = origKill
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		return errors.New("send failed")
	}
	killCalled := false
	tmuxKillSessionFn = func(session string) error {
		killCalled = true
		return nil
	}

	projectRoot := t.TempDir()
	session := "lisa-send-failure-cleanup"

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--command", "echo hello",
		}); code == 0 {
			t.Fatalf("expected spawn to fail when startup command send fails")
		}
	})

	if killCalled {
		t.Fatalf("did not expect startup creation failure to kill tmux session")
	}
	if !strings.Contains(stderr, "failed to create tmux session") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSessionSpawnCodexExecSocketPermissionHint(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		return errors.New("error creating /tmp/lisa-codex.sock (Operation not permitted)")
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--session", "lisa-codex-exec-perm",
			"--project-root", t.TempDir(),
			"--prompt", "spawn nested session",
		}); code == 0 {
			t.Fatalf("expected spawn failure")
		}
	})
	if !strings.Contains(stderr, "failed to create tmux session") {
		t.Fatalf("expected spawn failure message, got %q", stderr)
	}
	if !strings.Contains(stderr, "codex exec --full-auto sandbox blocks tmux sockets") {
		t.Fatalf("expected nested tmux hint, got %q", stderr)
	}
}

func TestCmdSessionSpawnNewSessionFailureCleansHeartbeatArtifact(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		return errors.New("tmux create failed")
	}

	projectRoot := t.TempDir()
	session := "lisa-new-session-failure-cleanup"
	heartbeatPath := sessionHeartbeatFile(projectRoot, session)
	if fileExists(heartbeatPath) {
		_ = os.Remove(heartbeatPath)
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--command", "echo hello",
		}); code == 0 {
			t.Fatalf("expected spawn to fail when tmux session creation fails")
		}
	})

	if !strings.Contains(stderr, "failed to create tmux session") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if fileExists(heartbeatPath) {
		t.Fatalf("expected heartbeat artifact cleanup after tmux session creation failure")
	}
}

func TestCmdSessionSpawnMetaFailureKillsSession(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	origKill := tmuxKillSessionFn
	origSaveMeta := saveSessionMetaFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
		tmuxKillSessionFn = origKill
		saveSessionMetaFn = origSaveMeta
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		return nil
	}
	killCalled := false
	tmuxKillSessionFn = func(session string) error {
		killCalled = true
		return nil
	}
	saveSessionMetaFn = func(projectRoot, session string, meta sessionMeta) error {
		return errors.New("forced metadata failure")
	}

	projectRoot := t.TempDir()
	session := "lisa-meta-failure"

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--command", "echo hello",
		}); code == 0 {
			t.Fatalf("expected spawn to fail when metadata persistence fails")
		}
	})

	if !killCalled {
		t.Fatalf("expected spawn metadata failure to kill tmux session")
	}
	if !strings.Contains(stderr, "failed to persist metadata") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSessionSendUsesMockableTextPath(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendText := tmuxSendTextFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendTextFn = origSendText
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	sendCalled := false
	tmuxSendTextFn = func(session, text string, enter bool) error {
		sendCalled = true
		if session != "lisa-send" || text != "hello,world" || !enter {
			t.Fatalf("unexpected send args session=%q text=%q enter=%v", session, text, enter)
		}
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionSend([]string{
			"--session", "lisa-send",
			"--text", "hello,world",
			"--enter",
		}); code != 0 {
			t.Fatalf("expected send success, got %d", code)
		}
	})

	if !sendCalled {
		t.Fatalf("expected tmuxSendTextFn to be called")
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if stdout != "ok" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func TestCmdSessionCaptureUsesMockableFns(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	receivedLines := 0
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		receivedLines = lines
		if session != "lisa-capture" {
			t.Fatalf("unexpected session: %q", session)
		}
		return "line1\nline2", nil
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionCapture([]string{
			"--session", "lisa-capture",
			"--lines", "123",
			"--raw",
			"--json",
		}); code != 0 {
			t.Fatalf("expected capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if receivedLines != 123 {
		t.Fatalf("expected capture lines=123, got %d", receivedLines)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse capture json: %v (%q)", err, stdout)
	}
	if payload["capture"] != "line1\nline2" {
		t.Fatalf("unexpected capture payload: %v", payload["capture"])
	}
}

func TestCmdSessionListUsesMockableFn(t *testing.T) {
	origList := tmuxListSessionsFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
	})

	called := false
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		called = true
		if !projectOnly {
			t.Fatalf("expected project-only flag")
		}
		return []string{"lisa-a", "lisa-b"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionList([]string{"--project-only", "--project-root", t.TempDir()}); code != 0 {
			t.Fatalf("expected list success, got %d", code)
		}
	})
	if !called {
		t.Fatalf("expected tmuxListSessionsFn to be called")
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if stdout != "lisa-a\nlisa-b" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func TestCmdSessionListReturnsErrorWhenListingFails(t *testing.T) {
	origList := tmuxListSessionsFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
	})

	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return nil, errors.New("tmux list failed")
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionList([]string{"--project-only", "--project-root", t.TempDir()}); code == 0 {
			t.Fatalf("expected list failure")
		}
	})
	if !strings.Contains(stderr, "failed to list sessions") {
		t.Fatalf("expected listing failure stderr, got %q", stderr)
	}
}

func TestTmuxListSessionsTreatsNoServerAsEmpty(t *testing.T) {
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`if [ "$1" = "-S" ]; then shift 2; fi`,
		`if [ "$1" = "list-sessions" ]; then`,
		`  echo "no server running on /tmp/tmux-501/default" >&2`,
		"  exit 1",
		"fi",
		"exit 0",
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

	sessions, err := tmuxListSessions(false, t.TempDir())
	if err != nil {
		t.Fatalf("expected no-server tmux output to be handled as empty list, got error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected zero sessions, got %v", sessions)
	}
}

func TestTmuxListSessionsReturnsErrorForUnexpectedTmuxFailure(t *testing.T) {
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`if [ "$1" = "-S" ]; then shift 2; fi`,
		`if [ "$1" = "list-sessions" ]; then`,
		`  echo "permission denied" >&2`,
		"  exit 1",
		"fi",
		"exit 0",
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

	if _, err := tmuxListSessions(false, t.TempDir()); err == nil {
		t.Fatalf("expected tmux list-sessions to return error for unexpected failures")
	}
}

func TestCmdSessionExistsUsesMockableFn(t *testing.T) {
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
	})

	tmuxHasSessionFn = func(session string) bool {
		return session == "lisa-present"
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionExists([]string{"--session", "lisa-present"}); code != 0 {
			t.Fatalf("expected exists success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if stdout != "true" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}

	stdout, _ = captureOutput(t, func() {
		if code := cmdSessionExists([]string{"--session", "lisa-missing"}); code == 0 {
			t.Fatalf("expected missing session to return non-zero")
		}
	})
	if stdout != "false" {
		t.Fatalf("unexpected stdout for missing session: %q", stdout)
	}
}

func TestCmdSessionStatusEscapesCSVFields(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "working on api,tests", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
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
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 1234, 1.5, nil
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionStatus([]string{"--session", "lisa-status", "--project-root", t.TempDir()}); code != 0 {
			t.Fatalf("expected status success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	record, err := csv.NewReader(strings.NewReader(stdout)).Read()
	if err != nil {
		t.Fatalf("failed to parse csv output: %v (%q)", err, stdout)
	}
	if len(record) != 6 {
		t.Fatalf("expected 6 csv fields, got %d (%q)", len(record), stdout)
	}
	if record[3] != "Claude running" {
		t.Fatalf("expected process-based active task label, got %q", record[3])
	}
}

func TestCmdSessionKillReturnsErrorWhenSessionNotFound(t *testing.T) {
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
	})

	tmuxHasSessionFn = func(session string) bool {
		return false
	}

	if code := cmdSessionKill([]string{"--session", "missing", "--project-root", t.TempDir()}); code == 0 {
		t.Fatalf("expected non-zero exit when session is missing")
	}
}

func TestCmdSessionKillCleansArtifactsWhenSessionMissing(t *testing.T) {
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
	})

	tmuxHasSessionFn = func(session string) bool {
		return false
	}

	projectRoot := t.TempDir()
	session := "lisa-missing-cleanup"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectRoot,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to write meta artifact: %v", err)
	}

	statePath := sessionStateFile(projectRoot, session)
	if err := saveSessionState(statePath, sessionState{PollCount: 1}); err != nil {
		t.Fatalf("failed to write state artifact: %v", err)
	}

	outputPath := sessionOutputFile(projectRoot, session)
	if err := os.WriteFile(outputPath, []byte("output"), 0o600); err != nil {
		t.Fatalf("failed to write output artifact: %v", err)
	}

	if code := cmdSessionKill([]string{"--session", session, "--project-root", projectRoot}); code == 0 {
		t.Fatalf("expected non-zero exit when session is missing")
	}
	if fileExists(sessionMetaFile(projectRoot, session)) || fileExists(statePath) || fileExists(outputPath) {
		t.Fatalf("expected stale artifacts to be cleaned when tmux session is missing")
	}
}

func TestCmdSessionMonitorRejectsInvalidStopOnWaitingValue(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		if code := cmdSessionMonitor([]string{"--stop-on-waiting", "maybe"}); code == 0 {
			t.Fatalf("expected non-zero exit for invalid stop-on-waiting value")
		}
	})
	if !strings.Contains(stderr, "invalid --stop-on-waiting") {
		t.Fatalf("expected invalid value error, got %q", stderr)
	}
}

func TestCmdSessionMonitorRejectsNonLiteralBooleanStopOnWaiting(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		if code := cmdSessionMonitor([]string{"--stop-on-waiting", "1"}); code == 0 {
			t.Fatalf("expected non-zero exit for non-literal stop-on-waiting value")
		}
	})
	if !strings.Contains(stderr, "invalid --stop-on-waiting") {
		t.Fatalf("expected invalid value error, got %q", stderr)
	}
}

func TestCmdSessionMonitorRetriesDegradedUntilTimeout(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origPaneStatus := tmuxPaneStatusFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origLock := withStateFileLockFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxPaneStatusFn = origPaneStatus
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		withStateFileLockFn = origLock
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "working", nil }
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 80, 1.0, nil }
	withStateFileLockFn = func(statePath string, fn func() error) (stateLockMeta, error) {
		return stateLockMeta{WaitMS: 77}, &stateLockTimeoutError{WaitMS: 77}
	}

	stdout, _ := captureOutput(t, func() {
		if code := cmdSessionMonitor([]string{"--session", "lisa-monitor-degraded", "--max-polls", "1", "--poll-interval", "1", "--json"}); code != 2 {
			t.Fatalf("expected exit code 2 for monitor timeout, got %d", code)
		}
	})
	if !strings.Contains(stdout, `"exitReason":"degraded_max_polls_exceeded"`) {
		t.Fatalf("expected degraded timeout exit reason, got %q", stdout)
	}
	if !strings.Contains(stdout, `"finalState":"timeout"`) {
		t.Fatalf("expected timeout final state, got %q", stdout)
	}
}

func TestCmdSessionKillAttemptsTmuxKillBeforeCleanup(t *testing.T) {
	origHas := tmuxHasSessionFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxKillSessionFn = origKill
	})

	tmuxHasSessionFn = func(session string) bool {
		return true
	}

	killCalled := false
	tmuxKillSessionFn = func(session string) error {
		killCalled = true
		return nil
	}

	projectRoot := t.TempDir()
	session := "lisa-kill-cleanup-order"
	metaPath := sessionMetaFile(projectRoot, session)
	if err := os.MkdirAll(metaPath, 0o700); err != nil {
		t.Fatalf("failed to create blocking meta directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaPath, "block"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(metaPath)
	})

	if code := cmdSessionKill([]string{"--session", session, "--project-root", projectRoot}); code == 0 {
		t.Fatalf("expected non-zero exit when cleanup fails")
	}
	if !killCalled {
		t.Fatalf("expected tmux kill to be attempted before cleanup")
	}
}

func TestCmdSessionKillReturnsErrorWhenTmuxKillFails(t *testing.T) {
	origHas := tmuxHasSessionFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxKillSessionFn = origKill
	})

	tmuxHasSessionFn = func(session string) bool {
		return true
	}
	tmuxKillSessionFn = func(session string) error {
		return errors.New("kill failed")
	}

	if code := cmdSessionKill([]string{"--session", "lisa-test", "--project-root", t.TempDir()}); code == 0 {
		t.Fatalf("expected non-zero exit when tmux kill fails")
	}
}

func TestCmdSessionKillStillCleansArtifactsWhenTmuxKillFails(t *testing.T) {
	origHas := tmuxHasSessionFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxKillSessionFn = origKill
	})

	projectRoot := t.TempDir()
	session := "lisa-kill-fail-cleanup"
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxKillSessionFn = func(session string) error {
		return errors.New("kill failed")
	}

	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectRoot,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to write meta artifact: %v", err)
	}

	statePath := sessionStateFile(projectRoot, session)
	if err := saveSessionState(statePath, sessionState{PollCount: 1}); err != nil {
		t.Fatalf("failed to write state artifact: %v", err)
	}

	if code := cmdSessionKill([]string{"--session", session, "--project-root", projectRoot}); code == 0 {
		t.Fatalf("expected non-zero exit when tmux kill fails")
	}
	if fileExists(sessionMetaFile(projectRoot, session)) || fileExists(statePath) {
		t.Fatalf("expected cleanup to run even when tmux kill fails")
	}
}

func TestCmdSessionKillAllReturnsErrorOnPartialFailure(t *testing.T) {
	origList := tmuxListSessionsFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		tmuxKillSessionFn = origKill
	})

	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-a", "lisa-b"}, nil
	}
	tmuxKillSessionFn = func(session string) error {
		if session == "lisa-b" {
			return errors.New("kill failed")
		}
		return nil
	}

	if code := cmdSessionKillAll([]string{"--project-root", t.TempDir()}); code == 0 {
		t.Fatalf("expected non-zero exit when any session kill fails")
	}
}

func TestCmdSessionKillAllStillCleansArtifactsWhenKillFails(t *testing.T) {
	origList := tmuxListSessionsFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		tmuxKillSessionFn = origKill
	})

	projectRoot := t.TempDir()
	session := "lisa-killall-fail-cleanup"
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{session}, nil
	}
	tmuxKillSessionFn = func(session string) error {
		return errors.New("kill failed")
	}

	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectRoot,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to write meta artifact: %v", err)
	}

	if code := cmdSessionKillAll([]string{"--project-root", projectRoot}); code == 0 {
		t.Fatalf("expected non-zero exit when tmux kill fails")
	}
	if fileExists(sessionMetaFile(projectRoot, session)) {
		t.Fatalf("expected cleanup to run for kill-all even when tmux kill fails")
	}
}

func TestCmdSessionKillAllAttemptsKillBeforeCleanup(t *testing.T) {
	origList := tmuxListSessionsFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		tmuxKillSessionFn = origKill
	})

	sessionA := "lisa-a"
	sessionB := "lisa-b"
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{sessionA, sessionB}, nil
	}

	killCalls := map[string]int{}
	tmuxKillSessionFn = func(session string) error {
		killCalls[session]++
		return nil
	}

	projectRoot := t.TempDir()
	metaPath := sessionMetaFile(projectRoot, sessionB)
	if err := os.MkdirAll(metaPath, 0o700); err != nil {
		t.Fatalf("failed to create blocking meta directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaPath, "block"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(metaPath)
	})

	if code := cmdSessionKillAll([]string{"--project-root", projectRoot}); code == 0 {
		t.Fatalf("expected non-zero exit when cleanup fails for any session")
	}
	if killCalls[sessionA] != 1 || killCalls[sessionB] != 1 {
		t.Fatalf("expected tmux kill attempts for all sessions; calls=%v", killCalls)
	}
}

func TestComputeSessionStatusDegradesOnTmuxReadFailures(t *testing.T) {
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

	tmuxHasSessionFn = func(session string) bool {
		return true
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\t123", nil
		default:
			return "", nil
		}
	}
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "", errors.New("capture failed")
	}
	status, err := computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected process-first payload when pane capture fails, got error: %v", err)
	}
	if status.SessionState != "just_started" || status.ClassificationReason != "grace_period_just_started" {
		t.Fatalf("expected pane capture failure to not affect process-based classification, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}

	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "output", nil
	}
	tmuxDisplayFn = func(session, format string) (string, error) {
		return "", errors.New("combined snapshot failed")
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "", errors.New("pane status failed")
	}
	status, err = computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected degraded payload when pane snapshot read fails, got error: %v", err)
	}
	if status.SessionState != "degraded" || status.ClassificationReason != "tmux_snapshot_error" {
		t.Fatalf("expected tmux snapshot failure to degrade, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if !strings.Contains(status.Signals.TMUXReadError, "pane status failed") {
		t.Fatalf("expected tmux snapshot error signal, got %q", status.Signals.TMUXReadError)
	}

	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\tnot-a-number", nil
		case "#{pane_current_command}":
			return "zsh", nil
		case "#{pane_pid}":
			return "not-a-number", nil
		default:
			return "", nil
		}
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}
	status, err = computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected degraded payload when pane pid parse fails, got error: %v", err)
	}
	if status.SessionState != "degraded" || status.ClassificationReason != "tmux_pane_pid_parse_error" {
		t.Fatalf("expected pane pid parse failure to degrade, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if !strings.Contains(status.Signals.TMUXReadError, "not-a-number") {
		t.Fatalf("expected pane pid parse error signal, got %q", status.Signals.TMUXReadError)
	}
}

func TestComputeSessionStatusUsesPaneTerminalStateWhenCaptureFails(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "1\t0\tzsh\t123", nil
		default:
			return "", nil
		}
	}
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "", errors.New("capture failed")
	}

	status, err := computeSessionStatus("lisa-pane-terminal", t.TempDir(), "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected terminal payload when capture fails on exited pane, got error: %v", err)
	}
	if status.SessionState != "completed" || status.ClassificationReason != "pane_exited_zero" {
		t.Fatalf("expected completed pane exit classification, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
}

func TestComputeSessionStatusReadErrorEventLoggingSkipsZeroPoll(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "", errors.New("capture failed")
	}
	tmuxDisplayFn = func(session, format string) (string, error) {
		return "", errors.New("display failed")
	}

	appendCalls := 0
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		appendCalls++
		return nil
	}

	if _, err := computeSessionStatus("lisa-read-error-no-poll", t.TempDir(), "auto", "auto", false, 0); err != nil {
		t.Fatalf("expected degraded status payload, got %v", err)
	}
	if appendCalls != 0 {
		t.Fatalf("expected no immediate read-error event when pollCount=0, got %d", appendCalls)
	}

	if _, err := computeSessionStatus("lisa-read-error-with-poll", t.TempDir(), "auto", "auto", false, 1); err != nil {
		t.Fatalf("expected degraded status payload, got %v", err)
	}
	if appendCalls != 1 {
		t.Fatalf("expected one immediate read-error event when pollCount>0, got %d", appendCalls)
	}
}

func TestComputeSessionStatusDetectsPreCompletedExecOnFirstPoll(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "running command\nsome output\n__LISA_EXEC_DONE__:0\nuser@host:~/repo$", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
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
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		if key == "LISA_MODE" {
			return "exec", nil
		}
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-pre-completed-exec"
	if err := os.WriteFile(sessionDoneFile(projectRoot, session), []byte("run-pre:0\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	// First poll with no prior state — process-first completion relies on done sidecar.
	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "completed" {
		t.Fatalf("expected pre-completed exec session to be detected from done file, got state=%s status=%s", status.SessionState, status.Status)
	}
}

func TestComputeSessionStatusDetectsExecCompletionWithoutModeHint(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "", nil }
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "bash", nil
		case "#{pane_pid}":
			return "100", nil
		default:
			return "", nil
		}
	}
	// Simulate mode resolution failing — env lookup returns error and no meta
	// exists, so mode defaults to "interactive".
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-exec-no-mode-hint"
	if err := os.WriteFile(sessionDoneFile(projectRoot, session), []byte("run-nohint:0\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "completed" {
		t.Fatalf("expected completion to be detected from done file even when mode resolves to interactive, got state=%s", status.SessionState)
	}
}

func TestFilterInputBoxStripsClaudeCLIChrome(t *testing.T) {
	input := strings.Join([]string{
		"some output",
		"10",
		"─────────────────────────────",
		"❯",
		"─────────────────────────────",
		"  lisa | ctx(26%) | 07:03:12",
		"  -- INSERT -- ⏵⏵ don't ask on (shift+tab to cycle)",
	}, "\n")
	filtered := filterInputBox(input)
	lines := strings.Split(filtered, "\n")
	var nonEmpty []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, strings.TrimSpace(l))
		}
	}
	if len(nonEmpty) != 3 {
		t.Fatalf("expected 3 non-empty lines after filtering, got %d: %v", len(nonEmpty), nonEmpty)
	}
	if nonEmpty[2] != "❯" {
		t.Fatalf("expected ❯ as last non-empty line, got %q", nonEmpty[2])
	}
}

func TestPromptWaitingNotTriggeredWhenProcessBusy(t *testing.T) {
	// When agent process is busy (high CPU), prompt (❯) should NOT classify
	// as waiting_input — the prompt is always visible in Claude's split UI
	// even during long-running commands.
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "some output\n10\n❯", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "node", nil
		case "#{pane_pid}":
			return "123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		switch key {
		case "LISA_AGENT":
			return "claude", nil
		case "LISA_MODE":
			return "interactive", nil
		default:
			return "", errors.New("missing")
		}
	}
	// Agent detected with HIGH CPU (>= 0.2) — process is busy
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 999, 0.5, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-prompt-busy-cpu"

	status, err := computeSessionStatus(session, projectRoot, "claude", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "in_progress" {
		t.Fatalf("expected in_progress when process is busy despite visible prompt, got state=%s reason=%s",
			status.SessionState, status.ClassificationReason)
	}
}

func TestPromptWaitingDuringLongBashCommand(t *testing.T) {
	// Regression: Claude's split UI shows ❯ prompt at the bottom even while
	// a long bash command (e.g. sleep 3 x20) is running. With activeProcessBusy=true,
	// the session should remain in_progress, not be misclassified as waiting_input.
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		// Simulates Claude running a long bash command — output with ❯ at bottom
		return "⏺ Running bash command: sleep 3\n\nCounting: 5 of 20\n\n❯", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "node", nil
		case "#{pane_pid}":
			return "456", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		switch key {
		case "LISA_AGENT":
			return "claude", nil
		case "LISA_MODE":
			return "interactive", nil
		default:
			return "", errors.New("missing")
		}
	}
	// Agent is actively running — high CPU from bash command execution
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 789, 0.8, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-long-bash-cmd"

	status, err := computeSessionStatus(session, projectRoot, "claude", "interactive", false, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "in_progress" {
		t.Fatalf("expected in_progress during long bash command, got state=%s reason=%s",
			status.SessionState, status.ClassificationReason)
	}
}

func TestInteractiveSessionDetectsWaitingInput(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "idle", nil }
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "node", nil
		case "#{pane_pid}":
			return "456", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		switch key {
		case "LISA_AGENT":
			return "claude", nil
		case "LISA_MODE":
			return "interactive", nil
		default:
			return "", errors.New("missing")
		}
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 999, 0.05, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-interactive-waiting"
	status, err := computeSessionStatus(session, projectRoot, "claude", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "waiting_input" {
		t.Fatalf("expected waiting_input, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if status.ClassificationReason != "interactive_idle_cpu" {
		t.Fatalf("expected interactive_idle_cpu reason, got %q", status.ClassificationReason)
	}
	if !status.Signals.InteractiveWaiting {
		t.Fatal("expected InteractiveWaiting=true")
	}
	if status.Signals.ActiveProcessBusy {
		t.Fatal("expected ActiveProcessBusy=false")
	}
}

func TestInteractiveShellIdleWithoutAgentProcessClassifiesWaitingInput(t *testing.T) {
	origHas := tmuxHasSessionFn
	origDisplay := tmuxDisplayFn
	origDetect := detectAgentProcessFn
	origInspect := inspectPaneProcessTreeFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxDisplayFn = origDisplay
		detectAgentProcessFn = origDetect
		inspectPaneProcessTreeFn = origInspect
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\t123", nil
		default:
			return "", nil
		}
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}
	inspectPaneProcessTreeFn = func(panePID int) (float64, bool, error) {
		return 0.01, false, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-shell-idle-waiting"
	if err := os.WriteFile(sessionHeartbeatFile(projectRoot, session), []byte("hb"), 0o600); err != nil {
		t.Fatalf("failed to seed heartbeat file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "codex", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "waiting_input" {
		t.Fatalf("expected waiting_input, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if status.ClassificationReason != "interactive_shell_idle" {
		t.Fatalf("expected interactive_shell_idle reason, got %q", status.ClassificationReason)
	}
	if !status.Signals.InteractiveWaiting {
		t.Fatal("expected InteractiveWaiting=true")
	}
}

func TestInteractiveShellChildProcessStaysInProgress(t *testing.T) {
	origHas := tmuxHasSessionFn
	origDisplay := tmuxDisplayFn
	origDetect := detectAgentProcessFn
	origInspect := inspectPaneProcessTreeFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxDisplayFn = origDisplay
		detectAgentProcessFn = origDetect
		inspectPaneProcessTreeFn = origInspect
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\t123", nil
		default:
			return "", nil
		}
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}
	inspectPaneProcessTreeFn = func(panePID int) (float64, bool, error) {
		return 0.01, true, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-shell-child-running"
	if err := os.WriteFile(sessionHeartbeatFile(projectRoot, session), []byte("hb"), 0o600); err != nil {
		t.Fatalf("failed to seed heartbeat file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "codex", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "in_progress" {
		t.Fatalf("expected in_progress, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if status.ClassificationReason != "interactive_child_process" {
		t.Fatalf("expected interactive_child_process reason, got %q", status.ClassificationReason)
	}
}

func TestInspectPaneProcessTreeIgnoresPassiveSleepChild(t *testing.T) {
	origListCached := listProcessesCachedFn
	t.Cleanup(func() {
		listProcessesCachedFn = origListCached
	})

	listProcessesCachedFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 100, PPID: 1, CPU: 0.04, Command: "/bin/bash /tmp/lisa-cmd.sh"},
			{PID: 101, PPID: 100, CPU: 0.00, Command: "sleep 2"},
		}, nil
	}

	paneCPU, hasNonShellDescendant, err := inspectPaneProcessTree(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasNonShellDescendant {
		t.Fatal("expected sleep helper child to be ignored")
	}
	if paneCPU <= 0 {
		t.Fatalf("expected pane CPU to be retained, got %.2f", paneCPU)
	}
}

func TestInteractiveSessionGracePeriodPreventsWaitingInput(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "idle", nil }
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "node", nil
		case "#{pane_pid}":
			return "456", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		switch key {
		case "LISA_AGENT":
			return "claude", nil
		case "LISA_MODE":
			return "interactive", nil
		default:
			return "", errors.New("missing")
		}
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 999, 0.05, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-interactive-grace"
	status, err := computeSessionStatus(session, projectRoot, "claude", "interactive", false, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "in_progress" {
		t.Fatalf("expected grace-period in_progress, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
}

func TestExecModeNeverClassifiesAsWaitingInput(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "idle", nil }
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "node", nil
		case "#{pane_pid}":
			return "456", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		switch key {
		case "LISA_AGENT":
			return "claude", nil
		case "LISA_MODE":
			return "exec", nil
		default:
			return "", errors.New("missing")
		}
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 999, 0.05, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-exec-never-waiting"
	status, err := computeSessionStatus(session, projectRoot, "claude", "exec", false, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "in_progress" {
		t.Fatalf("expected exec mode to stay in_progress, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
}

func TestBuildAgentCommandDefaultsToSkipPermissionsForClaude(t *testing.T) {
	// Regression: Claude agents spawned without --dangerously-skip-permissions
	// run in "don't ask" mode, causing Bash tool calls to be denied.
	// --dangerously-skip-permissions must be injected by default.

	cmd, err := buildAgentCommand("claude", "interactive", "hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Fatalf("expected --dangerously-skip-permissions in claude interactive command, got %q", cmd)
	}

	cmd, err = buildAgentCommand("claude", "exec", "hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Fatalf("expected --dangerously-skip-permissions in claude exec command, got %q", cmd)
	}
}

func TestBuildAgentCommandSkipPermissionsNotDuplicatedWhenAlreadyInAgentArgs(t *testing.T) {
	cmd, err := buildAgentCommand("claude", "interactive", "hello", "--dangerously-skip-permissions --model haiku")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(cmd, "--dangerously-skip-permissions") != 1 {
		t.Fatalf("expected exactly one --dangerously-skip-permissions, got %q", cmd)
	}
}

func TestBuildAgentCommandSkipPermissionsNotInjectedForCodex(t *testing.T) {
	cmd, err := buildAgentCommand("codex", "interactive", "hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Fatalf("codex should not get --dangerously-skip-permissions, got %q", cmd)
	}
}

func TestShellQuoteUsesANSICQuotingForControlChars(t *testing.T) {
	got := shellQuote("line1\nline2\tend")
	if got != "$'line1\\nline2\\tend'" {
		t.Fatalf("unexpected ANSI-C quoted string: %q", got)
	}

	plain := shellQuote("it's fine")
	if plain != "'it'\"'\"'s fine'" {
		t.Fatalf("expected plain single-quote escaping, got %q", plain)
	}
}

func TestBuildAgentCommandWithOptionsCanDisableSkipPermissions(t *testing.T) {
	cmd, err := buildAgentCommandWithOptions("claude", "interactive", "hello", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Fatalf("expected no --dangerously-skip-permissions when disabled, got %q", cmd)
	}
}

func TestComputeSessionStatusTranscriptTurnComplete(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origTranscript := checkTranscriptTurnCompleteFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		checkTranscriptTurnCompleteFn = origTranscript
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	capture := "working on stuff"
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return capture, nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tnode\t123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	// Agent alive but low CPU — not enough for interactiveWaiting because output is fresh
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 456, 0.05, nil }
	checkTranscriptTurnCompleteFn = func(projectRoot, prompt, createdAt, cachedSessionID string) (bool, int, string, error) {
		return true, 15, "mock-session-id", nil
	}

	projectRoot := t.TempDir()
	session := "lisa-transcript-test"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		Prompt:      "test prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	t.Cleanup(func() { os.Remove(sessionMetaFile(projectRoot, session)) })
	t.Cleanup(func() { os.Remove(sessionStateFile(projectRoot, session)) })

	status, err := computeSessionStatus(session, projectRoot, "claude", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "waiting_input" {
		t.Fatalf("expected waiting_input for idle interactive process, got %q", status.SessionState)
	}
	if status.ClassificationReason != "interactive_idle_cpu" {
		t.Fatalf("expected interactive_idle_cpu reason, got %q", status.ClassificationReason)
	}
	if !status.Signals.InteractiveWaiting {
		t.Fatal("expected InteractiveWaiting=true for idle interactive process")
	}
	if status.Signals.ActiveProcessBusy {
		t.Fatal("expected ActiveProcessBusy=false for low CPU")
	}
	if status.Signals.TranscriptTurnComplete {
		t.Fatal("expected TranscriptTurnComplete signal to remain false without transcript-based classification")
	}
	if status.Signals.TranscriptFileAge != 0 {
		t.Fatalf("expected TranscriptFileAge=0, got %d", status.Signals.TranscriptFileAge)
	}

	state, err := loadSessionStateWithError(sessionStateFile(projectRoot, session))
	if err != nil {
		t.Fatalf("expected session state to persist, got error: %v", err)
	}
	if state.ClaudeSessionID != "" {
		t.Fatalf("expected no cached ClaudeSessionID without transcript path, got %q", state.ClaudeSessionID)
	}
}

func TestComputeSessionStatusTranscriptPlusHighCPU(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origTranscript := checkTranscriptTurnCompleteFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		checkTranscriptTurnCompleteFn = origTranscript
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	capture := "working on stuff"
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return capture, nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tnode\t123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	// High CPU — normally would classify as agent_pid_alive
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 456, 5.0, nil }
	checkTranscriptTurnCompleteFn = func(projectRoot, prompt, createdAt, cachedSessionID string) (bool, int, string, error) {
		return true, 20, "mock-session-id", nil
	}

	projectRoot := t.TempDir()
	session := "lisa-transcript-cpu"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		Prompt:      "test prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	t.Cleanup(func() { os.Remove(sessionMetaFile(projectRoot, session)) })

	status, err := computeSessionStatus(session, projectRoot, "claude", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "in_progress" {
		t.Fatalf("expected in_progress despite transcript completion signals, got %q", status.SessionState)
	}
	if status.ClassificationReason != "agent_pid_alive" {
		t.Fatalf("expected agent_pid_alive, got %q", status.ClassificationReason)
	}
}

func TestComputeSessionStatusCodexTranscriptTurnComplete(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origCodexTranscript := checkCodexTranscriptTurnCompleteFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		checkCodexTranscriptTurnCompleteFn = origCodexTranscript
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	capture := "working on stuff"
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return capture, nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tnode\t123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 456, 0.05, nil }
	checkCodexTranscriptTurnCompleteFn = func(prompt, createdAt, cachedSessionID string) (bool, int, string, error) {
		return true, 12, "mock-codex-session-id", nil
	}

	projectRoot := t.TempDir()
	session := "lisa-codex-transcript-test"
	meta := sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		Prompt:      "test prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	t.Cleanup(func() { os.Remove(sessionMetaFile(projectRoot, session)) })
	t.Cleanup(func() { os.Remove(sessionStateFile(projectRoot, session)) })

	status, err := computeSessionStatus(session, projectRoot, "codex", "interactive", false, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "waiting_input" {
		t.Fatalf("expected waiting_input for idle interactive process, got %q", status.SessionState)
	}
	if status.ClassificationReason != "interactive_idle_cpu" {
		t.Fatalf("expected interactive_idle_cpu reason, got %q", status.ClassificationReason)
	}
	if !status.Signals.InteractiveWaiting {
		t.Fatal("expected InteractiveWaiting=true for idle interactive process")
	}
	if status.Signals.ActiveProcessBusy {
		t.Fatal("expected ActiveProcessBusy=false for low CPU")
	}
	if status.Signals.TranscriptTurnComplete {
		t.Fatal("expected TranscriptTurnComplete signal to remain false without transcript-based classification")
	}
	if status.Signals.TranscriptFileAge != 0 {
		t.Fatalf("expected TranscriptFileAge=0, got %d", status.Signals.TranscriptFileAge)
	}

	state, err := loadSessionStateWithError(sessionStateFile(projectRoot, session))
	if err != nil {
		t.Fatalf("expected session state to persist, got error: %v", err)
	}
	if state.CodexSessionID != "" {
		t.Fatalf("expected no cached CodexSessionID without transcript path, got %q", state.CodexSessionID)
	}
}
