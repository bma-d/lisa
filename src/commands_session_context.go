package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func cmdSessionObjective(args []string) int {
	projectRoot := canonicalProjectRoot(getPWD())
	id := ""
	goal := ""
	acceptance := ""
	budget := 0
	status := ""
	ttlHours := 0
	activate := false
	clear := false
	listOnly := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session objective")
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = canonicalProjectRoot(args[i+1])
			i++
		case "--id":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --id")
			}
			id = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--goal":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --goal")
			}
			goal = strings.TrimSpace(args[i+1])
			i++
		case "--acceptance":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --acceptance")
			}
			acceptance = strings.TrimSpace(args[i+1])
			i++
		case "--budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--budget")
			if err != nil {
				return commandError(jsonOut, "invalid_budget", err.Error())
			}
			budget = n
			i++
		case "--status":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --status")
			}
			status = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--ttl-hours":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --ttl-hours")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--ttl-hours")
			if err != nil {
				return commandError(jsonOut, "invalid_ttl_hours", err.Error())
			}
			ttlHours = n
			i++
		case "--activate":
			activate = true
		case "--clear":
			clear = true
		case "--list":
			listOnly = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if status != "" {
		switch status {
		case "open", "done", "paused":
		default:
			return commandErrorf(jsonOut, "invalid_status", "invalid --status: %s (expected open|done|paused)", status)
		}
	}
	if clear && id == "" {
		return commandError(jsonOut, "missing_required_flag", "--clear requires --id")
	}

	store, err := loadObjectiveStore(projectRoot)
	if err != nil {
		return commandErrorf(jsonOut, "objective_store_read_failed", "failed reading objective store: %v", err)
	}
	store = pruneExpiredObjectives(store)
	now := nowFn().UTC().Format(time.RFC3339)
	action := "read"

	if clear {
		delete(store.Objectives, id)
		if store.CurrentID == id {
			store.CurrentID = ""
		}
		action = "cleared"
	} else if strings.TrimSpace(goal) != "" || strings.TrimSpace(acceptance) != "" || budget > 0 || status != "" {
		if id == "" {
			return commandError(jsonOut, "missing_required_flag", "upsert requires --id")
		}
		record := store.Objectives[id]
		if strings.TrimSpace(record.ID) == "" {
			record.ID = id
			record.CreatedAt = now
			if status == "" {
				status = "open"
			}
		}
		if goal != "" {
			record.Goal = goal
		}
		if acceptance != "" {
			record.Acceptance = acceptance
		}
		if budget > 0 {
			record.Budget = budget
		}
		if status != "" {
			record.Status = status
		}
		record.UpdatedAt = now
		if ttlHours > 0 {
			record.ExpiresAt = nowFn().UTC().Add(time.Duration(ttlHours) * time.Hour).Format(time.RFC3339)
		}
		store.Objectives[id] = record
		store.CurrentID = id
		action = "upserted"
	} else if activate {
		if id == "" {
			return commandError(jsonOut, "missing_required_flag", "--activate requires --id")
		}
		if _, ok := store.Objectives[id]; !ok {
			return commandErrorf(jsonOut, "objective_not_found", "objective not found: %s", id)
		}
		store.CurrentID = id
		action = "activated"
	}

	if clear || action == "upserted" || action == "activated" {
		if err := saveObjectiveStore(projectRoot, store); err != nil {
			return commandErrorf(jsonOut, "objective_store_write_failed", "failed writing objective store: %v", err)
		}
	}

	names := make([]string, 0, len(store.Objectives))
	for name := range store.Objectives {
		names = append(names, name)
	}
	sort.Strings(names)
	objectives := make([]sessionObjectiveRecord, 0, len(names))
	for _, name := range names {
		objectives = append(objectives, store.Objectives[name])
	}

	selected := sessionObjectiveRecord{}
	foundSelected := false
	if id != "" {
		selected, foundSelected = store.Objectives[id]
	} else if store.CurrentID != "" {
		selected, foundSelected = store.Objectives[store.CurrentID]
	}

	payload := map[string]any{
		"action":      action,
		"projectRoot": projectRoot,
		"currentId":   store.CurrentID,
		"count":       len(objectives),
		"objectives":  objectives,
	}
	if foundSelected {
		payload["objective"] = selected
	}
	if id != "" {
		payload["id"] = id
	}

	if jsonOut {
		if !listOnly && id != "" && !foundSelected && !clear {
			payload["errorCode"] = "objective_not_found"
			writeJSON(payload)
			return 1
		}
		writeJSON(payload)
		return 0
	}

	if clear {
		fmt.Printf("objective cleared: %s\n", id)
		return 0
	}
	if foundSelected {
		fmt.Printf("objective=%s status=%s budget=%d\n", selected.ID, selected.Status, selected.Budget)
		if selected.Goal != "" {
			fmt.Println(selected.Goal)
		}
		return 0
	}
	if listOnly || len(objectives) > 0 {
		for _, record := range objectives {
			fmt.Printf("%s,%s,%d,%s\n", record.ID, record.Status, record.Budget, record.Goal)
		}
		return 0
	}
	fmt.Println("no objectives")
	return 0
}

func cmdSessionMemory(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	refresh := false
	semanticDiff := false
	ttlHours := 24
	maxLines := 80
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session memory")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
			i++
		case "--refresh":
			refresh = true
		case "--semantic-diff":
			semanticDiff = true
		case "--ttl-hours":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --ttl-hours")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--ttl-hours")
			if err != nil {
				return commandError(jsonOut, "invalid_ttl_hours", err.Error())
			}
			ttlHours = n
			i++
		case "--max-lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-lines")
			if err != nil {
				return commandError(jsonOut, "invalid_max_lines", err.Error())
			}
			maxLines = n
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}
	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	var (
		record sessionMemoryRecord
		delta  []string
		before []string
		err    error
		ok     bool
	)
	baselineLineCount := 0
	if refresh {
		previous, previousOK, readErr := loadSessionMemory(projectRoot, session)
		if readErr != nil {
			return commandErrorf(jsonOut, "memory_read_failed", "failed reading session memory: %v", readErr)
		}
		if previousOK {
			baselineLineCount = len(previous.Lines)
			before = append(before, previous.Lines...)
		}
		record, delta, err = buildSessionMemory(projectRoot, session, maxLines, ttlHours)
		if err != nil {
			return commandErrorf(jsonOut, "memory_refresh_failed", "failed refreshing session memory: %v", err)
		}
		ok = true
	} else {
		record, ok, err = loadSessionMemory(projectRoot, session)
		if err != nil {
			return commandErrorf(jsonOut, "memory_read_failed", "failed reading session memory: %v", err)
		}
		if !ok {
			return commandError(jsonOut, "memory_not_found", "no session memory found (run --refresh first)")
		}
		before = append(before, record.Lines...)
	}

	payload := map[string]any{
		"session":     session,
		"projectRoot": projectRoot,
		"updatedAt":   record.UpdatedAt,
		"expiresAt":   record.ExpiresAt,
		"maxLines":    record.MaxLines,
		"lineCount":   len(record.Lines),
		"lines":       record.Lines,
		"refresh":     refresh,
		"path":        sessionMemoryFile(projectRoot, session),
	}
	if refresh {
		payload["deltaLines"] = delta
		payload["deltaCount"] = len(delta)
		payload["deltaPath"] = sessionOutputFile(projectRoot, session)
		payload["deltaMetadata"] = map[string]any{
			"baselineLineCount": baselineLineCount,
			"currentLineCount":  len(record.Lines),
			"deltaCount":        len(delta),
			"maxLines":          record.MaxLines,
		}
	}
	semanticAdded := []map[string]any{}
	semanticRemoved := []map[string]any{}
	semanticUnchanged := 0
	if semanticDiff {
		current := record.Lines
		if !refresh {
			current = captureSessionSemanticLines(projectRoot, session, 320)
		}
		semanticAdded, semanticRemoved, semanticUnchanged = semanticDiffWithConfidence(current, before)
		payload["semanticDiff"] = map[string]any{
			"added":     semanticAdded,
			"removed":   semanticRemoved,
			"unchanged": semanticUnchanged,
		}
		if !refresh {
			payload["semanticDiffSource"] = "live_vs_cached"
		} else {
			payload["semanticDiffSource"] = "refresh_vs_previous"
		}
	}
	if jsonOut {
		writeJSON(payload)
		return 0
	}
	fmt.Printf("session=%s lines=%d refresh=%t\n", session, len(record.Lines), refresh)
	for _, line := range record.Lines {
		fmt.Println(line)
	}
	if semanticDiff {
		fmt.Printf("semantic_diff added=%d removed=%d\n", len(semanticAdded), len(semanticRemoved))
	}
	return 0
}

func semanticDiffWithConfidence(current, baseline []string) ([]map[string]any, []map[string]any, int) {
	currentSet := map[string]struct{}{}
	for _, line := range dedupeNonEmpty(current) {
		currentSet[line] = struct{}{}
	}
	baselineSet := map[string]struct{}{}
	for _, line := range dedupeNonEmpty(baseline) {
		baselineSet[line] = struct{}{}
	}
	added := make([]map[string]any, 0)
	for line := range currentSet {
		if _, ok := baselineSet[line]; ok {
			continue
		}
		confidence := "medium"
		if len(line) >= 24 {
			confidence = "high"
		}
		added = append(added, map[string]any{"line": line, "confidence": confidence})
	}
	removed := make([]map[string]any, 0)
	for line := range baselineSet {
		if _, ok := currentSet[line]; ok {
			continue
		}
		confidence := "medium"
		if len(line) >= 24 {
			confidence = "high"
		}
		removed = append(removed, map[string]any{"line": line, "confidence": confidence})
	}
	sort.Slice(added, func(i, j int) bool { return mapStringValue(added[i], "line") < mapStringValue(added[j], "line") })
	sort.Slice(removed, func(i, j int) bool { return mapStringValue(removed[i], "line") < mapStringValue(removed[j], "line") })
	unchanged := 0
	for line := range currentSet {
		if _, ok := baselineSet[line]; ok {
			unchanged++
		}
	}
	return added, removed, unchanged
}

func cmdSessionLane(args []string) int {
	projectRoot := canonicalProjectRoot(getPWD())
	name := ""
	goal := ""
	agent := ""
	mode := ""
	nestedPolicy := ""
	nestingIntent := ""
	prompt := ""
	model := ""
	topology := ""
	contract := ""
	budget := 0
	clear := false
	listOnly := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session lane")
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = canonicalProjectRoot(args[i+1])
			i++
		case "--name":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --name")
			}
			name = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--goal":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --goal")
			}
			goal = strings.TrimSpace(args[i+1])
			i++
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = strings.TrimSpace(args[i+1])
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			mode = strings.TrimSpace(args[i+1])
			i++
		case "--nested-policy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nested-policy")
			}
			nestedPolicy = strings.TrimSpace(args[i+1])
			i++
		case "--nesting-intent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nesting-intent")
			}
			nestingIntent = strings.TrimSpace(args[i+1])
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --prompt")
			}
			prompt = strings.TrimSpace(args[i+1])
			i++
		case "--model":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --model")
			}
			model = strings.TrimSpace(args[i+1])
			i++
		case "--budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--budget")
			if err != nil {
				return commandError(jsonOut, "invalid_budget", err.Error())
			}
			budget = n
			i++
		case "--topology":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --topology")
			}
			topology = strings.TrimSpace(args[i+1])
			i++
		case "--contract":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --contract")
			}
			contract = strings.TrimSpace(args[i+1])
			i++
		case "--clear":
			clear = true
		case "--list":
			listOnly = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	store, err := loadLaneStore(projectRoot)
	if err != nil {
		return commandErrorf(jsonOut, "lane_store_read_failed", "failed reading lane store: %v", err)
	}
	action := "read"
	if clear {
		if name == "" {
			return commandError(jsonOut, "missing_required_flag", "--clear requires --name")
		}
		delete(store.Lanes, name)
		action = "cleared"
		if err := saveLaneStore(projectRoot, store); err != nil {
			return commandErrorf(jsonOut, "lane_store_write_failed", "failed writing lane store: %v", err)
		}
	} else if goal != "" || agent != "" || mode != "" || nestedPolicy != "" || nestingIntent != "" || prompt != "" || model != "" || budget > 0 || topology != "" || contract != "" {
		if name == "" {
			return commandError(jsonOut, "missing_required_flag", "lane upsert requires --name")
		}
		record := store.Lanes[name]
		record.Name = name
		if goal != "" {
			record.Goal = goal
		}
		if agent != "" {
			record.Agent = agent
		}
		if mode != "" {
			record.Mode = mode
		}
		if nestedPolicy != "" {
			record.NestedPolicy = nestedPolicy
		}
		if nestingIntent != "" {
			record.NestingIntent = nestingIntent
		}
		if prompt != "" {
			record.Prompt = prompt
		}
		if model != "" {
			record.Model = model
		}
		if budget > 0 {
			record.Budget = budget
		}
		if topology != "" {
			record.Topology = topology
		}
		if contract != "" {
			record.Contract = contract
		}
		record.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
		store.Lanes[name] = record
		action = "upserted"
		if err := saveLaneStore(projectRoot, store); err != nil {
			return commandErrorf(jsonOut, "lane_store_write_failed", "failed writing lane store: %v", err)
		}
	}

	names := laneNames(store)
	lanes := make([]sessionLaneRecord, 0, len(names))
	for _, lane := range names {
		lanes = append(lanes, store.Lanes[lane])
	}
	selected, found := sessionLaneRecord{}, false
	if name != "" {
		selected, found = store.Lanes[name]
	}

	payload := map[string]any{
		"action":      action,
		"projectRoot": projectRoot,
		"count":       len(lanes),
		"lanes":       lanes,
	}
	if name != "" {
		payload["name"] = name
	}
	if found {
		payload["lane"] = selected
	}
	if jsonOut {
		if name != "" && !found && !clear && !listOnly && action == "read" {
			payload["errorCode"] = "lane_not_found"
			writeJSON(payload)
			return 1
		}
		writeJSON(payload)
		return 0
	}
	if clear {
		fmt.Printf("lane cleared: %s\n", name)
		return 0
	}
	if found {
		fmt.Printf("lane=%s goal=%s agent=%s mode=%s\n", selected.Name, selected.Goal, selected.Agent, selected.Mode)
		return 0
	}
	if listOnly || len(lanes) > 0 {
		for _, lane := range lanes {
			fmt.Printf("%s,%s,%s,%s\n", lane.Name, lane.Goal, lane.Agent, lane.Mode)
		}
		return 0
	}
	fmt.Println("no lanes")
	return 0
}

func parseOptionalInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}
