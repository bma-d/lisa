package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRedactionRulesValidAndDedupe(t *testing.T) {
	rules, err := parseRedactionRules(" emails,paths,emails,tokens ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	want := []string{"emails", "paths", "tokens"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("expected %v, got %v", want, rules)
	}
}

func TestParseRedactionRulesRejectsInvalidRule(t *testing.T) {
	_, err := parseRedactionRules("emails,invalid")
	if err == nil {
		t.Fatalf("expected error for invalid rule")
	}
	if !strings.Contains(err.Error(), "invalid --redact rule") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRedactionRulesRejectsNoneCombinations(t *testing.T) {
	_, err := parseRedactionRules("none,all")
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "none cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyRedactionRulesPerRuleReplacements(t *testing.T) {
	const input = "email dev@example.com token=abc12345678 auth Bearer abcdefghijkl id 1234567 path=/tmp/work"
	tests := []struct {
		name        string
		rules       []string
		expect      string
		notExpected []string
	}{
		{
			name:        "emails",
			rules:       []string{"emails"},
			expect:      "[REDACTED_EMAIL]",
			notExpected: []string{"[REDACTED_SECRET]", "[REDACTED_TOKEN]", "[REDACTED_NUMBER]", "[REDACTED_PATH]"},
		},
		{
			name:        "secrets",
			rules:       []string{"secrets"},
			expect:      "token=[REDACTED_SECRET]",
			notExpected: []string{"[REDACTED_EMAIL]", "[REDACTED_TOKEN]", "[REDACTED_NUMBER]", "[REDACTED_PATH]"},
		},
		{
			name:        "tokens",
			rules:       []string{"tokens"},
			expect:      "Bearer [REDACTED_TOKEN]",
			notExpected: []string{"[REDACTED_EMAIL]", "[REDACTED_SECRET]", "[REDACTED_NUMBER]", "[REDACTED_PATH]"},
		},
		{
			name:        "numbers",
			rules:       []string{"numbers"},
			expect:      "[REDACTED_NUMBER]",
			notExpected: []string{"[REDACTED_EMAIL]", "[REDACTED_SECRET]", "[REDACTED_TOKEN]", "[REDACTED_PATH]"},
		},
		{
			name:        "paths",
			rules:       []string{"paths"},
			expect:      "path=[REDACTED_PATH]",
			notExpected: []string{"[REDACTED_EMAIL]", "[REDACTED_SECRET]", "[REDACTED_TOKEN]", "[REDACTED_NUMBER]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := applyRedactionRules(input, tt.rules)
			if !strings.Contains(out, tt.expect) {
				t.Fatalf("expected %q in output: %q", tt.expect, out)
			}
			for _, marker := range tt.notExpected {
				if strings.Contains(out, marker) {
					t.Fatalf("did not expect %q in output: %q", marker, out)
				}
			}
		})
	}
}

func TestApplyRedactionRulesNoneAndAll(t *testing.T) {
	const input = "dev@example.com token=abc12345678 Bearer abcdefghijkl 1234567 /tmp/work"

	if out := applyRedactionRules(input, nil); out != input {
		t.Fatalf("expected unchanged text with nil rules, got %q", out)
	}
	if out := applyRedactionRules(input, []string{"none", "emails"}); out != input {
		t.Fatalf("expected none to short-circuit redaction, got %q", out)
	}
	out := applyRedactionRules(input, []string{"all"})
	for _, marker := range []string{
		"[REDACTED_EMAIL]",
		"[REDACTED_SECRET]",
		"[REDACTED_TOKEN]",
		"[REDACTED_NUMBER]",
		"[REDACTED_PATH]",
	} {
		if !strings.Contains(out, marker) {
			t.Fatalf("expected %q in all-rules output: %q", marker, out)
		}
	}
}

func TestParseTopologyRolesValidInvalidAndDuplicates(t *testing.T) {
	roles, err := parseTopologyRoles("planner,workers,planner,reviewer")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	want := []string{"planner", "workers", "reviewer"}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("expected %v, got %v", want, roles)
	}

	empty, err := parseTopologyRoles(" ")
	if err != nil {
		t.Fatalf("expected empty topology to be accepted, got %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no roles, got %v", empty)
	}

	_, err = parseTopologyRoles("planner,bad-role")
	if err == nil {
		t.Fatalf("expected invalid role error")
	}
	if !strings.Contains(err.Error(), "invalid --topology role") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = parseTopologyRoles(" , , ")
	if err == nil {
		t.Fatalf("expected empty role-list error")
	}
	if !strings.Contains(err.Error(), "expected comma-separated roles") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildTopologyGraphIncludesExpectedNodesAndEdges(t *testing.T) {
	graph := buildTopologyGraph([]string{"planner", "workers", "reviewer"})
	nodes, ok := graph["nodes"].([]map[string]any)
	if !ok {
		t.Fatalf("expected nodes slice, got %T", graph["nodes"])
	}
	edges, ok := graph["edges"].([]map[string]string)
	if !ok {
		t.Fatalf("expected edges slice, got %T", graph["edges"])
	}
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d (%v)", len(nodes), nodes)
	}
	if len(edges) != 4 {
		t.Fatalf("expected 4 edges, got %d (%v)", len(edges), edges)
	}

	graphNoWorkers := buildTopologyGraph([]string{"reviewer", "planner"})
	edgesNoWorkers, ok := graphNoWorkers["edges"].([]map[string]string)
	if !ok {
		t.Fatalf("expected edges slice, got %T", graphNoWorkers["edges"])
	}
	if len(edgesNoWorkers) != 1 {
		t.Fatalf("expected planner->reviewer edge only, got %v", edgesNoWorkers)
	}
	edge := edgesNoWorkers[0]
	if edge["from"] != "planner" || edge["to"] != "reviewer" {
		t.Fatalf("unexpected fallback edge: %v", edge)
	}
}

func TestEstimateRouteCostTopologyMultiplierAndBudget(t *testing.T) {
	base := estimateRouteCost("exec", "exec", 0, nil)
	if base["topologyMultiplier"] != "1.00" {
		t.Fatalf("expected baseline multiplier 1.00, got %v", base["topologyMultiplier"])
	}

	withDup := estimateRouteCost("exec", "exec", 0, []string{"workers", "workers", "planner"})
	if withDup["topologyMultiplier"] != "1.80" {
		t.Fatalf("expected deduped multiplier 1.80, got %v", withDup["topologyMultiplier"])
	}

	withAll := estimateRouteCost("exec", "exec", 0, []string{"planner", "workers", "reviewer"})
	if withAll["topologyMultiplier"] != "2.05" {
		t.Fatalf("expected full multiplier 2.05, got %v", withAll["topologyMultiplier"])
	}

	baseTokens := base["totalTokens"].(int)
	withDupTokens := withDup["totalTokens"].(int)
	withAllTokens := withAll["totalTokens"].(int)
	if !(baseTokens < withDupTokens && withDupTokens < withAllTokens) {
		t.Fatalf("expected token totals to scale with multiplier, got base=%d dup=%d all=%d", baseTokens, withDupTokens, withAllTokens)
	}

	if got := routeStepTokens(t, estimateRouteCost("exec", "exec", 100, nil), "capture"); got != 80 {
		t.Fatalf("expected capture tokens 80 for budget=100, got %d", got)
	}
	if got := routeStepTokens(t, estimateRouteCost("exec", "exec", 300, nil), "capture"); got != 150 {
		t.Fatalf("expected capture tokens 150 for budget=300, got %d", got)
	}
	if got := routeStepTokens(t, estimateRouteCost("exec", "exec", 1000, nil), "capture"); got != 220 {
		t.Fatalf("expected capture tokens capped at 220 for budget=1000, got %d", got)
	}
	if got := routeStepTokens(t, estimateRouteCost("exec", "exec", 100, []string{"workers"}), "capture"); got != 128 {
		t.Fatalf("expected capture tokens to include multiplier for workers (128), got %d", got)
	}
}

func TestParseProjectionFieldsValidation(t *testing.T) {
	fields, err := parseProjectionFields("session,status.reason,session,meta.next")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	want := []string{"session", "status.reason", "meta.next"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("expected %v, got %v", want, fields)
	}

	_, err = parseProjectionFields("status..reason")
	if err == nil {
		t.Fatalf("expected empty segment error")
	}
	if !strings.Contains(err.Error(), "empty path segment") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = parseProjectionFields(" , , ")
	if err == nil {
		t.Fatalf("expected empty field-list error")
	}
	if !strings.Contains(err.Error(), "expected comma-separated field paths") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectPayloadFieldsProjectsOnlySelectedExistingFields(t *testing.T) {
	payload := map[string]any{
		"session": "lisa-1",
		"status": map[string]any{
			"state":  "in_progress",
			"reason": "heartbeat_fresh",
		},
		"meta": map[string]any{
			"next": map[string]any{
				"offset": 17,
			},
			"ignored": "value",
		},
		"scalar": "leaf",
	}
	out := projectPayloadFields(payload, []string{
		"session",
		"status.reason",
		"meta.next.offset",
		"missing.value",
		"scalar.child",
	})
	want := map[string]any{
		"session": "lisa-1",
		"status": map[string]any{
			"reason": "heartbeat_fresh",
		},
		"meta": map[string]any{
			"next": map[string]any{
				"offset": 17,
			},
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("unexpected projection.\nwant=%v\ngot=%v", want, out)
	}
}

func routeStepTokens(t *testing.T, estimate map[string]any, name string) int {
	t.Helper()
	steps, ok := estimate["steps"].([]map[string]any)
	if !ok {
		t.Fatalf("expected steps slice, got %T", estimate["steps"])
	}
	for _, step := range steps {
		if step["step"] == name {
			tokens, ok := step["tokens"].(int)
			if !ok {
				t.Fatalf("expected int tokens for %s, got %T", name, step["tokens"])
			}
			return tokens
		}
	}
	t.Fatalf("step %q not found in %v", name, steps)
	return 0
}
