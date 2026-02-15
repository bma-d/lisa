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

func TestE2ESessionExistsLifecycle(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-exists-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "interactive",
		"--project-root", repoRoot,
		"--session", session,
		"--command", "sleep 10",
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

	existsRaw, existsErr := runCommand(repoRoot, nil,
		binPath, "session", "exists",
		"--session", session,
	)
	if existsErr != nil {
		t.Fatalf("expected session exists to exit zero while running: %v (%q)", existsErr, existsRaw)
	}
	if strings.TrimSpace(existsRaw) != "true" {
		t.Fatalf("expected session exists true, got %q", existsRaw)
	}

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "kill",
		"--session", session,
		"--project-root", repoRoot,
	)

	existsRaw, existsErr = runCommand(repoRoot, nil,
		binPath, "session", "exists",
		"--session", session,
	)
	if existsErr == nil {
		t.Fatalf("expected session exists to exit non-zero after kill, got success (%q)", existsRaw)
	}
	if strings.TrimSpace(existsRaw) != "false" {
		t.Fatalf("expected session exists false after kill, got %q", existsRaw)
	}
}

func TestE2EMonitorTimeoutReturnsExitCode2(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-monitor-timeout-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "interactive",
		"--project-root", repoRoot,
		"--session", session,
		"--command", "sleep 10",
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

	monitorRaw, monitorErr := runCommand(repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--poll-interval", "1",
		"--max-polls", "2",
		"--stop-on-waiting", "false",
		"--json",
	)
	if monitorErr == nil {
		t.Fatalf("expected timeout monitor to exit non-zero, got success (%s)", monitorRaw)
	}
	var monitor struct {
		FinalState string `json:"finalState"`
		ExitReason string `json:"exitReason"`
	}
	if err := json.Unmarshal([]byte(monitorRaw), &monitor); err != nil {
		t.Fatalf("failed to parse monitor json: %v (%q)", err, monitorRaw)
	}
	if monitor.FinalState != "timeout" {
		t.Fatalf("expected timeout final state, got %s (%s)", monitor.FinalState, monitorRaw)
	}
	if monitor.ExitReason != "max_polls_exceeded" && monitor.ExitReason != "degraded_max_polls_exceeded" {
		t.Fatalf("expected timeout exit reason, got %s (%s)", monitor.ExitReason, monitorRaw)
	}
}

func TestE2EExplainReturnsLifecycleEvents(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-explain-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", repoRoot)
	})

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "exec",
		"--project-root", repoRoot,
		"--session", session,
		"--command", "echo E2E_EXPLAIN_OK",
		"--json",
	)

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--poll-interval", "1",
		"--max-polls", "30",
		"--stop-on-waiting", "false",
		"--json",
	)

	explainRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "explain",
		"--session", session,
		"--project-root", repoRoot,
		"--events", "20",
		"--json",
	)

	var explain struct {
		Status struct {
			Session string `json:"session"`
		} `json:"status"`
		EventFile string `json:"eventFile"`
		Events    []struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(explainRaw), &explain); err != nil {
		t.Fatalf("failed to parse explain json: %v (%q)", err, explainRaw)
	}
	if explain.Status.Session != session {
		t.Fatalf("explain status session mismatch: want %s got %s", session, explain.Status.Session)
	}
	if strings.TrimSpace(explain.EventFile) == "" {
		t.Fatalf("expected non-empty eventFile in explain payload")
	}
	if len(explain.Events) == 0 {
		t.Fatalf("expected lifecycle events in explain payload")
	}
}
