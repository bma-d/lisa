package app

import (
	"errors"
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

	pid, cpu := detectAgentProcess(1, "codex")
	if pid != 10 || cpu != 0.0 {
		t.Fatalf("expected codex PID with zero CPU to be selected, got pid=%d cpu=%f", pid, cpu)
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
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	if err := cleanupSessionArtifacts(projectOne, session); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}

	if fileExists(metaPath) || fileExists(statePath) || fileExists(outputPath) || fileExists(scriptPath) {
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

func TestComputeSessionStatusReturnsErrorOnCriticalTmuxReadFailures(t *testing.T) {
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
	if _, err := computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1); err == nil {
		t.Fatalf("expected error when pane capture fails")
	}

	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "output", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) {
		return "", errors.New("pane status failed")
	}
	if _, err := computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1); err == nil {
		t.Fatalf("expected error when pane status read fails")
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64) {
		return 0, 0
	}
	if _, err := computeSessionStatus("lisa-test", t.TempDir(), "auto", "auto", false, 1); err == nil {
		t.Fatalf("expected error when pane pid cannot be parsed")
	}
}
