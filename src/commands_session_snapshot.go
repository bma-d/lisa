package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func cmdSessionSnapshot(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	lines := 200
	deltaFrom := ""
	stripNoise := true
	markersRaw := ""
	failNotFound := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session snapshot")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = args[i+1]
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
		case "--delta-from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --delta-from")
			}
			deltaFrom = strings.TrimSpace(args[i+1])
			i++
		case "--markers":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --markers")
			}
			markersRaw = args[i+1]
			i++
		case "--keep-noise":
			stripNoise = false
		case "--strip-noise":
			stripNoise = true
		case "--fail-not-found":
			failNotFound = true
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
	markers, err := parseCaptureMarkersFlag(markersRaw)
	if err != nil {
		return commandError(jsonOut, "invalid_markers", err.Error())
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

	capture := ""
	deltaMode := ""
	nextOffset := 0
	captureAvailable := status.SessionState != "not_found" && tmuxHasSessionFn(session)
	if captureAvailable {
		capture, err = tmuxCapturePaneFn(session, lines)
		if err != nil {
			return commandErrorf(jsonOut, "capture_failed", "failed to capture pane: %v", err)
		}
		capture = strings.Join(trimLines(capture), "\n")
		if stripNoise {
			capture = filterCaptureNoise(capture)
		}
		if err := updateCaptureState(projectRoot, session, capture); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: failed to update capture state: %v\n", err)
		}
		if deltaFrom != "" {
			capture, deltaMode, nextOffset, err = applyCaptureDelta(projectRoot, session, capture, deltaFrom)
			if err != nil {
				return commandError(jsonOut, "invalid_delta_from", err.Error())
			}
		} else {
			nextOffset = len(capture)
		}
	}

	markerSummary := captureMarkerSummary{}
	if len(markers) > 0 {
		markerSummary = buildCaptureMarkerSummary(capture, markers)
	}

	if jsonOut {
		payload := map[string]any{
			"session":      status.Session,
			"status":       status.Status,
			"sessionState": status.SessionState,
			"todosDone":    status.TodosDone,
			"todosTotal":   status.TodosTotal,
			"waitEstimate": status.WaitEstimate,
			"capture":      capture,
			"nextOffset":   nextOffset,
		}
		if deltaFrom != "" && !jsonMin {
			payload["deltaFrom"] = deltaFrom
			payload["deltaMode"] = deltaMode
		}
		if !captureAvailable {
			payload["capture"] = ""
			payload["nextOffset"] = 0
		}
		if len(markers) > 0 {
			delete(payload, "capture")
			payload["markers"] = markerSummary.Markers
			payload["markerMatches"] = markerSummary.Matches
			payload["foundMarkers"] = markerSummary.Found
			payload["missingMarkers"] = markerSummary.Missing
			if !jsonMin {
				payload["markerCounts"] = markerSummary.Counts
			}
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		if jsonMin {
			delete(payload, "todosDone")
			delete(payload, "todosTotal")
			delete(payload, "waitEstimate")
		}
		writeJSON(payload)
		if status.SessionState == "not_found" && failNotFound {
			return 1
		}
		return 0
	}

	if len(markers) > 0 {
		for _, marker := range markerSummary.Markers {
			fmt.Printf("%s=%t\n", marker, markerSummary.Matches[marker])
		}
		if status.SessionState == "not_found" && failNotFound {
			return 1
		}
		return 0
	}
	fmt.Println(capture)
	if status.SessionState == "not_found" && failNotFound {
		return 1
	}
	return 0
}

func parsePositiveIntFlag(raw, flagName string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s", flagName)
	}
	return n, nil
}
