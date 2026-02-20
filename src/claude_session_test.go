package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncodeClaudePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/joon/projects/tools/lisa", "-Users-joon-projects-tools-lisa"},
		{"/home/user/my.project", "-home-user-my-project"},
		{"/tmp", "-tmp"},
		{".", "-"},
	}
	for _, tc := range tests {
		got := encodeClaudePath(tc.input)
		if got != tc.want {
			t.Errorf("encodeClaudePath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractMessageTextUser(t *testing.T) {
	raw := json.RawMessage(`{"role":"user","content":"hello world"}`)
	got := extractMessageText(raw, "user")
	if got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}

	raw = json.RawMessage(`{"role":"user","content":"  trimmed  "}`)
	got = extractMessageText(raw, "user")
	if got != "trimmed" {
		t.Fatalf("expected 'trimmed', got %q", got)
	}
}

func TestExtractMessageTextAssistant(t *testing.T) {
	raw := json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"hello"},{"type":"thinking","thinking":"secret"},{"type":"text","text":"world"}]}`)
	got := extractMessageText(raw, "assistant")
	if got != "hello\n\nworld" {
		t.Fatalf("expected 'hello\\n\\nworld', got %q", got)
	}

	// Only thinking blocks → empty
	raw = json.RawMessage(`{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"}]}`)
	got = extractMessageText(raw, "assistant")
	if got != "" {
		t.Fatalf("expected empty for thinking-only, got %q", got)
	}
}

func TestReadClaudeTranscript(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "test-session.jsonl")

	lines := []string{
		`{"type":"file-history-snapshot","message":null,"timestamp":"2026-01-01T00:00:00Z"}`,
		`{"type":"user","sessionId":"abc","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"hi there"}]}}`,
		`{"type":"progress","sessionId":"abc","timestamp":"2026-01-01T00:00:03Z","message":null}`,
		`{"type":"user","sessionId":"abc","timestamp":"2026-01-01T00:00:04Z","message":{"role":"user","content":"thanks"}}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatalf("failed to write test jsonl: %v", err)
	}

	messages, err := readClaudeTranscript(jsonlPath)
	if err != nil {
		t.Fatalf("readClaudeTranscript failed: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Text != "hello" {
		t.Fatalf("unexpected first message: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Text != "hi there" {
		t.Fatalf("unexpected second message: %+v", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Text != "thanks" {
		t.Fatalf("unexpected third message: %+v", messages[2])
	}
}

func TestFormatTranscriptPlain(t *testing.T) {
	messages := []transcriptMessage{
		{Role: "user", Text: "hello"},
		{Role: "assistant", Text: "hi there"},
		{Role: "user", Text: "thanks"},
	}
	got := formatTranscriptPlain(messages)
	want := "> hello\n\nhi there\n\n> thanks\n"
	if got != want {
		t.Fatalf("formatTranscriptPlain mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	// Empty
	got = formatTranscriptPlain(nil)
	if got != "" {
		t.Fatalf("expected empty for nil messages, got %q", got)
	}
}

func TestFindClaudeSessionIDFromHistory(t *testing.T) {
	dir := t.TempDir()

	// Create a fake home structure
	claudeDir := filepath.Join(dir, ".claude")
	projectRoot := "/Users/test/myproject"
	projDir := filepath.Join(claudeDir, "projects", encodeClaudePath(projectRoot))
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create history.jsonl
	now := time.Now()
	historyEntry := claudeHistoryEntry{
		Display:   "Say hello",
		Timestamp: now.UnixMilli(),
		Project:   projectRoot,
		SessionID: "found-session-id",
	}
	historyData, _ := json.Marshal(historyEntry)
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	if err := os.WriteFile(historyPath, append(historyData, '\n'), 0o600); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}

	// Create session JSONL in project dir
	sessionJSONL := filepath.Join(projDir, "found-session-id.jsonl")
	if err := os.WriteFile(sessionJSONL, []byte(`{"type":"user","message":{"role":"user","content":"Say hello"},"timestamp":"`+now.Format(time.RFC3339)+`"}`+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write session jsonl: %v", err)
	}

	// Override HOME for the test
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	sessionID, err := findClaudeSessionID(projectRoot, "Say hello", now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("findClaudeSessionID failed: %v", err)
	}
	if sessionID != "found-session-id" {
		t.Fatalf("expected 'found-session-id', got %q", sessionID)
	}
}

func TestCmdSessionCaptureDefaultTranscript(t *testing.T) {
	origFindFn := findClaudeSessionIDFn
	origReadFn := readClaudeTranscriptFn
	origMetaGlobFn := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		findClaudeSessionIDFn = origFindFn
		readClaudeTranscriptFn = origReadFn
		loadSessionMetaByGlobFn = origMetaGlobFn
	})

	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{
			Session:     session,
			Agent:       "claude",
			ProjectRoot: "/tmp/test-project",
			Prompt:      "Say hello",
			CreatedAt:   "2026-01-01T00:00:00Z",
		}, nil
	}
	findClaudeSessionIDFn = func(projectRoot, prompt, createdAt string) (string, error) {
		return "mock-claude-session", nil
	}
	readClaudeTranscriptFn = func(jsonlPath string) ([]transcriptMessage, error) {
		return []transcriptMessage{
			{Role: "user", Text: "Say hello", Timestamp: "2026-01-01T00:00:01Z"},
			{Role: "assistant", Text: "Hello!", Timestamp: "2026-01-01T00:00:02Z"},
		}, nil
	}

	// Default (no --raw) should use transcript
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-transcript",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected transcript capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"claudeSession":"mock-claude-session"`) {
		t.Fatalf("expected claudeSession in output, got %q", stdout)
	}
	if !strings.Contains(stdout, `"role":"user"`) || !strings.Contains(stdout, `"role":"assistant"`) {
		t.Fatalf("expected user and assistant messages in output, got %q", stdout)
	}
}

func TestCmdSessionCaptureDefaultPrefersCWDMetadataWithDuplicateSessionName(t *testing.T) {
	origFindFn := findClaudeSessionIDFn
	origReadFn := readClaudeTranscriptFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		findClaudeSessionIDFn = origFindFn
		readClaudeTranscriptFn = origReadFn
		tmuxHasSessionFn = origHas
	})

	projectRootA := t.TempDir()
	projectRootB := t.TempDir()
	session := "lisa-collision-session"

	metaA := sessionMeta{
		Session:     session,
		Agent:       "claude",
		ProjectRoot: canonicalProjectRoot(projectRootA),
		Prompt:      "Say hello",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	metaB := sessionMeta{
		Session:     session,
		Agent:       "claude",
		ProjectRoot: canonicalProjectRoot(projectRootB),
		Prompt:      "Other prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	if err := saveSessionMeta(projectRootA, session, metaA); err != nil {
		t.Fatalf("failed to save metadata A: %v", err)
	}
	if err := saveSessionMeta(projectRootB, session, metaB); err != nil {
		t.Fatalf("failed to save metadata B: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(sessionMetaFile(projectRootA, session))
		_ = os.Remove(sessionMetaFile(projectRootB, session))
	})

	findClaudeSessionIDFn = func(projectRoot, prompt, createdAt string) (string, error) {
		if canonicalProjectRoot(projectRoot) != canonicalProjectRoot(projectRootA) {
			t.Fatalf("expected cwd project root, got %q", projectRoot)
		}
		return "cwd-session-id", nil
	}
	readClaudeTranscriptFn = func(jsonlPath string) ([]transcriptMessage, error) {
		return []transcriptMessage{
			{Role: "assistant", Text: "Hello from cwd", Timestamp: "2026-01-01T00:00:01Z"},
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read cwd: %v", err)
	}
	if err := os.Chdir(projectRootA); err != nil {
		t.Fatalf("failed to chdir to project root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", session,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected transcript capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"claudeSession":"cwd-session-id"`) {
		t.Fatalf("expected cwd transcript capture, got %q", stdout)
	}
}

func TestCmdSessionCaptureRawFlag(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "raw pane output", nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-raw",
			"--raw",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected raw capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"capture":"raw pane output"`) {
		t.Fatalf("expected raw capture in output, got %q", stdout)
	}
}

func TestCmdSessionCaptureStripsNoiseByDefault(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return strings.Join([]string{
			"2026-02-19T00:00:00Z  WARN codex_state::runtime: failed to open state db at /Users/test/.codex/state_5.sqlite: migration mismatch",
			"mcp startup: ready: context7, linear; failed: notion",
			"real output line",
		}, "\n"), nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-noise",
			"--raw",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected filtered raw capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.Contains(stdout, "codex_state::runtime") || strings.Contains(stdout, "mcp startup:") {
		t.Fatalf("expected noise lines to be removed, got %q", stdout)
	}
	if !strings.Contains(stdout, "real output line") {
		t.Fatalf("expected non-noise line to remain, got %q", stdout)
	}
}

func TestCmdSessionCaptureKeepNoise(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return strings.Join([]string{
			"2026-02-19T00:00:00Z  WARN codex_state::runtime: failed to open state db at /Users/test/.codex/state_5.sqlite: migration mismatch",
			"real output line",
		}, "\n"), nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-noise-keep",
			"--raw",
			"--keep-noise",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected keep-noise raw capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, "codex_state::runtime") {
		t.Fatalf("expected noise line to remain with --keep-noise, got %q", stdout)
	}
	if !strings.Contains(stdout, "real output line") {
		t.Fatalf("expected non-noise line to remain, got %q", stdout)
	}
}

func TestCmdSessionCaptureStripsCodexAuthRefreshNoise(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return strings.Join([]string{
			"2026-02-20T06:21:24.946557Z ERROR rmcp::transport::worker: worker quit with fatal: Transport channel closed, when Auth(TokenRefreshFailed(\"Server returned error response: invalid_grant: Invalid refresh token\"))",
			"_client::StreamableHttpClientWorker<rmcp::transport::auth::AuthClient<reqwest::async_impl::client::Client>>>] error: Auth error: OAuth token refresh failed: Server returned error response: invalid_grant: Invalid refresh",
			"token, when send initialize request",
			"real output line",
		}, "\n"), nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-auth-noise",
			"--raw",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected filtered raw capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.Contains(stdout, "invalid_grant") || strings.Contains(stdout, "rmcp::transport::worker") || strings.Contains(stdout, "when send initialize request") {
		t.Fatalf("expected auth refresh noise lines to be removed, got %q", stdout)
	}
	if !strings.Contains(stdout, "real output line") {
		t.Fatalf("expected non-noise line to remain, got %q", stdout)
	}
}

func TestCmdSessionCaptureDefaultFallsBackToRawForNonClaude(t *testing.T) {
	origMetaGlobFn := loadSessionMetaByGlobFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMetaGlobFn
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{
			Session:     session,
			Agent:       "codex",
			ProjectRoot: "/tmp/test-project",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "raw pane output", nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-default-codex-capture",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected raw capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"capture":"raw pane output"`) {
		t.Fatalf("expected raw capture in output, got %q", stdout)
	}
	if strings.Contains(stdout, `"claudeSession"`) {
		t.Fatalf("did not expect transcript output for non-claude session, got %q", stdout)
	}
}

func TestCmdSessionCaptureDefaultFallsBackToRawWhenTranscriptReadFails(t *testing.T) {
	origMetaGlobFn := loadSessionMetaByGlobFn
	origFindFn := findClaudeSessionIDFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMetaGlobFn
		findClaudeSessionIDFn = origFindFn
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{
			Session:     session,
			Agent:       "claude",
			ProjectRoot: "/tmp/test-project",
			Prompt:      "Say hello",
			CreatedAt:   "2026-01-01T00:00:00Z",
		}, nil
	}
	findClaudeSessionIDFn = func(projectRoot, prompt, createdAt string) (string, error) {
		return "", fmt.Errorf("no transcript session")
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "raw pane output", nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-transcript-fallback",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected raw fallback capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"capture":"raw pane output"`) {
		t.Fatalf("expected raw fallback capture in output, got %q", stdout)
	}
	if strings.Contains(stdout, `"claudeSession"`) {
		t.Fatalf("did not expect transcript output on fallback, got %q", stdout)
	}
}

func TestCheckTranscriptTurnCompleteTextBlock(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, ".claude", "projects", encodeClaudePath("/test/project"))
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	jsonlPath := filepath.Join(projDir, "test-session.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-01-01T00:00:01Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]},"timestamp":"2026-01-01T00:00:02Z"}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write jsonl: %v", err)
	}
	// Backdate mtime so fileAge >= 3
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(jsonlPath, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, fileAge, sid, err := checkTranscriptTurnComplete("/test/project", "", "", "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !turnComplete {
		t.Fatal("expected turnComplete=true for assistant text block")
	}
	if fileAge < 3 {
		t.Fatalf("expected fileAge >= 3, got %d", fileAge)
	}
	if sid != "test-session" {
		t.Fatalf("expected sid 'test-session', got %q", sid)
	}
}

func TestCheckTranscriptTurnCompleteToolUse(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, ".claude", "projects", encodeClaudePath("/test/project"))
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	jsonlPath := filepath.Join(projDir, "mid-turn.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"fix the bug"},"timestamp":"2026-01-01T00:00:01Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{}}]},"timestamp":"2026-01-01T00:00:02Z"}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write jsonl: %v", err)
	}
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(jsonlPath, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, _, _, err := checkTranscriptTurnComplete("/test/project", "", "", "mid-turn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turnComplete {
		t.Fatal("expected turnComplete=false for tool_use-only assistant")
	}
}

func TestCheckTranscriptTurnCompleteSkipsProgress(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, ".claude", "projects", encodeClaudePath("/test/project"))
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	jsonlPath := filepath.Join(projDir, "progress-trail.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-01-01T00:00:01Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done."}]},"timestamp":"2026-01-01T00:00:02Z"}`,
		`{"type":"progress","message":null,"timestamp":"2026-01-01T00:00:03Z"}`,
		`{"type":"system","message":null,"timestamp":"2026-01-01T00:00:04Z"}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write jsonl: %v", err)
	}
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(jsonlPath, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, _, _, err := checkTranscriptTurnComplete("/test/project", "", "", "progress-trail")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !turnComplete {
		t.Fatal("expected turnComplete=true — progress/system entries should be skipped")
	}
}

func TestCheckTranscriptTurnCompleteFreshFile(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, ".claude", "projects", encodeClaudePath("/test/project"))
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	jsonlPath := filepath.Join(projDir, "fresh.jsonl")
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]},"timestamp":"2026-01-01T00:00:02Z"}`,
	}
	// Write with current mtime (fresh)
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write jsonl: %v", err)
	}

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, fileAge, _, err := checkTranscriptTurnComplete("/test/project", "", "", "fresh")
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

func TestCheckTranscriptTurnCompleteUserLast(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, ".claude", "projects", encodeClaudePath("/test/project"))
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	jsonlPath := filepath.Join(projDir, "user-last.jsonl")
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Sure."}]},"timestamp":"2026-01-01T00:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":"do more"},"timestamp":"2026-01-01T00:00:02Z"}`,
	}
	if err := os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write jsonl: %v", err)
	}
	past := time.Now().Add(-10 * time.Second)
	os.Chtimes(jsonlPath, past, past)

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)

	turnComplete, _, _, err := checkTranscriptTurnComplete("/test/project", "", "", "user-last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turnComplete {
		t.Fatal("expected turnComplete=false when last meaningful entry is user")
	}
}

func TestLoadSessionMetaByGlob(t *testing.T) {
	dir := t.TempDir()
	session := fmt.Sprintf("lisa-test-glob-session-%d", time.Now().UnixNano())
	aid := sessionArtifactID(session)

	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "interactive",
		ProjectRoot: "/tmp/test-project",
		Prompt:      "test prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal meta: %v", err)
	}

	// Write to a path matching the glob pattern
	metaPath := filepath.Join(dir, ".lisa-abcd1234-session-"+aid+"-meta.json")
	if err := os.WriteFile(metaPath, data, 0o600); err != nil {
		t.Fatalf("failed to write meta file: %v", err)
	}

	// loadSessionMetaByGlob uses /tmp/ pattern, so we test the function directly
	// by writing to /tmp/ temporarily
	tmpMeta := filepath.Join("/tmp", filepath.Base(metaPath))
	if err := os.WriteFile(tmpMeta, data, 0o600); err != nil {
		t.Fatalf("failed to write tmp meta file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpMeta) })

	got, err := loadSessionMetaByGlob(session)
	if err != nil {
		t.Fatalf("loadSessionMetaByGlob failed: %v", err)
	}
	if got.Session != session {
		t.Fatalf("expected session %q, got %q", session, got.Session)
	}
	if got.ProjectRoot != "/tmp/test-project" {
		t.Fatalf("expected project root /tmp/test-project, got %q", got.ProjectRoot)
	}
}

func TestLoadSessionMetaByGlobRejectsAmbiguousProjects(t *testing.T) {
	session := fmt.Sprintf("lisa-test-glob-ambiguous-%d", time.Now().UnixNano())
	aid := sessionArtifactID(session)

	metaOne := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "interactive",
		ProjectRoot: "/tmp/project-one",
		Prompt:      "test prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	dataOne, err := json.MarshalIndent(metaOne, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal first meta: %v", err)
	}
	pathOne := filepath.Join("/tmp", ".lisa-abcd1111-session-"+aid+"-meta.json")
	if err := os.WriteFile(pathOne, dataOne, 0o600); err != nil {
		t.Fatalf("failed to write first meta file: %v", err)
	}
	t.Cleanup(func() { os.Remove(pathOne) })

	metaTwo := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "interactive",
		ProjectRoot: "/tmp/project-two",
		Prompt:      "test prompt",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	dataTwo, err := json.MarshalIndent(metaTwo, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal second meta: %v", err)
	}
	pathTwo := filepath.Join("/tmp", ".lisa-abcd2222-session-"+aid+"-meta.json")
	if err := os.WriteFile(pathTwo, dataTwo, 0o600); err != nil {
		t.Fatalf("failed to write second meta file: %v", err)
	}
	t.Cleanup(func() { os.Remove(pathTwo) })

	_, err = loadSessionMetaByGlob(session)
	if err == nil {
		t.Fatal("expected ambiguous-project metadata lookup error")
	}
	if !strings.Contains(err.Error(), "multiple metadata files found") {
		t.Fatalf("expected ambiguity error, got: %v", err)
	}
}
