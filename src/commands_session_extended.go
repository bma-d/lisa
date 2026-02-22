package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sessionCheckpointBundle struct {
	Version      string               `json:"version"`
	SavedAt      string               `json:"savedAt"`
	ProjectRoot  string               `json:"projectRoot"`
	Session      string               `json:"session"`
	Status       string               `json:"status"`
	SessionState string               `json:"sessionState"`
	Reason       string               `json:"reason"`
	NextAction   string               `json:"nextAction"`
	NextOffset   int                  `json:"nextOffset"`
	Recent       []sessionHandoffItem `json:"recent,omitempty"`
	ContextPack  string               `json:"contextPack,omitempty"`
	CaptureTail  string               `json:"captureTail,omitempty"`
}

type sessionDedupeRecord struct {
	Session     string `json:"session"`
	ProjectRoot string `json:"projectRoot"`
	ClaimedAt   string `json:"claimedAt"`
}

type sessionDedupeRegistry struct {
	UpdatedAt string                         `json:"updatedAt"`
	Items     map[string]sessionDedupeRecord `json:"items"`
}

func cmdSessionSchema(args []string) int {
	commandName := ""
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session schema")
		case "--command":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --command")
			}
			commandName = strings.TrimSpace(args[i+1])
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	catalog := sessionSchemaCatalog()
	if commandName != "" {
		normalized := normalizeSchemaCommandName(commandName)
		schema, ok := catalog[normalized]
		if !ok {
			return commandErrorf(jsonOut, "unknown_schema_command", "unknown --command: %s", commandName)
		}
		if jsonOut {
			writeJSON(map[string]any{
				"command": normalized,
				"schema":  schema,
			})
			return 0
		}
		fmt.Printf("%s\n", normalized)
		raw, _ := json.MarshalIndent(schema, "", "  ")
		fmt.Println(string(raw))
		return 0
	}

	if jsonOut {
		writeJSON(map[string]any{
			"commands": catalog,
		})
		return 0
	}
	names := make([]string, 0, len(catalog))
	for name := range catalog {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println(strings.Join(names, "\n"))
	return 0
}

func sessionSchemaCatalog() map[string]map[string]any {
	return map[string]map[string]any{
		"session status": {
			"type":     "object",
			"required": []string{"session", "status", "sessionState"},
			"properties": map[string]any{
				"session":      map[string]any{"type": "string"},
				"status":       map[string]any{"type": "string"},
				"sessionState": map[string]any{"type": "string"},
				"errorCode":    map[string]any{"type": "string"},
			},
		},
		"session monitor": {
			"type":     "object",
			"required": []string{"session", "finalState", "exitReason", "polls"},
			"properties": map[string]any{
				"session":    map[string]any{"type": "string"},
				"finalState": map[string]any{"type": "string"},
				"exitReason": map[string]any{"type": "string"},
				"polls":      map[string]any{"type": "integer"},
				"errorCode":  map[string]any{"type": "string"},
			},
		},
		"session capture": {
			"type": "object",
			"properties": map[string]any{
				"session":            map[string]any{"type": "string"},
				"capture":            map[string]any{"type": "string"},
				"summary":            map[string]any{"type": "string"},
				"markers":            map[string]any{"type": "array"},
				"markerMatches":      map[string]any{"type": "object"},
				"foundMarkers":       map[string]any{"type": "array"},
				"missingMarkers":     map[string]any{"type": "array"},
				"markerCounts":       map[string]any{"type": "object"},
				"markerHits":         map[string]any{"type": "array"},
				"deltaFrom":          map[string]any{"type": "string"},
				"deltaMode":          map[string]any{"type": "string"},
				"semanticDelta":      map[string]any{"type": "string"},
				"semanticDeltaCount": map[string]any{"type": "integer"},
				"semanticLines":      map[string]any{"type": "array"},
				"nextOffset":         map[string]any{"type": "integer"},
				"cursorFile":         map[string]any{"type": "string"},
			},
		},
		"session packet": {
			"type": "object",
			"properties": map[string]any{
				"session":         map[string]any{"type": "string"},
				"status":          map[string]any{"type": "string"},
				"sessionState":    map[string]any{"type": "string"},
				"summary":         map[string]any{"type": "string"},
				"nextAction":      map[string]any{"type": "string"},
				"nextOffset":      map[string]any{"type": "integer"},
				"nextDeltaOffset": map[string]any{"type": "integer"},
			},
		},
		"session handoff": {
			"type": "object",
			"properties": map[string]any{
				"session":         map[string]any{"type": "string"},
				"status":          map[string]any{"type": "string"},
				"sessionState":    map[string]any{"type": "string"},
				"nextAction":      map[string]any{"type": "string"},
				"nextDeltaOffset": map[string]any{"type": "integer"},
				"recent":          map[string]any{"type": "array"},
			},
		},
		"session context-pack": {
			"type": "object",
			"properties": map[string]any{
				"session":       map[string]any{"type": "string"},
				"sessionState":  map[string]any{"type": "string"},
				"status":        map[string]any{"type": "string"},
				"reason":        map[string]any{"type": "string"},
				"nextAction":    map[string]any{"type": "string"},
				"nextOffset":    map[string]any{"type": "integer"},
				"pack":          map[string]any{"type": "string"},
				"strategy":      map[string]any{"type": "string"},
				"tokenBudget":   map[string]any{"type": "integer"},
				"truncated":     map[string]any{"type": "boolean"},
				"projectRoot":   map[string]any{"type": "string"},
				"events":        map[string]any{"type": "integer"},
				"droppedRecent": map[string]any{"type": "integer"},
				"fromHandoff":   map[string]any{"type": "string"},
				"redactRules":   map[string]any{"type": "array"},
				"errorCode":     map[string]any{"type": "string"},
			},
		},
		"session route": {
			"type": "object",
			"properties": map[string]any{
				"goal":         map[string]any{"type": "string"},
				"mode":         map[string]any{"type": "string"},
				"nestedPolicy": map[string]any{"type": "string"},
				"command":      map[string]any{"type": "string"},
				"runbook":      map[string]any{"type": "object"},
				"topology":     map[string]any{"type": "object"},
				"costEstimate": map[string]any{"type": "object"},
			},
		},
		"session autopilot": {
			"type": "object",
			"properties": map[string]any{
				"ok":         map[string]any{"type": "boolean"},
				"goal":       map[string]any{"type": "string"},
				"mode":       map[string]any{"type": "string"},
				"session":    map[string]any{"type": "string"},
				"failedStep": map[string]any{"type": "string"},
				"errorCode":  map[string]any{"type": "string"},
			},
		},
		"session guard": {
			"type": "object",
			"properties": map[string]any{
				"sharedTmux":          map[string]any{"type": "boolean"},
				"projectRoot":         map[string]any{"type": "string"},
				"defaultSessionCount": map[string]any{"type": "integer"},
				"defaultSessions":     map[string]any{"type": "array"},
				"command":             map[string]any{"type": "string"},
				"safe":                map[string]any{"type": "boolean"},
				"commandRisk":         map[string]any{"type": "string"},
				"enforce":             map[string]any{"type": "boolean"},
				"adviceOnly":          map[string]any{"type": "boolean"},
				"machinePolicy":       map[string]any{"type": "string"},
				"warnings":            map[string]any{"type": "array"},
				"riskReasons":         map[string]any{"type": "array"},
				"remediation":         map[string]any{"type": "array"},
				"errorCode":           map[string]any{"type": "string"},
			},
		},
		"session list": {
			"type": "object",
			"properties": map[string]any{
				"sessions": map[string]any{"type": "array"},
				"items":    map[string]any{"type": "array"},
				"delta":    map[string]any{"type": "object"},
			},
		},
		"session smoke": {
			"type": "object",
			"properties": map[string]any{
				"ok":          map[string]any{"type": "boolean"},
				"levels":      map[string]any{"type": "integer"},
				"chaos":       map[string]any{"type": "string"},
				"chaosResult": map[string]any{"type": "object"},
				"errorCode":   map[string]any{"type": "string"},
			},
		},
		"session checkpoint": {
			"type": "object",
			"properties": map[string]any{
				"action":       map[string]any{"type": "string"},
				"session":      map[string]any{"type": "string"},
				"file":         map[string]any{"type": "string"},
				"nextAction":   map[string]any{"type": "string"},
				"sessionState": map[string]any{"type": "string"},
			},
		},
		"session dedupe": {
			"type": "object",
			"properties": map[string]any{
				"taskHash":        map[string]any{"type": "string"},
				"duplicate":       map[string]any{"type": "boolean"},
				"existingSession": map[string]any{"type": "string"},
				"existingRoot":    map[string]any{"type": "string"},
				"released":        map[string]any{"type": "boolean"},
				"claimed":         map[string]any{"type": "boolean"},
				"projectRoot":     map[string]any{"type": "string"},
				"errorCode":       map[string]any{"type": "string"},
			},
		},
	}
}

func normalizeSchemaCommandName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(name, "session ") {
		name = "session " + name
	}
	return name
}

func cmdSessionCheckpoint(args []string) int {
	action := "save"
	session := ""
	filePath := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	events := 8
	lines := 120
	strategy := "balanced"
	tokenBudget := 700
	jsonOut := hasJSONFlag(args)

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = strings.ToLower(strings.TrimSpace(args[0]))
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session checkpoint")
		case "--action":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --action")
			}
			action = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = strings.TrimSpace(args[i+1])
			i++
		case "--file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --file")
			}
			filePath = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
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
		case "--strategy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --strategy")
			}
			strategy = args[i+1]
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
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	switch action {
	case "save", "resume":
	default:
		return commandErrorf(jsonOut, "invalid_action", "invalid action: %s (expected save|resume)", action)
	}
	if strings.TrimSpace(filePath) == "" {
		return commandError(jsonOut, "missing_required_flag", "--file is required")
	}
	resolvedFile, err := expandAndCleanPath(filePath)
	if err != nil {
		return commandErrorf(jsonOut, "invalid_file_path", "invalid --file: %v", err)
	}
	filePath = resolvedFile

	if action == "resume" {
		bundle, err := loadCheckpointBundle(filePath)
		if err != nil {
			return commandErrorf(jsonOut, "checkpoint_read_failed", "failed reading checkpoint: %v", err)
		}
		if session != "" && session != bundle.Session {
			return commandErrorf(jsonOut, "checkpoint_session_mismatch", "session mismatch: --session=%s checkpoint=%s", session, bundle.Session)
		}
		if jsonOut {
			writeJSON(map[string]any{
				"action":       "resume",
				"file":         filePath,
				"session":      bundle.Session,
				"projectRoot":  bundle.ProjectRoot,
				"sessionState": bundle.SessionState,
				"nextAction":   bundle.NextAction,
				"checkpoint":   bundle,
			})
			return 0
		}
		fmt.Printf("session=%s state=%s next=%s\n", bundle.Session, bundle.SessionState, bundle.NextAction)
		return 0
	}

	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required for save")
	}
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	status, err := computeSessionStatusFn(session, projectRoot, "auto", "auto", false, 0)
	if err != nil {
		return commandErrorf(jsonOut, "status_compute_failed", "failed to compute status: %v", err)
	}
	status = normalizeStatusForSessionStatusOutput(status)
	if status.SessionState == "not_found" {
		return commandError(jsonOut, "session_not_found", "session not found")
	}

	strategyConfig, err := parseContextPackStrategy(strategy)
	if err != nil {
		return commandError(jsonOut, "invalid_strategy", err.Error())
	}
	if events <= 0 {
		events = strategyConfig.Events
	}
	if lines <= 0 {
		lines = strategyConfig.Lines
	}
	tail, _ := readSessionEventTailFn(projectRoot, session, events)
	recent := make([]sessionHandoffItem, 0, len(tail.Events))
	recentText := make([]string, 0, len(tail.Events))
	for _, event := range tail.Events {
		recent = append(recent, sessionHandoffItem{
			At:     event.At,
			Type:   event.Type,
			State:  event.State,
			Status: event.Status,
			Reason: event.Reason,
		})
		recentText = append(recentText, fmt.Sprintf("%s %s/%s %s", event.At, event.State, event.Status, event.Reason))
	}
	captureTail := "(no live capture)"
	if tmuxHasSessionFn(session) {
		if capture, captureErr := tmuxCapturePaneFn(session, lines); captureErr == nil {
			captureTail = strings.Join(trimLines(filterCaptureNoise(capture)), "\n")
		}
	}
	rawPack := buildContextPackRaw(strategyConfig.Name, session, status, recentText, captureTail)
	pack, _ := truncateToTokenBudget(rawPack, tokenBudget)

	bundle := sessionCheckpointBundle{
		Version:      "1",
		SavedAt:      nowFn().UTC().Format("2006-01-02T15:04:05Z"),
		ProjectRoot:  projectRoot,
		Session:      session,
		Status:       status.Status,
		SessionState: status.SessionState,
		Reason:       status.ClassificationReason,
		NextAction:   nextActionForState(status.SessionState),
		NextOffset:   computeSessionCaptureNextOffset(session),
		Recent:       recent,
		ContextPack:  pack,
		CaptureTail:  captureTail,
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return commandErrorf(jsonOut, "checkpoint_marshal_failed", "failed to encode checkpoint: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		return commandErrorf(jsonOut, "checkpoint_write_failed", "failed to create checkpoint dir: %v", err)
	}
	if err := writeFileAtomic(filePath, data); err != nil {
		return commandErrorf(jsonOut, "checkpoint_write_failed", "failed writing checkpoint: %v", err)
	}

	if jsonOut {
		writeJSON(map[string]any{
			"action":       "save",
			"file":         filePath,
			"session":      session,
			"projectRoot":  projectRoot,
			"sessionState": bundle.SessionState,
			"nextAction":   bundle.NextAction,
		})
		return 0
	}
	fmt.Printf("saved checkpoint: %s\n", filePath)
	return 0
}

func loadCheckpointBundle(path string) (sessionCheckpointBundle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionCheckpointBundle{}, err
	}
	bundle := sessionCheckpointBundle{}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return sessionCheckpointBundle{}, err
	}
	if strings.TrimSpace(bundle.Session) == "" {
		return sessionCheckpointBundle{}, fmt.Errorf("checkpoint missing session")
	}
	return bundle, nil
}

func cmdSessionDedupe(args []string) int {
	taskHash := ""
	session := ""
	release := false
	projectRoot := getPWD()
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session dedupe")
		case "--task-hash":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --task-hash")
			}
			taskHash = strings.TrimSpace(args[i+1])
			i++
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = strings.TrimSpace(args[i+1])
			i++
		case "--release":
			release = true
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	taskHash = strings.TrimSpace(taskHash)
	if taskHash == "" {
		return commandError(jsonOut, "missing_required_flag", "--task-hash is required")
	}
	projectRoot = canonicalProjectRoot(projectRoot)
	registryPath := fmt.Sprintf("/tmp/.lisa-%s-dedupe.json", projectHash(projectRoot))
	registry, err := loadSessionDedupeRegistry(registryPath)
	if err != nil {
		return commandErrorf(jsonOut, "dedupe_registry_read_failed", "failed reading dedupe registry: %v", err)
	}

	changed := false
	for hash, record := range registry.Items {
		recordRoot := canonicalProjectRoot(record.ProjectRoot)
		restore := withProjectRuntimeEnv(recordRoot)
		active := tmuxHasSessionFn(record.Session)
		restore()
		if active {
			continue
		}
		delete(registry.Items, hash)
		changed = true
	}

	if release {
		if _, ok := registry.Items[taskHash]; ok {
			delete(registry.Items, taskHash)
			changed = true
		}
		if changed {
			if err := saveSessionDedupeRegistry(registryPath, registry); err != nil {
				return commandErrorf(jsonOut, "dedupe_registry_write_failed", "failed writing dedupe registry: %v", err)
			}
		}
		if jsonOut {
			writeJSON(map[string]any{
				"taskHash":  taskHash,
				"released":  true,
				"duplicate": false,
			})
			return 0
		}
		fmt.Println("released")
		return 0
	}

	existing, exists := registry.Items[taskHash]
	if session != "" {
		if exists && existing.Session != session {
			restore := withProjectRuntimeEnv(existing.ProjectRoot)
			active := tmuxHasSessionFn(existing.Session)
			restore()
			if active {
				if jsonOut {
					writeJSON(map[string]any{
						"taskHash":        taskHash,
						"duplicate":       true,
						"existingSession": existing.Session,
						"existingRoot":    existing.ProjectRoot,
						"errorCode":       "task_duplicate_detected",
					})
				}
				return 1
			}
		}
		registry.Items[taskHash] = sessionDedupeRecord{
			Session:     session,
			ProjectRoot: projectRoot,
			ClaimedAt:   nowFn().UTC().Format("2006-01-02T15:04:05Z"),
		}
		changed = true
		if changed {
			if err := saveSessionDedupeRegistry(registryPath, registry); err != nil {
				return commandErrorf(jsonOut, "dedupe_registry_write_failed", "failed writing dedupe registry: %v", err)
			}
		}
		if jsonOut {
			writeJSON(map[string]any{
				"taskHash":        taskHash,
				"duplicate":       false,
				"claimed":         true,
				"existingSession": session,
				"existingRoot":    projectRoot,
			})
			return 0
		}
		fmt.Println(session)
		return 0
	}

	duplicate := false
	if exists {
		restore := withProjectRuntimeEnv(existing.ProjectRoot)
		active := tmuxHasSessionFn(existing.Session)
		restore()
		duplicate = active
		if !active {
			delete(registry.Items, taskHash)
			exists = false
			duplicate = false
			changed = true
		}
	}
	if changed {
		if err := saveSessionDedupeRegistry(registryPath, registry); err != nil {
			return commandErrorf(jsonOut, "dedupe_registry_write_failed", "failed writing dedupe registry: %v", err)
		}
	}
	if jsonOut {
		payload := map[string]any{
			"taskHash":  taskHash,
			"duplicate": duplicate,
		}
		if exists {
			payload["existingSession"] = existing.Session
			payload["existingRoot"] = existing.ProjectRoot
		}
		if duplicate {
			payload["errorCode"] = "task_duplicate_detected"
		}
		writeJSON(payload)
		if duplicate {
			return 1
		}
		return 0
	}
	if duplicate {
		fmt.Println(existing.Session)
		return 1
	}
	fmt.Println("none")
	return 0
}

func loadSessionDedupeRegistry(path string) (sessionDedupeRegistry, error) {
	out := sessionDedupeRegistry{
		Items: map[string]sessionDedupeRecord{},
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if out.Items == nil {
		out.Items = map[string]sessionDedupeRecord{}
	}
	return out, nil
}

func saveSessionDedupeRegistry(path string, registry sessionDedupeRegistry) error {
	if registry.Items == nil {
		registry.Items = map[string]sessionDedupeRecord{}
	}
	registry.UpdatedAt = nowFn().UTC().Format("2006-01-02T15:04:05Z")
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}
