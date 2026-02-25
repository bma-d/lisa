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

type sessionContractCheckResult struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
	Checked int    `json:"checked,omitempty"`
	Failed  int    `json:"failed,omitempty"`
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

func cmdSessionContractCheck(args []string) int {
	projectRoot := getPWD()
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session contract-check")
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

	projectRoot = canonicalProjectRoot(projectRoot)
	results, ok := runSessionContractChecks(projectRoot)
	payload := map[string]any{
		"ok":          ok,
		"projectRoot": projectRoot,
		"checks":      results,
	}
	if !ok {
		payload["errorCode"] = "session_contract_check_failed"
	}
	if jsonOut {
		writeJSON(payload)
		return boolExit(ok)
	}

	for _, check := range results {
		state := "ok"
		if !check.OK {
			state = "fail"
		}
		if check.Detail != "" {
			fmt.Printf("%s: %s (%s)\n", check.Name, state, check.Detail)
		} else {
			fmt.Printf("%s: %s\n", check.Name, state)
		}
	}
	return boolExit(ok)
}

func runSessionContractChecks(projectRoot string) ([]sessionContractCheckResult, bool) {
	results := make([]sessionContractCheckResult, 0, 4)
	overallOK := true

	schemaRequired := []string{
		"session status",
		"session monitor",
		"session capture",
		"session packet",
		"session handoff",
		"session context-pack",
		"session route",
		"session list",
		"session smoke",
		"session checkpoint",
		"session dedupe",
		"session objective",
		"session memory",
		"session lane",
		"session budget-plan",
	}
	catalog := sessionSchemaCatalog()
	schemaMissing := make([]string, 0)
	for _, key := range schemaRequired {
		if _, ok := catalog[key]; !ok {
			schemaMissing = append(schemaMissing, key)
		}
	}
	schemaResult := sessionContractCheckResult{
		Name:    "schema_coverage",
		OK:      len(schemaMissing) == 0,
		Checked: len(schemaRequired),
		Failed:  len(schemaMissing),
	}
	if len(schemaMissing) > 0 {
		schemaResult.Detail = "missing: " + strings.Join(schemaMissing, ", ")
		overallOK = false
	}
	results = append(results, schemaResult)

	commandsPath := filepath.Join(projectRoot, "skills", "lisa", "data", "commands.md")
	raw, err := os.ReadFile(commandsPath)
	if err != nil {
		results = append(results, sessionContractCheckResult{
			Name:   "skill_commands_doc_read",
			OK:     false,
			Detail: err.Error(),
		})
		return results, false
	}
	text := string(raw)
	results = append(results, sessionContractCheckResult{
		Name:    "skill_commands_doc_read",
		OK:      true,
		Checked: 1,
	})

	missingCommands := make([]string, 0)
	for _, cap := range commandCapabilities {
		if !strings.Contains(text, cap.Name) {
			missingCommands = append(missingCommands, cap.Name)
		}
	}
	commandsResult := sessionContractCheckResult{
		Name:    "skill_command_index_coverage",
		OK:      len(missingCommands) == 0,
		Checked: len(commandCapabilities),
		Failed:  len(missingCommands),
	}
	if len(missingCommands) > 0 {
		commandsResult.Detail = "missing commands: " + strings.Join(missingCommands, ", ")
		overallOK = false
	}
	results = append(results, commandsResult)

	missingFlags := make([]string, 0)
	checkedFlags := 0
	for _, cap := range commandCapabilities {
		for _, flag := range cap.Flags {
			checkedFlags++
			if !strings.Contains(text, flag) {
				missingFlags = append(missingFlags, cap.Name+":"+flag)
			}
		}
	}
	flagsResult := sessionContractCheckResult{
		Name:    "skill_flag_surface_coverage",
		OK:      len(missingFlags) == 0,
		Checked: checkedFlags,
		Failed:  len(missingFlags),
	}
	if len(missingFlags) > 0 {
		preview := missingFlags
		if len(preview) > 6 {
			preview = preview[:6]
		}
		flagsResult.Detail = "missing flags sample: " + strings.Join(preview, ", ")
		overallOK = false
	}
	results = append(results, flagsResult)

	return results, overallOK
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
				"schema":          map[string]any{"type": "string"},
				"state":           map[string]any{"type": "object"},
				"nextAction": map[string]any{
					"oneOf": []any{
						map[string]any{"type": "string"},
						map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name":    map[string]any{"type": "string"},
								"command": map[string]any{"type": "string"},
								"id":      map[string]any{"type": "string"},
								"commandAst": map[string]any{
									"type": "object",
								},
							},
						},
					},
				},
				"nextDeltaOffset": map[string]any{"type": "integer"},
				"recent":          map[string]any{"type": "array"},
				"objective":       map[string]any{"type": "object"},
				"memory":          map[string]any{"type": "object"},
				"lane":            map[string]any{"type": "object"},
				"risks":           map[string]any{"type": "array"},
				"openQuestions":   map[string]any{"type": "array"},
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
				"queue":        map[string]any{"type": "array"},
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
				"policyFile":          map[string]any{"type": "string"},
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
		"session objective": {
			"type": "object",
			"properties": map[string]any{
				"action":     map[string]any{"type": "string"},
				"currentId":  map[string]any{"type": "string"},
				"objectives": map[string]any{"type": "array"},
				"objective":  map[string]any{"type": "object"},
				"errorCode":  map[string]any{"type": "string"},
			},
		},
		"session memory": {
			"type": "object",
			"properties": map[string]any{
				"session":       map[string]any{"type": "string"},
				"projectRoot":   map[string]any{"type": "string"},
				"updatedAt":     map[string]any{"type": "string"},
				"expiresAt":     map[string]any{"type": "string"},
				"maxLines":      map[string]any{"type": "integer"},
				"lineCount":     map[string]any{"type": "integer"},
				"lines":         map[string]any{"type": "array"},
				"refresh":       map[string]any{"type": "boolean"},
				"path":          map[string]any{"type": "string"},
				"deltaLines":    map[string]any{"type": "array"},
				"deltaCount":    map[string]any{"type": "integer"},
				"deltaPath":     map[string]any{"type": "string"},
				"deltaMetadata": map[string]any{"type": "object"},
				"errorCode":     map[string]any{"type": "string"},
			},
		},
		"session lane": {
			"type": "object",
			"properties": map[string]any{
				"action":      map[string]any{"type": "string"},
				"projectRoot": map[string]any{"type": "string"},
				"count":       map[string]any{"type": "integer"},
				"name":        map[string]any{"type": "string"},
				"lanes":       map[string]any{"type": "array"},
				"lane":        map[string]any{"type": "object"},
				"errorCode":   map[string]any{"type": "string"},
			},
		},
		"session budget-plan": {
			"type": "object",
			"properties": map[string]any{
				"goal":         map[string]any{"type": "string"},
				"agent":        map[string]any{"type": "string"},
				"mode":         map[string]any{"type": "string"},
				"costEstimate": map[string]any{"type": "object"},
				"hardStop":     map[string]any{"type": "object"},
				"runbook":      map[string]any{"type": "object"},
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
	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot
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
