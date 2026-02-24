package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdSessionHandoffSchemaV4IncludesTypedNextAction(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-handoff-v4"

	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:              sessionName,
			Status:               "idle",
			SessionState:         "waiting_input",
			ClassificationReason: "interactive_idle_cpu",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--schema", "v4",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	payload := parseJSONMap(t, stdout)
	if payload["schema"] != "v4" {
		t.Fatalf("expected schema v4, got %v", payload["schema"])
	}
	nextAction, ok := payload["nextAction"].(map[string]any)
	if !ok {
		t.Fatalf("expected object nextAction, got %T (%v)", payload["nextAction"], payload["nextAction"])
	}
	if !strings.HasPrefix(mapStringValue(nextAction, "id"), "hid-") {
		t.Fatalf("expected deterministic id in nextAction, got %v", nextAction)
	}
	if mapStringValue(nextAction, "name") != "session send" {
		t.Fatalf("expected session send next action, got %v", nextAction["name"])
	}
	commandAst, ok := nextAction["commandAst"].(map[string]any)
	if !ok {
		t.Fatalf("expected commandAst object, got %T (%v)", nextAction["commandAst"], nextAction["commandAst"])
	}
	if mapStringValue(commandAst, "schema") != "lisa.command.ast.v1" {
		t.Fatalf("expected commandAst schema lisa.command.ast.v1, got %v", commandAst["schema"])
	}
	args, ok := commandAst["args"].([]any)
	if !ok || len(args) == 0 {
		t.Fatalf("expected non-empty typed args, got %v", commandAst["args"])
	}
	foundSession := false
	foundEnter := false
	for _, raw := range args {
		arg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch mapStringValue(arg, "name") {
		case "session":
			if mapStringValue(arg, "type") == "string" && mapStringValue(arg, "value") == session {
				foundSession = true
			}
		case "enter":
			if mapStringValue(arg, "type") == "bool" {
				if value, ok := arg["value"].(bool); ok && value {
					foundEnter = true
				}
			}
		}
	}
	if !foundSession {
		t.Fatalf("expected typed session arg in commandAst args, got %v", args)
	}
	if !foundEnter {
		t.Fatalf("expected typed enter=true bool arg in commandAst args, got %v", args)
	}
}

func TestCmdSessionRouteFromStatePreservesTypedNextAction(t *testing.T) {
	projectRoot := t.TempDir()
	fromStatePath := filepath.Join(projectRoot, "from-state-v4.json")
	raw := `{"session":"lisa-route-from-state","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextAction":{"id":"hid-fixed","name":"session send","command":"./lisa session send --session 'lisa-route-from-state' --json-min","commandAst":{"schema":"lisa.command.ast.v1","binary":"./lisa","subcommands":["session","send"],"args":[{"name":"session","flag":"--session","type":"string","value":"lisa-route-from-state"},{"name":"enter","flag":"--enter","type":"bool","value":true},{"name":"jsonMin","flag":"--json-min","type":"bool","value":true}]}}}`
	if err := os.WriteFile(fromStatePath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing --from-state payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "analysis",
			"--project-root", projectRoot,
			"--from-state", fromStatePath,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	payload := parseJSONMap(t, stdout)
	fromState, ok := payload["fromState"].(map[string]any)
	if !ok {
		t.Fatalf("expected fromState object, got %T (%v)", payload["fromState"], payload["fromState"])
	}
	nextAction, ok := fromState["nextAction"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured nextAction object, got %T (%v)", fromState["nextAction"], fromState["nextAction"])
	}
	commandAst, ok := nextAction["commandAst"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured commandAst object, got %T (%v)", nextAction["commandAst"], nextAction["commandAst"])
	}
	args, ok := commandAst["args"].([]any)
	if !ok || len(args) != 3 {
		t.Fatalf("expected 3 typed args in commandAst, got %v", commandAst["args"])
	}
	enterArgOK := false
	for _, rawArg := range args {
		arg, ok := rawArg.(map[string]any)
		if !ok {
			continue
		}
		if mapStringValue(arg, "name") == "enter" {
			if mapStringValue(arg, "type") == "bool" {
				if value, ok := arg["value"].(bool); ok && value {
					enterArgOK = true
				}
			}
		}
	}
	if !enterArgOK {
		t.Fatalf("expected bool typed enter arg preserved, got %v", args)
	}
	prompt := mapStringValue(payload, "prompt")
	if strings.Contains(prompt, "map[") {
		t.Fatalf("expected prompt without map stringification, got %q", prompt)
	}
	if !strings.Contains(prompt, "Recommended next action: session send") {
		t.Fatalf("expected prompt to include structured next action summary, got %q", prompt)
	}
}

func TestCmdSessionRouteFromStateStrictAcceptsCommandASTArgsObject(t *testing.T) {
	projectRoot := t.TempDir()
	fromStatePath := filepath.Join(projectRoot, "from-state-args-object.json")
	raw := `{"session":"lisa-route-args-map","status":"idle","sessionState":"waiting_input","reason":"interactive_idle_cpu","nextAction":{"name":"session send","commandAst":{"schema":"lisa.command.ast.v1","binary":"./lisa","subcommands":["session","send"],"args":{"session":"lisa-route-args-map","text":"continue","enter":true}}}}`
	if err := os.WriteFile(fromStatePath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing --from-state payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "analysis",
			"--project-root", projectRoot,
			"--from-state", fromStatePath,
			"--strict",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected strict route success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	prompt := mapStringValue(payload, "prompt")
	if strings.Contains(prompt, "map[") {
		t.Fatalf("expected prompt without map stringification, got %q", prompt)
	}
	if !strings.Contains(prompt, "Recommended command: ./lisa session send") {
		t.Fatalf("expected prompt command from commandAst map args, got %q", prompt)
	}
}

func TestCmdSessionRouteFromStateStrictRejectsMalformedInput(t *testing.T) {
	projectRoot := t.TempDir()
	fromStatePath := filepath.Join(projectRoot, "from-state-invalid.json")
	raw := `{"session":"lisa-route-strict","sessionState":"waiting_input","nextAction":{"name":42}}`
	if err := os.WriteFile(fromStatePath, []byte(raw), 0o644); err != nil {
		t.Fatalf("failed writing malformed --from-state payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "analysis",
			"--project-root", projectRoot,
			"--from-state", fromStatePath,
			"--strict",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected strict validation failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "invalid_from_state_strict" {
		t.Fatalf("expected invalid_from_state_strict, got %v", payload["errorCode"])
	}
	if !strings.Contains(mapStringValue(payload, "error"), "nextAction.name must be a string") {
		t.Fatalf("expected precise nextAction.name type error, got %v", payload["error"])
	}
}
