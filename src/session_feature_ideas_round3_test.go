package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdSessionObjectiveUpsertAndActivate(t *testing.T) {
	root := t.TempDir()
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionObjective([]string{
			"--project-root", root,
			"--id", "sprint",
			"--goal", "Ship orchestration update",
			"--acceptance", "Contract matrix passes",
			"--budget", "420",
			"--activate",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	if payload["currentId"] != "sprint" {
		t.Fatalf("expected currentId sprint, got %v", payload["currentId"])
	}
}

func TestCmdSessionSpawnDryRunIncludesLaneObjective(t *testing.T) {
	root := t.TempDir()
	if code := cmdSessionLane([]string{
		"--project-root", root,
		"--name", "planner",
		"--agent", "codex",
		"--mode", "interactive",
		"--contract", "handoff_v2_required",
		"--json",
	}); code != 0 {
		t.Fatalf("lane setup failed: %d", code)
	}
	if code := cmdSessionObjective([]string{
		"--project-root", root,
		"--id", "main",
		"--goal", "Objective from register",
		"--activate",
		"--json",
	}); code != 0 {
		t.Fatalf("objective setup failed: %d", code)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--project-root", root,
			"--lane", "planner",
			"--prompt", "Continue the task",
			"--dry-run",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	if payload["lane"] != "planner" {
		t.Fatalf("expected lane planner, got %v", payload["lane"])
	}
	if _, ok := payload["objective"].(map[string]any); !ok {
		t.Fatalf("expected objective payload, got %v", payload["objective"])
	}
	if !strings.Contains(payload["command"].(string), "Objective context") {
		t.Fatalf("expected objective propagation in command: %q", payload["command"])
	}
}

func TestCmdSessionHandoffSchemaV2IncludesTypedFields(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-handoff-v2"
	meta := sessionMeta{
		Session:             session,
		Agent:               "codex",
		Mode:                "interactive",
		ProjectRoot:         projectRoot,
		Lane:                "planner",
		ObjectiveID:         "main",
		ObjectiveGoal:       "Ship",
		ObjectiveAcceptance: "All checks pass",
		ObjectiveBudget:     320,
		StartCmd:            "echo",
		CreatedAt:           "2026-02-23T00:00:00Z",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	if code := cmdSessionLane([]string{
		"--project-root", projectRoot,
		"--name", "planner",
		"--contract", "handoff_v2_required",
		"--json",
	}); code != 0 {
		t.Fatalf("lane setup failed: %d", code)
	}
	if err := saveSessionMemory(projectRoot, session, sessionMemoryRecord{Session: session, MaxLines: 10, UpdatedAt: "2026-02-23T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z", Lines: []string{"changed A", "changed B"}}); err != nil {
		t.Fatalf("save memory: %v", err)
	}

	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "idle", SessionState: "waiting_input", ClassificationReason: "interactive_idle_cpu"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{"--session", session, "--project-root", projectRoot, "--schema", "v2", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	if payload["schema"] != "v2" {
		t.Fatalf("expected schema v2, got %v", payload["schema"])
	}
	if _, ok := payload["state"].(map[string]any); !ok {
		t.Fatalf("expected typed state payload, got %v", payload["state"])
	}
	if _, ok := payload["risks"].([]any); !ok {
		t.Fatalf("expected risks array, got %v", payload["risks"])
	}
	if _, ok := payload["openQuestions"].([]any); !ok {
		t.Fatalf("expected openQuestions array, got %v", payload["openQuestions"])
	}
}

func TestCmdSessionRouteQueue(t *testing.T) {
	root := t.TempDir()
	origList := tmuxListSessionsFn
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origCompute
	})
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-q-a", "lisa-q-b"}, nil
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if session == "lisa-q-a" {
			return sessionStatus{Session: session, Status: "idle", SessionState: "waiting_input"}, nil
		}
		return sessionStatus{Session: session, Status: "completed", SessionState: "completed"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{"--goal", "analysis", "--project-root", root, "--queue", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	queue := payload["queue"].([]any)
	if len(queue) == 0 {
		t.Fatalf("expected queue payload")
	}
	first := queue[0].(map[string]any)
	if first["session"] != "lisa-q-a" {
		t.Fatalf("expected waiting_input session first, got %v", first["session"])
	}
}

func TestCmdSessionRouteQueueEnumerationProjectScoped(t *testing.T) {
	projectRoot := t.TempDir()
	otherRoot := t.TempDir()
	localSession := generateSessionName(projectRoot, "codex", "interactive", "qscope")
	foreignSession := generateSessionName(otherRoot, "codex", "interactive", "qscope")

	if err := saveSessionMeta(otherRoot, foreignSession, sessionMeta{
		Session:     foreignSession,
		ProjectRoot: otherRoot,
	}); err != nil {
		t.Fatalf("save foreign meta: %v", err)
	}

	origList := tmuxListSessionsFn
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origCompute
	})

	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		if !projectOnly {
			t.Fatalf("expected project-only tmux listing for queue enumeration")
		}
		if canonicalProjectRoot(root) != canonicalProjectRoot(projectRoot) {
			t.Fatalf("unexpected project root: got %q want %q", root, projectRoot)
		}
		return []string{localSession, foreignSession}, nil
	}

	computed := map[string]int{}
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		computed[sessionName]++
		return sessionStatus{Session: sessionName, Status: "idle", SessionState: "waiting_input"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "analysis",
			"--project-root", projectRoot,
			"--queue",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	queue := payload["queue"].([]any)
	if len(queue) != 1 {
		t.Fatalf("expected only project-scoped queue item, got %d (%v)", len(queue), queue)
	}
	first := queue[0].(map[string]any)
	if first["session"] != localSession {
		t.Fatalf("expected local session in queue, got %v", first["session"])
	}
	if computed[foreignSession] != 0 {
		t.Fatalf("expected foreign session to be skipped, compute calls=%d", computed[foreignSession])
	}
}

func TestCmdSessionGuardPolicyFile(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(policyPath, []byte(`{"machinePolicy":"strict","deniedCommands":["cleanup --include-tmux-default"]}`), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionGuard([]string{"--shared-tmux", "--enforce", "--policy-file", policyPath, "--command", "./lisa cleanup --include-tmux-default", "--json"})
		if code == 0 {
			t.Fatalf("expected policy-enforced failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	if payload["policyFile"] == "" {
		t.Fatalf("expected policyFile in payload: %v", payload)
	}
}

func TestCmdSessionBudgetPlanJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionBudgetPlan([]string{"--goal", "analysis", "--project-root", t.TempDir(), "--budget", "300", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	hardStop := payload["hardStop"].(map[string]any)
	if hardStop["enforceCommand"] == "" {
		t.Fatalf("expected enforceCommand: %v", hardStop)
	}
}

func TestCmdSessionMonitorAutoRecover(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origSendKeys := tmuxSendKeysFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
		tmuxSendKeysFn = origSendKeys
	})
	calls := 0
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		calls++
		if calls == 1 {
			return sessionStatus{Session: session, Status: "idle", SessionState: "degraded"}, nil
		}
		return sessionStatus{Session: session, Status: "completed", SessionState: "completed"}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	sendCount := 0
	tmuxSendKeysFn = func(session string, keys []string, enter bool) error {
		sendCount++
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-recover",
			"--project-root", t.TempDir(),
			"--poll-interval", "1",
			"--max-polls", "1",
			"--auto-recover",
			"--recover-max", "1",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success after recovery, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if sendCount != 1 {
		t.Fatalf("expected one recovery send, got %d", sendCount)
	}
	if !strings.Contains(stdout, `"finalState":"completed"`) {
		t.Fatalf("expected completed payload, got %q", stdout)
	}
}
