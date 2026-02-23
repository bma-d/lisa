package app

import (
	"os"
	"strings"
	"testing"
)

func TestShouldRecordInputTimestamp(t *testing.T) {
	if shouldRecordInputTimestamp("hello", nil, false) {
		t.Fatal("text without enter should not be treated as submitted input")
	}
	if !shouldRecordInputTimestamp("hello", nil, true) {
		t.Fatal("text with enter should be treated as submitted input")
	}
	if shouldRecordInputTimestamp("", []string{"Escape"}, false) {
		t.Fatal("escape key should not be treated as submitted input")
	}
	if !shouldRecordInputTimestamp("", []string{"Enter"}, false) {
		t.Fatal("enter key should be treated as submitted input")
	}
}

func TestCmdSessionSendRecordsSubmittedInputTimestamp(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendText := tmuxSendTextFn
	origSendKeys := tmuxSendKeysFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendTextFn = origSendText
		tmuxSendKeysFn = origSendKeys
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxSendTextFn = func(session, text string, enter bool) error { return nil }
	tmuxSendKeysFn = func(session string, keys []string, enter bool) error { return nil }
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error { return nil }

	projectRoot := t.TempDir()
	session := "lisa-send-record-input"
	statePath := sessionStateFile(projectRoot, session)
	t.Cleanup(func() { _ = os.Remove(statePath) })

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSend([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--text", "hello",
			"--enter",
			"--json",
		}); code != 0 {
			t.Fatalf("expected send text success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	state, err := loadSessionStateWithError(statePath)
	if err != nil {
		t.Fatalf("failed to read state after send: %v", err)
	}
	if state.LastInputAt <= 0 || state.LastInputAtNanos <= 0 {
		t.Fatalf("expected submitted input timestamp to be recorded, got %+v", state)
	}
	firstNanos := state.LastInputAtNanos

	_, stderr = captureOutput(t, func() {
		if code := cmdSessionSend([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--keys", "Escape",
			"--json",
		}); code != 0 {
			t.Fatalf("expected send keys success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	state, err = loadSessionStateWithError(statePath)
	if err != nil {
		t.Fatalf("failed to read state after non-submitted key send: %v", err)
	}
	if state.LastInputAtNanos != firstNanos {
		t.Fatalf("expected non-submitted key send to keep input timestamp unchanged, before=%d after=%d", firstNanos, state.LastInputAtNanos)
	}
}

func TestCmdSessionSendSplitsCodexInteractiveSubmit(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendText := tmuxSendTextFn
	origSendKeys := tmuxSendKeysFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendTextFn = origSendText
		tmuxSendKeysFn = origSendKeys
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error { return nil }

	projectRoot := t.TempDir()
	session := "lisa-send-codex-split-submit"
	meta := sessionMeta{
		Session:             session,
		ProjectRoot:         projectRoot,
		Agent:               "codex",
		Mode:                "interactive",
		ObjectiveID:         "obj-1",
		ObjectiveAcceptance: "ok",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	var sentText string
	var sentTextEnter bool
	tmuxSendTextFn = func(session, text string, enter bool) error {
		sentText = text
		sentTextEnter = enter
		return nil
	}
	enterCount := 0
	tmuxSendKeysFn = func(session string, keys []string, enter bool) error {
		if len(keys) == 1 && strings.EqualFold(keys[0], "Enter") && !enter {
			enterCount++
		}
		return nil
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSend([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--text", "Continue",
			"--enter",
			"--json",
		}); code != 0 {
			t.Fatalf("expected send text success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if sentText == "" {
		t.Fatal("expected text payload to be sent")
	}
	if sentTextEnter {
		t.Fatalf("expected codex split-submit path to send text without inline enter, got enter=%v", sentTextEnter)
	}
	if enterCount != 1 {
		t.Fatalf("expected one explicit enter send for codex split-submit path, got %d", enterCount)
	}
}
