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
	mustContain(t, monitorSection, "--timeout-seconds")
	mustContain(t, monitorSection, "--auto-recover")
	mustContain(t, monitorSection, "--recover-max")
	mustContain(t, monitorSection, "--recover-budget")

	contextPackSection := usageSection(t, usage, "session context-pack")
	mustContain(t, contextPackSection, "--from-handoff")
	mustContain(t, contextPackSection, "v2/v3/v4")

	handoffSection := usageSection(t, usage, "session handoff")
	mustContain(t, handoffSection, "v1|v2|v3|v4")
	mustContain(t, handoffSection, "v2|v3|v4")

	routeSection := usageSection(t, usage, "session route")
	mustContain(t, routeSection, "--budget")
	mustContain(t, routeSection, "--profile")
	mustContain(t, routeSection, "--queue")
	mustContain(t, routeSection, "--sessions")
	mustContain(t, routeSection, "--queue-limit")
	mustContain(t, routeSection, "--concurrency")

	guardSection := usageSection(t, usage, "session guard")
	mustContain(t, guardSection, "--enforce")
	mustContain(t, guardSection, "--policy-file")

	smokeSection := usageSection(t, usage, "session smoke")
	mustContain(t, smokeSection, "--chaos MODE")
	mustContain(t, smokeSection, "none|delay|drop-marker|fail-child|mixed")
	mustContain(t, smokeSection, "--llm-profile")
	mustContain(t, smokeSection, "--export-artifacts")

	autopilotSection := usageSection(t, usage, "session autopilot")
	mustContain(t, autopilotSection, "--lane NAME")
}

func TestSkillDocsHandoffSchemaV4Contract(t *testing.T) {
	skillRaw, err := os.ReadFile("../skills/lisa/SKILL.md")
	if err != nil {
		t.Fatalf("failed to read skills/lisa/SKILL.md: %v", err)
	}
	skillDoc := string(skillRaw)
	mustContain(t, skillDoc, "--schema v2|v3|v4")

	commandsRaw, err := os.ReadFile("../skills/lisa/data/commands.md")
	if err != nil {
		t.Fatalf("failed to read skills/lisa/data/commands.md: %v", err)
	}
	commandsDoc := string(commandsRaw)
	mustContain(t, commandsDoc, "Handoff schema: `v1|v2|v3|v4`")
	mustContain(t, commandsDoc, "require `--schema v2` (or `v3|v4`)")
	mustNotContain(t, commandsDoc, "Handoff schema: `v1|v2|v3`;")

	validationRaw, err := os.ReadFile("../skills/lisa/data/validation.md")
	if err != nil {
		t.Fatalf("failed to read skills/lisa/data/validation.md: %v", err)
	}
	validationDoc := string(validationRaw)
	mustContain(t, validationDoc, "session handoff --schema v2|v3|v4")
	mustContain(t, validationDoc, "`schema v2|v3|v4`")
	mustNotContain(t, validationDoc, "session handoff --schema v2|v3`).")
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

func mustNotContain(t *testing.T, body string, token string) {
	t.Helper()
	if strings.Contains(body, token) {
		t.Fatalf("unexpected token %q in doc segment", token)
	}
}
