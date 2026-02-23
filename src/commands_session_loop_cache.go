package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sessionContextCacheRecord struct {
	Key       string   `json:"key"`
	UpdatedAt string   `json:"updatedAt"`
	ExpiresAt string   `json:"expiresAt"`
	Sessions  []string `json:"sessions,omitempty"`
	Lines     []string `json:"lines,omitempty"`
}

type sessionContextCacheStore struct {
	UpdatedAt string                               `json:"updatedAt"`
	Items     map[string]sessionContextCacheRecord `json:"items"`
}

var withSessionContextCacheLockFn = withSessionContextCacheLock

func cmdSessionLoop(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	pollInterval := 2
	maxPolls := 40
	strategy := "balanced"
	events := 8
	lines := 120
	tokenBudget := 700
	cursorFile := ""
	handoffCursorFile := ""
	schema := "v2"
	steps := 1
	maxTokens := 0
	maxSeconds := 0
	maxSteps := 0
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session loop")
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
		case "--poll-interval":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --poll-interval")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--poll-interval")
			if err != nil {
				return commandError(jsonOut, "invalid_poll_interval", err.Error())
			}
			pollInterval = n
			i++
		case "--max-polls":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-polls")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-polls")
			if err != nil {
				return commandError(jsonOut, "invalid_max_polls", err.Error())
			}
			maxPolls = n
			i++
		case "--strategy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --strategy")
			}
			strategy = strings.TrimSpace(args[i+1])
			i++
		case "--events":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --events")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--events")
			if err != nil {
				return commandError(jsonOut, "invalid_events", err.Error())
			}
			events = n
			i++
		case "--lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--lines")
			if err != nil {
				return commandError(jsonOut, "invalid_lines", err.Error())
			}
			lines = n
			i++
		case "--token-budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --token-budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--token-budget")
			if err != nil {
				return commandError(jsonOut, "invalid_token_budget", err.Error())
			}
			tokenBudget = n
			i++
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--handoff-cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --handoff-cursor-file")
			}
			handoffCursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--schema":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --schema")
			}
			schema = strings.TrimSpace(args[i+1])
			i++
		case "--steps":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --steps")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--steps")
			if err != nil {
				return commandError(jsonOut, "invalid_steps", err.Error())
			}
			steps = n
			i++
		case "--max-tokens":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-tokens")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-tokens")
			if err != nil {
				return commandError(jsonOut, "invalid_max_tokens", err.Error())
			}
			maxTokens = n
			i++
		case "--max-seconds":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-seconds")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-seconds")
			if err != nil {
				return commandError(jsonOut, "invalid_max_seconds", err.Error())
			}
			maxSeconds = n
			i++
		case "--max-steps":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-steps")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-steps")
			if err != nil {
				return commandError(jsonOut, "invalid_max_steps", err.Error())
			}
			maxSteps = n
			i++
		case "--json":
			jsonOut = true
		case "--json-min":
			jsonOut = true
			jsonMin = true
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
	if cursorFile == "" {
		cursorFile = fmt.Sprintf("/tmp/.lisa-%s-session-%s-loop-pack.cursor", projectHash(projectRoot), sessionArtifactID(session))
	}
	var err error
	cursorFile, err = expandAndCleanPath(cursorFile)
	if err != nil {
		return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
	}
	if handoffCursorFile == "" {
		handoffCursorFile = fmt.Sprintf("/tmp/.lisa-%s-session-%s-loop-handoff.cursor", projectHash(projectRoot), sessionArtifactID(session))
	}
	handoffCursorFile, err = expandAndCleanPath(handoffCursorFile)
	if err != nil {
		return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --handoff-cursor-file: %v", err)
	}
	switch strings.ToLower(strings.TrimSpace(schema)) {
	case "v1", "1":
		schema = "v1"
	case "v2", "2", "":
		schema = "v2"
	case "v3", "3":
		schema = "v3"
	default:
		return commandErrorf(jsonOut, "invalid_schema", "invalid --schema: %s (expected v1|v2|v3)", schema)
	}

	binPath, err := osExecutableFn()
	if err != nil {
		return commandErrorf(jsonOut, "binary_path_resolve_failed", "failed to resolve lisa binary path: %v", err)
	}
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		return commandError(jsonOut, "binary_path_empty", "failed to resolve lisa binary path")
	}

	runStep := func(stepName string, stepArgs []string) (map[string]any, int, string) {
		stdout, stderrText, runErr := runLisaSubcommandFn(binPath, stepArgs...)
		if runErr != nil {
			exitCode := commandExitCode(runErr)
			msg := strings.TrimSpace(stderrText)
			if msg == "" {
				msg = runErr.Error()
			}
			return nil, exitCode, msg
		}
		payload := map[string]any{}
		if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); decodeErr != nil {
			return nil, 1, fmt.Sprintf("%s output parse failed: %v", stepName, decodeErr)
		}
		return payload, 0, ""
	}

	observed := map[string]int{"tokens": 0, "seconds": 0, "steps": 0}
	stepPayloads := make([]map[string]any, 0, steps)
	ok := true
	errorCode := ""
	errorText := ""
	failedStep := ""

	for idx := 0; idx < steps; idx++ {
		start := nowFn()
		monitorOut, monitorCode, monitorErr := runStep("monitor", []string{
			"session", "monitor",
			"--session", session,
			"--project-root", projectRoot,
			"--poll-interval", strconv.Itoa(pollInterval),
			"--max-polls", strconv.Itoa(maxPolls),
			"--stop-on-waiting", "true",
			"--json-min",
		})
		if monitorCode != 0 {
			ok = false
			errorCode = "loop_monitor_failed"
			errorText = monitorErr
			failedStep = "monitor"
			break
		}
		diffOut, diffCode, diffErr := runStep("diff-pack", []string{
			"session", "diff-pack",
			"--session", session,
			"--project-root", projectRoot,
			"--strategy", strategy,
			"--events", strconv.Itoa(events),
			"--lines", strconv.Itoa(lines),
			"--token-budget", strconv.Itoa(tokenBudget),
			"--cursor-file", cursorFile,
			"--semantic-only",
			"--json-min",
		})
		if diffCode != 0 {
			ok = false
			errorCode = "loop_diff_pack_failed"
			errorText = diffErr
			failedStep = "diff-pack"
			break
		}
		handoffOut, handoffCode, handoffErr := runStep("handoff", []string{
			"session", "handoff",
			"--session", session,
			"--project-root", projectRoot,
			"--cursor-file", handoffCursorFile,
			"--schema", schema,
			"--json-min",
		})
		if handoffCode != 0 {
			ok = false
			errorCode = "loop_handoff_failed"
			errorText = handoffErr
			failedStep = "handoff"
			break
		}
		nextOut, nextCode, nextErr := runStep("next", []string{
			"session", "next",
			"--session", session,
			"--project-root", projectRoot,
			"--budget", strconv.Itoa(tokenBudget),
			"--json",
		})
		if nextCode != 0 {
			ok = false
			errorCode = "loop_next_failed"
			errorText = nextErr
			failedStep = "next"
			break
		}

		elapsedSeconds := int(nowFn().Sub(start).Seconds())
		if elapsedSeconds <= 0 {
			elapsedSeconds = 1
		}
		stepTokens := tokenBudget
		if parsed, ok := numberFromAny(diffOut["tokenBudget"]); ok {
			stepTokens = parsed
		}
		observed["tokens"] += maxInt(0, stepTokens)
		observed["seconds"] += elapsedSeconds
		observed["steps"]++

		stepPayload := map[string]any{
			"index":          idx + 1,
			"elapsedSeconds": elapsedSeconds,
		}
		if jsonMin {
			stepPayload["monitorState"] = mapStringValue(monitorOut, "finalState")
			stepPayload["nextAction"] = mapStringValue(nextOut, "nextAction")
			stepPayload["changed"] = diffOut["changed"]
			stepPayload["deltaCount"] = handoffOut["deltaCount"]
		} else {
			stepPayload["monitor"] = monitorOut
			stepPayload["diffPack"] = diffOut
			stepPayload["handoff"] = handoffOut
			stepPayload["next"] = nextOut
		}
		stepPayloads = append(stepPayloads, stepPayload)

		violations := budgetViolations(observed, maxTokens, maxSeconds, maxSteps)
		if len(violations) > 0 {
			ok = false
			errorCode = "budget_limit_exceeded"
			errorText = "loop budget limit reached"
			failedStep = "budget"
			break
		}
	}

	payload := map[string]any{
		"ok":                ok,
		"session":           session,
		"projectRoot":       projectRoot,
		"requestedSteps":    steps,
		"completedSteps":    len(stepPayloads),
		"cursorFile":        cursorFile,
		"handoffCursorFile": handoffCursorFile,
		"observed": map[string]int{
			"tokens":  observed["tokens"],
			"seconds": observed["seconds"],
			"steps":   observed["steps"],
		},
		"limits": map[string]int{
			"maxTokens":  maxTokens,
			"maxSeconds": maxSeconds,
			"maxSteps":   maxSteps,
		},
		"steps": stepPayloads,
	}
	violations := budgetViolations(observed, maxTokens, maxSeconds, maxSteps)
	if len(violations) > 0 {
		payload["violations"] = violations
	}
	if !ok {
		payload["errorCode"] = errorCode
		payload["error"] = errorText
		payload["failedStep"] = failedStep
	}

	if jsonOut {
		writeJSON(payload)
		return boolExit(ok)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "loop failed at %s: %s\n", failedStep, errorText)
		return 1
	}
	fmt.Printf("loop ok session=%s completed_steps=%d\n", session, len(stepPayloads))
	return 0
}

func cmdSessionContextCache(args []string) int {
	key := ""
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	refresh := false
	listOnly := false
	clear := false
	from := ""
	ttlHours := 48
	maxLines := 240
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session context-cache")
		case "--key":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --key")
			}
			key = strings.TrimSpace(args[i+1])
			i++
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
		case "--list":
			listOnly = true
		case "--clear":
			clear = true
		case "--from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --from")
			}
			from = strings.TrimSpace(args[i+1])
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

	if session != "" {
		resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
		if resolveErr != nil {
			return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
		}
		projectRoot = resolvedRoot
	} else {
		projectRoot = canonicalProjectRoot(projectRoot)
	}
	if key == "" && session != "" {
		key = "session:" + session
	}
	if listOnly && clear {
		return commandError(jsonOut, "invalid_flag_combination", "--list cannot be combined with --clear")
	}
	if !listOnly && key == "" {
		return commandError(jsonOut, "missing_required_flag", "--key is required (or provide --session)")
	}
	if clear && key == "" {
		return commandError(jsonOut, "missing_required_flag", "--clear requires --key")
	}
	if refresh && session == "" {
		return commandError(jsonOut, "missing_required_flag", "--refresh requires --session")
	}

	lockWrite := clear || refresh || strings.TrimSpace(from) != ""
	resultAction := "read"
	resultExists := false
	resultRecord := sessionContextCacheRecord{}
	resultKeys := []string{}
	resultItems := []sessionContextCacheRecord{}
	opErrCode := ""
	opErrText := ""

	lockErr := withSessionContextCacheLockFn(projectRoot, lockWrite, func() error {
		store, err := loadSessionContextCacheStore(projectRoot)
		if err != nil {
			opErrCode = "context_cache_read_failed"
			opErrText = fmt.Sprintf("failed reading context cache: %v", err)
			return nil
		}
		store = pruneSessionContextCacheStore(store)

		if listOnly {
			keys := make([]string, 0, len(store.Items))
			records := make([]sessionContextCacheRecord, 0, len(store.Items))
			for name, item := range store.Items {
				keys = append(keys, name)
				records = append(records, item)
			}
			sort.Strings(keys)
			sort.SliceStable(records, func(i, j int) bool { return records[i].Key < records[j].Key })
			resultKeys = keys
			resultItems = records
			return nil
		}

		action := "read"
		record, exists := store.Items[key]
		if clear {
			delete(store.Items, key)
			if err := saveSessionContextCacheStore(projectRoot, store); err != nil {
				opErrCode = "context_cache_write_failed"
				opErrText = fmt.Sprintf("failed writing context cache: %v", err)
				return nil
			}
			action = "cleared"
		}

		if action != "cleared" {
			lines := []string{}
			if from != "" {
				sourceLines, sourceErr := loadContextCacheLinesFromSource(from)
				if sourceErr != nil {
					opErrCode = "invalid_from"
					opErrText = fmt.Sprintf("failed loading --from: %v", sourceErr)
					return nil
				}
				lines = append(lines, sourceLines...)
			}
			if refresh {
				memoryRecord, _, refreshErr := buildSessionMemory(projectRoot, session, minInt(maxLines, 120), 24)
				if refreshErr != nil {
					opErrCode = "context_cache_refresh_failed"
					opErrText = fmt.Sprintf("failed refreshing session memory: %v", refreshErr)
					return nil
				}
				lines = append(lines, memoryRecord.Lines...)
			}
			if len(lines) > 0 {
				merged := dedupeNonEmpty(append(record.Lines, lines...))
				if len(merged) > maxLines {
					merged = merged[len(merged)-maxLines:]
				}
				record.Key = key
				record.Lines = merged
				record.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
				record.ExpiresAt = nowFn().UTC().Add(time.Duration(ttlHours) * time.Hour).Format(time.RFC3339)
				if session != "" {
					record.Sessions = dedupeNonEmpty(append(record.Sessions, session))
				}
				store.Items[key] = record
				if err := saveSessionContextCacheStore(projectRoot, store); err != nil {
					opErrCode = "context_cache_write_failed"
					opErrText = fmt.Sprintf("failed writing context cache: %v", err)
					return nil
				}
				action = "updated"
				exists = true
			}
		}

		resultAction = action
		resultExists = exists
		if exists {
			resultRecord = store.Items[key]
		}
		return nil
	})
	if lockErr != nil {
		return commandErrorf(jsonOut, "context_cache_lock_failed", "failed locking context cache: %v", lockErr)
	}
	if opErrCode != "" {
		return commandError(jsonOut, opErrCode, opErrText)
	}

	if listOnly {
		payload := map[string]any{
			"projectRoot": projectRoot,
			"count":       len(resultKeys),
			"keys":        resultKeys,
			"items":       resultItems,
		}
		if jsonOut {
			writeJSON(payload)
			return 0
		}
		for _, name := range resultKeys {
			fmt.Println(name)
		}
		return 0
	}

	payload := map[string]any{
		"action":      resultAction,
		"projectRoot": projectRoot,
		"key":         key,
		"exists":      resultExists,
	}
	if resultExists {
		payload["record"] = resultRecord
		payload["lineCount"] = len(resultRecord.Lines)
	}
	if !resultExists && resultAction == "read" {
		payload["errorCode"] = "context_cache_not_found"
	}
	if jsonOut {
		writeJSON(payload)
		if !resultExists && resultAction == "read" {
			return 1
		}
		return 0
	}
	if resultAction == "cleared" {
		fmt.Printf("context-cache cleared key=%s\n", key)
		return 0
	}
	if !resultExists {
		fmt.Println("context-cache not found")
		return 1
	}
	fmt.Printf("context-cache key=%s lines=%d\n", key, len(resultRecord.Lines))
	for _, line := range resultRecord.Lines {
		fmt.Println(line)
	}
	return 0
}

func sessionContextCacheFile(projectRoot string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-context-cache.json", projectHash(projectRoot))
}

func withSessionContextCacheLock(projectRoot string, exclusive bool, fn func() error) error {
	lockPath := sessionContextCacheFile(projectRoot) + ".lock"
	lockTimeout := getIntEnv("LISA_EVENT_LOCK_TIMEOUT_MS", defaultEventLockTimeoutMS)
	if exclusive {
		return withExclusiveFileLock(lockPath, lockTimeout, fn)
	}
	return withSharedFileLock(lockPath, lockTimeout, fn)
}

func loadSessionContextCacheStore(projectRoot string) (sessionContextCacheStore, error) {
	path := sessionContextCacheFile(projectRoot)
	if !fileExists(path) {
		return sessionContextCacheStore{Items: map[string]sessionContextCacheRecord{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionContextCacheStore{}, err
	}
	store := sessionContextCacheStore{}
	if err := json.Unmarshal(raw, &store); err != nil {
		return sessionContextCacheStore{}, err
	}
	if store.Items == nil {
		store.Items = map[string]sessionContextCacheRecord{}
	}
	return store, nil
}

func saveSessionContextCacheStore(projectRoot string, store sessionContextCacheStore) error {
	if store.Items == nil {
		store.Items = map[string]sessionContextCacheRecord{}
	}
	store.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(sessionContextCacheFile(projectRoot), data)
}

func pruneSessionContextCacheStore(store sessionContextCacheStore) sessionContextCacheStore {
	if store.Items == nil {
		store.Items = map[string]sessionContextCacheRecord{}
		return store
	}
	now := nowFn().UTC()
	for key, record := range store.Items {
		expiresAt := strings.TrimSpace(record.ExpiresAt)
		if expiresAt == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			continue
		}
		if now.After(ts) {
			delete(store.Items, key)
		}
	}
	return store
}

func loadContextCacheLinesFromSource(source string) ([]string, error) {
	rawSource := strings.TrimSpace(source)
	if rawSource == "" {
		return nil, fmt.Errorf("empty source")
	}
	var raw []byte
	var err error
	switch rawSource {
	case "-":
		raw, err = io.ReadAll(os.Stdin)
	default:
		path, resolveErr := expandAndCleanPath(rawSource)
		if resolveErr != nil {
			return nil, resolveErr
		}
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	rawTrimmed := strings.TrimSpace(string(raw))
	if rawTrimmed == "" {
		return []string{}, nil
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err == nil {
		lines := []string{}
		if values, ok := asMap["lines"].([]any); ok {
			for _, value := range values {
				lines = append(lines, strings.TrimSpace(fmt.Sprintf("%v", value)))
			}
		}
		for _, key := range []string{"pack", "combinedPack", "capture", "summary"} {
			if text := strings.TrimSpace(mapStringValue(asMap, key)); text != "" {
				lines = append(lines, extractSemanticLines(text)...)
			}
		}
		return dedupeNonEmpty(lines), nil
	}
	var asList []string
	if err := json.Unmarshal(raw, &asList); err == nil {
		return dedupeNonEmpty(asList), nil
	}
	return extractSemanticLines(string(raw)), nil
}

func budgetViolations(observed map[string]int, maxTokens, maxSeconds, maxSteps int) []map[string]any {
	violations := make([]map[string]any, 0)
	if maxTokens > 0 && observed["tokens"] >= maxTokens {
		violations = append(violations, map[string]any{"metric": "tokens", "observed": observed["tokens"], "limit": maxTokens})
	}
	if maxSeconds > 0 && observed["seconds"] >= maxSeconds {
		violations = append(violations, map[string]any{"metric": "seconds", "observed": observed["seconds"], "limit": maxSeconds})
	}
	if maxSteps > 0 && observed["steps"] >= maxSteps {
		violations = append(violations, map[string]any{"metric": "steps", "observed": observed["steps"], "limit": maxSteps})
	}
	return violations
}
