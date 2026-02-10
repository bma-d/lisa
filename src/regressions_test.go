package app

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
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
		"trap '__lisa_ec=130; exit \"$__lisa_ec\"' INT TERM HUP",
		"echo hello",
	} {
		if !strings.Contains(wrapped, token) {
			t.Fatalf("expected wrapped session command to contain %q, got %q", token, wrapped)
		}
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

func TestLooksLikePromptWaitingIgnoresHistoricalMarkers(t *testing.T) {
	codexCapture := strings.Join([]string{
		"previous output",
		"tokens used: 1234",
		"running tests now",
	}, "\n")
	if looksLikePromptWaiting("codex", codexCapture) {
		t.Fatalf("expected codex historical marker to not force waiting state")
	}

	claudeCapture := strings.Join([]string{
		"previous output",
		"press enter to send",
		"writing fix for parser",
	}, "\n")
	if looksLikePromptWaiting("claude", claudeCapture) {
		t.Fatalf("expected claude historical marker to not force waiting state")
	}
}

func TestLooksLikePromptWaitingDetectsRecentPromptContext(t *testing.T) {
	codexCapture := strings.Join([]string{
		"tokens used: 4321",
		"❯ 12:34:56",
	}, "\n")
	if !looksLikePromptWaiting("codex", codexCapture) {
		t.Fatalf("expected codex prompt context to be detected as waiting")
	}

	claudeCapture := strings.Join([]string{
		"press enter to send",
		">",
	}, "\n")
	if !looksLikePromptWaiting("claude", claudeCapture) {
		t.Fatalf("expected claude prompt context to be detected as waiting")
	}
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

func TestComputeSessionStatusInteractiveUsesSessionDoneMarker(t *testing.T) {
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
		return "__LISA_SESSION_DONE__:0\nuser@host:~/repo$ ", nil
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
			return "interactive", nil
		}
		return "", errors.New("missing")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 0, 0, nil
	}

	status, err := computeSessionStatus("lisa-interactive-done", t.TempDir(), "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "completed" || status.Status != "idle" {
		t.Fatalf("expected session done marker to mark completion, got state=%s status=%s", status.SessionState, status.Status)
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

func TestCleanupSessionArtifactsRemovesCrossProjectHashArtifacts(t *testing.T) {
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
	scriptPath := sessionCommandScriptPath(session, time.Now().UnixNano())

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
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	if err := cleanupSessionArtifacts(projectOne, session); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}

	if fileExists(metaPath) || fileExists(statePath) || fileExists(outputPath) || fileExists(heartbeatPath) || fileExists(scriptPath) {
		t.Fatalf("expected all artifacts to be removed across project hashes")
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

func TestCmdSessionSpawnRejectsCustomSessionWithoutLisaPrefix(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	newCalled := false
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error {
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
	origNew := tmuxNewSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	newCalled := false
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error {
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
	origNew := tmuxNewSessionFn
	origSend := tmuxSendCommandWithFallbackFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
		tmuxSendCommandWithFallbackFn = origSend
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error { return nil }

	sentCommand := ""
	tmuxSendCommandWithFallbackFn = func(session, command string, enter bool) error {
		if !enter {
			t.Fatalf("expected startup command send to include enter")
		}
		sentCommand = command
		return nil
	}

	session := "lisa-spawn-json"
	projectRoot := t.TempDir()
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

func TestCmdSessionSpawnSendFailureCleansArtifacts(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionFn
	origSend := tmuxSendCommandWithFallbackFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
		tmuxSendCommandWithFallbackFn = origSend
		tmuxKillSessionFn = origKill
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error { return nil }
	tmuxSendCommandWithFallbackFn = func(session, command string, enter bool) error {
		return errors.New("send failed")
	}
	killCalled := false
	tmuxKillSessionFn = func(session string) error {
		killCalled = true
		return nil
	}

	projectRoot := t.TempDir()
	session := "lisa-send-failure-cleanup"
	scriptPath := sessionCommandScriptPath(session, time.Now().UnixNano())
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
		t.Fatalf("failed to create script artifact: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(scriptPath)
	})

	_, _ = captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--command", "echo hello",
		}); code == 0 {
			t.Fatalf("expected spawn to fail when startup command send fails")
		}
	})

	if !killCalled {
		t.Fatalf("expected failed spawn to kill tmux session")
	}
	if fileExists(scriptPath) {
		t.Fatalf("expected cleanup to remove command script after failed spawn")
	}
}

func TestCmdSessionSpawnMetaFailureKillsSession(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionFn
	origSend := tmuxSendCommandWithFallbackFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
		tmuxSendCommandWithFallbackFn = origSend
		tmuxKillSessionFn = origKill
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error { return nil }
	tmuxSendCommandWithFallbackFn = func(session, command string, enter bool) error { return nil }
	killCalled := false
	tmuxKillSessionFn = func(session string) error {
		killCalled = true
		return nil
	}

	projectRoot := t.TempDir()
	session := "lisa-meta-failure"
	metaPath := sessionMetaFile(projectRoot, session)
	tmpPath := filepath.Join(filepath.Dir(metaPath), "."+filepath.Base(metaPath)+".tmp")
	if err := os.Mkdir(tmpPath, 0o700); err != nil {
		t.Fatalf("failed to create tmp path blocker: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpPath)
	})

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

func TestTmuxListSessionsTreatsNoServerAsEmpty(t *testing.T) {
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
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
	if record[3] != "working on api,tests" {
		t.Fatalf("expected active task to preserve comma, got %q", record[3])
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

func TestCmdSessionMonitorTreatsDegradedAsTerminal(t *testing.T) {
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
			t.Fatalf("expected exit code 2 for degraded monitor stop, got %d", code)
		}
	})
	if !strings.Contains(stdout, `"exitReason":"degraded"`) {
		t.Fatalf("expected degraded exit reason, got %q", stdout)
	}
	if !strings.Contains(stdout, `"finalState":"degraded"`) {
		t.Fatalf("expected degraded final state, got %q", stdout)
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "", errors.New("capture failed")
	}
	status, err := computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected degraded payload when pane capture fails, got error: %v", err)
	}
	if status.SessionState != "degraded" || status.ClassificationReason != "tmux_capture_error" {
		t.Fatalf("expected tmux capture failure to degrade, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if !strings.Contains(status.Signals.TMUXReadError, "capture failed") {
		t.Fatalf("expected tmux capture error signal, got %q", status.Signals.TMUXReadError)
	}

	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "output", nil
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

	tmuxPaneStatusFn = func(session string) (string, error) {
		return "alive", nil
	}
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
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

func TestComputeSessionStatusReadErrorEventLoggingSkipsZeroPoll(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "", errors.New("capture failed")
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

	// First poll with no prior state — simulates monitoring a session that
	// already finished before polling started.
	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "completed" {
		t.Fatalf("expected pre-completed exec session to be detected as completed on first poll, got state=%s status=%s", status.SessionState, status.Status)
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "output\n__LISA_EXEC_DONE__:0\nuser@host:~/repo$", nil
	}
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

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionState != "completed" {
		t.Fatalf("expected exec completion marker to be detected even when mode resolves to interactive, got state=%s", status.SessionState)
	}
}
