package app

import (
	"encoding/json"
	"testing"
)

func TestCmdSessionPreflightNormalizesCodexSparkAlias(t *testing.T) {
	origLookPath := lookPathFn
	origProbe := sessionPreflightModelCheckFn
	t.Cleanup(func() {
		lookPathFn = origLookPath
		sessionPreflightModelCheckFn = origProbe
	})

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}

	probedModel := ""
	sessionPreflightModelCheckFn = func(agent, model string) sessionPreflightModelCheck {
		probedModel = model
		return sessionPreflightModelCheck{
			Agent:  agent,
			Model:  model,
			OK:     true,
			Detail: "ok",
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPreflight([]string{
			"--project-root", t.TempDir(),
			"--agent", "codex",
			"--model", "codex-spark",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected preflight success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		OK         bool `json:"ok"`
		ModelCheck struct {
			Model string `json:"model"`
		} `json:"modelCheck"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse preflight JSON: %v (%q)", err, stdout)
	}
	if !payload.OK {
		t.Fatalf("expected preflight ok=true, got false: %s", stdout)
	}
	if payload.ModelCheck.Model != "gpt-5.3-codex-spark" {
		t.Fatalf("expected normalized model in payload, got %q", payload.ModelCheck.Model)
	}
	if probedModel != "gpt-5.3-codex-spark" {
		t.Fatalf("expected normalized model passed to probe, got %q", probedModel)
	}
}
