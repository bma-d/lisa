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

func TestE2ECodexRunsEntireSuiteWithAgentsContext(t *testing.T) {
	if os.Getenv("LISA_E2E_CODEX") != "1" {
		t.Skip("set LISA_E2E_CODEX=1 to run Codex e2e")
	}

	for _, bin := range []string{"go", "tmux", "codex"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("AGENTS.md is required for this e2e test: %v", err)
	}

	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-codex-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	prompt := strings.Join([]string{
		"Read AGENTS.md at repository root.",
		"Print exactly one line with prefix 'AGENTS_HEAD: ' followed by the first heading line from AGENTS.md.",
		"Run the entire Go test suite with this exact command: LISA_E2E_CLAUDE=0 LISA_E2E_CODEX=0 go test ./... && echo GO_TEST_EXIT:0 || echo GO_TEST_EXIT:1",
		"When done, print exactly: LISA_E2E_DONE",
	}, " ")

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "codex",
		"--mode", "exec",
		"--project-root", repoRoot,
		"--session", session,
		"--prompt", prompt,
		"--json",
	)

	var spawn struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(spawnRaw), &spawn); err != nil {
		t.Fatalf("failed to parse spawn json: %v\noutput:\n%s", err, spawnRaw)
	}
	if spawn.Session != session {
		t.Fatalf("spawned unexpected session: want %q got %q", session, spawn.Session)
	}

	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--poll-interval", "5",
		"--max-polls", "60",
		"--stop-on-waiting", "false",
		"--json",
	)

	var monitor struct {
		FinalState string `json:"finalState"`
		ExitReason string `json:"exitReason"`
	}
	if err := json.Unmarshal([]byte(monitorRaw), &monitor); err != nil {
		t.Fatalf("failed to parse monitor json: %v\noutput:\n%s", err, monitorRaw)
	}

	captureRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "capture",
		"--session", session,
		"--lines", "600",
		"--json",
	)

	var capture struct {
		Capture string `json:"capture"`
	}
	if err := json.Unmarshal([]byte(captureRaw), &capture); err != nil {
		t.Fatalf("failed to parse capture json: %v\noutput:\n%s", err, captureRaw)
	}

	if monitor.FinalState != "completed" {
		t.Fatalf("unexpected final state: %s (reason: %s)\nmonitor:\n%s\ncapture:\n%s",
			monitor.FinalState, monitor.ExitReason, monitorRaw, capture.Capture)
	}
	if !strings.Contains(capture.Capture, "AGENTS_HEAD: ") {
		t.Fatalf("missing AGENTS reference marker in capture:\n%s", capture.Capture)
	}
	if !strings.Contains(capture.Capture, "LISA_E2E_DONE") {
		t.Fatalf("missing completion marker in capture:\n%s", capture.Capture)
	}
	if !strings.Contains(capture.Capture, "GO_TEST_EXIT:0") {
		t.Fatalf("go test suite did not report success marker:\n%s", capture.Capture)
	}
}
