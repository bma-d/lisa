package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCmdSessionTreeFlatOutputsRows(t *testing.T) {
	projectRoot := t.TempDir()
	rootSession := "lisa-tree-flat-root"
	childSession := "lisa-tree-flat-child"
	now := "2026-02-21T00:00:00Z"

	if err := saveSessionMeta(projectRoot, rootSession, sessionMeta{
		Session:     rootSession,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		StartCmd:    "echo root",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save root meta failed: %v", err)
	}
	if err := saveSessionMeta(projectRoot, childSession, sessionMeta{
		Session:       childSession,
		ParentSession: rootSession,
		Agent:         "codex",
		Mode:          "interactive",
		ProjectRoot:   projectRoot,
		StartCmd:      "echo child",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("save child meta failed: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTree([]string{"--project-root", projectRoot, "--flat"})
		if code != 0 {
			t.Fatalf("expected flat tree success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header + two rows, got %q", stdout)
	}
	if lines[0] != "session\tparentSession\tagent\tmode\tprojectRoot\tcreatedAt" {
		t.Fatalf("unexpected flat header: %q", lines[0])
	}
	if !strings.Contains(stdout, rootSession+"\t\tcodex\tinteractive\t"+projectRoot) {
		t.Fatalf("expected root row in flat output, got %q", stdout)
	}
	if !strings.Contains(stdout, childSession+"\t"+rootSession+"\tcodex\tinteractive\t"+projectRoot) {
		t.Fatalf("expected child row in flat output, got %q", stdout)
	}
}

func TestCmdSessionTreeFlatJSONIncludesRows(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-tree-flat-json"
	now := "2026-02-21T00:05:00Z"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		StartCmd:    "echo json",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save meta failed: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTree([]string{"--project-root", projectRoot, "--flat", "--json"})
		if code != 0 {
			t.Fatalf("expected flat json tree success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload struct {
		Flat bool `json:"flat"`
		Rows []struct {
			Session string `json:"session"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse tree flat json: %v (%q)", err, stdout)
	}
	if !payload.Flat {
		t.Fatalf("expected flat=true in payload")
	}
	if len(payload.Rows) == 0 || payload.Rows[0].Session == "" {
		t.Fatalf("expected non-empty rows payload: %+v", payload)
	}
}
