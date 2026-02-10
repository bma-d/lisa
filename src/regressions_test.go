package app

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBuildFallbackScriptBodyPreservesExecCompletionMarkerOnFailure(t *testing.T) {
	command := wrapExecCommand("false")
	body := buildFallbackScriptBody(command)

	if !strings.Contains(body, "set +e\n") {
		t.Fatalf("expected fallback script to disable errexit for wrapped exec commands")
	}
	if !strings.Contains(body, execDonePrefix) {
		t.Fatalf("expected fallback script to include exec completion marker")
	}
}

func TestBuildFallbackScriptBodyLeavesNonExecCommandsUntouched(t *testing.T) {
	body := buildFallbackScriptBody("echo hello")
	if strings.Contains(body, "set +e\n") {
		t.Fatalf("did not expect errexit override for non-exec command")
	}
}

func TestTailLinesKeepsNewestOutput(t *testing.T) {
	values := make([]string, 300)
	for i := 0; i < 300; i++ {
		values[i] = "line-" + strconv.Itoa(i+1)
	}

	tail := tailLines(values, 260)
	if len(tail) != 260 {
		t.Fatalf("expected 260 lines, got %d", len(tail))
	}
	if tail[0] != values[40] {
		t.Fatalf("expected oldest retained line to be %q, got %q", values[40], tail[0])
	}
	if tail[len(tail)-1] != values[len(values)-1] {
		t.Fatalf("expected newest retained line to be %q, got %q", values[len(values)-1], tail[len(tail)-1])
	}
}

func TestSessionMatchesProjectRootUsesProjectHashAndLegacyMetaFallback(t *testing.T) {
	original := tmuxShowEnvironmentFn
	t.Cleanup(func() {
		tmuxShowEnvironmentFn = original
	})

	projectOne := filepath.Join(t.TempDir(), "a", "same")
	projectTwo := filepath.Join(t.TempDir(), "b", "same")
	if err := os.MkdirAll(projectOne, 0o755); err != nil {
		t.Fatalf("failed to create projectOne: %v", err)
	}
	if err := os.MkdirAll(projectTwo, 0o755); err != nil {
		t.Fatalf("failed to create projectTwo: %v", err)
	}

	session := "lisa-same-test"
	meta := sessionMeta{
		Session:     session,
		Agent:       "claude",
		Mode:        "exec",
		ProjectRoot: projectOne,
	}
	if err := saveSessionMeta(projectOne, session, meta); err != nil {
		t.Fatalf("failed to write legacy session metadata: %v", err)
	}

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return "", errors.New("missing")
	}
	if !sessionMatchesProjectRoot(session, projectOne, "") {
		t.Fatalf("expected legacy fallback to match session in originating project")
	}
	if sessionMatchesProjectRoot(session, projectTwo, "") {
		t.Fatalf("did not expect fallback match for different root sharing basename")
	}

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return projectHash(projectOne), nil
	}
	if !sessionMatchesProjectRoot(session, projectOne, projectHash(projectOne)) {
		t.Fatalf("expected hash-based project match")
	}

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		return projectHash(projectTwo), nil
	}
	if sessionMatchesProjectRoot(session, projectOne, projectHash(projectOne)) {
		t.Fatalf("did not expect hash mismatch to match")
	}
}
