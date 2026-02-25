package app

import (
	"strings"
	"testing"
)

func TestParseModel(t *testing.T) {
	model, err := parseModel("")
	if err != nil {
		t.Fatalf("expected empty model to pass, got %v", err)
	}
	if model != "" {
		t.Fatalf("expected empty model, got %q", model)
	}

	model, err = parseModel(" GPT-5.3-Codex-Spark ")
	if err != nil {
		t.Fatalf("expected trimmed model to pass, got %v", err)
	}
	if model != "GPT-5.3-Codex-Spark" {
		t.Fatalf("unexpected model result: %q", model)
	}

	model, err = parseModel("codex-spark")
	if err != nil {
		t.Fatalf("expected codex-spark alias to pass, got %v", err)
	}
	if model != "gpt-5.3-codex-spark" {
		t.Fatalf("expected normalized codex-spark alias, got %q", model)
	}

	model, err = parseModel("CoDeX-SpArK")
	if err != nil {
		t.Fatalf("expected mixed-case codex-spark alias to pass, got %v", err)
	}
	if model != "gpt-5.3-codex-spark" {
		t.Fatalf("expected normalized mixed-case codex-spark alias, got %q", model)
	}

	_, err = parseModel("bad\nmodel")
	if err == nil {
		t.Fatalf("expected control-character model rejection")
	}
}

func TestApplyModelToAgentArgs(t *testing.T) {
	got, err := applyModelToAgentArgs("codex", "", "GPT-5.3-Codex-Spark")
	if err != nil {
		t.Fatalf("expected codex model apply to pass, got %v", err)
	}
	if got != "--model 'GPT-5.3-Codex-Spark'" {
		t.Fatalf("unexpected applied args: %q", got)
	}

	got, err = applyModelToAgentArgs("codex", "--search", "GPT-5.3-Codex-Spark")
	if err != nil {
		t.Fatalf("expected codex model append to pass, got %v", err)
	}
	if !strings.Contains(got, "--search") || !strings.Contains(got, "--model 'GPT-5.3-Codex-Spark'") {
		t.Fatalf("unexpected combined args: %q", got)
	}

	_, err = applyModelToAgentArgs("claude", "", "GPT-5.3-Codex-Spark")
	if err == nil {
		t.Fatalf("expected non-codex model rejection")
	}

	_, err = applyModelToAgentArgs("codex", "--model existing", "GPT-5.3-Codex-Spark")
	if err == nil {
		t.Fatalf("expected duplicate model rejection")
	}
}
