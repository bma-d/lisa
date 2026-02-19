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

func TestE2ENestedInteractiveThreeLevels(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	runSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sessionL1 := "lisa-nested-l1-" + runSuffix
	sessionL2 := "lisa-nested-l2-" + runSuffix
	sessionL3 := "lisa-nested-l3-" + runSuffix

	l3Script := filepath.Join(t.TempDir(), "nested-l3.sh")
	l2Script := filepath.Join(t.TempDir(), "nested-l2.sh")
	l1Script := filepath.Join(t.TempDir(), "nested-l1.sh")

	writeScript := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatalf("failed to write script %s: %v", path, err)
		}
	}

	writeScript(l3Script, strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"echo NESTED_L3_DONE=1",
	}, "\n")+"\n")

	writeScript(l2Script, strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"BIN=" + shellQuote(binPath),
		"ROOT=" + shellQuote(repoRoot),
		`"$BIN" session spawn --agent codex --mode interactive --project-root "$ROOT" --session ` + sessionL3 + ` --command "/bin/bash ` + l3Script + `" --json`,
		`"$BIN" session monitor --session ` + sessionL3 + ` --project-root "$ROOT" --poll-interval 1 --max-polls 60 --json`,
		`"$BIN" session capture --session ` + sessionL3 + ` --project-root "$ROOT" --raw --lines 120`,
		"echo NESTED_L2_DONE=1",
	}, "\n")+"\n")

	writeScript(l1Script, strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"BIN=" + shellQuote(binPath),
		"ROOT=" + shellQuote(repoRoot),
		`"$BIN" session spawn --agent codex --mode interactive --project-root "$ROOT" --session ` + sessionL2 + ` --command "/bin/bash ` + l2Script + `" --json`,
		`"$BIN" session monitor --session ` + sessionL2 + ` --project-root "$ROOT" --poll-interval 1 --max-polls 90 --json`,
		`"$BIN" session capture --session ` + sessionL2 + ` --project-root "$ROOT" --raw --lines 160`,
		"echo NESTED_L1_DONE=1",
	}, "\n")+"\n")

	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", sessionL3, "--project-root", repoRoot)
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", sessionL2, "--project-root", repoRoot)
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", sessionL1, "--project-root", repoRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "codex",
		"--mode", "interactive",
		"--project-root", repoRoot,
		"--session", sessionL1,
		"--command", "/bin/bash "+l1Script,
		"--json",
	)

	var spawn struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(spawnRaw), &spawn); err != nil {
		t.Fatalf("failed to parse spawn json: %v (%q)", err, spawnRaw)
	}
	if spawn.Session != sessionL1 {
		t.Fatalf("unexpected session: got %q want %q", spawn.Session, sessionL1)
	}

	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", sessionL1,
		"--project-root", repoRoot,
		"--poll-interval", "1",
		"--max-polls", "120",
		"--json",
	)
	var monitor struct {
		FinalState string `json:"finalState"`
	}
	if err := json.Unmarshal([]byte(monitorRaw), &monitor); err != nil {
		t.Fatalf("failed to parse monitor json: %v (%q)", err, monitorRaw)
	}
	if monitor.FinalState != "completed" {
		t.Fatalf("expected completed monitor state, got %q (%s)", monitor.FinalState, monitorRaw)
	}

	captureRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "capture",
		"--session", sessionL1,
		"--project-root", repoRoot,
		"--raw",
		"--lines", "320",
		"--json",
	)
	var capture struct {
		Capture string `json:"capture"`
	}
	if err := json.Unmarshal([]byte(captureRaw), &capture); err != nil {
		t.Fatalf("failed to parse capture json: %v (%q)", err, captureRaw)
	}
	for _, marker := range []string{"NESTED_L3_DONE=1", "NESTED_L2_DONE=1", "NESTED_L1_DONE=1"} {
		if !strings.Contains(capture.Capture, marker) {
			t.Fatalf("missing marker %q in capture:\n%s", marker, capture.Capture)
		}
	}
}
