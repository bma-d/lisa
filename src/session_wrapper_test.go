package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseSessionCompletionForRunMatchesRunID(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_SESSION_DONE__:run-1:0",
		"user@host:~/repo$",
	}, "\n")

	done, code, markerRunID, mismatch := parseSessionCompletionForRun(capture, "run-1")
	if !done || code != 0 || markerRunID != "run-1" || mismatch {
		t.Fatalf("expected matching marker to complete, got done=%v code=%d markerRunID=%q mismatch=%v", done, code, markerRunID, mismatch)
	}
}

func TestParseSessionCompletionForRunRejectsMismatchedRunID(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_SESSION_DONE__:run-2:0",
		"user@host:~/repo$",
	}, "\n")

	done, code, markerRunID, mismatch := parseSessionCompletionForRun(capture, "run-1")
	if done || !mismatch || markerRunID != "run-2" || code != 0 {
		t.Fatalf("expected run mismatch rejection, got done=%v code=%d markerRunID=%q mismatch=%v", done, code, markerRunID, mismatch)
	}
}

func TestParseSessionCompletionForRunAcceptsLegacyMarker(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"__LISA_SESSION_DONE__:0",
		"user@host:~/repo$",
	}, "\n")

	done, code, markerRunID, mismatch := parseSessionCompletionForRun(capture, "run-1")
	if !done || code != 0 || markerRunID != "" || mismatch {
		t.Fatalf("expected legacy marker acceptance, got done=%v code=%d markerRunID=%q mismatch=%v", done, code, markerRunID, mismatch)
	}
}

func TestParseSessionCompletionForRunHandlesANSIMarkers(t *testing.T) {
	capture := strings.Join([]string{
		"final output",
		"\x1b[32m__LISA_SESSION_DONE__:run-ansi:2\x1b[0m",
		"user@host:~/repo$",
	}, "\n")

	done, code, markerRunID, mismatch := parseSessionCompletionForRun(capture, "run-ansi")
	if !done || code != 2 || markerRunID != "run-ansi" || mismatch {
		t.Fatalf("expected ANSI marker parsing, got done=%v code=%d markerRunID=%q mismatch=%v", done, code, markerRunID, mismatch)
	}
}

func TestReadSessionDoneFileMatchesRunID(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-done-file"
	donePath := sessionDoneFile(projectRoot, session)
	if err := os.WriteFile(donePath, []byte("run-1:0\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	done, code, fileRunID, mismatch, err := readSessionDoneFile(projectRoot, session, "run-1")
	if err != nil {
		t.Fatalf("unexpected done file read error: %v", err)
	}
	if !done || mismatch || code != 0 || fileRunID != "run-1" {
		t.Fatalf("unexpected done-file parse result done=%v code=%d run=%q mismatch=%v", done, code, fileRunID, mismatch)
	}
}

func TestReadSessionDoneFileRejectsMismatchedRunID(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-done-file-mismatch"
	donePath := sessionDoneFile(projectRoot, session)
	if err := os.WriteFile(donePath, []byte("run-2:3\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	done, code, fileRunID, mismatch, err := readSessionDoneFile(projectRoot, session, "run-1")
	if err != nil {
		t.Fatalf("unexpected done file read error: %v", err)
	}
	if done || !mismatch || code != 3 || fileRunID != "run-2" {
		t.Fatalf("unexpected done-file mismatch result done=%v code=%d run=%q mismatch=%v", done, code, fileRunID, mismatch)
	}
}

func TestSessionHeartbeatAgeUsesMTime(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-heartbeat-mtime"
	now := time.Now()
	path := sessionHeartbeatFile(projectRoot, session)

	if _, ok := sessionHeartbeatAge(projectRoot, session, now.Unix()); ok {
		t.Fatalf("expected missing heartbeat file to return ok=false")
	}

	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("failed writing heartbeat file: %v", err)
	}
	wantAge := 15
	past := now.Add(-time.Duration(wantAge) * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("failed to set heartbeat mtime: %v", err)
	}

	age, ok := sessionHeartbeatAge(projectRoot, session, now.Unix())
	if !ok {
		t.Fatalf("expected heartbeat file to be detected")
	}
	if age < wantAge-1 || age > wantAge+1 {
		t.Fatalf("expected heartbeat age near %d, got %d", wantAge, age)
	}
}

func TestIsAgeFreshBoundaries(t *testing.T) {
	if isAgeFresh(-1, 3) {
		t.Fatalf("negative ages should be stale")
	}
	if !isAgeFresh(2, 3) {
		t.Fatalf("age below threshold should be fresh")
	}
	if !isAgeFresh(3, 3) {
		t.Fatalf("age equal to threshold should be fresh")
	}
	if isAgeFresh(4, 3) {
		t.Fatalf("age above threshold should be stale")
	}
}

func TestComputeSessionStatusHeartbeatStaleFallsBackToStuck(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origPaneStatus := tmuxPaneStatusFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origHBStale := os.Getenv("LISA_HEARTBEAT_STALE_SECONDS")
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxPaneStatusFn = origPaneStatus
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		_ = os.Setenv("LISA_HEARTBEAT_STALE_SECONDS", origHBStale)
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	capture := "no fresh output"
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
	if err := os.Setenv("LISA_HEARTBEAT_STALE_SECONDS", "3"); err != nil {
		t.Fatalf("failed to set heartbeat stale env: %v", err)
	}

	projectRoot := t.TempDir()
	session := "lisa-heartbeat-stale"
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastOutputHash: md5Hex8(capture),
		LastOutputAt:   time.Now().Add(-15 * time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("failed to seed stale output state: %v", err)
	}
	hbPath := sessionHeartbeatFile(projectRoot, session)
	if err := os.WriteFile(hbPath, []byte(""), 0o600); err != nil {
		t.Fatalf("failed to seed heartbeat file: %v", err)
	}
	past := time.Now().Add(-20 * time.Second)
	if err := os.Chtimes(hbPath, past, past); err != nil {
		t.Fatalf("failed to age heartbeat file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "stuck" || status.Status != "idle" {
		t.Fatalf("expected stale heartbeat session to be stuck, got state=%s status=%s", status.SessionState, status.Status)
	}
	if status.ClassificationReason != "stuck_no_signals" {
		t.Fatalf("expected stuck_no_signals reason, got %s", status.ClassificationReason)
	}
}

func TestComputeSessionStatusUsesDoneFileWhenCaptureHasNoMarker(t *testing.T) {
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
		return "output chunk\nstill output\nwithout done marker", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "zsh", nil
		case "#{pane_pid}":
			return "101", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("unset")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 0, 0, nil }

	projectRoot := t.TempDir()
	session := "lisa-donefile-complete"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		RunID:       "run-done",
		ProjectRoot: projectRoot,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	if err := os.WriteFile(sessionDoneFile(projectRoot, session), []byte("run-done:0\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", true, 4)
	if err != nil {
		t.Fatalf("expected status compute to succeed: %v", err)
	}
	if status.SessionState != "completed" || status.Status != "idle" {
		t.Fatalf("expected done-file completion, got state=%s status=%s", status.SessionState, status.Status)
	}
	if status.ClassificationReason != "done_file" {
		t.Fatalf("expected done_file classification, got %s", status.ClassificationReason)
	}
	if !status.Signals.DoneFileSeen || status.Signals.DoneFileExitCode != 0 {
		t.Fatalf("expected done-file signals, got %+v", status.Signals)
	}
}

func TestComputeSessionStatusMalformedDoneFileIsDegraded(t *testing.T) {
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
		return "idle output", nil
	}
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "zsh", nil
		case "#{pane_pid}":
			return "101", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("unset")
	}
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 0, 0, nil }

	projectRoot := t.TempDir()
	session := "lisa-donefile-malformed"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		RunID:       "run-bad",
		ProjectRoot: projectRoot,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	if err := os.WriteFile(sessionDoneFile(projectRoot, session), []byte("bad marker payload\n"), 0o600); err != nil {
		t.Fatalf("failed to write malformed done file: %v", err)
	}
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastOutputHash: md5Hex8("idle output"),
		LastOutputAt:   time.Now().Add(-10 * time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("failed to save stale state: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status compute to succeed: %v", err)
	}
	if status.SessionState != "degraded" || status.ClassificationReason != "done_file_read_error" {
		t.Fatalf("expected malformed done file to degrade classification, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if status.Signals.DoneFileReadError == "" {
		t.Fatalf("expected done-file read error signal")
	}
}

func TestComputeSessionStatusSessionDoneNonZeroIsCrashedWithRunID(t *testing.T) {
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
	session := "lisa-session-done-crash"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{Session: session, Agent: "claude", Mode: "interactive", RunID: "run-9"}); err != nil {
		t.Fatalf("failed to save session meta: %v", err)
	}
	if err := os.WriteFile(sessionDoneFile(projectRoot, session), []byte("run-9:9\n"), 0o600); err != nil {
		t.Fatalf("failed to write done file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "crashed" || status.Status != "idle" {
		t.Fatalf("expected done file with non-zero exit to crash, got state=%s status=%s", status.SessionState, status.Status)
	}
	if status.ClassificationReason != "done_file" {
		t.Fatalf("expected done_file reason, got %s", status.ClassificationReason)
	}
}

func TestComputeSessionStatusMarkerRunMismatchDoesNotComplete(t *testing.T) {
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
	session := "lisa-session-marker-mismatch"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{Session: session, Agent: "claude", Mode: "interactive", RunID: "run-current"}); err != nil {
		t.Fatalf("failed to save session meta: %v", err)
	}
	if err := os.WriteFile(sessionDoneFile(projectRoot, session), []byte("run-other:0\n"), 0o600); err != nil {
		t.Fatalf("failed to write mismatched done file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status computation to succeed, got %v", err)
	}
	if status.SessionState != "stuck" {
		t.Fatalf("expected marker mismatch to avoid completion, got %s", status.SessionState)
	}
	if !status.Signals.DoneFileRunMismatch {
		t.Fatalf("expected done-file mismatch signal")
	}
	if status.ClassificationReason != "stuck_marker_run_mismatch" {
		t.Fatalf("expected mismatch reason, got %s", status.ClassificationReason)
	}
}

func TestComputeSessionStatusEmitsTransitionAndSnapshotEvents(t *testing.T) {
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
		return "building project", nil
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
		return 55, 1.1, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-event-sequence"
	if _, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1); err != nil {
		t.Fatalf("first status failed: %v", err)
	}
	if _, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 2); err != nil {
		t.Fatalf("second status failed: %v", err)
	}

	tail, err := readSessionEventTail(projectRoot, session, 10)
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	events := tail.Events
	if len(events) < 2 {
		t.Fatalf("expected at least two events, got %d", len(events))
	}
	if events[len(events)-2].Type != "transition" {
		t.Fatalf("expected first event to be transition, got %s", events[len(events)-2].Type)
	}
	if events[len(events)-1].Type != "snapshot" {
		t.Fatalf("expected second event to be snapshot, got %s", events[len(events)-1].Type)
	}
}

func TestComputeSessionStatusEventIncludesStateLockWait(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origPaneStatus := tmuxPaneStatusFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origLock := withStateFileLockFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxPaneStatusFn = origPaneStatus
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		withStateFileLockFn = origLock
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "working", nil }
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 90, 1.2, nil }

	withStateFileLockFn = func(statePath string, fn func() error) (stateLockMeta, error) {
		if err := fn(); err != nil {
			return stateLockMeta{}, err
		}
		return stateLockMeta{WaitMS: 42}, nil
	}

	var observed sessionEvent
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		observed = event
		return nil
	}

	status, err := computeSessionStatus("lisa-lock-wait-event", t.TempDir(), "auto", "auto", false, 2)
	if err != nil {
		t.Fatalf("expected status payload, got %v", err)
	}
	if status.Signals.StateLockWaitMS != 42 {
		t.Fatalf("expected status lock wait to be propagated, got %d", status.Signals.StateLockWaitMS)
	}
	if observed.Signals.StateLockWaitMS != 42 {
		t.Fatalf("expected event lock wait to match status, got %d", observed.Signals.StateLockWaitMS)
	}
}

func TestComputeSessionStatusStateLockTimeoutFallsBackToDegraded(t *testing.T) {
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

	status, err := computeSessionStatus("lisa-lock-timeout", t.TempDir(), "auto", "auto", false, 2)
	if err != nil {
		t.Fatalf("expected timeout to degrade to status payload, got %v", err)
	}
	if status.SessionState != "degraded" || status.ClassificationReason != "state_lock_timeout" {
		t.Fatalf("expected state_lock_timeout classification, got state=%s reason=%s", status.SessionState, status.ClassificationReason)
	}
	if !status.Signals.StateLockTimedOut || status.Signals.StateLockWaitMS != 77 {
		t.Fatalf("expected lock timeout signals, got timedOut=%v wait=%d", status.Signals.StateLockTimedOut, status.Signals.StateLockWaitMS)
	}
}

func TestComputeSessionStatusMetaReadErrorDoesNotTrustSessionDoneMarker(t *testing.T) {
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 0, 0, nil }

	projectRoot := t.TempDir()
	session := "lisa-meta-unreadable"
	metaPath := sessionMetaFile(projectRoot, session)
	if err := os.WriteFile(metaPath, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("failed to seed malformed metadata: %v", err)
	}
	staleCapture := "__LISA_SESSION_DONE__:run-4:0\nuser@host:~/repo$ "
	_ = staleCapture

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 4)
	if err != nil {
		t.Fatalf("expected status payload despite meta decode failure, got %v", err)
	}
	if status.SessionState == "completed" || status.SessionState == "crashed" {
		t.Fatalf("expected marker to be ignored when metadata is unreadable, got state=%s", status.SessionState)
	}
	if status.Signals.MetaReadError == "" {
		t.Fatalf("expected meta read error signal to be set")
	}
}

func TestComputeSessionStatusReportsEventWriteErrors(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origPaneStatus := tmuxPaneStatusFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxPaneStatusFn = origPaneStatus
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "working", nil }
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 80, 1.0, nil }
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		return errors.New("disk full")
	}

	status, err := computeSessionStatus("lisa-events-write-failure", t.TempDir(), "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected status payload despite event write failure, got %v", err)
	}
	if status.Signals.EventsWriteError == "" {
		t.Fatalf("expected events write error signal to be set")
	}
}

func TestComputeSessionStatusReportsStateReadErrors(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "working", nil }
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 80, 1.0, nil }

	projectRoot := t.TempDir()
	session := "lisa-state-corrupt"
	if err := os.WriteFile(sessionStateFile(projectRoot, session), []byte("{"), 0o600); err != nil {
		t.Fatalf("failed to seed malformed state file: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("expected status payload despite state decode failure, got %v", err)
	}
	if status.Signals.StateReadError == "" {
		t.Fatalf("expected state read error signal to be set")
	}
}

func TestReadSessionEventTailSkipsMalformedLines(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-events-malformed"
	path := sessionEventsFile(projectRoot, session)
	data := strings.Join([]string{
		`{"at":"2026-01-01T00:00:00Z","type":"snapshot","session":"` + session + `","state":"in_progress","status":"active","reason":"x","poll":1,"signals":{}}`,
		`{"at":"broken-json"`,
		`{"at":"2026-01-01T00:00:01Z","type":"transition","session":"` + session + `","state":"waiting_input","status":"idle","reason":"y","poll":2,"signals":{}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("failed writing event file: %v", err)
	}

	tail, err := readSessionEventTail(projectRoot, session, 10)
	if err != nil {
		t.Fatalf("expected successful decode with dropped lines, got %v", err)
	}
	if len(tail.Events) != 2 {
		t.Fatalf("expected 2 valid events, got %d", len(tail.Events))
	}
	if tail.DroppedLines != 1 {
		t.Fatalf("expected 1 dropped line, got %d", tail.DroppedLines)
	}
}

func TestReadSessionEventTailHandlesLargeLine(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-events-large-line"
	if err := appendSessionEvent(projectRoot, session, sessionEvent{
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Type:    "snapshot",
		Session: session,
		State:   "in_progress",
		Status:  "active",
		Reason:  strings.Repeat("x", 120000),
		Poll:    1,
		Signals: statusSignals{},
	}); err != nil {
		t.Fatalf("appendSessionEvent failed: %v", err)
	}

	tail, err := readSessionEventTail(projectRoot, session, 10)
	if err != nil {
		t.Fatalf("expected large event line to decode, got %v", err)
	}
	if len(tail.Events) != 1 {
		t.Fatalf("expected one decoded event, got %d", len(tail.Events))
	}
	if tail.DroppedLines != 0 {
		t.Fatalf("expected no dropped lines, got %d", tail.DroppedLines)
	}
}

func TestReadSessionEventTailNormalizesNonPositiveLimit(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-events-limit-zero"
	if err := appendSessionEvent(projectRoot, session, sessionEvent{
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Type:    "snapshot",
		Session: session,
		State:   "in_progress",
		Status:  "active",
		Reason:  "r1",
		Poll:    1,
		Signals: statusSignals{},
	}); err != nil {
		t.Fatalf("appendSessionEvent failed: %v", err)
	}
	if err := appendSessionEvent(projectRoot, session, sessionEvent{
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Type:    "snapshot",
		Session: session,
		State:   "in_progress",
		Status:  "active",
		Reason:  "r2",
		Poll:    2,
		Signals: statusSignals{},
	}); err != nil {
		t.Fatalf("appendSessionEvent failed: %v", err)
	}

	tail, err := readSessionEventTail(projectRoot, session, 0)
	if err != nil {
		t.Fatalf("expected non-positive limit to be handled, got %v", err)
	}
	if len(tail.Events) != 1 {
		t.Fatalf("expected normalized limit to return one event, got %d", len(tail.Events))
	}
}

func TestAppendSessionEventTrimsOversizedEventLog(t *testing.T) {
	origMaxBytes := os.Getenv("LISA_EVENTS_MAX_BYTES")
	origMaxLines := os.Getenv("LISA_EVENTS_MAX_LINES")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_EVENTS_MAX_BYTES", origMaxBytes)
		_ = os.Setenv("LISA_EVENTS_MAX_LINES", origMaxLines)
	})

	if err := os.Setenv("LISA_EVENTS_MAX_BYTES", "400"); err != nil {
		t.Fatalf("failed to set LISA_EVENTS_MAX_BYTES: %v", err)
	}
	if err := os.Setenv("LISA_EVENTS_MAX_LINES", "6"); err != nil {
		t.Fatalf("failed to set LISA_EVENTS_MAX_LINES: %v", err)
	}

	projectRoot := t.TempDir()
	session := "lisa-events-trim"
	for i := 0; i < 30; i++ {
		err := appendSessionEvent(projectRoot, session, sessionEvent{
			At:      time.Now().UTC().Format(time.RFC3339Nano),
			Type:    "snapshot",
			Session: session,
			State:   "in_progress",
			Status:  "active",
			Reason:  strings.Repeat("reason-", 6),
			Poll:    i + 1,
			Signals: statusSignals{},
		})
		if err != nil {
			t.Fatalf("appendSessionEvent failed: %v", err)
		}
	}

	path := sessionEventsFile(projectRoot, session)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat event file: %v", err)
	}
	if info.Size() > 400 {
		t.Fatalf("expected trimmed event file <= 400 bytes, got %d", info.Size())
	}
}

func TestComputeSessionStatusUsesCachedProcessScanBetweenIntervals(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	origPaneStatus := tmuxPaneStatusFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origScanInterval := os.Getenv("LISA_PROCESS_SCAN_INTERVAL_SECONDS")
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
		tmuxPaneStatusFn = origPaneStatus
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		_ = os.Setenv("LISA_PROCESS_SCAN_INTERVAL_SECONDS", origScanInterval)
	})

	if err := os.Setenv("LISA_PROCESS_SCAN_INTERVAL_SECONDS", "120"); err != nil {
		t.Fatalf("failed to set scan interval: %v", err)
	}

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "work", nil }
	tmuxPaneStatusFn = func(session string) (string, error) { return "alive", nil }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_current_command}":
			return "zsh", nil
		case "#{pane_pid}":
			return "321", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }

	detectCalls := 0
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		detectCalls++
		return 777, 2.5, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-process-scan-cache"
	first, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1)
	if err != nil {
		t.Fatalf("first computeSessionStatus failed: %v", err)
	}
	second, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 2)
	if err != nil {
		t.Fatalf("second computeSessionStatus failed: %v", err)
	}

	if detectCalls != 1 {
		t.Fatalf("expected one process scan with caching, got %d", detectCalls)
	}
	if first.Signals.AgentScanCached {
		t.Fatalf("expected first poll to use live process scan")
	}
	if !second.Signals.AgentScanCached {
		t.Fatalf("expected second poll to use cached process scan")
	}
}

func TestComputeSessionStatusConcurrentStateWritesNoCorruption(t *testing.T) {
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
		return "parallel work", nil
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
		return 77, 2.0, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-parallel-state"
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		poll := i + 1
		go func() {
			defer wg.Done()
			_, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, poll)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent status call failed: %v", err)
		}
	}

	state := loadSessionState(sessionStateFile(projectRoot, session))
	if state.PollCount <= 0 {
		t.Fatalf("expected poll count to be persisted, got %d", state.PollCount)
	}
	if state.LastAgentPID != 77 {
		t.Fatalf("expected last agent PID to be persisted, got %d", state.LastAgentPID)
	}
}

func TestWrapSessionCommandTrapEmitsDoneOnInterrupt(t *testing.T) {
	runID := "run-interrupt"
	heartbeatPath := filepath.Join(t.TempDir(), "heartbeat.txt")
	donePath := filepath.Join(t.TempDir(), "done.txt")
	cmd := exec.Command("bash", "-lc", wrapSessionCommand("sleep 5", runID))
	cmd.Env = append(os.Environ(),
		"LISA_HEARTBEAT_FILE="+heartbeatPath,
		"LISA_DONE_FILE="+donePath,
	)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start wrapped command: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if fileExists(heartbeatPath) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for heartbeat file from wrapped command")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to interrupt process: %v", err)
	}
	waitErr := cmd.Wait()
	if waitErr == nil {
		t.Fatalf("expected interrupt to produce non-zero exit")
	}
	output := outBuf.String()
	if !strings.Contains(output, sessionDonePrefix+runID+":130") {
		t.Fatalf("expected done marker with interrupt exit code, output=%q", output)
	}
	if !fileExists(heartbeatPath) {
		t.Fatalf("expected heartbeat file to exist")
	}
	doneRaw, err := os.ReadFile(donePath)
	if err != nil {
		t.Fatalf("expected done file to exist: %v", err)
	}
	if strings.TrimSpace(string(doneRaw)) != runID+":130" {
		t.Fatalf("unexpected done file payload: %q", doneRaw)
	}
}

func TestCmdSessionSpawnFailsWhenHeartbeatPreparationFails(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionFn
	origHB := ensureHeartbeatWritableFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
		ensureHeartbeatWritableFn = origHB
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	newCalled := false
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error {
		newCalled = true
		return nil
	}
	ensureHeartbeatWritableFn = func(path string) error {
		return errors.New("permission denied")
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{"--session", "lisa-heartbeat-fail", "--command", "echo ok"}); code == 0 {
			t.Fatalf("expected spawn failure when heartbeat preparation fails")
		}
	})
	if newCalled {
		t.Fatalf("did not expect tmux session creation when heartbeat preparation fails")
	}
	if !strings.Contains(stderr, "failed to prepare heartbeat file") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSessionExplainJSONIncludesStatusAndEvents(t *testing.T) {
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
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "working", nil }
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
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 80, 1.0, nil }

	projectRoot := t.TempDir()
	session := "lisa-explain-json"
	if _, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 1); err != nil {
		t.Fatalf("failed to seed event log: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionExplain([]string{"--session", session, "--project-root", projectRoot, "--json"}); code != 0 {
			t.Fatalf("expected explain success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse explain json: %v (%q)", err, stdout)
	}
	statusObj, ok := payload["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status object in explain payload")
	}
	if statusObj["classificationReason"] == "" {
		t.Fatalf("expected classificationReason in explain payload")
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("expected non-empty events in explain payload")
	}
}
