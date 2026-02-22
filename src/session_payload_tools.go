package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	emailRedactRe     = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	pathRedactRe      = regexp.MustCompile(`(?:^|[\s"'=])(/[^ \n\r\t"'=]+)`)
	secretAssignRe    = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password|passwd)\b\s*[:=]\s*["']?([^\s"']+)`)
	longNumberRedact  = regexp.MustCompile(`\b\d{6,}\b`)
	bearerTokenRedact = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]{8,}`)
	semanticNoiseRe   = regexp.MustCompile(`(?i)^(tokens used|provider:|approval:|sandbox:|reasoning effort:|reasoning summaries:|session id:|workdir:|model:|--------)$`)
)

func parseProjectionFields(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			continue
		}
		path := strings.Split(field, ".")
		for _, segment := range path {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				return nil, fmt.Errorf("invalid --fields: empty path segment in %q", field)
			}
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid --fields: expected comma-separated field paths")
	}
	return out, nil
}

func projectPayloadFields(payload map[string]any, fields []string) map[string]any {
	out := map[string]any{}
	for _, field := range fields {
		path := strings.Split(field, ".")
		value, ok := lookupPath(payload, path)
		if !ok {
			continue
		}
		assignPath(out, path, value)
	}
	return out
}

func lookupPath(payload map[string]any, path []string) (any, bool) {
	var current any = payload
	for _, segment := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[segment]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func assignPath(payload map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := payload
	for _, segment := range path[:len(path)-1] {
		next, ok := current[segment]
		if !ok {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			child = map[string]any{}
			current[segment] = child
		}
		current = child
	}
	current[path[len(path)-1]] = value
}

func parseRedactionRules(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		rule := strings.ToLower(strings.TrimSpace(part))
		if rule == "" {
			continue
		}
		switch rule {
		case "none", "all", "paths", "emails", "secrets", "numbers", "tokens":
		default:
			return nil, fmt.Errorf("invalid --redact rule: %s (expected none|all|paths|emails|secrets|numbers|tokens)", rule)
		}
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		out = append(out, rule)
	}
	if len(out) == 0 {
		return []string{}, nil
	}
	if len(out) > 1 {
		if _, hasNone := seen["none"]; hasNone {
			return nil, fmt.Errorf("invalid --redact rules: none cannot be combined with other rules")
		}
	}
	return out, nil
}

func applyRedactionRules(text string, rules []string) string {
	if len(rules) == 0 {
		return text
	}
	enabled := map[string]bool{}
	for _, rule := range rules {
		enabled[rule] = true
	}
	if enabled["none"] {
		return text
	}
	if enabled["all"] {
		enabled["paths"] = true
		enabled["emails"] = true
		enabled["secrets"] = true
		enabled["numbers"] = true
		enabled["tokens"] = true
	}
	out := text
	if enabled["emails"] {
		out = emailRedactRe.ReplaceAllString(out, "[REDACTED_EMAIL]")
	}
	if enabled["secrets"] {
		out = secretAssignRe.ReplaceAllString(out, "$1=[REDACTED_SECRET]")
	}
	if enabled["tokens"] {
		out = bearerTokenRedact.ReplaceAllString(out, "Bearer [REDACTED_TOKEN]")
	}
	if enabled["numbers"] {
		out = longNumberRedact.ReplaceAllString(out, "[REDACTED_NUMBER]")
	}
	if enabled["paths"] {
		out = pathRedactRe.ReplaceAllStringFunc(out, func(match string) string {
			prefix := ""
			if strings.HasPrefix(match, " ") || strings.HasPrefix(match, "\"") || strings.HasPrefix(match, "'") || strings.HasPrefix(match, "=") {
				prefix = match[:1]
			}
			return prefix + "[REDACTED_PATH]"
		})
	}
	return out
}

func semanticCursorPath(cursorFile string) string {
	return cursorFile + ".semantic.json"
}

func loadSemanticCursor(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return []string{}, nil
	}
	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return dedupeNonEmpty(payload.Lines), nil
}

func saveSemanticCursor(path string, lines []string) error {
	payload := map[string]any{
		"updatedAt": nowFn().UTC().Format("2006-01-02T15:04:05Z"),
		"lines":     dedupeNonEmpty(lines),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func dedupeNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func extractSemanticLines(text string) []string {
	lines := trimLines(text)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "__LISA_SESSION_START__") || strings.HasPrefix(line, "__LISA_EXEC_DONE__") || strings.HasPrefix(line, "__LISA_SESSION_DONE__") {
			continue
		}
		if semanticNoiseRe.MatchString(line) {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "codex>") || strings.HasPrefix(lower, "lisa ") {
			continue
		}
		out = append(out, line)
	}
	return dedupeNonEmpty(out)
}

func computeSemanticDelta(current, baseline []string) []string {
	baselineSet := map[string]struct{}{}
	for _, line := range baseline {
		baselineSet[line] = struct{}{}
	}
	delta := make([]string, 0, len(current))
	for _, line := range current {
		if _, ok := baselineSet[line]; ok {
			continue
		}
		delta = append(delta, line)
	}
	return delta
}

func parseTopologyRoles(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		role := strings.ToLower(strings.TrimSpace(part))
		if role == "" {
			continue
		}
		switch role {
		case "planner", "workers", "reviewer":
		default:
			return nil, fmt.Errorf("invalid --topology role: %s (expected planner|workers|reviewer)", role)
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid --topology: expected comma-separated roles")
	}
	return out, nil
}

func buildTopologyGraph(roles []string) map[string]any {
	nodes := make([]map[string]any, 0)
	edges := make([]map[string]string, 0)
	has := map[string]bool{}
	for _, role := range roles {
		has[role] = true
	}
	if has["planner"] {
		nodes = append(nodes, map[string]any{"id": "planner", "role": "planner"})
	}
	if has["workers"] {
		nodes = append(nodes, map[string]any{"id": "worker-1", "role": "worker"})
		nodes = append(nodes, map[string]any{"id": "worker-2", "role": "worker"})
	}
	if has["reviewer"] {
		nodes = append(nodes, map[string]any{"id": "reviewer", "role": "reviewer"})
	}
	if has["planner"] && has["workers"] {
		edges = append(edges, map[string]string{"from": "planner", "to": "worker-1"})
		edges = append(edges, map[string]string{"from": "planner", "to": "worker-2"})
	}
	if has["workers"] && has["reviewer"] {
		edges = append(edges, map[string]string{"from": "worker-1", "to": "reviewer"})
		edges = append(edges, map[string]string{"from": "worker-2", "to": "reviewer"})
	}
	if has["planner"] && has["reviewer"] && !has["workers"] {
		edges = append(edges, map[string]string{"from": "planner", "to": "reviewer"})
	}
	return map[string]any{
		"roles": roles,
		"nodes": nodes,
		"edges": edges,
	}
}

func estimateRouteCost(goal, mode string, budget int, topologyRoles []string) map[string]any {
	perStep := []map[string]any{
		{"step": "preflight", "tokens": 60, "seconds": 1},
		{"step": "spawn", "tokens": 140, "seconds": 2},
		{"step": "monitor", "tokens": 180, "seconds": 35},
		{"step": "capture", "tokens": 220, "seconds": 3},
		{"step": "handoff", "tokens": 90, "seconds": 1},
		{"step": "cleanup", "tokens": 40, "seconds": 1},
	}
	if mode == "interactive" {
		perStep[2]["tokens"] = 220
		perStep[2]["seconds"] = 50
	}
	if goal == "nested" {
		perStep[1]["tokens"] = 180
		perStep[2]["tokens"] = 260
		perStep[2]["seconds"] = 65
	}
	if budget > 0 {
		perStep[3]["tokens"] = minInt(220, maxInt(80, budget/2))
	}
	multiplier := 1.0
	if len(topologyRoles) > 0 {
		roleSet := map[string]bool{}
		for _, role := range topologyRoles {
			roleSet[role] = true
		}
		if roleSet["workers"] {
			multiplier += 0.6
		}
		if roleSet["planner"] {
			multiplier += 0.2
		}
		if roleSet["reviewer"] {
			multiplier += 0.25
		}
	}
	totalTokens := 0
	totalSeconds := 0
	for _, step := range perStep {
		tokens := int(float64(step["tokens"].(int)) * multiplier)
		seconds := int(float64(step["seconds"].(int)) * multiplier)
		step["tokens"] = tokens
		step["seconds"] = seconds
		totalTokens += tokens
		totalSeconds += seconds
	}
	return map[string]any{
		"goal":               goal,
		"mode":               mode,
		"topologyMultiplier": strconv.FormatFloat(multiplier, 'f', 2, 64),
		"totalTokens":        totalTokens,
		"totalSeconds":       totalSeconds,
		"steps":              perStep,
	}
}

func computeSessionPriority(status sessionStatus) (int, string) {
	state := strings.ToLower(strings.TrimSpace(status.SessionState))
	switch state {
	case "crashed":
		return 100, "critical"
	case "stuck":
		return 96, "critical"
	case "degraded":
		return 92, "high"
	case "waiting_input":
		return 86, "needs_input"
	case "in_progress":
		return 70, "active"
	case "completed":
		return 20, "complete"
	case "not_found":
		return 5, "missing"
	default:
		return 40, "unknown"
	}
}

func sortSessionListByPriority(items []sessionListItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PriorityScore == items[j].PriorityScore {
			return items[i].Session < items[j].Session
		}
		return items[i].PriorityScore > items[j].PriorityScore
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
