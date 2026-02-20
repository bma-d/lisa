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
	requireGoAndTmux(t)

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

func TestE2EInteractiveCatMonitorStopsOnWaitingInput(t *testing.T) {
	requireGoAndTmux(t)

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	session := fmt.Sprintf("lisa-e2e-interactive-cat-wait-%d", time.Now().UnixNano())
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

	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", repoRoot,
		"--poll-interval", "1",
		"--max-polls", "12",
		"--stop-on-waiting", "true",
		"--json",
	)
	var monitor struct {
		FinalState string `json:"finalState"`
		ExitReason string `json:"exitReason"`
	}
	if err := json.Unmarshal([]byte(monitorRaw), &monitor); err != nil {
		t.Fatalf("failed to parse monitor json: %v (%q)", err, monitorRaw)
	}
	if monitor.FinalState != "waiting_input" {
		t.Fatalf("expected waiting_input final state, got %s (%s)", monitor.FinalState, monitorRaw)
	}
	if monitor.ExitReason != "waiting_input" {
		t.Fatalf("expected waiting_input exit reason, got %s (%s)", monitor.ExitReason, monitorRaw)
	}
}

func TestE2EInteractiveFinishHookWritesDoneAndRemovesItself(t *testing.T) {
	requireGoAndTmux(t)

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	projectRoot := t.TempDir()
	fakeClaudePath := writeFakeClaudeLoopScript(t, t.TempDir())
	hookPath := writeFinishHookScript(t, t.TempDir())
	settingsPath := writeProjectScopedSettings(t, projectRoot, hookPath)

	session := fmt.Sprintf("lisa-e2e-interactive-hook-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", projectRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "interactive",
		"--project-root", projectRoot,
		"--session", session,
		"--command", shellQuote(fakeClaudePath),
		"--json",
	)
	var spawn struct {
		Session string `json:"session"`
		RunID   string `json:"runId"`
	}
	if err := json.Unmarshal([]byte(spawnRaw), &spawn); err != nil {
		t.Fatalf("failed to parse spawn json: %v (%q)", err, spawnRaw)
	}
	if spawn.Session != session {
		t.Fatalf("unexpected session: %q", spawn.Session)
	}
	if strings.TrimSpace(spawn.RunID) == "" {
		t.Fatalf("expected runId in spawn response: %q", spawnRaw)
	}

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "send",
		"--session", session,
		"--text", "run-hook",
		"--enter",
		"--json",
	)

	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", projectRoot,
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

	doneRaw, err := os.ReadFile(sessionDoneFile(projectRoot, session))
	if err != nil {
		t.Fatalf("failed to read done file: %v", err)
	}
	if strings.TrimSpace(string(doneRaw)) != spawn.RunID+":0" {
		t.Fatalf("unexpected done file payload: %q", string(doneRaw))
	}

	settingsRaw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}
	if strings.Contains(string(settingsRaw), "lisa_finish_hook_command") {
		t.Fatalf("expected finish hook to remove itself from settings, got: %q", string(settingsRaw))
	}

	existsRaw, existsErr := runCommand(repoRoot, nil,
		binPath, "session", "exists",
		"--session", session,
	)
	if existsErr != nil || strings.TrimSpace(existsRaw) != "true" {
		t.Fatalf("expected interactive session to remain alive after hook completion, err=%v out=%q", existsErr, existsRaw)
	}
}

func TestE2EInteractiveFinishHookWaitsForSubagentsBeforeDone(t *testing.T) {
	requireGoAndTmux(t)

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	projectRoot := t.TempDir()
	fakeClaudePath := writeFakeClaudeLoopScript(t, t.TempDir())
	hookPath := writeFinishHookScript(t, t.TempDir())
	settingsPath := writeProjectScopedSettings(t, projectRoot, hookPath)

	pendingFile := filepath.Join(t.TempDir(), "subagent.pending")
	subagentDoneFile := filepath.Join(t.TempDir(), "subagent.done")
	hookDoneFile := filepath.Join(t.TempDir(), "hook.done")

	command := strings.Join([]string{
		"LISA_WAIT_FOR_SUBAGENTS=1",
		"LISA_SUBAGENT_PENDING_FILE=" + shellQuote(pendingFile),
		"LISA_SUBAGENT_DELAY_SECS=2",
		"LISA_SUBAGENT_DONE_FILE=" + shellQuote(subagentDoneFile),
		"LISA_HOOK_DONE_FILE=" + shellQuote(hookDoneFile),
		shellQuote(fakeClaudePath),
	}, " ")

	session := fmt.Sprintf("lisa-e2e-interactive-hook-subagents-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = runCommand(repoRoot, nil, binPath, "session", "kill", "--session", session, "--project-root", projectRoot)
	})

	spawnRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "spawn",
		"--agent", "claude",
		"--mode", "interactive",
		"--project-root", projectRoot,
		"--session", session,
		"--command", command,
		"--json",
	)
	var spawn struct {
		Session string `json:"session"`
		RunID   string `json:"runId"`
	}
	if err := json.Unmarshal([]byte(spawnRaw), &spawn); err != nil {
		t.Fatalf("failed to parse spawn json: %v (%q)", err, spawnRaw)
	}
	if spawn.Session != session {
		t.Fatalf("unexpected session: %q", spawn.Session)
	}
	if strings.TrimSpace(spawn.RunID) == "" {
		t.Fatalf("expected runId in spawn response: %q", spawnRaw)
	}

	runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "send",
		"--session", session,
		"--text", "run-with-subagent",
		"--enter",
		"--json",
	)

	start := time.Now()
	monitorRaw := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "monitor",
		"--session", session,
		"--project-root", projectRoot,
		"--poll-interval", "1",
		"--max-polls", "30",
		"--stop-on-waiting", "false",
		"--json",
	)
	elapsed := time.Since(start)

	var monitor struct {
		FinalState string `json:"finalState"`
	}
	if err := json.Unmarshal([]byte(monitorRaw), &monitor); err != nil {
		t.Fatalf("failed to parse monitor json: %v (%q)", err, monitorRaw)
	}
	if monitor.FinalState != "completed" {
		t.Fatalf("expected completed terminal state, got %s (%s)", monitor.FinalState, monitorRaw)
	}
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("expected monitor to wait for subagent completion, elapsed=%s", elapsed)
	}

	doneRaw, err := os.ReadFile(sessionDoneFile(projectRoot, session))
	if err != nil {
		t.Fatalf("failed to read done file: %v", err)
	}
	if strings.TrimSpace(string(doneRaw)) != spawn.RunID+":0" {
		t.Fatalf("unexpected done file payload: %q", string(doneRaw))
	}

	subagentInfo, err := os.Stat(subagentDoneFile)
	if err != nil {
		t.Fatalf("expected subagent completion marker: %v", err)
	}
	hookInfo, err := os.Stat(hookDoneFile)
	if err != nil {
		t.Fatalf("expected hook completion marker: %v", err)
	}
	if hookInfo.ModTime().Before(subagentInfo.ModTime()) {
		t.Fatalf("expected hook to finish after subagent completion, subagent=%s hook=%s", subagentInfo.ModTime(), hookInfo.ModTime())
	}

	settingsRaw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}
	if strings.Contains(string(settingsRaw), "lisa_finish_hook_command") {
		t.Fatalf("expected finish hook to remove itself from settings, got: %q", string(settingsRaw))
	}
}

func requireGoAndTmux(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}
}

func writeProjectScopedSettings(t *testing.T, projectRoot, hookPath string) string {
	t.Helper()
	claudeDir := filepath.Join(projectRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	body := fmt.Sprintf("{\n  \"lisa_finish_hook_command\": %q\n}\n", hookPath)
	if err := os.WriteFile(settingsPath, []byte(body), 0o600); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}
	return settingsPath
}

func writeFinishHookScript(t *testing.T, dir string) string {
	t.Helper()
	hookPath := filepath.Join(dir, "lisa-finish-hook.sh")
	hookBody := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -eu",
		"settings_path=\"$1\"",
		"if [ \"${LISA_WAIT_FOR_SUBAGENTS:-0}\" = \"1\" ] && [ -n \"${LISA_SUBAGENT_PENDING_FILE:-}\" ]; then",
		"  while [ -e \"${LISA_SUBAGENT_PENDING_FILE}\" ]; do",
		"    sleep 0.1",
		"  done",
		"fi",
		"if [ -z \"${LISA_RUN_ID:-}\" ] || [ -z \"${LISA_DONE_FILE:-}\" ]; then",
		"  echo \"missing LISA_RUN_ID/LISA_DONE_FILE\" >&2",
		"  exit 1",
		"fi",
		"printf '%s:%d\\n' \"${LISA_RUN_ID}\" 0 > \"${LISA_DONE_FILE}.tmp\"",
		"mv \"${LISA_DONE_FILE}.tmp\" \"${LISA_DONE_FILE}\"",
		"printf '{}\\n' > \"$settings_path\"",
		"if [ -n \"${LISA_HOOK_DONE_FILE:-}\" ]; then",
		"  date +%s > \"${LISA_HOOK_DONE_FILE}\"",
		"fi",
		"echo HOOK_DONE",
		"",
	}, "\n")
	if err := os.WriteFile(hookPath, []byte(hookBody), 0o700); err != nil {
		t.Fatalf("failed to write finish hook script: %v", err)
	}
	return hookPath
}

func writeFakeClaudeLoopScript(t *testing.T, dir string) string {
	t.Helper()
	scriptPath := filepath.Join(dir, "fake-claude-loop.sh")
	scriptBody := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -eu",
		"find_settings() {",
		"  search=\"$PWD\"",
		"  while :; do",
		"    candidate=\"$search/.claude/settings.json\"",
		"    if [ -f \"$candidate\" ]; then",
		"      printf '%s\\n' \"$candidate\"",
		"      return 0",
		"    fi",
		"    if [ \"$search\" = \"/\" ]; then",
		"      return 1",
		"    fi",
		"    search=\"$(dirname \"$search\")\"",
		"  done",
		"}",
		"read_hook_path() {",
		"  settings=\"$1\"",
		"  sed -n 's/.*\"lisa_finish_hook_command\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p' \"$settings\" | head -n 1",
		"}",
		"settings_path=\"$(find_settings || true)\"",
		"while IFS= read -r line; do",
		"  if [ -z \"$line\" ]; then",
		"    continue",
		"  fi",
		"  echo \"PROMPT:$line\"",
		"  if [ \"$line\" = \"run-with-subagent\" ] && [ -n \"${LISA_SUBAGENT_PENDING_FILE:-}\" ]; then",
		"    : > \"${LISA_SUBAGENT_PENDING_FILE}\"",
		"    (",
		"      sleep \"${LISA_SUBAGENT_DELAY_SECS:-2}\"",
		"      rm -f \"${LISA_SUBAGENT_PENDING_FILE}\"",
		"      if [ -n \"${LISA_SUBAGENT_DONE_FILE:-}\" ]; then",
		"        date +%s > \"${LISA_SUBAGENT_DONE_FILE}\"",
		"      fi",
		"      echo SUBAGENT_DONE",
		"    ) &",
		"  fi",
		"  if [ -n \"$settings_path\" ] && [ -f \"$settings_path\" ]; then",
		"    hook_path=\"$(read_hook_path \"$settings_path\")\"",
		"    if [ -n \"$hook_path\" ] && [ -x \"$hook_path\" ]; then",
		"      \"$hook_path\" \"$settings_path\"",
		"    fi",
		"  fi",
		"done",
		"",
	}, "\n")
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o700); err != nil {
		t.Fatalf("failed to write fake claude loop script: %v", err)
	}
	return scriptPath
}
