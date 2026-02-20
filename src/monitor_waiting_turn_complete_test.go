package app

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestCmdSessionMonitorWaitingRequiresTurnCompleteStopsWhenReady(t *testing.T) {
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
			Agent:        "claude",
			Mode:         "interactive",
			Status:       "idle",
			SessionState: "waiting_input",
		}, nil
	}

	monitorWaitingTurnCompleteFn = func(session, projectRoot string, status sessionStatus) bool { return true }

	var observedReason string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		observedReason = event.Reason
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-waiting-turn-complete",
			"--poll-interval", "1",
			"--max-polls", "3",
			"--waiting-requires-turn-complete", "true",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected waiting-input turn-complete monitor exit 0, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"finalState":"waiting_input"`) || !strings.Contains(stdout, `"exitReason":"waiting_input_turn_complete"`) {
		t.Fatalf("expected waiting_input_turn_complete monitor payload, got %q", stdout)
	}
	if observedReason != "monitor_waiting_input_turn_complete" {
		t.Fatalf("expected lifecycle reason monitor_waiting_input_turn_complete, got %q", observedReason)
	}
}

func TestCmdSessionMonitorWaitingRequiresTurnCompleteDoesNotStopWhenNotReady(t *testing.T) {
	origCompute := computeSessionStatusFn
	origWaitingTurnComplete := monitorWaitingTurnCompleteFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		monitorWaitingTurnCompleteFn = origWaitingTurnComplete
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Agent:        "claude",
			Mode:         "interactive",
			Status:       "idle",
			SessionState: "waiting_input",
		}, nil
	}

	monitorWaitingTurnCompleteFn = func(session, projectRoot string, status sessionStatus) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-waiting-turn-not-ready",
			"--project-root", t.TempDir(),
			"--poll-interval", "1",
			"--max-polls", "1",
			"--waiting-requires-turn-complete", "true",
			"--json",
		})
		if code != 2 {
			t.Fatalf("expected timeout monitor exit 2 when waiting turn is incomplete, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"finalState":"timeout"`) {
		t.Fatalf("expected timeout final state when turn complete requirement is unmet, got %q", stdout)
	}
	if !strings.Contains(stdout, `"exitReason":"max_polls_exceeded"`) {
		t.Fatalf("expected max_polls_exceeded reason when turn complete requirement is unmet, got %q", stdout)
	}
}

func TestMonitorWaitingTurnCompleteRequiresTranscriptFreshnessAfterLatestInput(t *testing.T) {
	origNow := nowFn
	origCodexTranscript := checkCodexTranscriptTurnCompleteFn
	origCodexSince := codexHasAssistantTurnSinceFn
	t.Cleanup(func() {
		nowFn = origNow
		checkCodexTranscriptTurnCompleteFn = origCodexTranscript
		codexHasAssistantTurnSinceFn = origCodexSince
	})

	fixedNow := time.Date(2026, 2, 20, 8, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixedNow }
	checkCodexTranscriptTurnCompleteFn = func(prompt, createdAt, cachedSessionID string) (bool, int, string, error) {
		return true, 90, "codex-session-stale", nil
	}
	codexHasAssistantTurnSinceFn = func(sessionID string, minUserAtNanos int64) (bool, error) {
		return true, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-monitor-turn-freshness"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		Prompt:      "prompt",
		CreatedAt:   fixedNow.Add(-5 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(sessionMetaFile(projectRoot, session)) })
	t.Cleanup(func() { _ = os.Remove(sessionStateFile(projectRoot, session)) })
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastInputAt:      fixedNow.Add(-5 * time.Second).Unix(),
		LastInputAtNanos: fixedNow.Add(-5 * time.Second).UnixNano(),
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	ready := monitorWaitingTurnComplete(session, projectRoot, sessionStatus{
		Session:      session,
		Agent:        "codex",
		Mode:         "interactive",
		SessionState: "waiting_input",
	})
	if ready {
		t.Fatalf("expected stale transcript to block waiting_input_turn_complete")
	}

	state, err := loadSessionStateWithError(sessionStateFile(projectRoot, session))
	if err != nil {
		t.Fatalf("failed to load session state: %v", err)
	}
	if state.CodexSessionID != "codex-session-stale" {
		t.Fatalf("expected codex session id cache update, got %q", state.CodexSessionID)
	}
}

func TestMonitorWaitingTurnCompleteAllowsFreshTranscriptAfterInput(t *testing.T) {
	origNow := nowFn
	origCodexTranscript := checkCodexTranscriptTurnCompleteFn
	origCodexSince := codexHasAssistantTurnSinceFn
	t.Cleanup(func() {
		nowFn = origNow
		checkCodexTranscriptTurnCompleteFn = origCodexTranscript
		codexHasAssistantTurnSinceFn = origCodexSince
	})

	fixedNow := time.Date(2026, 2, 20, 8, 10, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixedNow }
	checkCodexTranscriptTurnCompleteFn = func(prompt, createdAt, cachedSessionID string) (bool, int, string, error) {
		return true, 1, "codex-session-fresh", nil
	}
	codexHasAssistantTurnSinceFn = func(sessionID string, minUserAtNanos int64) (bool, error) {
		return true, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-monitor-turn-fresh"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		Prompt:      "prompt",
		CreatedAt:   fixedNow.Add(-5 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(sessionMetaFile(projectRoot, session)) })
	t.Cleanup(func() { _ = os.Remove(sessionStateFile(projectRoot, session)) })
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{
		LastInputAt:      fixedNow.Add(-10 * time.Second).Unix(),
		LastInputAtNanos: fixedNow.Add(-10 * time.Second).UnixNano(),
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	ready := monitorWaitingTurnComplete(session, projectRoot, sessionStatus{
		Session:      session,
		Agent:        "codex",
		Mode:         "interactive",
		SessionState: "waiting_input",
	})
	if !ready {
		t.Fatalf("expected fresh transcript to satisfy waiting_input_turn_complete")
	}
}
