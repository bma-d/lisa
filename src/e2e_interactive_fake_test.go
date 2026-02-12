package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EInteractiveLifecycleWithCat(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	waitScript := filepath.Join(t.TempDir(), "wait-one-line.sh")
	waitScriptBody := strings.Join([]string{
		"#!/usr/bin/env sh",
		"IFS= read -r line",
		"echo \"$line\"",
		"exit 0",
		"",
	}, "\n")
	if err := os.WriteFile(waitScript, []byte(waitScriptBody), 0o700); err != nil {
		t.Fatalf("failed to write wait script: %v", err)
	}

	session := fmt.Sprintf("lisa-e2e-interactive-cat-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "interactive",
		"--project-root", repoRoot,
		"--session", session,
		"--command", waitScript,
		"--json",
	)
	var spawn struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(spawnRaw), &spawn); err != nil {
		t.Fatalf("failed to parse spawn json: %v (%q)", err, spawnRaw)
	}
	if spawn.Session != session {
		t.Fatalf("unexpected session: %q", spawn.Session)
	}

	statusRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "status",
		"--session", session,
		"--project-root", repoRoot,
		"--json",
	)
	var statusPayload struct {
		SessionState string `json:"sessionState"`
	}
	if err := json.Unmarshal([]byte(statusRaw), &statusPayload); err != nil {
		t.Fatalf("failed to parse status json: %v (%q)", err, statusRaw)
	}
	switch statusPayload.SessionState {
	case "in_progress", "just_started":
	default:
		t.Fatalf("unexpected early state: %s (%s)", statusPayload.SessionState, statusRaw)
	}

	sendTextRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "send",
		"--session", session,
		"--keys", "done",
		"--enter",
		"--json",
	)
	if !strings.Contains(sendTextRaw, `"ok":true`) {
		t.Fatalf("unexpected send-text response: %q", sendTextRaw)
	}

	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--poll-interval", "1",
		"--max-polls", "20",
		"--stop-on-waiting", "false",
		"--json",
	)
	var monitor struct {
		FinalState string `json:"finalState"`
	}
	if err := json.Unmarshal([]byte(monitorRaw), &monitor); err != nil {
		t.Fatalf("failed to parse monitor json: %v (%q)", err, monitorRaw)
	}
	if monitor.FinalState != "completed" {
		t.Fatalf("expected completed terminal state, got %s (%s)", monitor.FinalState, monitorRaw)
	}

	captureRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "capture",
		"--session", session,
		"--raw",
		"--lines", "200",
		"--json",
	)
	var capture struct {
		Capture string `json:"capture"`
	}
	if err := json.Unmarshal([]byte(captureRaw), &capture); err != nil {
		t.Fatalf("failed to parse capture json: %v (%q)", err, captureRaw)
	}
	if !strings.Contains(capture.Capture, "done") {
		t.Fatalf("expected echoed text in capture, got %q", capture.Capture)
	}
}
