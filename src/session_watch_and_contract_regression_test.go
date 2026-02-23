package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func TestCmdSessionListWatchForcesJSONMin(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	calls := make([][]string, 0, 1)
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		copied := append([]string{}, args...)
		calls = append(calls, copied)
		return `{"count":0,"delta":{"count":0},"sessions":[]}`, "", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{
			"--watch-json",
			"--watch-interval", "2",
			"--watch-cycles", "1",
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
	if !strings.Contains(stdout, `"delta":{"count":0}`) {
		t.Fatalf("expected watch output passthrough, got %q", stdout)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one watch call, got %d", len(calls))
	}
	args := calls[0]
	if len(args) < 2 || args[0] != "session" || args[1] != "list" {
		t.Fatalf("expected session list invocation, got %v", args)
	}
	if !hasArg(args, "--delta-json") {
		t.Fatalf("expected --delta-json enforced in watch mode, got %v", args)
	}
	if !hasArg(args, "--json-min") {
		t.Fatalf("expected --json-min enforced in watch mode, got %v", args)
	}
	if hasArg(args, "--watch-json") || hasArg(args, "--watch-interval") || hasArg(args, "--watch-cycles") {
		t.Fatalf("watch control flags should not be forwarded, got %v", args)
	}
}

func TestCmdSessionHandoffCompressZstdOmitsRecent(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
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
	session := "lisa-handoff-compress"
	eventsPath := sessionEventsFile(root, session)
	if err := os.WriteFile(eventsPath, []byte(strings.Join([]string{
		`{"at":"t1","type":"snapshot","session":"lisa-handoff-compress","state":"in_progress","status":"active","reason":"r1"}`,
	}, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("failed writing events: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", root,
			"--delta-from", "0",
			"--compress", "zstd",
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
	if payload["compression"] != "zstd" {
		t.Fatalf("expected zstd compression marker, got %v", payload["compression"])
	}
	if payload["encoding"] != "base64-gzip" {
		t.Fatalf("expected base64-gzip encoding, got %v", payload["encoding"])
	}
	if strings.TrimSpace(fmt.Sprintf("%v", payload["compressedPayload"])) == "" {
		t.Fatalf("expected compressed payload body, got %v", payload["compressedPayload"])
	}
	if payload["uncompressedBytes"].(float64) <= 0 || payload["compressedBytes"].(float64) <= 0 {
		t.Fatalf("expected byte counts > 0, got %v", payload)
	}
	if _, ok := payload["recent"]; ok {
		t.Fatalf("expected compressed payload to omit recent field, got %v", payload["recent"])
	}
	if payload["deltaCount"].(float64) != 1 {
		t.Fatalf("expected deltaCount=1, got %v", payload["deltaCount"])
	}
}

func TestMaybeExportSmokeArtifactsCopiesWorkdir(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "artifact.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write workdir artifact: %v", err)
	}

	summary := &sessionSmokeSummary{
		WorkDir:         workDir,
		ExportArtifacts: t.TempDir(),
	}
	maybeExportSmokeArtifacts(summary)
	if summary.ExportError != "" {
		t.Fatalf("expected no export error, got %q", summary.ExportError)
	}
	if strings.TrimSpace(summary.ExportedPath) == "" {
		t.Fatalf("expected exported path to be set")
	}
	data, err := os.ReadFile(filepath.Join(summary.ExportedPath, "artifact.txt"))
	if err != nil {
		t.Fatalf("read exported artifact: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("unexpected exported artifact content: %q", string(data))
	}
}

func TestCmdSessionRouteProfilePreset(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "exec",
			"--profile", "codex-spark",
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
	if payload["agent"] != "codex" {
		t.Fatalf("expected profile agent codex, got %v", payload["agent"])
	}
	if payload["model"] != "gpt-5.3-codex-spark" {
		t.Fatalf("expected codex-spark model, got %v", payload["model"])
	}

	stdoutOverride, stderrOverride := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "exec",
			"--profile", "codex-spark",
			"--model", "gpt-5-codex",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success with model override, got %d", code)
		}
	})
	if stderrOverride != "" {
		t.Fatalf("expected empty stderr on override, got %q", stderrOverride)
	}
	payloadOverride := parseJSONMap(t, stdoutOverride)
	if payloadOverride["model"] != "gpt-5-codex" {
		t.Fatalf("expected explicit model override to win, got %v", payloadOverride["model"])
	}
}

func TestCmdSessionListActiveOnlyWithoutNextAction(t *testing.T) {
	origList := tmuxListSessionsFn
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origCompute
	})
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-active", "lisa-missing"}, nil
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		if session == "lisa-missing" {
			return sessionStatus{Session: session, Status: "not_found", SessionState: "not_found"}, nil
		}
		return sessionStatus{Session: session, Status: "active", SessionState: "in_progress"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{
			"--project-root", t.TempDir(),
			"--active-only",
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
	if len(sessions) != 1 || sessions[0] != "lisa-active" {
		t.Fatalf("expected only active session, got %v", sessions)
	}
}
