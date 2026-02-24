package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

var sessionTurnSendFn = cmdSessionSend
var sessionTurnMonitorFn = cmdSessionMonitor
var sessionTurnPacketFn = cmdSessionPacket

type sessionTurnStepResult struct {
	ExitCode int
	Payload  map[string]any
	Stdout   string
	Stderr   string
}

func cmdSessionTurn(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	text := ""
	keys := ""
	enter := false
	agentHint := ""
	modeHint := ""
	expect := ""
	pollInterval := 0
	maxPolls := 0
	timeoutSeconds := 0
	stopOnWaitingSet := false
	stopOnWaiting := true
	waitingTurnCompleteSet := false
	waitingTurnComplete := false
	untilMarker := ""
	untilState := ""
	untilJSONPath := ""
	autoRecover := false
	recoverMax := 0
	recoverBudget := 0
	packetLines := 0
	packetEvents := 0
	tokenBudget := 0
	summaryStyle := ""
	cursorFile := ""
	fields := ""
	jsonMin := false
	jsonOut := true

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printSessionTurnUsage()
			return 0
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
		case "--text":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --text")
			}
			text = args[i+1]
			i++
		case "--keys":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --keys")
			}
			keys = args[i+1]
			i++
		case "--enter":
			enter = true
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agentHint = strings.TrimSpace(args[i+1])
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			modeHint = strings.TrimSpace(args[i+1])
			i++
		case "--expect":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --expect")
			}
			expect = strings.TrimSpace(args[i+1])
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
		case "--timeout-seconds":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --timeout-seconds")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--timeout-seconds")
			if err != nil {
				return commandError(jsonOut, "invalid_timeout_seconds", err.Error())
			}
			timeoutSeconds = n
			i++
		case "--stop-on-waiting":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --stop-on-waiting")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				return commandErrorf(jsonOut, "invalid_stop_on_waiting", "invalid --stop-on-waiting: %s (expected true|false)", args[i+1])
			}
			stopOnWaitingSet = true
			stopOnWaiting = parsed
			i++
		case "--waiting-requires-turn-complete":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --waiting-requires-turn-complete")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				return commandErrorf(jsonOut, "invalid_waiting_requires_turn_complete", "invalid --waiting-requires-turn-complete: %s (expected true|false)", args[i+1])
			}
			waitingTurnCompleteSet = true
			waitingTurnComplete = parsed
			i++
		case "--until-marker":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --until-marker")
			}
			untilMarker = strings.TrimSpace(args[i+1])
			i++
		case "--until-state":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --until-state")
			}
			untilState = strings.TrimSpace(args[i+1])
			i++
		case "--until-jsonpath":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --until-jsonpath")
			}
			untilJSONPath = strings.TrimSpace(args[i+1])
			i++
		case "--auto-recover":
			autoRecover = true
		case "--recover-max":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --recover-max")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--recover-max")
			if err != nil {
				return commandError(jsonOut, "invalid_recover_max", err.Error())
			}
			recoverMax = n
			i++
		case "--recover-budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --recover-budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--recover-budget")
			if err != nil {
				return commandError(jsonOut, "invalid_recover_budget", err.Error())
			}
			recoverBudget = n
			i++
		case "--lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--lines")
			if err != nil {
				return commandError(jsonOut, "invalid_lines", err.Error())
			}
			packetLines = n
			i++
		case "--events":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --events")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--events")
			if err != nil {
				return commandError(jsonOut, "invalid_events", err.Error())
			}
			packetEvents = n
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
			summaryStyle = strings.TrimSpace(args[i+1])
			i++
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--fields":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --fields")
			}
			fields = strings.TrimSpace(args[i+1])
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
	if strings.TrimSpace(text) == "" && strings.TrimSpace(keys) == "" {
		return commandError(jsonOut, "missing_send_payload", "provide --text or --keys")
	}
	if strings.TrimSpace(text) != "" && strings.TrimSpace(keys) != "" {
		return commandError(jsonOut, "send_payload_conflict", "use either --text or --keys, not both")
	}
	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot

	sendArgs := []string{
		"--session", session,
		"--project-root", projectRoot,
	}
	if strings.TrimSpace(text) != "" {
		sendArgs = append(sendArgs, "--text", text)
	} else {
		sendArgs = append(sendArgs, "--keys", keys)
	}
	if enter {
		sendArgs = append(sendArgs, "--enter")
	}
	sendArgs = append(sendArgs, "--json")

	sendStep, err := runSessionTurnSubcommand(sessionTurnSendFn, sendArgs)
	if err != nil {
		return commandErrorf(jsonOut, "turn_send_parse_failed", "failed reading send output: %v", err)
	}
	if sendStep.ExitCode != 0 {
		return emitSessionTurnStepFailure("send", session, projectRoot, sendStep.ExitCode, sendStep, sessionTurnStepResult{}, sessionTurnStepResult{}, jsonMin)
	}

	monitorArgs := []string{
		"--session", session,
		"--project-root", projectRoot,
		"--json",
	}
	if agentHint != "" {
		monitorArgs = append(monitorArgs, "--agent", agentHint)
	}
	if modeHint != "" {
		monitorArgs = append(monitorArgs, "--mode", modeHint)
	}
	if expect != "" {
		monitorArgs = append(monitorArgs, "--expect", expect)
	}
	if pollInterval > 0 {
		monitorArgs = append(monitorArgs, "--poll-interval", fmt.Sprintf("%d", pollInterval))
	}
	if maxPolls > 0 {
		monitorArgs = append(monitorArgs, "--max-polls", fmt.Sprintf("%d", maxPolls))
	}
	if timeoutSeconds > 0 {
		monitorArgs = append(monitorArgs, "--timeout-seconds", fmt.Sprintf("%d", timeoutSeconds))
	}
	if stopOnWaitingSet {
		monitorArgs = append(monitorArgs, "--stop-on-waiting", fmt.Sprintf("%t", stopOnWaiting))
	}
	if waitingTurnCompleteSet {
		monitorArgs = append(monitorArgs, "--waiting-requires-turn-complete", fmt.Sprintf("%t", waitingTurnComplete))
	}
	if untilMarker != "" {
		monitorArgs = append(monitorArgs, "--until-marker", untilMarker)
	}
	if untilState != "" {
		monitorArgs = append(monitorArgs, "--until-state", untilState)
	}
	if untilJSONPath != "" {
		monitorArgs = append(monitorArgs, "--until-jsonpath", untilJSONPath)
	}
	if autoRecover {
		monitorArgs = append(monitorArgs, "--auto-recover")
	}
	if recoverMax > 0 {
		monitorArgs = append(monitorArgs, "--recover-max", fmt.Sprintf("%d", recoverMax))
	}
	if recoverBudget > 0 {
		monitorArgs = append(monitorArgs, "--recover-budget", fmt.Sprintf("%d", recoverBudget))
	}

	monitorStep, err := runSessionTurnSubcommand(sessionTurnMonitorFn, monitorArgs)
	if err != nil {
		return commandErrorf(jsonOut, "turn_monitor_parse_failed", "failed reading monitor output: %v", err)
	}
	if monitorStep.ExitCode != 0 {
		return emitSessionTurnStepFailure("monitor", session, projectRoot, monitorStep.ExitCode, sendStep, monitorStep, sessionTurnStepResult{}, jsonMin)
	}

	packetArgs := []string{
		"--session", session,
		"--project-root", projectRoot,
		"--json",
	}
	if agentHint != "" {
		packetArgs = append(packetArgs, "--agent", agentHint)
	}
	if modeHint != "" {
		packetArgs = append(packetArgs, "--mode", modeHint)
	}
	if packetLines > 0 {
		packetArgs = append(packetArgs, "--lines", fmt.Sprintf("%d", packetLines))
	}
	if packetEvents > 0 {
		packetArgs = append(packetArgs, "--events", fmt.Sprintf("%d", packetEvents))
	}
	if tokenBudget > 0 {
		packetArgs = append(packetArgs, "--token-budget", fmt.Sprintf("%d", tokenBudget))
	}
	if summaryStyle != "" {
		packetArgs = append(packetArgs, "--summary-style", summaryStyle)
	}
	if cursorFile != "" {
		packetArgs = append(packetArgs, "--cursor-file", cursorFile)
	}
	if fields != "" {
		packetArgs = append(packetArgs, "--fields", fields)
	}

	packetStep, err := runSessionTurnSubcommand(sessionTurnPacketFn, packetArgs)
	if err != nil {
		return commandErrorf(jsonOut, "turn_packet_parse_failed", "failed reading packet output: %v", err)
	}
	if packetStep.ExitCode != 0 {
		return emitSessionTurnStepFailure("packet", session, projectRoot, packetStep.ExitCode, sendStep, monitorStep, packetStep, jsonMin)
	}

	if jsonMin {
		payload := map[string]any{
			"ok":          true,
			"session":     session,
			"projectRoot": projectRoot,
		}
		if monitorStep.Payload != nil {
			if finalState := sessionTurnString(monitorStep.Payload, "finalState"); finalState != "" {
				payload["finalState"] = finalState
			}
			if exitReason := sessionTurnString(monitorStep.Payload, "exitReason"); exitReason != "" {
				payload["exitReason"] = exitReason
			}
		}
		if packetStep.Payload != nil {
			if state := sessionTurnString(packetStep.Payload, "sessionState"); state != "" {
				payload["sessionState"] = state
			}
			if status := sessionTurnString(packetStep.Payload, "status"); status != "" {
				payload["status"] = status
			}
			if next := sessionTurnString(packetStep.Payload, "nextAction"); next != "" {
				payload["nextAction"] = next
			}
		}
		writeJSON(payload)
		return 0
	}

	writeJSON(map[string]any{
		"ok":              true,
		"session":         session,
		"projectRoot":     projectRoot,
		"send":            sendStep.Payload,
		"sendExitCode":    sendStep.ExitCode,
		"monitor":         monitorStep.Payload,
		"monitorExitCode": monitorStep.ExitCode,
		"packet":          packetStep.Payload,
		"packetExitCode":  packetStep.ExitCode,
	})
	return 0
}

func emitSessionTurnStepFailure(failedStep, session, projectRoot string, exitCode int, sendStep, monitorStep, packetStep sessionTurnStepResult, jsonMin bool) int {
	errCode := "turn_" + failedStep + "_failed"
	errMessage := failedStep + " step failed"
	stepPayload := sendStep.Payload
	switch failedStep {
	case "monitor":
		stepPayload = monitorStep.Payload
	case "packet":
		stepPayload = packetStep.Payload
	}
	if stepPayload != nil {
		if subCode := sessionTurnString(stepPayload, "errorCode"); subCode != "" {
			errCode = "turn_" + failedStep + "_failed"
			errMessage = subCode
		}
		if subMessage := sessionTurnString(stepPayload, "error"); subMessage != "" {
			errMessage = subMessage
		}
	}
	payload := map[string]any{
		"ok":          false,
		"session":     session,
		"projectRoot": projectRoot,
		"failedStep":  failedStep,
		"errorCode":   errCode,
		"error":       errMessage,
	}
	if !jsonMin {
		if sendStep.Payload != nil {
			payload["send"] = sendStep.Payload
		}
		if monitorStep.Payload != nil {
			payload["monitor"] = monitorStep.Payload
		}
		if packetStep.Payload != nil {
			payload["packet"] = packetStep.Payload
		}
		payload["sendExitCode"] = sendStep.ExitCode
		payload["monitorExitCode"] = monitorStep.ExitCode
		payload["packetExitCode"] = packetStep.ExitCode
	}
	writeJSON(payload)
	if exitCode > 0 {
		return exitCode
	}
	return 1
}

func runSessionTurnSubcommand(fn func([]string) int, args []string) (sessionTurnStepResult, error) {
	origStdout := os.Stdout
	origStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return sessionTurnStepResult{}, err
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return sessionTurnStepResult{}, err
	}
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	exitCode := fn(args)
	_ = stdoutW.Close()
	_ = stderrW.Close()

	stdoutRaw, readErr := io.ReadAll(stdoutR)
	_ = stdoutR.Close()
	if readErr != nil {
		_ = stderrR.Close()
		return sessionTurnStepResult{}, readErr
	}
	stderrRaw, readErr := io.ReadAll(stderrR)
	_ = stderrR.Close()
	if readErr != nil {
		return sessionTurnStepResult{}, readErr
	}

	result := sessionTurnStepResult{
		ExitCode: exitCode,
		Stdout:   strings.TrimSpace(string(stdoutRaw)),
		Stderr:   strings.TrimSpace(string(stderrRaw)),
	}
	if result.Stdout != "" {
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
			return sessionTurnStepResult{}, fmt.Errorf("invalid JSON output %q", result.Stdout)
		}
		result.Payload = payload
	}
	return result, nil
}

func sessionTurnString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func printSessionTurnUsage() {
	fmt.Fprintln(os.Stderr, "lisa session turn --session NAME (--text TEXT|--keys \"...\") [flags]")
	fmt.Fprintln(os.Stderr, "One-shot orchestration turn: send -> monitor -> packet.")
	fmt.Fprintln(os.Stderr, "Flags: --project-root --enter --poll-interval --max-polls --timeout-seconds --stop-on-waiting")
	fmt.Fprintln(os.Stderr, "       --waiting-requires-turn-complete --until-marker --until-state --until-jsonpath")
	fmt.Fprintln(os.Stderr, "       --expect --auto-recover --recover-max --recover-budget")
	fmt.Fprintln(os.Stderr, "       --lines --events --token-budget --summary-style --cursor-file --fields --json --json-min")
}
