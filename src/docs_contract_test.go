package app

import (
	"os"
	"strings"
	"testing"
)

func TestUsageDocSessionContract(t *testing.T) {
	raw, err := os.ReadFile("../USAGE.md")
	if err != nil {
		t.Fatalf("failed to read USAGE.md: %v", err)
	}
	usage := string(raw)

	mustContain(t, usage, "lisa session autopilot")
	mustContain(t, usage, "- `session autopilot`")

	monitorSection := usageSection(t, usage, "session monitor")
	mustContain(t, monitorSection, "--until-jsonpath")

	contextPackSection := usageSection(t, usage, "session context-pack")
	mustContain(t, contextPackSection, "--from-handoff")

	routeSection := usageSection(t, usage, "session route")
	mustContain(t, routeSection, "--budget")

	guardSection := usageSection(t, usage, "session guard")
	mustContain(t, guardSection, "--enforce")

	smokeSection := usageSection(t, usage, "session smoke")
	mustContain(t, smokeSection, "--chaos MODE")
	mustContain(t, smokeSection, "none|delay|drop-marker|fail-child|mixed")

	_ = usageSection(t, usage, "session autopilot")
}

func usageSection(t *testing.T, doc string, name string) string {
	t.Helper()
	header := "### `" + name + "`"
	start := strings.Index(doc, header)
	if start == -1 {
		t.Fatalf("missing section header %q", header)
	}
	rest := doc[start+len(header):]
	next := strings.Index(rest, "\n### `")
	if next == -1 {
		return rest
	}
	return rest[:next]
}

func mustContain(t *testing.T, body string, token string) {
	t.Helper()
	if !strings.Contains(body, token) {
		t.Fatalf("expected token %q in doc segment", token)
	}
}
