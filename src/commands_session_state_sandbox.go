package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sessionStateSandboxSnapshot struct {
	Version        string                `json:"version"`
	ProjectRoot    string                `json:"projectRoot"`
	ObjectiveStore sessionObjectiveStore `json:"objectiveStore"`
	LaneStore      sessionLaneStore      `json:"laneStore"`
}

func cmdSessionStateSandbox(args []string) int {
	action := "list"
	projectRoot := canonicalProjectRoot(getPWD())
	projectRootExplicit := false
	filePath := ""
	jsonOut := true
	jsonMin := false

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = strings.ToLower(strings.TrimSpace(args[0]))
		args = args[1:]
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printSessionStateSandboxUsage()
			return 0
		case "--action":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --action")
			}
			action = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = canonicalProjectRoot(args[i+1])
			projectRootExplicit = true
			i++
		case "--file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --file")
			}
			filePath = strings.TrimSpace(args[i+1])
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

	switch action {
	case "list":
		return sessionStateSandboxList(projectRoot, jsonMin)
	case "clear":
		return sessionStateSandboxClear(projectRoot, jsonMin)
	case "snapshot":
		return sessionStateSandboxSnapshotAction(projectRoot, filePath, jsonMin)
	case "restore":
		if strings.TrimSpace(filePath) == "" {
			return commandError(jsonOut, "missing_required_flag", "--file is required for restore")
		}
		resolvedFile, err := expandAndCleanPath(filePath)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_file_path", "invalid --file: %v", err)
		}
		targetRoot := projectRoot
		if !projectRootExplicit {
			targetRoot = ""
		}
		return sessionStateSandboxRestoreAction(targetRoot, resolvedFile, jsonMin)
	default:
		return commandErrorf(jsonOut, "invalid_action", "invalid action: %s (expected list|snapshot|restore|clear)", action)
	}
}

func sessionStateSandboxList(projectRoot string, jsonMin bool) int {
	objectiveStore, err := loadObjectiveStore(projectRoot)
	if err != nil {
		return commandErrorf(true, "objective_store_read_failed", "failed reading objective store: %v", err)
	}
	laneStore, err := loadLaneStore(projectRoot)
	if err != nil {
		return commandErrorf(true, "lane_store_read_failed", "failed reading lane store: %v", err)
	}
	objectiveIDs := make([]string, 0, len(objectiveStore.Objectives))
	for id := range objectiveStore.Objectives {
		objectiveIDs = append(objectiveIDs, id)
	}
	sort.Strings(objectiveIDs)
	objectives := make([]sessionObjectiveRecord, 0, len(objectiveIDs))
	for _, id := range objectiveIDs {
		objectives = append(objectives, objectiveStore.Objectives[id])
	}

	lanes := make([]sessionLaneRecord, 0, len(laneStore.Lanes))
	for _, name := range laneNames(laneStore) {
		lanes = append(lanes, laneStore.Lanes[name])
	}

	payload := map[string]any{
		"action":         "list",
		"projectRoot":    projectRoot,
		"objectiveCount": len(objectives),
		"laneCount":      len(lanes),
		"currentId":      objectiveStore.CurrentID,
	}
	if !jsonMin {
		payload["objectiveStorePath"] = objectivesRegistryFile(projectRoot)
		payload["laneStorePath"] = lanesRegistryFile(projectRoot)
		payload["objectives"] = objectives
		payload["lanes"] = lanes
	}
	writeJSON(payload)
	return 0
}

func sessionStateSandboxClear(projectRoot string, jsonMin bool) int {
	objectivePath := objectivesRegistryFile(projectRoot)
	lanePath := lanesRegistryFile(projectRoot)
	clearedObjectives := false
	clearedLanes := false

	if err := os.Remove(objectivePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return commandErrorf(true, "objective_store_clear_failed", "failed clearing objective store: %v", err)
		}
	} else {
		clearedObjectives = true
	}
	if err := os.Remove(lanePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return commandErrorf(true, "lane_store_clear_failed", "failed clearing lane store: %v", err)
		}
	} else {
		clearedLanes = true
	}

	payload := map[string]any{
		"action":            "clear",
		"projectRoot":       projectRoot,
		"clearedObjectives": clearedObjectives,
		"clearedLanes":      clearedLanes,
	}
	if !jsonMin {
		payload["objectiveStorePath"] = objectivePath
		payload["laneStorePath"] = lanePath
	}
	writeJSON(payload)
	return 0
}

func sessionStateSandboxSnapshotAction(projectRoot, filePath string, jsonMin bool) int {
	objectiveStore, err := loadObjectiveStore(projectRoot)
	if err != nil {
		return commandErrorf(true, "objective_store_read_failed", "failed reading objective store: %v", err)
	}
	laneStore, err := loadLaneStore(projectRoot)
	if err != nil {
		return commandErrorf(true, "lane_store_read_failed", "failed reading lane store: %v", err)
	}
	if objectiveStore.Objectives == nil {
		objectiveStore.Objectives = map[string]sessionObjectiveRecord{}
	}
	if laneStore.Lanes == nil {
		laneStore.Lanes = map[string]sessionLaneRecord{}
	}
	snapshot := sessionStateSandboxSnapshot{
		Version:        "1",
		ProjectRoot:    projectRoot,
		ObjectiveStore: objectiveStore,
		LaneStore:      laneStore,
	}

	if strings.TrimSpace(filePath) != "" {
		resolvedFile, pathErr := expandAndCleanPath(filePath)
		if pathErr != nil {
			return commandErrorf(true, "invalid_file_path", "invalid --file: %v", pathErr)
		}
		filePath = resolvedFile
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			return commandErrorf(true, "snapshot_write_failed", "failed creating snapshot dir: %v", err)
		}
		raw, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			return commandErrorf(true, "snapshot_encode_failed", "failed encoding snapshot: %v", err)
		}
		if err := writeFileAtomic(filePath, raw); err != nil {
			return commandErrorf(true, "snapshot_write_failed", "failed writing snapshot: %v", err)
		}
	}

	payload := map[string]any{
		"action":         "snapshot",
		"projectRoot":    projectRoot,
		"objectiveCount": len(snapshot.ObjectiveStore.Objectives),
		"laneCount":      len(snapshot.LaneStore.Lanes),
	}
	if filePath != "" {
		payload["file"] = filePath
	}
	if !jsonMin || filePath == "" {
		payload["snapshot"] = snapshot
	}
	writeJSON(payload)
	return 0
}

func sessionStateSandboxRestoreAction(targetRoot, filePath string, jsonMin bool) int {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return commandErrorf(true, "snapshot_read_failed", "failed reading snapshot: %v", err)
	}
	snapshot := sessionStateSandboxSnapshot{}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return commandErrorf(true, "snapshot_parse_failed", "failed parsing snapshot: %v", err)
	}
	if strings.TrimSpace(snapshot.Version) == "" {
		return commandError(true, "snapshot_version_missing", "snapshot missing version")
	}
	if strings.TrimSpace(targetRoot) == "" {
		targetRoot = canonicalProjectRoot(snapshot.ProjectRoot)
	}
	if strings.TrimSpace(targetRoot) == "" {
		targetRoot = canonicalProjectRoot(getPWD())
	}
	if snapshot.ObjectiveStore.Objectives == nil {
		snapshot.ObjectiveStore.Objectives = map[string]sessionObjectiveRecord{}
	}
	if snapshot.LaneStore.Lanes == nil {
		snapshot.LaneStore.Lanes = map[string]sessionLaneRecord{}
	}
	if err := saveObjectiveStoreExact(targetRoot, snapshot.ObjectiveStore); err != nil {
		return commandErrorf(true, "objective_store_write_failed", "failed restoring objective store: %v", err)
	}
	if err := saveLaneStoreExact(targetRoot, snapshot.LaneStore); err != nil {
		return commandErrorf(true, "lane_store_write_failed", "failed restoring lane store: %v", err)
	}

	payload := map[string]any{
		"action":         "restore",
		"projectRoot":    targetRoot,
		"sourceFile":     filePath,
		"version":        snapshot.Version,
		"objectiveCount": len(snapshot.ObjectiveStore.Objectives),
		"laneCount":      len(snapshot.LaneStore.Lanes),
	}
	if !jsonMin {
		payload["objectiveStorePath"] = objectivesRegistryFile(targetRoot)
		payload["laneStorePath"] = lanesRegistryFile(targetRoot)
	}
	writeJSON(payload)
	return 0
}

func saveObjectiveStoreExact(projectRoot string, store sessionObjectiveStore) error {
	if store.Objectives == nil {
		store.Objectives = map[string]sessionObjectiveRecord{}
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(objectivesRegistryFile(projectRoot), raw)
}

func saveLaneStoreExact(projectRoot string, store sessionLaneStore) error {
	if store.Lanes == nil {
		store.Lanes = map[string]sessionLaneRecord{}
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(lanesRegistryFile(projectRoot), raw)
}

func printSessionStateSandboxUsage() {
	fmt.Fprintln(os.Stderr, "lisa session state-sandbox [list|snapshot|restore|clear] [flags]")
	fmt.Fprintln(os.Stderr, "Deterministic objective+lane store sandbox helper.")
	fmt.Fprintln(os.Stderr, "Flags: --project-root --file --action --json --json-min")
}
