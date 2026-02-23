package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EInteractiveFollowupMarkerFlowSameSession(t *testing.T) {
	requireGoAndTmux(t)

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-interactive-followup-%d", time.Now().UnixNano())
	firstMarker := fmt.Sprintf("FIRST-MARKER-%d", time.Now().UnixNano())
	secondMarker := fmt.Sprintf("SECOND-MARKER-%d", time.Now().UnixNano())

	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "codex",
		"--mode", "interactive",
		"--project-root", repoRoot,
		"--session", session,
		"--command", "cat",
		"--json",
	)

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "send",
		"--session", session,
		"--project-root", repoRoot,
		"--text", firstMarker,
		"--enter",
		"--json",
	)
	monitor1Raw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--until-marker", firstMarker,
		"--stop-on-waiting", "false",
		"--poll-interval", "1",
		"--max-polls", "30",
		"--json",
	)
	var monitor1 struct {
		ExitReason string `json:"exitReason"`
	}
	if err := json.Unmarshal([]byte(monitor1Raw), &monitor1); err != nil {
		t.Fatalf("failed to parse first monitor json: %v (%q)", err, monitor1Raw)
	}
	if monitor1.ExitReason != "marker_found" {
		t.Fatalf("expected first marker_found exit reason, got %s (%s)", monitor1.ExitReason, monitor1Raw)
	}

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "send",
		"--session", session,
		"--project-root", repoRoot,
		"--text", secondMarker,
		"--enter",
		"--json",
	)
	monitor2Raw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--until-marker", secondMarker,
		"--stop-on-waiting", "false",
		"--poll-interval", "1",
		"--max-polls", "30",
		"--json",
	)
	var monitor2 struct {
		ExitReason string `json:"exitReason"`
	}
	if err := json.Unmarshal([]byte(monitor2Raw), &monitor2); err != nil {
		t.Fatalf("failed to parse second monitor json: %v (%q)", err, monitor2Raw)
	}
	if monitor2.ExitReason != "marker_found" {
		t.Fatalf("expected second marker_found exit reason, got %s (%s)", monitor2.ExitReason, monitor2Raw)
	}

	captureRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "capture",
		"--session", session,
		"--project-root", repoRoot,
		"--raw",
		"--lines", "300",
		"--json",
	)
	var capture struct {
		Capture string `json:"capture"`
	}
	if err := json.Unmarshal([]byte(captureRaw), &capture); err != nil {
		t.Fatalf("failed to parse capture json: %v (%q)", err, captureRaw)
	}
	if !strings.Contains(capture.Capture, firstMarker) {
		t.Fatalf("expected first marker in capture, marker=%q capture=%q", firstMarker, capture.Capture)
	}
	if !strings.Contains(capture.Capture, secondMarker) {
		t.Fatalf("expected second marker in capture, marker=%q capture=%q", secondMarker, capture.Capture)
	}
}
