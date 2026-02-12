package app

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EExecLifecycleWithLocalCommand(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-exec-fake-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "exec",
		"--project-root", repoRoot,
		"--session", session,
		"--command", "echo E2E_EXEC_OK",
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

	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--poll-interval", "1",
		"--max-polls", "30",
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
		t.Fatalf("expected completed state, got %s (%s)", monitor.FinalState, monitorRaw)
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
	if !strings.Contains(capture.Capture, "E2E_EXEC_OK") {
		t.Fatalf("expected command output in capture, got %q", capture.Capture)
	}
}
