package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindCodexSessionIDFromHistory(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("failed to create codex dir: %v", err)
	}

	now := time.Now()
	entries := []codexHistoryEntry{
		{SessionID: "old-session", Ts: now.Add(-10 * time.Minute).Unix(), Text: "some old prompt"},
		{SessionID: "target-session", Ts: now.Unix(), Text: "Say hello to the world"},
	}
	var lines []string
	for _, e := range entries {
		data, _ := json.Marshal(e)
		lines = append(lines, string(data))
	}
	historyPath := filepath.Join(codexDir, "history.jsonl")
	if err := os.WriteFile(historyPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	sessionID, err := findCodexSessionID("Say hello to the world", now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("findCodexSessionID failed: %v", err)
	}
	if sessionID != "target-session" {
		t.Fatalf("expected 'target-session', got %q", sessionID)
	}
}

func TestFindCodexSessionFile(t *testing.T) {
	dir := t.TempDir()
	sessionID := "abc123-def456"

	// Create session file in sessions dir
	sessDir := filepath.Join(dir, ".codex", "sessions", "2026", "02", "12")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}
	sessFile := filepath.Join(sessDir, "rollout-1707700000-"+sessionID+".jsonl")
	if err := os.WriteFile(sessFile, []byte(`{"type":"session_meta"}`+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	path, err := findCodexSessionFile(sessionID)
	if err != nil {
		t.Fatalf("findCodexSessionFile failed: %v", err)
	}
	if path != sessFile {
		t.Fatalf("expected %q, got %q", sessFile, path)
	}
}

func TestFindCodexSessionFileArchived(t *testing.T) {
	dir := t.TempDir()
	sessionID := "archived-uuid-123"

	archDir := filepath.Join(dir, ".codex", "archived_sessions")
	if err := os.MkdirAll(archDir, 0o755); err != nil {
		t.Fatalf("failed to create archived dir: %v", err)
	}
	archFile := filepath.Join(archDir, "rollout-1707700000-"+sessionID+".jsonl")
	if err := os.WriteFile(archFile, []byte(`{"type":"session_meta"}`+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write archived session file: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	path, err := findCodexSessionFile(sessionID)
	if err != nil {
		t.Fatalf("findCodexSessionFile failed: %v", err)
	}
	if path != archFile {
		t.Fatalf("expected %q, got %q", archFile, path)
	}
}

func TestCheckCodexTranscriptTurnCompleteAssistantMessage(t *testing.T) {
	dir := t.TempDir()
	sessionID := "codex-test-session"

	sessDir := filepath.Join(dir, ".codex", "sessions", "2026", "02", "12")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	// Create history for session ID lookup
	codexDir := filepath.Join(dir, ".codex")
	now := time.Now()
	histEntry := codexHistoryEntry{SessionID: sessionID, Ts: now.Unix(), Text: "test prompt"}
	histData, _ := json.Marshal(histEntry)
	if err := os.WriteFile(filepath.Join(codexDir, "history.jsonl"), append(histData, '\n'), 0o600); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}

	// Create session file with assistant response_item
	sessFile := filepath.Join(sessDir, "rollout-1707700000-"+sessionID+".jsonl")
	payload := codexResponseItemPayload{
		Type: "message",
		Role: "assistant",
		Content: []codexContentBlock{
			{Type: "output_text", Text: "Done!"},
		},
	}
	payloadJSON, _ := json.Marshal(payload)
	entry := codexJSONLEntry{Timestamp: now.Format(time.RFC3339), Type: "response_item", Payload: payloadJSON}
	entryJSON, _ := json.Marshal(entry)
	if err := os.WriteFile(sessFile, append(entryJSON, '\n'), 0o600); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
	// Backdate mtime
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(sessFile, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, fileAge, sid, err := checkCodexTranscriptTurnComplete("test prompt", now.Format(time.RFC3339), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !turnComplete {
		t.Fatal("expected turnComplete=true for assistant response_item with output_text")
	}
	if fileAge < 3 {
		t.Fatalf("expected fileAge >= 3, got %d", fileAge)
	}
	if sid != sessionID {
		t.Fatalf("expected sid %q, got %q", sessionID, sid)
	}
}

func TestCheckCodexTranscriptTurnCompleteFunctionCall(t *testing.T) {
	dir := t.TempDir()
	sessionID := "codex-func-call"

	sessDir := filepath.Join(dir, ".codex", "sessions", "2026", "02", "12")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	sessFile := filepath.Join(sessDir, "rollout-1707700000-"+sessionID+".jsonl")
	payload := codexResponseItemPayload{Type: "function_call", Role: "assistant"}
	payloadJSON, _ := json.Marshal(payload)
	entry := codexJSONLEntry{Timestamp: "2026-01-01T00:00:02Z", Type: "response_item", Payload: payloadJSON}
	entryJSON, _ := json.Marshal(entry)
	if err := os.WriteFile(sessFile, append(entryJSON, '\n'), 0o600); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(sessFile, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, _, _, err := checkCodexTranscriptTurnComplete("", "", sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turnComplete {
		t.Fatal("expected turnComplete=false for function_call response_item")
	}
}

func TestCheckCodexTranscriptTurnCompleteSkipsTokenCount(t *testing.T) {
	dir := t.TempDir()
	sessionID := "codex-skip-token"

	sessDir := filepath.Join(dir, ".codex", "sessions", "2026", "02", "12")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	sessFile := filepath.Join(sessDir, "rollout-1707700000-"+sessionID+".jsonl")

	// Assistant message followed by token_count event_msg
	assistPayload := codexResponseItemPayload{
		Type: "message",
		Role: "assistant",
		Content: []codexContentBlock{
			{Type: "output_text", Text: "All done."},
		},
	}
	assistPayloadJSON, _ := json.Marshal(assistPayload)
	assistEntry := codexJSONLEntry{Timestamp: "2026-01-01T00:00:02Z", Type: "response_item", Payload: assistPayloadJSON}
	assistJSON, _ := json.Marshal(assistEntry)

	tokenPayload := codexEventMsgPayload{Type: "token_count"}
	tokenPayloadJSON, _ := json.Marshal(tokenPayload)
	tokenEntry := codexJSONLEntry{Timestamp: "2026-01-01T00:00:03Z", Type: "event_msg", Payload: tokenPayloadJSON}
	tokenJSON, _ := json.Marshal(tokenEntry)

	content := string(assistJSON) + "\n" + string(tokenJSON) + "\n"
	if err := os.WriteFile(sessFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(sessFile, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, _, _, err := checkCodexTranscriptTurnComplete("", "", sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !turnComplete {
		t.Fatal("expected turnComplete=true — token_count should be skipped, finding assistant message behind it")
	}
}

func TestCheckCodexTranscriptTurnCompleteSkipsTaskComplete(t *testing.T) {
	dir := t.TempDir()
	sessionID := "codex-skip-task-complete"

	sessDir := filepath.Join(dir, ".codex", "sessions", "2026", "02", "12")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	sessFile := filepath.Join(sessDir, "rollout-1707700000-"+sessionID+".jsonl")

	assistPayload := codexResponseItemPayload{
		Type: "message",
		Role: "assistant",
		Content: []codexContentBlock{
			{Type: "output_text", Text: "All done."},
		},
	}
	assistPayloadJSON, _ := json.Marshal(assistPayload)
	assistEntry := codexJSONLEntry{Timestamp: "2026-01-01T00:00:02Z", Type: "response_item", Payload: assistPayloadJSON}
	assistJSON, _ := json.Marshal(assistEntry)

	taskCompletePayload := codexEventMsgPayload{Type: "task_complete"}
	taskCompletePayloadJSON, _ := json.Marshal(taskCompletePayload)
	taskCompleteEntry := codexJSONLEntry{Timestamp: "2026-01-01T00:00:03Z", Type: "event_msg", Payload: taskCompletePayloadJSON}
	taskCompleteJSON, _ := json.Marshal(taskCompleteEntry)

	content := string(assistJSON) + "\n" + string(taskCompleteJSON) + "\n"
	if err := os.WriteFile(sessFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(sessFile, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, _, _, err := checkCodexTranscriptTurnComplete("", "", sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !turnComplete {
		t.Fatal("expected turnComplete=true — task_complete should be skipped, finding assistant message behind it")
	}
}

func TestCheckCodexTranscriptTurnCompleteFreshFile(t *testing.T) {
	dir := t.TempDir()
	sessionID := "codex-fresh"

	sessDir := filepath.Join(dir, ".codex", "sessions", "2026", "02", "12")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}

	sessFile := filepath.Join(sessDir, "rollout-1707700000-"+sessionID+".jsonl")
	payload := codexResponseItemPayload{
		Type: "message",
		Role: "assistant",
		Content: []codexContentBlock{
			{Type: "output_text", Text: "Done."},
		},
	}
	payloadJSON, _ := json.Marshal(payload)
	entry := codexJSONLEntry{Timestamp: "2026-01-01T00:00:02Z", Type: "response_item", Payload: payloadJSON}
	entryJSON, _ := json.Marshal(entry)
	// Write with current mtime (fresh)
	if err := os.WriteFile(sessFile, append(entryJSON, '\n'), 0o600); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, fileAge, _, err := checkCodexTranscriptTurnComplete("", "", sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turnComplete {
		t.Fatal("expected turnComplete=false for fresh file (age < 3)")
	}
	if fileAge >= 3 {
		t.Fatalf("expected fileAge < 3 for just-written file, got %d", fileAge)
	}
}
