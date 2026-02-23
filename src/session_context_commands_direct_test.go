package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func decodeJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode payload: %v (%q)", err, raw)
	}
	return payload
}

func jsonInt(t *testing.T, raw any) int {
	t.Helper()
	n, ok := raw.(float64)
	if !ok {
		t.Fatalf("expected JSON number, got %T (%v)", raw, raw)
	}
	return int(n)
}

func jsonStringSlice(t *testing.T, raw any) []string {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected JSON array, got %T (%v)", raw, raw)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("expected JSON string item, got %T (%v)", item, item)
		}
		out = append(out, text)
	}
	return out
}

func TestCmdSessionLaneCRUDJSONContract(t *testing.T) {
	projectRoot := t.TempDir()
	fixedNow := time.Date(2026, time.February, 23, 9, 30, 0, 0, time.UTC)
	origNow := nowFn
	t.Cleanup(func() { nowFn = origNow })
	nowFn = func() time.Time { return fixedNow }

	upsertOut, upsertErr := captureOutput(t, func() {
		code := cmdSessionLane([]string{
			"--project-root", projectRoot,
			"--name", "planner",
			"--goal", "analysis",
			"--agent", "codex",
			"--mode", "interactive",
			"--nested-policy", "off",
			"--nesting-intent", "neutral",
			"--prompt", "Continue planning",
			"--model", "gpt-5.3-codex-spark",
			"--budget", "420",
			"--topology", "planner,workers",
			"--contract", "handoff_v2_required",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected upsert success, got %d", code)
		}
	})
	if upsertErr != "" {
		t.Fatalf("unexpected stderr on upsert: %q", upsertErr)
	}
	upsert := decodeJSONMap(t, upsertOut)
	if upsert["action"] != "upserted" {
		t.Fatalf("expected action=upserted, got %v", upsert["action"])
	}
	if upsert["name"] != "planner" {
		t.Fatalf("expected name=planner, got %v", upsert["name"])
	}
	if jsonInt(t, upsert["count"]) != 1 {
		t.Fatalf("expected count=1, got %v", upsert["count"])
	}
	lane, ok := upsert["lane"].(map[string]any)
	if !ok {
		t.Fatalf("expected lane object in upsert payload, got %v", upsert["lane"])
	}
	if lane["contract"] != "handoff_v2_required" {
		t.Fatalf("expected contract in lane payload, got %v", lane["contract"])
	}
	if lane["updatedAt"] != fixedNow.Format(time.RFC3339) {
		t.Fatalf("expected deterministic updatedAt, got %v", lane["updatedAt"])
	}

	readOut, readErr := captureOutput(t, func() {
		code := cmdSessionLane([]string{"--project-root", projectRoot, "--name", "planner", "--json"})
		if code != 0 {
			t.Fatalf("expected read success, got %d", code)
		}
	})
	if readErr != "" {
		t.Fatalf("unexpected stderr on read: %q", readErr)
	}
	readPayload := decodeJSONMap(t, readOut)
	if readPayload["action"] != "read" {
		t.Fatalf("expected action=read, got %v", readPayload["action"])
	}
	if _, ok := readPayload["lane"].(map[string]any); !ok {
		t.Fatalf("expected lane object in read payload, got %v", readPayload["lane"])
	}

	listOut, listErr := captureOutput(t, func() {
		code := cmdSessionLane([]string{"--project-root", projectRoot, "--list", "--json"})
		if code != 0 {
			t.Fatalf("expected list success, got %d", code)
		}
	})
	if listErr != "" {
		t.Fatalf("unexpected stderr on list: %q", listErr)
	}
	listPayload := decodeJSONMap(t, listOut)
	if jsonInt(t, listPayload["count"]) != 1 {
		t.Fatalf("expected list count=1, got %v", listPayload["count"])
	}
	lanes, ok := listPayload["lanes"].([]any)
	if !ok {
		t.Fatalf("expected lanes array, got %T (%v)", listPayload["lanes"], listPayload["lanes"])
	}
	if len(lanes) == 0 {
		t.Fatalf("expected non-empty lanes payload")
	}

	clearOut, clearErr := captureOutput(t, func() {
		code := cmdSessionLane([]string{"--project-root", projectRoot, "--name", "planner", "--clear", "--json"})
		if code != 0 {
			t.Fatalf("expected clear success, got %d", code)
		}
	})
	if clearErr != "" {
		t.Fatalf("unexpected stderr on clear: %q", clearErr)
	}
	clearPayload := decodeJSONMap(t, clearOut)
	if clearPayload["action"] != "cleared" {
		t.Fatalf("expected action=cleared, got %v", clearPayload["action"])
	}
	if jsonInt(t, clearPayload["count"]) != 0 {
		t.Fatalf("expected count=0 after clear, got %v", clearPayload["count"])
	}

	missingOut, missingErr := captureOutput(t, func() {
		code := cmdSessionLane([]string{"--project-root", projectRoot, "--name", "planner", "--json"})
		if code == 0 {
			t.Fatalf("expected missing lane to fail")
		}
	})
	if missingErr != "" {
		t.Fatalf("unexpected stderr on missing lane: %q", missingErr)
	}
	missing := decodeJSONMap(t, missingOut)
	if missing["errorCode"] != "lane_not_found" {
		t.Fatalf("expected lane_not_found, got %v", missing["errorCode"])
	}
}

func TestCmdSessionMemoryRefreshJSONIncludesDeltaMetadataAndPath(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-memory-direct"
	fixedNow := time.Date(2026, time.February, 23, 10, 0, 0, 0, time.UTC)
	origNow := nowFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		nowFn = origNow
		tmuxHasSessionFn = origHas
	})
	nowFn = func() time.Time { return fixedNow }
	tmuxHasSessionFn = func(session string) bool { return false }

	seed := sessionMemoryRecord{
		Session:   session,
		UpdatedAt: fixedNow.Add(-time.Hour).Format(time.RFC3339),
		ExpiresAt: fixedNow.Add(23 * time.Hour).Format(time.RFC3339),
		MaxLines:  80,
		Lines:     []string{"alpha"},
	}
	if err := saveSessionMemory(projectRoot, session, seed); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	outputPath := sessionOutputFile(projectRoot, session)
	if err := os.WriteFile(outputPath, []byte("alpha\nbeta\ngamma\n"), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}

	refreshOut, refreshErr := captureOutput(t, func() {
		code := cmdSessionMemory([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--refresh",
			"--ttl-hours", "2",
			"--max-lines", "5",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected refresh success, got %d", code)
		}
	})
	if refreshErr != "" {
		t.Fatalf("unexpected stderr on refresh: %q", refreshErr)
	}
	refreshPayload := decodeJSONMap(t, refreshOut)
	if refreshPayload["refresh"] != true {
		t.Fatalf("expected refresh=true, got %v", refreshPayload["refresh"])
	}
	if refreshPayload["path"] != sessionMemoryFile(projectRoot, session) {
		t.Fatalf("expected memory path, got %v", refreshPayload["path"])
	}
	if refreshPayload["deltaPath"] != outputPath {
		t.Fatalf("expected deltaPath=%q, got %v", outputPath, refreshPayload["deltaPath"])
	}
	if refreshPayload["updatedAt"] != fixedNow.Format(time.RFC3339) {
		t.Fatalf("expected updatedAt=%q, got %v", fixedNow.Format(time.RFC3339), refreshPayload["updatedAt"])
	}
	if refreshPayload["expiresAt"] != fixedNow.Add(2*time.Hour).Format(time.RFC3339) {
		t.Fatalf("expected expiresAt=%q, got %v", fixedNow.Add(2*time.Hour).Format(time.RFC3339), refreshPayload["expiresAt"])
	}
	if jsonInt(t, refreshPayload["lineCount"]) != 3 {
		t.Fatalf("expected lineCount=3, got %v", refreshPayload["lineCount"])
	}
	deltaLines := jsonStringSlice(t, refreshPayload["deltaLines"])
	sort.Strings(deltaLines)
	if len(deltaLines) != 2 || deltaLines[0] != "beta" || deltaLines[1] != "gamma" {
		t.Fatalf("expected deltaLines [beta gamma], got %v", deltaLines)
	}
	meta, ok := refreshPayload["deltaMetadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected deltaMetadata object, got %v", refreshPayload["deltaMetadata"])
	}
	if jsonInt(t, meta["baselineLineCount"]) != 1 {
		t.Fatalf("expected baselineLineCount=1, got %v", meta["baselineLineCount"])
	}
	if jsonInt(t, meta["currentLineCount"]) != 3 {
		t.Fatalf("expected currentLineCount=3, got %v", meta["currentLineCount"])
	}
	if jsonInt(t, meta["deltaCount"]) != 2 {
		t.Fatalf("expected deltaCount=2, got %v", meta["deltaCount"])
	}
	if jsonInt(t, meta["maxLines"]) != 5 {
		t.Fatalf("expected maxLines=5 in deltaMetadata, got %v", meta["maxLines"])
	}

	readOut, readErr := captureOutput(t, func() {
		code := cmdSessionMemory([]string{"--session", session, "--project-root", projectRoot, "--json"})
		if code != 0 {
			t.Fatalf("expected read success, got %d", code)
		}
	})
	if readErr != "" {
		t.Fatalf("unexpected stderr on read: %q", readErr)
	}
	readPayload := decodeJSONMap(t, readOut)
	if readPayload["refresh"] != false {
		t.Fatalf("expected refresh=false for read, got %v", readPayload["refresh"])
	}
	if _, ok := readPayload["deltaMetadata"]; ok {
		t.Fatalf("did not expect deltaMetadata in read payload, got %v", readPayload["deltaMetadata"])
	}
	if _, ok := readPayload["deltaLines"]; ok {
		t.Fatalf("did not expect deltaLines in read payload, got %v", readPayload["deltaLines"])
	}
}

func TestCmdSessionMemorySemanticDiffTextSummaryMatchesJSONCounts(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-memory-semantic-diff-text"
	if err := saveSessionMemory(projectRoot, session, sessionMemoryRecord{
		Session:   session,
		UpdatedAt: "2026-02-20T00:00:00Z",
		ExpiresAt: "2099-01-01T00:00:00Z",
		MaxLines:  80,
		Lines:     []string{"old-a", "shared-b"},
	}); err != nil {
		t.Fatalf("save memory: %v", err)
	}
	if err := os.WriteFile(sessionOutputFile(projectRoot, session), []byte("shared-b\nnew-c\n"), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}

	origHas := tmuxHasSessionFn
	t.Cleanup(func() { tmuxHasSessionFn = origHas })
	tmuxHasSessionFn = func(session string) bool { return false }

	jsonOut, jsonErr := captureOutput(t, func() {
		code := cmdSessionMemory([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--semantic-diff",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected json success, got %d", code)
		}
	})
	if jsonErr != "" {
		t.Fatalf("unexpected stderr for json run: %q", jsonErr)
	}
	payload := decodeJSONMap(t, jsonOut)
	semantic, ok := payload["semanticDiff"].(map[string]any)
	if !ok {
		t.Fatalf("expected semanticDiff object, got %v", payload["semanticDiff"])
	}
	addedRaw, ok := semantic["added"].([]any)
	if !ok {
		t.Fatalf("expected semanticDiff.added array, got %T (%v)", semantic["added"], semantic["added"])
	}
	removedRaw, ok := semantic["removed"].([]any)
	if !ok {
		t.Fatalf("expected semanticDiff.removed array, got %T (%v)", semantic["removed"], semantic["removed"])
	}

	textOut, textErr := captureOutput(t, func() {
		code := cmdSessionMemory([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--semantic-diff",
		})
		if code != 0 {
			t.Fatalf("expected text success, got %d", code)
		}
	})
	if textErr != "" {
		t.Fatalf("unexpected stderr for text run: %q", textErr)
	}
	expected := fmt.Sprintf("semantic_diff added=%d removed=%d", len(addedRaw), len(removedRaw))
	if !strings.Contains(textOut, expected) {
		t.Fatalf("expected text semantic diff summary %q, got %q", expected, textOut)
	}
}

func TestCmdSessionSchemaMemoryAndLaneContracts(t *testing.T) {
	memoryOut, memoryErr := captureOutput(t, func() {
		code := cmdSessionSchema([]string{"--command", "memory", "--json"})
		if code != 0 {
			t.Fatalf("expected memory schema success, got %d", code)
		}
	})
	if memoryErr != "" {
		t.Fatalf("unexpected stderr on memory schema: %q", memoryErr)
	}
	memoryPayload := decodeJSONMap(t, memoryOut)
	memorySchema, ok := memoryPayload["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected memory schema object, got %v", memoryPayload["schema"])
	}
	memoryProps, ok := memorySchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected memory schema properties, got %v", memorySchema["properties"])
	}
	for _, key := range []string{"session", "projectRoot", "updatedAt", "expiresAt", "maxLines", "lineCount", "lines", "refresh", "path", "deltaLines", "deltaCount", "deltaPath", "deltaMetadata", "errorCode"} {
		if _, ok := memoryProps[key]; !ok {
			t.Fatalf("expected memory schema property %q, got %v", key, memoryProps)
		}
	}

	laneOut, laneErr := captureOutput(t, func() {
		code := cmdSessionSchema([]string{"--command", "lane", "--json"})
		if code != 0 {
			t.Fatalf("expected lane schema success, got %d", code)
		}
	})
	if laneErr != "" {
		t.Fatalf("unexpected stderr on lane schema: %q", laneErr)
	}
	lanePayload := decodeJSONMap(t, laneOut)
	laneSchema, ok := lanePayload["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected lane schema object, got %v", lanePayload["schema"])
	}
	laneProps, ok := laneSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected lane schema properties, got %v", laneSchema["properties"])
	}
	for _, key := range []string{"action", "projectRoot", "count", "name", "lanes", "lane", "errorCode"} {
		if _, ok := laneProps[key]; !ok {
			t.Fatalf("expected lane schema property %q, got %v", key, laneProps)
		}
	}
}
