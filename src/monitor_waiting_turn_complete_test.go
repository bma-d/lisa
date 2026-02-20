package app

import (
	"strings"
	"testing"
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
