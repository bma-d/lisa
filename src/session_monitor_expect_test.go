package app

import (
	"strings"
	"testing"
)

func TestCmdSessionMonitorExpectMarkerRequiresUntilMarker(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-expect-marker",
			"--expect", "marker",
		})
		if code == 0 {
			t.Fatalf("expected failure when --expect marker is set without --until-marker")
		}
	})
	if !strings.Contains(stderr, "--expect marker requires --until-marker") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSessionMonitorExpectTerminalFailsOnMarkerFound(t *testing.T) {
	origCompute := computeSessionStatusFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "active",
			SessionState: "in_progress",
		}, nil
	}
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "work\nMARKER_OK\n", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-expect-terminal",
			"--project-root", t.TempDir(),
			"--poll-interval", "1",
			"--max-polls", "1",
			"--until-marker", "MARKER_OK",
			"--expect", "terminal",
			"--json",
		})
		if code != 2 {
			t.Fatalf("expected expectation mismatch exit 2, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"exitReason":"expected_terminal_got_marker_found"`) {
		t.Fatalf("expected terminal expectation mismatch payload, got %q", stdout)
	}
}

func TestCmdSessionMonitorExpectMarkerFailsOnCompleted(t *testing.T) {
	origCompute := computeSessionStatusFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "completed",
			SessionState: "completed",
		}, nil
	}
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "no marker", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-expect-marker-fail",
			"--project-root", t.TempDir(),
			"--poll-interval", "1",
			"--max-polls", "1",
			"--until-marker", "MISSING_MARKER",
			"--expect", "marker",
			"--json",
		})
		if code != 2 {
			t.Fatalf("expected expectation mismatch exit 2, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"exitReason":"expected_marker_got_completed"`) {
		t.Fatalf("expected marker expectation mismatch payload, got %q", stdout)
	}
}
