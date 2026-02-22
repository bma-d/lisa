package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdSessionSchemaJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSchema([]string{"--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload struct {
		Commands map[string]any `json:"commands"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v (%q)", err, stdout)
	}
	if _, ok := payload.Commands["session packet"]; !ok {
		t.Fatalf("expected session packet schema, got %v", payload.Commands)
	}
}

func TestCmdSessionSchemaCommandAlias(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSchema([]string{"--command", "packet", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"command":"session packet"`) {
		t.Fatalf("expected normalized command name, got %q", stdout)
	}
}

func TestCmdSessionPacketFieldsProjection(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-fields-projection"

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "active", SessionState: "in_progress", ClassificationReason: "heartbeat_fresh"}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "field projection", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--fields", "session,nextAction,nextOffset",
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
		t.Fatalf("decode error: %v (%q)", err, stdout)
	}
	if payload["session"] != session {
		t.Fatalf("expected session, got %v", payload["session"])
	}
	if _, ok := payload["nextAction"]; !ok {
		t.Fatalf("expected nextAction in projected payload, got %v", payload)
	}
	if _, ok := payload["nextOffset"]; !ok {
		t.Fatalf("expected nextOffset in projected payload, got %v", payload)
	}
}

func TestCmdSessionPacketFieldsRequireJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", "lisa-packet-no-json",
			"--fields", "session,nextAction",
		})
		if code == 0 {
			t.Fatalf("expected --fields without --json to fail")
		}
	})
	if stdout != "" {
		t.Fatalf("expected no stdout payload in non-json mode, got %q", stdout)
	}
	if !strings.Contains(stderr, "--fields requires --json") {
		t.Fatalf("expected fields/json validation error, got %q", stderr)
	}
}

func TestCmdSessionPacketRejectsInvalidFields(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", "lisa-packet-invalid-fields",
			"--fields", "status..reason",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected invalid --fields to fail")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"invalid_fields"`) {
		t.Fatalf("expected invalid_fields error, got %q", stdout)
	}
}

func TestCmdSessionMonitorEventBudgetRequiresEmitHandoff(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-budget",
			"--event-budget", "64",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected failure without --emit-handoff")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"event_budget_requires_emit_handoff"`) {
		t.Fatalf("expected contract error code, got %q", stdout)
	}
}

func TestCmdSessionContextPackRedact(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-redact-pack"

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "active", SessionState: "in_progress", ClassificationReason: "heartbeat_fresh"}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "token=abc12345678 email=dev@example.com path=/Users/example/project", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{
			"--for", session,
			"--project-root", projectRoot,
			"--redact", "all",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, "[REDACTED_") {
		t.Fatalf("expected redacted payload, got %q", stdout)
	}
}

func TestCmdSessionRouteTopologyCostEstimate(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "nested",
			"--topology", "planner,workers,reviewer",
			"--cost-estimate",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"topology"`) || !strings.Contains(stdout, `"costEstimate"`) {
		t.Fatalf("expected topology + costEstimate in payload, got %q", stdout)
	}
}

func TestCmdSessionListPrioritySortsItems(t *testing.T) {
	projectRoot := t.TempDir()
	origList := tmuxListSessionsFn
	origStatus := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origStatus
	})
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-a", "lisa-b"}, nil
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		if session == "lisa-a" {
			return sessionStatus{Session: session, Status: "active", SessionState: "in_progress"}, nil
		}
		return sessionStatus{Session: session, Status: "idle", SessionState: "waiting_input"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", projectRoot, "--priority", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"priorityScore"`) {
		t.Fatalf("expected priority fields, got %q", stdout)
	}
	if strings.Index(stdout, "lisa-b") > strings.Index(stdout, "lisa-a") {
		t.Fatalf("expected waiting_input session lisa-b before lisa-a, got %q", stdout)
	}
}

func TestCmdSessionCaptureSemanticDeltaCursor(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-semantic-cursor"
	cursorFile := filepath.Join(projectRoot, "capture.cursor")
	captures := []string{
		"alpha\nbeta\n",
		"alpha\nbeta\ngamma\n",
	}

	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	idx := 0
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		value := captures[idx]
		if idx < len(captures)-1 {
			idx++
		}
		return value, nil
	}

	_, _ = captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--raw",
			"--cursor-file", cursorFile,
			"--semantic-delta",
			"--json",
		})
		if code != 0 {
			t.Fatalf("first capture expected success, got %d", code)
		}
	})
	stdout2, stderr2 := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--raw",
			"--cursor-file", cursorFile,
			"--semantic-delta",
			"--json",
		})
		if code != 0 {
			t.Fatalf("second capture expected success, got %d", code)
		}
	})
	if stderr2 != "" {
		t.Fatalf("unexpected stderr: %q", stderr2)
	}
	if !strings.Contains(stdout2, `"semanticDelta":"gamma"`) {
		t.Fatalf("expected semantic delta to include gamma only, got %q", stdout2)
	}
}

func TestCmdSessionCheckpointSaveResume(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-checkpoint"
	filePath := filepath.Join(projectRoot, "checkpoint.json")

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "heartbeat_fresh",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	_, _ = captureOutput(t, func() {
		code := cmdSessionCheckpoint([]string{
			"save",
			"--session", session,
			"--project-root", projectRoot,
			"--file", filePath,
			"--json",
		})
		if code != 0 {
			t.Fatalf("save expected success, got %d", code)
		}
	})
	stdout2, stderr2 := captureOutput(t, func() {
		code := cmdSessionCheckpoint([]string{
			"resume",
			"--file", filePath,
			"--json",
		})
		if code != 0 {
			t.Fatalf("resume expected success, got %d", code)
		}
	})
	if stderr2 != "" {
		t.Fatalf("unexpected stderr: %q", stderr2)
	}
	if !strings.Contains(stdout2, `"action":"resume"`) || !strings.Contains(stdout2, `"session":"`+session+`"`) {
		t.Fatalf("expected resume payload, got %q", stdout2)
	}
}

func TestCmdSessionCheckpointRejectsInvalidAction(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "checkpoint.json")
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCheckpoint([]string{
			"--action", "rewind",
			"--file", filePath,
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected invalid action to fail")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"invalid_action"`) {
		t.Fatalf("expected invalid_action error, got %q", stdout)
	}
}

func TestCmdSessionCheckpointRequiresFileFlag(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCheckpoint([]string{
			"save",
			"--session", "lisa-checkpoint-no-file",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected missing --file to fail")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"missing_required_flag"`) || !strings.Contains(stdout, "--file is required") {
		t.Fatalf("expected missing_required_flag for --file, got %q", stdout)
	}
}

func TestCmdSessionDedupeClaimAndDuplicate(t *testing.T) {
	projectRoot := t.TempDir()

	origHas := tmuxHasSessionFn
	t.Cleanup(func() { tmuxHasSessionFn = origHas })
	tmuxHasSessionFn = func(session string) bool { return session == "lisa-claim-1" }

	_, _ = captureOutput(t, func() {
		code := cmdSessionDedupe([]string{
			"--task-hash", "task-abc",
			"--session", "lisa-claim-1",
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("first claim expected success, got %d", code)
		}
	})
	stdout2, stderr2 := captureOutput(t, func() {
		code := cmdSessionDedupe([]string{
			"--task-hash", "task-abc",
			"--session", "lisa-claim-2",
			"--project-root", projectRoot,
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected duplicate failure")
		}
	})
	if stderr2 != "" {
		t.Fatalf("unexpected stderr: %q", stderr2)
	}
	if !strings.Contains(stdout2, `"errorCode":"task_duplicate_detected"`) {
		t.Fatalf("expected duplicate error code, got %q", stdout2)
	}
}

func TestCmdSessionDedupeRequiresTaskHash(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionDedupe([]string{
			"--session", "lisa-dedupe-no-hash",
			"--project-root", t.TempDir(),
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected missing task hash to fail")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"missing_required_flag"`) || !strings.Contains(stdout, "--task-hash is required") {
		t.Fatalf("expected missing_required_flag for task hash, got %q", stdout)
	}
}

func TestCmdSessionDedupeMissingTaskHashValue(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionDedupe([]string{
			"--task-hash",
		})
		if code == 0 {
			t.Fatalf("expected missing value for --task-hash to fail")
		}
	})
	if stdout != "" {
		t.Fatalf("expected no stdout payload in non-json mode, got %q", stdout)
	}
	if !strings.Contains(stderr, "missing value for --task-hash") {
		t.Fatalf("expected missing value error, got %q", stderr)
	}
}

func TestCmdSessionAutopilotResumeCarriesModeFromSummary(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	root := t.TempDir()
	resumePath := filepath.Join(root, "resume.json")
	resumePayload := `{
		"ok": false,
		"goal": "exec",
		"agent": "codex",
		"mode": "exec",
		"projectRoot": "` + strings.ReplaceAll(root, `\`, `\\`) + `",
		"session": "lisa-resume-mode",
		"killAfter": false,
		"spawn": {"ok": true, "exitCode": 0, "output": {"session":"lisa-resume-mode"}},
		"monitor": {"ok": false, "exitCode": 2, "error": "timeout"},
		"capture": {"ok": true, "exitCode": 0},
		"handoff": {"ok": true, "exitCode": 0},
		"failedStep": "monitor"
	}`
	if err := os.WriteFile(resumePath, []byte(resumePayload), 0o644); err != nil {
		t.Fatalf("write resume payload: %v", err)
	}

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		switch args[1] {
		case "monitor":
			return `{"finalState":"completed","exitReason":"completed","session":"lisa-resume-mode","polls":1}`, "", nil
		case "capture":
			return `{"summary":"ok"}`, "", nil
		case "handoff":
			return `{"nextAction":"session capture"}`, "", nil
		default:
			return `{"ok":true}`, "", nil
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionAutopilot([]string{
			"--resume-from", resumePath,
			"--project-root", root,
			"--json",
		})
		if code != 0 {
			t.Fatalf("resume expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"mode":"exec"`) || !strings.Contains(stdout, `"goal":"exec"`) {
		t.Fatalf("expected resume to preserve mode/goal, got %q", stdout)
	}
}

func TestCmdSessionSmokeUsesTerminalMonitorAndChaosReport(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	monitorSawStopOnWaitingFalse := false
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) >= 2 && args[0] == "session" && args[1] == "monitor" {
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "--stop-on-waiting" && args[i+1] == "false" {
					monitorSawStopOnWaitingFalse = true
				}
			}
			return `{"finalState":"completed","session":"lisa-smoke-l1","exitReason":"completed","polls":1,"finalStatus":"completed"}`, "", nil
		}
		if len(args) >= 2 && args[0] == "session" && args[1] == "capture" {
			return `{"capture":"LISA_SMOKE_L1_DONE=1\n"}`, "", nil
		}
		if len(args) >= 2 && args[0] == "session" && args[1] == "tree" {
			return `{"roots":[]}`, "", nil
		}
		return `{"session":"ok"}`, "", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--levels", "1",
			"--chaos-report",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected smoke success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !monitorSawStopOnWaitingFalse {
		t.Fatalf("expected smoke monitor to set --stop-on-waiting false")
	}
	if !strings.Contains(stdout, `"chaosReport":true`) || !strings.Contains(stdout, `"chaosResult"`) {
		t.Fatalf("expected chaos report payload, got %q", stdout)
	}
}
