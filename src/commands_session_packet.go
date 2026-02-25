package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sessionPacketDeltaCursor struct {
	UpdatedAt string         `json:"updatedAt"`
	Fields    map[string]any `json:"fields"`
	Offset    int            `json:"offset,omitempty"`
}

type sessionPacketDeltaFieldChange struct {
	Field  string `json:"field"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

func cmdSessionPacket(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	lines := 120
	events := 8
	tokenBudget := 320
	summaryStyle := "ops"
	cursorFile := ""
	fieldsRaw := ""
	deltaJSON := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session packet")
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
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agentHint = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			modeHint = args[i+1]
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
		case "--summary-style":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --summary-style")
			}
			summaryStyle = args[i+1]
			i++
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--delta-json":
			deltaJSON = true
			jsonOut = true
		case "--fields":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --fields")
			}
			fieldsRaw = strings.TrimSpace(args[i+1])
			i++
		case "--json":
			jsonOut = true
		case "--json-min":
			jsonMin = true
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}
	if deltaJSON && cursorFile == "" {
		return commandError(jsonOut, "cursor_file_required_for_delta_json", "--delta-json requires --cursor-file")
	}
	fields := []string{}
	if fieldsRaw != "" {
		var parseErr error
		fields, parseErr = parseProjectionFields(fieldsRaw)
		if parseErr != nil {
			return commandError(jsonOut, "invalid_fields", parseErr.Error())
		}
		if !jsonOut {
			return commandError(jsonOut, "fields_requires_json", "--fields requires --json")
		}
	}
	var err error
	summaryStyle, err = parseCaptureSummaryStyle(summaryStyle)
	if err != nil {
		return commandError(jsonOut, "invalid_summary_style", err.Error())
	}
	if cursorFile != "" {
		cursorFile, err = expandAndCleanPath(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
	}
	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	agentHint, err = parseAgentHint(agentHint)
	if err != nil {
		return commandError(jsonOut, "invalid_agent_hint", err.Error())
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		return commandError(jsonOut, "invalid_mode_hint", err.Error())
	}

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, false, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)

	captureText := ""
	if tmuxHasSessionFn(session) {
		captureText, err = tmuxCapturePaneFn(session, lines)
		if err != nil {
			return commandErrorf(jsonOut, "capture_failed", "failed to capture pane: %v", err)
		}
		captureText = strings.Join(trimLines(filterCaptureNoise(captureText)), "\n")
	} else if strings.TrimSpace(status.OutputFile) != "" {
		if raw, readErr := os.ReadFile(status.OutputFile); readErr == nil {
			captureText = strings.Join(trimLines(filterCaptureNoise(string(raw))), "\n")
		}
	}
	if strings.TrimSpace(captureText) == "" {
		captureText = "(no live capture)"
	}
	summaryText, truncated := summarizeCaptureTextByStyle(session, projectRoot, captureText, tokenBudget, summaryStyle)
	nextAction := nextActionForState(status.SessionState)
	nextOffset := computeSessionCaptureNextOffset(session)

	if deltaJSON {
		current := map[string]any{
			"session":      session,
			"status":       status.Status,
			"sessionState": status.SessionState,
			"reason":       status.ClassificationReason,
			"nextAction":   nextAction,
			"nextOffset":   nextOffset,
			"summary":      summaryText,
			"summaryStyle": summaryStyle,
			"tokenBudget":  tokenBudget,
			"truncated":    truncated,
		}
		if status.SessionState == "not_found" {
			current["errorCode"] = "session_not_found"
		}
		if len(fields) > 0 {
			current = projectPayloadFields(current, fields)
		}
		flat := flattenSessionPacketDeltaFields(current)
		cursor, loadErr := loadSessionPacketDeltaCursor(cursorFile)
		if loadErr != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", loadErr)
		}
		added, removed, changed, deltaCount := computeSessionPacketDelta(flat, cursor.Fields)
		cursor.Fields = flat
		cursor.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
		if err := writeSessionPacketDeltaCursor(cursorFile, cursor); err != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
		}

		payload := map[string]any{
			"session": session,
			"delta": map[string]any{
				"added":   added,
				"removed": removed,
				"changed": changed,
				"count":   deltaCount,
			},
			"deltaCount": deltaCount,
		}
		if !jsonMin {
			payload["cursorFile"] = cursorFile
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		writeJSON(payload)
		if status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	items := make([]sessionHandoffItem, 0)
	droppedRecent := 0
	deltaFrom := -1
	nextDeltaOffset := -1
	if cursorFile != "" {
		deltaFrom, err = loadCursorOffset(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
		items, droppedRecent, nextDeltaOffset, err = readSessionHandoffDelta(projectRoot, session, deltaFrom, events)
		if err != nil {
			return commandErrorf(jsonOut, "handoff_delta_read_failed", "failed to read handoff delta: %v", err)
		}
		if err := writeCursorOffset(cursorFile, nextDeltaOffset); err != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
		}
	} else {
		tail, _ := readSessionEventTailFn(projectRoot, session, events)
		droppedRecent = tail.DroppedLines
		for _, event := range tail.Events {
			items = append(items, sessionHandoffItem{
				At:     event.At,
				Type:   event.Type,
				State:  event.State,
				Status: event.Status,
				Reason: event.Reason,
			})
		}
	}

	if jsonOut {
		payload := map[string]any{
			"session":      session,
			"status":       status.Status,
			"sessionState": status.SessionState,
			"reason":       status.ClassificationReason,
			"nextAction":   nextAction,
			"nextOffset":   nextOffset,
			"summary":      summaryText,
			"summaryStyle": summaryStyle,
			"tokenBudget":  tokenBudget,
			"truncated":    truncated,
		}
		if !jsonMin {
			payload["projectRoot"] = projectRoot
			payload["capture"] = map[string]any{
				"lines": lines,
			}
			payload["handoff"] = map[string]any{
				"events":        items,
				"droppedRecent": droppedRecent,
			}
		} else {
			payload["recent"] = items
		}
		if cursorFile != "" {
			payload["deltaFrom"] = deltaFrom
			payload["nextDeltaOffset"] = nextDeltaOffset
			payload["deltaCount"] = len(items)
			if !jsonMin {
				payload["cursorFile"] = cursorFile
			}
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		if len(fields) > 0 {
			payload = projectPayloadFields(payload, fields)
		}
		writeJSON(payload)
		if status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	fmt.Printf("state=%s status=%s reason=%s next=%s\n", status.SessionState, status.Status, status.ClassificationReason, nextAction)
	fmt.Println(summaryText)
	return boolExit(status.SessionState != "not_found")
}

func loadSessionPacketDeltaCursor(path string) (sessionPacketDeltaCursor, error) {
	cursor := sessionPacketDeltaCursor{Fields: map[string]any{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cursor, nil
		}
		return cursor, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return cursor, nil
	}
	parseErr := json.Unmarshal(raw, &cursor)
	if parseErr == nil {
		if cursor.Fields == nil {
			cursor.Fields = map[string]any{}
		}
		if cursor.Offset < 0 {
			cursor.Offset = 0
		}
		return cursor, nil
	}
	legacyOffset, intErr := strconv.Atoi(trimmed)
	if intErr == nil {
		if legacyOffset < 0 {
			legacyOffset = 0
		}
		cursor.Offset = legacyOffset
		return cursor, nil
	}
	return sessionPacketDeltaCursor{}, parseErr
}

func writeSessionPacketDeltaCursor(path string, cursor sessionPacketDeltaCursor) error {
	if cursor.Fields == nil {
		cursor.Fields = map[string]any{}
	}
	if cursor.Offset < 0 {
		cursor.Offset = 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func flattenSessionPacketDeltaFields(payload map[string]any) map[string]any {
	out := map[string]any{}
	flattenSessionPacketDeltaValue("", payload, out)
	return out
}

func flattenSessionPacketDeltaValue(prefix string, value any, out map[string]any) {
	typed, ok := value.(map[string]any)
	if !ok {
		if prefix != "" {
			out[prefix] = value
		}
		return
	}
	keys := make([]string, 0, len(typed))
	for key := range typed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		nextPrefix := key
		if prefix != "" {
			nextPrefix = prefix + "." + key
		}
		flattenSessionPacketDeltaValue(nextPrefix, typed[key], out)
	}
}

func computeSessionPacketDelta(current, previous map[string]any) ([]sessionPacketDeltaFieldChange, []sessionPacketDeltaFieldChange, []sessionPacketDeltaFieldChange, int) {
	if previous == nil {
		previous = map[string]any{}
	}
	added := make([]sessionPacketDeltaFieldChange, 0)
	removed := make([]sessionPacketDeltaFieldChange, 0)
	changed := make([]sessionPacketDeltaFieldChange, 0)

	currentKeys := make([]string, 0, len(current))
	for field := range current {
		currentKeys = append(currentKeys, field)
	}
	sort.Strings(currentKeys)
	for _, field := range currentKeys {
		currentValue := current[field]
		previousValue, ok := previous[field]
		if !ok {
			added = append(added, sessionPacketDeltaFieldChange{Field: field, After: currentValue})
			continue
		}
		if sessionPacketDeltaValueSignature(currentValue) != sessionPacketDeltaValueSignature(previousValue) {
			changed = append(changed, sessionPacketDeltaFieldChange{Field: field, Before: previousValue, After: currentValue})
		}
	}

	previousKeys := make([]string, 0, len(previous))
	for field := range previous {
		previousKeys = append(previousKeys, field)
	}
	sort.Strings(previousKeys)
	for _, field := range previousKeys {
		if _, ok := current[field]; ok {
			continue
		}
		removed = append(removed, sessionPacketDeltaFieldChange{Field: field, Before: previous[field]})
	}
	return added, removed, changed, len(added) + len(removed) + len(changed)
}

func sessionPacketDeltaValueSignature(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
