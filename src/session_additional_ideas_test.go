package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("failed parsing json: %v (%q)", err, raw)
	}
	return out
}

func TestSessionAutopilotJSONSuccess(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) < 2 {
			return "", "", fmt.Errorf("invalid args")
		}
		switch args[1] {
		case "spawn":
			return `{"session":"lisa-auto-1","status":"ok"}`, "", nil
		case "monitor":
			return `{"finalState":"waiting_input","exitReason":"waiting_input"}`, "", nil
		case "capture":
			return `{"summary":"ops","summaryStyle":"ops"}`, "", nil
		case "handoff":
			return `{"nextAction":"session send"}`, "", nil
		case "kill":
			return `{"ok":true}`, "", nil
		default:
			return "", "", fmt.Errorf("unexpected subcommand: %s", args[1])
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionAutopilot([]string{
			"--goal", "analysis",
			"--project-root", t.TempDir(),
			"--summary",
			"--summary-style", "ops",
			"--kill-after", "true",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if payload["session"] != "lisa-auto-1" {
		t.Fatalf("expected session lisa-auto-1, got %v", payload["session"])
	}
	cleanup, ok := payload["cleanup"].(map[string]any)
	if !ok || cleanup["ok"] != true {
		t.Fatalf("expected cleanup step to succeed, got %v", payload["cleanup"])
	}
}

func TestSessionHandoffCursorFile(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHasSession := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHasSession
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "idle",
			SessionState:         "waiting_input",
			ClassificationReason: "interactive_idle_cpu",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	root := t.TempDir()
	session := "lisa-handoff-cursor"
	eventsPath := sessionEventsFile(root, session)
	if err := os.WriteFile(eventsPath, []byte(strings.Join([]string{
		`{"at":"t1","type":"snapshot","session":"lisa-handoff-cursor","state":"in_progress","status":"active","reason":"r1"}`,
		`{"at":"t2","type":"snapshot","session":"lisa-handoff-cursor","state":"waiting_input","status":"idle","reason":"r2"}`,
	}, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("failed writing events: %v", err)
	}
	cursor := filepath.Join(root, "handoff.cursor")

	stdout1, stderr1 := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", root,
			"--cursor-file", cursor,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr1 != "" {
		t.Fatalf("expected empty stderr, got %q", stderr1)
	}
	payload1 := parseJSONMap(t, stdout1)
	if payload1["deltaCount"].(float64) != 2 {
		t.Fatalf("expected deltaCount=2, got %v", payload1["deltaCount"])
	}

	stdout2, _ := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", root,
			"--cursor-file", cursor,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected second success, got %d", code)
		}
	})
	payload2 := parseJSONMap(t, stdout2)
	if payload2["deltaCount"].(float64) != 0 {
		t.Fatalf("expected deltaCount=0 on second read, got %v", payload2["deltaCount"])
	}
}

func TestSessionContextPackFromHandoff(t *testing.T) {
	root := t.TempDir()
	payloadPath := filepath.Join(root, "handoff.json")
	raw := `{"session":"lisa-pack-from-handoff","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextOffset":77,"recent":[{"at":"t1","state":"waiting_input","status":"idle","reason":"interactive_idle_cpu"}]}`
	if err := os.WriteFile(payloadPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing handoff payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{
			"--from-handoff", payloadPath,
			"--strategy", "terse",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["session"] != "lisa-pack-from-handoff" {
		t.Fatalf("unexpected session: %v", payload["session"])
	}
	if payload["nextOffset"].(float64) != 77 {
		t.Fatalf("expected nextOffset=77, got %v", payload["nextOffset"])
	}
	pack := payload["pack"].(string)
	if !strings.Contains(pack, "state=waiting_input") {
		t.Fatalf("expected waiting_input in pack, got %q", pack)
	}
}

func TestSessionContextPackFromHandoffSchemaV2NextActionObject(t *testing.T) {
	root := t.TempDir()
	payloadPath := filepath.Join(root, "handoff-v2.json")
	raw := `{"session":"lisa-pack-from-handoff-v2","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextAction":{"name":"session send","command":"./lisa session send --json-min"},"nextOffset":77,"recent":[{"at":"t1","state":"waiting_input","status":"idle","reason":"interactive_idle_cpu"}]}`
	if err := os.WriteFile(payloadPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing handoff payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{
			"--from-handoff", payloadPath,
			"--strategy", "terse",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["session"] != "lisa-pack-from-handoff-v2" {
		t.Fatalf("unexpected session: %v", payload["session"])
	}
	if payload["nextAction"] != "session send" {
		t.Fatalf("expected nextAction session send, got %v", payload["nextAction"])
	}
	if payload["nextOffset"].(float64) != 77 {
		t.Fatalf("expected nextOffset=77, got %v", payload["nextOffset"])
	}
	pack := payload["pack"].(string)
	if !strings.Contains(pack, "t1 waiting_input/idle interactive_idle_cpu") {
		t.Fatalf("expected recent entry in pack, got %q", pack)
	}
}

func TestSessionContextPackFromHandoffSchemaV2NextActionCommandFallback(t *testing.T) {
	root := t.TempDir()
	payloadPath := filepath.Join(root, "handoff-v2-command.json")
	raw := `{"session":"lisa-pack-from-handoff-v2-command","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextAction":{"command":"./lisa session send --json-min"},"nextOffset":77,"recent":[{"at":"t1","state":"waiting_input","status":"idle","reason":"interactive_idle_cpu"}]}`
	if err := os.WriteFile(payloadPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing handoff payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{
			"--from-handoff", payloadPath,
			"--strategy", "terse",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["session"] != "lisa-pack-from-handoff-v2-command" {
		t.Fatalf("unexpected session: %v", payload["session"])
	}
	if payload["nextAction"] != "./lisa session send --json-min" {
		t.Fatalf("expected nextAction command fallback, got %v", payload["nextAction"])
	}
}

func TestSessionContextPackFromHandoffSessionMismatch(t *testing.T) {
	root := t.TempDir()
	payloadPath := filepath.Join(root, "handoff.json")
	raw := `{"session":"lisa-pack-from-handoff","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextOffset":77}`
	if err := os.WriteFile(payloadPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing handoff payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{
			"--for", "lisa-pack-manual",
			"--from-handoff", payloadPath,
			"--json-min",
		})
		if code == 0 {
			t.Fatalf("expected mismatch failure")
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "from_handoff_session_mismatch" {
		t.Fatalf("expected from_handoff_session_mismatch, got %v", payload["errorCode"])
	}
}

func TestSessionContextPackFromHandoffInvalidRecent(t *testing.T) {
	root := t.TempDir()
	payloadPath := filepath.Join(root, "handoff-invalid-recent.json")
	raw := `{"session":"lisa-pack-from-handoff-invalid-recent","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextOffset":77,"recent":"invalid"}`
	if err := os.WriteFile(payloadPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing handoff payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{
			"--from-handoff", payloadPath,
			"--json-min",
		})
		if code == 0 {
			t.Fatalf("expected failure for invalid recent payload")
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "invalid_from_handoff" {
		t.Fatalf("expected invalid_from_handoff, got %v", payload["errorCode"])
	}
	if !strings.Contains(mapStringValue(payload, "error"), "invalid handoff recent") {
		t.Fatalf("expected error to include invalid handoff recent, got %v", payload["error"])
	}
}

func TestSessionMonitorUntilJSONPath(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHasSession := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHasSession
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "idle",
			SessionState:         "waiting_input",
			ClassificationReason: "interactive_idle_cpu",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-jsonpath-monitor",
			"--project-root", t.TempDir(),
			"--until-jsonpath", "$.sessionState=waiting_input",
			"--max-polls", "2",
			"--poll-interval", "1",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["exitReason"] != "jsonpath_matched" {
		t.Fatalf("expected jsonpath_matched exit reason, got %v", payload["exitReason"])
	}
	if _, ok := payload["errorCode"]; ok {
		t.Fatalf("expected no errorCode for successful jsonpath match, got %v", payload["errorCode"])
	}
}

func TestSessionMonitorUntilJSONPathPrecedesUntilMarker(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHasSession := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHasSession
		tmuxCapturePaneFn = origCapture
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "idle",
			SessionState:         "waiting_input",
			ClassificationReason: "interactive_idle_cpu",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "READY_MARKER", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-jsonpath-priority",
			"--project-root", t.TempDir(),
			"--until-jsonpath", "$.sessionState=waiting_input",
			"--until-marker", "READY_MARKER",
			"--max-polls", "2",
			"--poll-interval", "1",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["exitReason"] != "jsonpath_matched" {
		t.Fatalf("expected jsonpath_matched precedence, got %v", payload["exitReason"])
	}
	if _, ok := payload["errorCode"]; ok {
		t.Fatalf("expected no errorCode for successful jsonpath match, got %v", payload["errorCode"])
	}
}

func TestSessionMonitorUntilJSONPathNumericExpectation(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHasSession := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHasSession
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "idle",
			SessionState:         "waiting_input",
			ClassificationReason: "interactive_idle_cpu",
			TodosDone:            0,
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-jsonpath-numeric",
			"--project-root", t.TempDir(),
			"--until-jsonpath", "$.todosDone=0",
			"--max-polls", "2",
			"--poll-interval", "1",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["exitReason"] != "jsonpath_matched" {
		t.Fatalf("expected jsonpath_matched exit reason, got %v", payload["exitReason"])
	}
	if _, ok := payload["errorCode"]; ok {
		t.Fatalf("expected no errorCode for successful jsonpath match, got %v", payload["errorCode"])
	}
}

func TestSessionMonitorUntilJSONPathInvalidExpression(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-jsonpath-invalid",
			"--project-root", t.TempDir(),
			"--until-jsonpath", "$.sessionState=",
			"--json-min",
		})
		if code != 1 {
			t.Fatalf("expected invalid expression failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "invalid_until_jsonpath" {
		t.Fatalf("expected invalid_until_jsonpath, got %v", payload["errorCode"])
	}
}

func TestSessionRouteBudgetPropagatesToRunbook(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "exec",
			"--budget", "123",
			"--emit-runbook",
			"--project-root", t.TempDir(),
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["budget"].(float64) != 123 {
		t.Fatalf("expected budget=123, got %v", payload["budget"])
	}
	runbook := payload["runbook"].(map[string]any)
	steps := runbook["steps"].([]any)
	joined := ""
	for _, s := range steps {
		step := s.(map[string]any)
		joined += step["command"].(string) + "\n"
	}
	if !strings.Contains(joined, "--summary-style ops") || !strings.Contains(joined, "--token-budget 123") {
		t.Fatalf("expected budget-aware runbook commands, got %q", joined)
	}
}

func TestSessionListActiveOnlyWithNextAction(t *testing.T) {
	origList := tmuxListSessionsFn
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origCompute
	})
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-a", "lisa-b"}, nil
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		if session == "lisa-b" {
			return sessionStatus{Session: session, Status: "not_found", SessionState: "not_found"}, nil
		}
		return sessionStatus{Session: session, Status: "idle", SessionState: "waiting_input"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{
			"--project-root", t.TempDir(),
			"--active-only",
			"--with-next-action",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	sessions := payload["sessions"].([]any)
	if len(sessions) != 1 || sessions[0].(string) != "lisa-a" {
		t.Fatalf("expected only lisa-a session, got %v", sessions)
	}
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	if item["nextAction"] != "session send" {
		t.Fatalf("expected nextAction session send, got %v", item["nextAction"])
	}
}

func TestSessionCaptureSummaryStyleOps(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "__LISA_SESSION_DONE__:run:0\noperation error occurred\nlast line", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-cap-style",
			"--project-root", t.TempDir(),
			"--raw",
			"--summary",
			"--summary-style", "ops",
			"--token-budget", "200",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["summaryStyle"] != "ops" {
		t.Fatalf("expected summaryStyle ops, got %v", payload["summaryStyle"])
	}
	if !strings.Contains(payload["summary"].(string), "ops_summary:") {
		t.Fatalf("expected ops summary body, got %q", payload["summary"])
	}
}

func TestSessionSmokeInvalidChaosMode(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{"--project-root", t.TempDir(), "--chaos", "nope", "--json"})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "invalid_chaos_mode" {
		t.Fatalf("expected invalid_chaos_mode, got %v", payload["errorCode"])
	}
}

func TestSessionSmokeMixedDropsMarkerWithoutDroppingSessionLine(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) < 2 {
			return "", "", fmt.Errorf("invalid args")
		}
		switch args[1] {
		case "spawn":
			command := ""
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "--command" {
					command = strings.TrimSpace(args[i+1])
					break
				}
			}
			if !strings.HasPrefix(command, "/bin/bash ") {
				return "", "", fmt.Errorf("unexpected spawn command: %q", command)
			}
			scriptPath := strings.TrimPrefix(command, "/bin/bash ")
			body, err := os.ReadFile(scriptPath)
			if err != nil {
				return "", "", fmt.Errorf("read smoke script: %w", err)
			}
			text := string(body)
			if !strings.Contains(text, "echo LISA_SMOKE_SESSION=") {
				return "", "", fmt.Errorf("expected session line to remain in mixed chaos script")
			}
			if strings.Contains(text, "echo LISA_SMOKE_L1_DONE=1") {
				return "", "", fmt.Errorf("expected mixed chaos to drop marker echo line")
			}
			if !strings.Contains(text, "echo LISA_SMOKE_CHAOS_MIXED_MARKER_DROPPED=1") {
				return "", "", fmt.Errorf("expected mixed chaos drop marker diagnostic line")
			}
			return `{"session":"lisa-smoke-l1","status":"ok"}`, "", nil
		case "monitor":
			return `{"finalState":"completed","exitReason":"completed"}`, "", nil
		case "capture":
			return `{"capture":"LISA_SMOKE_L1_DONE=1"}`, "", nil
		case "kill":
			return `{"ok":true}`, "", nil
		default:
			return "", "", nil
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--levels", "1",
			"--chaos", "mixed",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
}

func TestSessionSmokeDropMarkerReturnsMissingMarkers(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) < 2 {
			return "", "", fmt.Errorf("invalid args")
		}
		switch args[1] {
		case "spawn":
			return `{"session":"lisa-smoke-l1","status":"ok"}`, "", nil
		case "monitor":
			return `{"finalState":"completed","exitReason":"completed"}`, "", nil
		case "capture":
			return `{"capture":"smoke output without required marker"}`, "", nil
		case "kill":
			return `{"ok":true}`, "", nil
		default:
			return "", "", nil
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--levels", "1",
			"--chaos", "drop-marker",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "smoke_marker_assertion_failed" {
		t.Fatalf("expected smoke_marker_assertion_failed, got %v", payload["errorCode"])
	}
	missing := payload["missingMarkers"].([]any)
	if len(missing) != 1 || missing[0] != "LISA_SMOKE_L1_DONE=1" {
		t.Fatalf("expected missing marker payload, got %v", missing)
	}
}

func TestSessionSmokeReportMinIncludesChaos(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) < 2 {
			return "", "", fmt.Errorf("invalid args")
		}
		switch args[1] {
		case "spawn":
			return `{"session":"lisa-smoke-l1","status":"ok"}`, "", nil
		case "monitor":
			return `{"finalState":"completed","exitReason":"completed"}`, "", nil
		case "capture":
			return `{"capture":"missing marker output"}`, "", nil
		case "kill":
			return `{"ok":true}`, "", nil
		default:
			return "", "", nil
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--levels", "1",
			"--chaos", "drop-marker",
			"--report-min",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["chaos"] != "drop-marker" {
		t.Fatalf("expected chaos field in report-min payload, got %v", payload["chaos"])
	}
}

func TestSessionGuardEnforceBlocksMediumRisk(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionGuard([]string{"--shared-tmux", "--enforce", "--command", "./lisa cleanup", "--json"})
		if code != 1 {
			t.Fatalf("expected guard failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "shared_tmux_guard_enforced" {
		t.Fatalf("expected shared_tmux_guard_enforced, got %v", payload["errorCode"])
	}
}

func TestSessionGuardStrictBlocksMediumRiskWithoutEnforce(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionGuard([]string{"--shared-tmux", "--machine-policy", "strict", "--command", "./lisa cleanup", "--json"})
		if code != 1 {
			t.Fatalf("expected guard failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["commandRisk"] != "medium" {
		t.Fatalf("expected medium command risk, got %v", payload["commandRisk"])
	}
	if payload["errorCode"] != "shared_tmux_risk_detected" {
		t.Fatalf("expected shared_tmux_risk_detected, got %v", payload["errorCode"])
	}
}

func TestSkillsDoctorExplainDriftAddsRemediation(t *testing.T) {
	origHome := osUserHomeDirFn
	t.Cleanup(func() { osUserHomeDirFn = origHome })

	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", "lisa")
	if err := os.MkdirAll(filepath.Join(repoSkill, "data"), 0o755); err != nil {
		t.Fatalf("mkdir repo skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkill, "SKILL.md"), []byte("version: 9.9.9\n"), 0o644); err != nil {
		t.Fatalf("write repo skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkill, "data", "commands.md"), []byte("session spawn\n"), 0o644); err != nil {
		t.Fatalf("write repo commands: %v", err)
	}

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	codexSkill := filepath.Join(home, ".codex", "skills", "lisa")
	if err := os.MkdirAll(filepath.Join(codexSkill, "data"), 0o755); err != nil {
		t.Fatalf("mkdir codex skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexSkill, "SKILL.md"), []byte("version: 1.0.0\n"), 0o644); err != nil {
		t.Fatalf("write codex skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexSkill, "data", "commands.md"), []byte("session spawn\n"), 0o644); err != nil {
		t.Fatalf("write codex commands: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--explain-drift", "--json"})
		if code != 1 {
			t.Fatalf("expected doctor drift failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	targets := payload["targets"].([]any)
	if len(targets) == 0 {
		t.Fatalf("expected targets in payload")
	}
	first := targets[0].(map[string]any)
	remediation, ok := first["remediation"].([]any)
	if !ok || len(remediation) == 0 {
		t.Fatalf("expected remediation hints, got %v", first["remediation"])
	}
}
