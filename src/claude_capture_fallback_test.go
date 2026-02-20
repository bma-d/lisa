package app

import (
	"strings"
	"testing"
)

func TestCmdSessionCaptureDefaultFallsBackToRawWhenMetadataPromptMissing(t *testing.T) {
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
			Prompt:      "",
			CreatedAt:   "2026-01-01T00:00:00Z",
		}, nil
	}
	findClaudeSessionIDFn = func(projectRoot, prompt, createdAt string) (string, error) {
		t.Fatalf("findClaudeSessionID should not be called when prompt is missing")
		return "", nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "raw pane output", nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-test-transcript-missing-prompt",
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
		t.Fatalf("did not expect transcript output when metadata prompt is missing, got %q", stdout)
	}
}
