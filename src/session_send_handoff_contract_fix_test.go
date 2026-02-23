package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCmdSessionSendPrependsObjectiveReminderWhenGoalEmpty(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendText := tmuxSendTextFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendTextFn = origSendText
		appendSessionEventFn = origAppend
	})

	projectRoot := t.TempDir()
	session := "lisa-send-objective-prefix"
	meta := sessionMeta{
		Session:             session,
		ProjectRoot:         projectRoot,
		Lane:                "planner",
		ObjectiveID:         "obj-42",
		ObjectiveGoal:       "",
		ObjectiveAcceptance: "Definition done",
		ObjectiveBudget:     120,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	tmuxHasSessionFn = func(session string) bool { return true }
	sentText := ""
	tmuxSendTextFn = func(session, text string, enter bool) error {
		sentText = text
		return nil
	}
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error { return nil }

	_, stderr := captureOutput(t, func() {
		code := cmdSessionSend([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--text", "Continue with execution",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	wantPrefix := "Objective reminder: id=obj-42 | acceptance=Definition done | budget=120 | lane=planner"
	if !strings.HasPrefix(sentText, wantPrefix+"\n") {
		t.Fatalf("expected objective reminder prefix, got %q", sentText)
	}
}

func TestCmdSessionSendSkipsDuplicateObjectiveReminderWhenGoalEmpty(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendText := tmuxSendTextFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendTextFn = origSendText
		appendSessionEventFn = origAppend
	})

	projectRoot := t.TempDir()
	session := "lisa-send-objective-prefix-duplicate"
	meta := sessionMeta{
		Session:             session,
		ProjectRoot:         projectRoot,
		ObjectiveID:         "obj-42",
		ObjectiveGoal:       "",
		ObjectiveAcceptance: "Definition done",
		ObjectiveBudget:     120,
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	tmuxHasSessionFn = func(session string) bool { return true }
	sentText := ""
	tmuxSendTextFn = func(session, text string, enter bool) error {
		sentText = text
		return nil
	}
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error { return nil }

	input := "Objective reminder: id=obj-42 | acceptance=Definition done | budget=120\nContinue with execution"
	_, stderr := captureOutput(t, func() {
		code := cmdSessionSend([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--text", input,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if sentText != input {
		t.Fatalf("expected reminder not duplicated, got %q", sentText)
	}
}

func TestCmdSessionHandoffRejectsSchemaV1WhenLaneRequiresV2(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-handoff-contract-v2"
	meta := sessionMeta{
		Session:     session,
		ProjectRoot: projectRoot,
		Lane:        "planner",
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

	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		t.Fatalf("status compute should not run when schema v1 is blocked by lane contract")
		return sessionStatus{}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--schema", "v1",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected schema rejection")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	if payload["errorCode"] != "handoff_schema_v2_required" {
		t.Fatalf("expected handoff_schema_v2_required, got %v", payload["errorCode"])
	}
	if !strings.Contains(strings.ToLower(mapStringValue(payload, "error")), "--schema v2") {
		t.Fatalf("expected error text to include --schema v2 guidance, got %v", payload["error"])
	}

	_, stderr = captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--schema", "v1",
		})
		if code == 0 {
			t.Fatalf("expected schema rejection in text mode")
		}
	})
	if !strings.Contains(stderr, "handoff_schema_v2_required") {
		t.Fatalf("expected text error to contain error code, got %q", stderr)
	}
}
