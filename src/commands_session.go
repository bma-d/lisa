package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func cmdSession(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lisa session <subcommand>")
		return 1
	}

	if args[0] == "--help" || args[0] == "-h" {
		return showHelp("session")
	}
	if args[0] == "help" {
		if len(args) > 1 {
			return showHelp("session " + args[1])
		}
		return showHelp("session")
	}

	switch args[0] {
	case "name":
		return cmdSessionName(args[1:])
	case "spawn":
		return cmdSessionSpawn(args[1:])
	case "detect-nested":
		return cmdSessionDetectNested(args[1:])
	case "send":
		return cmdSessionSend(args[1:])
	case "snapshot":
		return cmdSessionSnapshot(args[1:])
	case "status":
		return cmdSessionStatus(args[1:])
	case "explain":
		return cmdSessionExplain(args[1:])
	case "monitor":
		return cmdSessionMonitor(args[1:])
	case "capture":
		return cmdSessionCapture(args[1:])
	case "packet":
		return cmdSessionPacket(args[1:])
	case "schema":
		return cmdSessionSchema(args[1:])
	case "contract-check":
		return cmdSessionContractCheck(args[1:])
	case "checkpoint":
		return cmdSessionCheckpoint(args[1:])
	case "dedupe":
		return cmdSessionDedupe(args[1:])
	case "next":
		return cmdSessionNext(args[1:])
	case "aggregate":
		return cmdSessionAggregate(args[1:])
	case "prompt-lint":
		return cmdSessionPromptLint(args[1:])
	case "diff-pack":
		return cmdSessionDiffPack(args[1:])
	case "anomaly":
		return cmdSessionAnomaly(args[1:])
	case "budget-enforce":
		return cmdSessionBudgetEnforce(args[1:])
	case "replay":
		return cmdSessionReplay(args[1:])
	case "budget-plan":
		return cmdSessionBudgetPlan(args[1:])
	case "handoff":
		return cmdSessionHandoff(args[1:])
	case "context-pack":
		return cmdSessionContextPack(args[1:])
	case "route":
		return cmdSessionRoute(args[1:])
	case "router":
		return cmdSessionRoute(args[1:])
	case "autopilot":
		return cmdSessionAutopilot(args[1:])
	case "guard":
		return cmdSessionGuard(args[1:])
	case "objective":
		return cmdSessionObjective(args[1:])
	case "memory":
		return cmdSessionMemory(args[1:])
	case "lane":
		return cmdSessionLane(args[1:])
	case "tree":
		return cmdSessionTree(args[1:])
	case "smoke":
		return cmdSessionSmoke(args[1:])
	case "preflight":
		return cmdSessionPreflight(args[1:])
	case "list":
		return cmdSessionList(args[1:])
	case "exists":
		return cmdSessionExists(args[1:])
	case "kill":
		return cmdSessionKill(args[1:])
	case "kill-all":
		return cmdSessionKillAll(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown session subcommand: %s\n", args[0])
		return 1
	}
}

func cmdSessionName(args []string) int {
	agent := "claude"
	mode := "interactive"
	projectRoot := getPWD()
	tag := ""
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session name")
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			mode = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--tag":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --tag")
			}
			tag = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	var err error
	agent, err = parseAgent(agent)
	if err != nil {
		return commandError(jsonOut, "invalid_agent", err.Error())
	}
	mode, err = parseMode(mode)
	if err != nil {
		return commandError(jsonOut, "invalid_mode", err.Error())
	}
	projectRoot = canonicalProjectRoot(projectRoot)

	name := generateSessionName(projectRoot, agent, mode, tag)
	if jsonOut {
		writeJSON(map[string]any{
			"session":     name,
			"agent":       agent,
			"mode":        mode,
			"projectRoot": projectRoot,
			"tag":         tag,
		})
		return 0
	}
	fmt.Println(name)
	return 0
}

func cmdSessionSpawn(args []string) int {
	agent := "claude"
	mode := "interactive"
	lane := ""
	nestedPolicy := "auto"
	nestingIntent := "auto"
	projectRoot := getPWD()
	session := ""
	prompt := ""
	command := ""
	agentArgs := ""
	model := ""
	width := defaultTmuxWidth
	height := defaultTmuxHeight
	cleanupAllHashes := false
	skipPermissions := true
	dryRun := false
	detectNested := false
	jsonOut := hasJSONFlag(args)
	agentSet := false
	modeSet := false
	promptSet := false
	modelSet := false
	nestedPolicySet := false
	nestingIntentSet := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session spawn")
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = args[i+1]
			agentSet = true
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			mode = args[i+1]
			modeSet = true
			i++
		case "--lane":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lane")
			}
			lane = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--nested-policy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nested-policy")
			}
			nestedPolicy = args[i+1]
			nestedPolicySet = true
			i++
		case "--nesting-intent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nesting-intent")
			}
			nestingIntent = args[i+1]
			nestingIntentSet = true
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --prompt")
			}
			prompt = args[i+1]
			promptSet = true
			i++
		case "--command":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --command")
			}
			command = args[i+1]
			i++
		case "--agent-args":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent-args")
			}
			agentArgs = args[i+1]
			i++
		case "--model":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --model")
			}
			model = args[i+1]
			modelSet = true
			i++
		case "--width":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --width")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_width", "invalid --width")
			}
			width = n
			i++
		case "--height":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --height")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_height", "invalid --height")
			}
			height = n
			i++
		case "--cleanup-all-hashes":
			cleanupAllHashes = true
		case "--dry-run":
			dryRun = true
		case "--detect-nested":
			detectNested = true
		case "--no-dangerously-skip-permissions":
			skipPermissions = false
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	if lane != "" {
		laneRecord, found, laneErr := loadLaneRecord(projectRoot, lane)
		if laneErr != nil {
			return commandErrorf(jsonOut, "lane_load_failed", "failed loading lane %q: %v", lane, laneErr)
		}
		if !found {
			return commandErrorf(jsonOut, "lane_not_found", "lane not found: %s", lane)
		}
		if !agentSet && strings.TrimSpace(laneRecord.Agent) != "" {
			agent = laneRecord.Agent
		}
		if !modeSet && strings.TrimSpace(laneRecord.Mode) != "" {
			mode = laneRecord.Mode
		}
		if !nestedPolicySet && strings.TrimSpace(laneRecord.NestedPolicy) != "" {
			nestedPolicy = laneRecord.NestedPolicy
		}
		if !nestingIntentSet && strings.TrimSpace(laneRecord.NestingIntent) != "" {
			nestingIntent = laneRecord.NestingIntent
		}
		if !promptSet && strings.TrimSpace(laneRecord.Prompt) != "" {
			prompt = laneRecord.Prompt
		}
		if !modelSet && strings.TrimSpace(laneRecord.Model) != "" {
			model = laneRecord.Model
		}
	}
	objective, hasObjective := getCurrentObjective(projectRoot)
	if hasObjective {
		prompt = injectObjectiveIntoPrompt(prompt, objective, lane)
	}

	var err error
	agent, err = parseAgent(agent)
	if err != nil {
		return commandError(jsonOut, "invalid_agent", err.Error())
	}
	mode, err = parseMode(mode)
	if err != nil {
		return commandError(jsonOut, "invalid_mode", err.Error())
	}
	model, err = parseModel(model)
	if err != nil {
		return commandError(jsonOut, "invalid_model", err.Error())
	}
	agentArgs, err = applyModelToAgentArgs(agent, agentArgs, model)
	if err != nil {
		return commandError(jsonOut, "invalid_model_configuration", err.Error())
	}
	nestedPolicy, err = parseNestedPolicy(nestedPolicy)
	if err != nil {
		return commandError(jsonOut, "invalid_nested_policy", err.Error())
	}
	nestingIntent, err = parseNestingIntent(nestingIntent)
	if err != nil {
		return commandError(jsonOut, "invalid_nesting_intent", err.Error())
	}
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	session = strings.TrimSpace(session)
	if session != "" && !strings.HasPrefix(session, "lisa-") {
		return commandError(jsonOut, "invalid_session_name", `invalid --session: must start with "lisa-"`)
	}

	if session == "" {
		session = generateSessionName(projectRoot, agent, mode, "")
	}
	if tmuxHasSessionFn(session) {
		return commandErrorf(jsonOut, "session_already_exists", "session already exists: %s", session)
	}
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	emitSpawnFailureEvent := func(reason string) {
		if dryRun {
			return
		}
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "degraded", "idle", reason); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
		}
	}
	nestedDetection, adjustedArgs, nestedErr := applyNestedPolicyToAgentArgs(agent, mode, prompt, agentArgs, nestedPolicy, nestingIntent)
	if nestedErr != nil {
		return commandError(jsonOut, "invalid_nested_policy_combination", nestedErr.Error())
	}
	agentArgs = adjustedArgs
	if command == "" {
		command, err = buildAgentCommandWithOptions(agent, mode, prompt, agentArgs, skipPermissions)
		if err != nil {
			emitSpawnFailureEvent("spawn_command_build_error")
			return commandError(jsonOut, "agent_command_build_failed", err.Error())
		}
	} else {
		nestedDetection.Reason = "custom_command_override"
	}
	nestedDetection.EffectiveBypass = hasFlagToken(command, "--dangerously-bypass-approvals-and-sandbox")
	nestedDetection.EffectiveFullAuto = hasFlagToken(command, "--full-auto")
	commandToSend := command
	if mode == "exec" && command != "" {
		commandToSend = wrapExecCommand(command)
	}
	if strings.TrimSpace(commandToSend) != "" {
		commandToSend = wrapSessionCommand(commandToSend, runID)
	}
	oauthTokenID := ""
	oauthTokenPreview := claudeOAuthTokenSelection{}
	oauthTokenPreviewID := ""
	oauthReservationOwner := ""
	oauthReservationHeld := false
	if agent == "claude" {
		if dryRun {
			previewSelection, hasPreview, previewErr := previewClaudeOAuthTokenSelectionFn()
			if previewErr != nil {
				fmt.Fprintf(os.Stderr, "oauth warning: failed reading token pool: %v\n", previewErr)
			} else if hasPreview {
				oauthTokenPreview = previewSelection
				oauthTokenPreviewID = previewSelection.ID
			}
		} else {
			oauthReservationOwner = runID
			reservationSelection, hasReservation, reserveErr := reserveClaudeOAuthTokenForOwnerFn(oauthReservationOwner)
			if reserveErr != nil {
				fmt.Fprintf(os.Stderr, "oauth warning: failed reserving token: %v\n", reserveErr)
			} else if hasReservation {
				oauthTokenPreview = reservationSelection
				oauthTokenPreviewID = reservationSelection.ID
				oauthReservationHeld = true
			}
		}
		if oauthTokenPreviewID != "" && !dryRun {
			restoreOAuth := setEnvScoped(lisaClaudeOAuthTokenRuntimeEnv, oauthTokenPreview.Token)
			defer restoreOAuth()
		}
	}
	if oauthReservationHeld {
		defer func() {
			if !oauthReservationHeld {
				return
			}
			if _, releaseErr := releaseClaudeOAuthTokenReservationForOwnerFn(oauthReservationOwner, oauthTokenPreviewID); releaseErr != nil {
				fmt.Fprintf(os.Stderr, "oauth warning: failed releasing token reservation: %v\n", releaseErr)
			}
		}()
	}

	if dryRun {
		socketPath := tmuxSocketPathForProjectRoot(projectRoot)
		envPayload := map[string]string{
			"LISA_SESSION":        "true",
			"LISA_SESSION_NAME":   session,
			"LISA_AGENT":          agent,
			"LISA_MODE":           mode,
			"LISA_PROJECT_ROOT":   projectRoot,
			"LISA_TMUX_SOCKET":    socketPath,
			"LISA_PROJECT_HASH":   projectHash(projectRoot),
			"LISA_HEARTBEAT_FILE": sessionHeartbeatFile(projectRoot, session),
			"LISA_DONE_FILE":      sessionDoneFile(projectRoot, session),
		}
		if agent == "claude" && oauthTokenPreviewID != "" {
			envPayload[claudeOAuthTokenEnv] = "[managed-by-lisa]"
		}
		payload := map[string]any{
			"dryRun":         true,
			"session":        session,
			"agent":          agent,
			"mode":           mode,
			"lane":           lane,
			"model":          model,
			"nestedPolicy":   nestedPolicy,
			"nestingIntent":  nestingIntent,
			"runId":          runID,
			"projectRoot":    projectRoot,
			"command":        command,
			"startupCommand": commandToSend,
			"socketPath":     socketPath,
			"width":          width,
			"height":         height,
			"env":            envPayload,
		}
		if oauthTokenPreviewID != "" {
			payload["oauthTokenId"] = oauthTokenPreviewID
		}
		if hasObjective {
			payload["objective"] = map[string]any{
				"id":         objective.ID,
				"goal":       objective.Goal,
				"acceptance": objective.Acceptance,
				"budget":     objective.Budget,
			}
		}
		if detectNested {
			payload["nestedDetection"] = nestedDetection
		}
		if jsonOut {
			writeJSON(payload)
			return 0
		}
		fmt.Println(session)
		fmt.Printf("dry-run socket: %s\n", socketPath)
		fmt.Printf("dry-run command: %s\n", command)
		return 0
	}
	if err := pruneStaleSessionEventArtifactsFn(); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}

	cleanupOpts := cleanupOptions{AllHashes: cleanupAllHashes}
	if err := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts); err != nil {
		emitSpawnFailureEvent("spawn_cleanup_error")
		return commandErrorf(jsonOut, "spawn_cleanup_failed", "failed to reset previous session artifacts: %v", err)
	}
	if err := ensureHeartbeatWritableFn(sessionHeartbeatFile(projectRoot, session)); err != nil {
		emitSpawnFailureEvent("spawn_heartbeat_prepare_error")
		return commandErrorf(jsonOut, "spawn_heartbeat_prepare_failed", "failed to prepare heartbeat file: %v", err)
	}

	if err := tmuxNewSessionWithStartupFn(session, projectRoot, agent, mode, width, height, commandToSend); err != nil {
		msg := fmt.Sprintf("failed to create tmux session: %v", err)
		if shouldPrintCodexExecNestedTmuxHint(agent, mode, err) {
			msg += "; hint: codex exec --full-auto sandbox can block nested tmux sockets; use --mode interactive (then session send) or pass --agent-args '--dangerously-bypass-approvals-and-sandbox' (lisa omits --full-auto for that spawn)"
		}
		if cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", cleanupErr)
		}
		emitSpawnFailureEvent("spawn_tmux_new_error")
		return commandError(jsonOut, "spawn_tmux_new_failed", msg)
	}

	meta := sessionMeta{
		Session:       session,
		ParentSession: parentSessionFromEnv(session),
		Agent:         agent,
		Mode:          mode,
		Lane:          lane,
		OAuthTokenID:  oauthTokenPreviewID,
		RunID:         runID,
		ProjectRoot:   projectRoot,
		SocketPath:    tmuxSocketPathForProjectRoot(projectRoot),
		StartCmd:      command,
		Prompt:        prompt,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if hasObjective {
		meta.ObjectiveID = objective.ID
		meta.ObjectiveGoal = objective.Goal
		meta.ObjectiveAcceptance = objective.Acceptance
		meta.ObjectiveBudget = objective.Budget
	}
	if err := saveSessionMetaFn(projectRoot, session, meta); err != nil {
		killErr := tmuxKillSessionFn(session)
		cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts)
		msg := fmt.Sprintf("failed to persist metadata: %v", err)
		if killErr != nil {
			msg += fmt.Sprintf("; failed to kill session after metadata error: %v", killErr)
		}
		if cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", cleanupErr)
		}
		emitSpawnFailureEvent("spawn_meta_persist_error")
		return commandError(jsonOut, "spawn_meta_persist_failed", msg)
	}
	if agent == "claude" && oauthReservationHeld {
		selection, consumed, consumeErr := consumeReservedClaudeOAuthTokenForOwnerFn(oauthReservationOwner, oauthTokenPreviewID)
		if consumeErr != nil || !consumed {
			killErr := tmuxKillSessionFn(session)
			cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts)
			msg := "failed to finalize oauth token selection"
			if consumeErr != nil {
				msg += fmt.Sprintf(": %v", consumeErr)
			} else {
				msg += fmt.Sprintf(": token %s no longer available", oauthTokenPreviewID)
			}
			if killErr != nil {
				msg += fmt.Sprintf("; failed to kill session after oauth selection error: %v", killErr)
			}
			if cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", cleanupErr)
			}
			emitSpawnFailureEvent("spawn_oauth_selection_error")
			return commandError(jsonOut, "spawn_oauth_selection_failed", msg)
		}
		oauthReservationHeld = false
		oauthTokenID = selection.ID
	}
	_ = os.Remove(sessionStateFile(projectRoot, session))
	if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "spawned", "active", "spawn_success"); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}

	if jsonOut {
		payload := map[string]any{
			"session":       session,
			"agent":         agent,
			"mode":          mode,
			"lane":          lane,
			"nestedPolicy":  nestedPolicy,
			"nestingIntent": nestingIntent,
			"runId":         runID,
			"projectRoot":   projectRoot,
			"socketPath":    tmuxSocketPathForProjectRoot(projectRoot),
			"command":       command,
		}
		if oauthTokenID != "" {
			payload["oauthTokenId"] = oauthTokenID
		}
		if hasObjective {
			payload["objective"] = map[string]any{
				"id":         objective.ID,
				"goal":       objective.Goal,
				"acceptance": objective.Acceptance,
				"budget":     objective.Budget,
			}
		}
		if detectNested {
			payload["nestedDetection"] = nestedDetection
		}
		writeJSON(payload)
		return 0
	}
	fmt.Println(session)
	return 0
}

func shouldPrintCodexExecNestedTmuxHint(agent, mode string, err error) bool {
	if err == nil || agent != "codex" || mode != "exec" {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "error creating /tmp/") ||
		strings.Contains(msg, "error connecting to /var/folders/") ||
		strings.Contains(msg, "error connecting to /private/tmp/")
}

func parentSessionFromEnv(currentSession string) string {
	parent := strings.TrimSpace(os.Getenv("LISA_SESSION_NAME"))
	if parent == "" || parent == currentSession {
		return ""
	}
	if !strings.HasPrefix(parent, "lisa-") {
		return ""
	}
	return parent
}

type nestedCodexDetection struct {
	Eligible          bool   `json:"eligible"`
	NestedPolicy      string `json:"nestedPolicy,omitempty"`
	NestingIntent     string `json:"nestingIntent,omitempty"`
	AutoBypass        bool   `json:"autoBypass"`
	Reason            string `json:"reason"`
	MatchedHint       string `json:"matchedHint,omitempty"`
	HasBypassArg      bool   `json:"hasBypassArg"`
	HasFullAutoArg    bool   `json:"hasFullAutoArg"`
	EffectiveBypass   bool   `json:"effectiveBypass,omitempty"`
	EffectiveFullAuto bool   `json:"effectiveFullAuto,omitempty"`
}

func detectNestedCodexBypass(agent, mode, prompt, agentArgs, nestingIntent string) nestedCodexDetection {
	detection := nestedCodexDetection{
		Eligible:       normalizeAgent(agent) == "codex" && normalizeMode(mode) == "exec",
		HasBypassArg:   hasFlagToken(agentArgs, "--dangerously-bypass-approvals-and-sandbox"),
		HasFullAutoArg: hasFlagToken(agentArgs, "--full-auto"),
		Reason:         "no_nested_hint",
		NestingIntent:  nestingIntent,
	}
	if !detection.Eligible {
		detection.Reason = "not_codex_exec"
		return detection
	}
	if detection.HasBypassArg {
		detection.Reason = "agent_args_has_bypass"
		return detection
	}
	if detection.HasFullAutoArg {
		detection.Reason = "agent_args_has_full_auto"
		return detection
	}
	switch nestingIntent {
	case "nested":
		detection.AutoBypass = true
		detection.MatchedHint = "nesting-intent:nested"
		detection.Reason = "nesting_intent_nested"
		return detection
	case "neutral":
		detection.AutoBypass = false
		detection.MatchedHint = ""
		detection.Reason = "nesting_intent_neutral"
		return detection
	}
	lowerPrompt := strings.ToLower(prompt)
	switch {
	case strings.Contains(lowerPrompt, "lisa session spawn") && !isNonExecutableNestedMention(lowerPrompt, "lisa session spawn"):
		detection.AutoBypass = true
		detection.MatchedHint = "lisa session spawn"
		detection.Reason = "prompt_contains_lisa_session_spawn"
	case strings.Contains(lowerPrompt, "nested lisa") && !isNonExecutableNestedMention(lowerPrompt, "nested lisa"):
		detection.AutoBypass = true
		detection.MatchedHint = "nested lisa"
		detection.Reason = "prompt_contains_nested_lisa"
	case strings.Contains(lowerPrompt, "./lisa") && !isNonExecutableNestedMention(lowerPrompt, "./lisa"):
		detection.AutoBypass = true
		detection.MatchedHint = "./lisa"
		detection.Reason = "prompt_contains_dot_slash_lisa"
	}
	return detection
}

func isNonExecutableNestedMention(lowerPrompt, hint string) bool {
	indices := findAllSubstringIndices(lowerPrompt, hint)
	if len(indices) == 0 {
		return false
	}
	for _, idx := range indices {
		start := idx - 48
		if start < 0 {
			start = 0
		}
		end := idx + len(hint) + 48
		if end > len(lowerPrompt) {
			end = len(lowerPrompt)
		}
		window := lowerPrompt[start:end]
		hasDocContext := containsAnyKeyword(window, []string{
			"docs", "documentation", "readme", "string", "literal", "quote", "quoted",
			"mention", "mentions", "example", "examples", "appears", "appear", "shown", "text",
		})
		if !hasDocContext {
			return false
		}
		hasActionContext := containsAnyKeyword(window, []string{
			"run", "use", "invoke", "spawn", "execute", "launch", "call",
		})
		if hasActionContext {
			return false
		}
	}
	return true
}

func findAllSubstringIndices(haystack, needle string) []int {
	if needle == "" {
		return nil
	}
	indices := make([]int, 0, 2)
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			break
		}
		indices = append(indices, start+idx)
		start += idx + len(needle)
		if start >= len(haystack) {
			break
		}
	}
	return indices
}

func containsAnyKeyword(input string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(input, keyword) {
			return true
		}
	}
	return false
}

func parseNestedPolicy(policy string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "auto":
		return "auto", nil
	case "force":
		return "force", nil
	case "off":
		return "off", nil
	default:
		return "", fmt.Errorf("invalid --nested-policy: %s (expected auto|force|off)", policy)
	}
}

func parseNestingIntent(intent string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(intent)) {
	case "", "auto":
		return "auto", nil
	case "nested", "bypass":
		return "nested", nil
	case "neutral", "none", "off":
		return "neutral", nil
	default:
		return "", fmt.Errorf("invalid --nesting-intent: %s (expected auto|nested|neutral)", intent)
	}
}

func applyNestedPolicyToAgentArgs(agent, mode, prompt, agentArgs, nestedPolicy, nestingIntent string) (nestedCodexDetection, string, error) {
	detection := detectNestedCodexBypass(agent, mode, prompt, agentArgs, nestingIntent)
	detection.NestedPolicy = nestedPolicy

	switch nestedPolicy {
	case "auto":
		if detection.AutoBypass {
			agentArgs = strings.TrimSpace(agentArgs + " --dangerously-bypass-approvals-and-sandbox")
		}
		return detection, agentArgs, nil
	case "off":
		if detection.Eligible && !detection.HasBypassArg {
			detection.AutoBypass = false
			detection.MatchedHint = ""
			detection.Reason = "nested_policy_off"
		}
		return detection, agentArgs, nil
	case "force":
		if !detection.Eligible {
			detection.AutoBypass = false
			detection.Reason = "nested_policy_force_not_applicable"
			return detection, agentArgs, nil
		}
		if detection.HasFullAutoArg && !detection.HasBypassArg {
			return detection, agentArgs, fmt.Errorf("invalid --nested-policy force with --agent-args --full-auto: codex exec bypass cannot be combined with full-auto")
		}
		detection.AutoBypass = true
		detection.MatchedHint = "nested-policy:force"
		detection.Reason = "nested_policy_force"
		if !detection.HasBypassArg {
			agentArgs = strings.TrimSpace(agentArgs + " --dangerously-bypass-approvals-and-sandbox")
		}
		return detection, agentArgs, nil
	default:
		return detection, agentArgs, fmt.Errorf("invalid --nested-policy: %s", nestedPolicy)
	}
}

func shouldAutoEnableNestedCodexBypass(agent, mode, prompt, agentArgs, nestingIntent string) bool {
	return detectNestedCodexBypass(agent, mode, prompt, agentArgs, nestingIntent).AutoBypass
}

func shouldRecordInputTimestamp(text string, keyList []string, enter bool) bool {
	if text != "" {
		return enter
	}
	if enter {
		return true
	}
	for _, key := range keyList {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "enter" || normalized == "kpenter" || normalized == "c-m" {
			return true
		}
	}
	return false
}

func recordSessionInputTimestamp(projectRoot, session string, at time.Time) error {
	statePath := sessionStateFile(projectRoot, session)
	_, err := withStateFileLockFn(statePath, func() error {
		state, stateErr := loadSessionStateWithError(statePath)
		if stateErr != nil {
			state = sessionState{}
		}
		state.LastInputAt = at.Unix()
		state.LastInputAtNanos = at.UnixNano()
		return saveSessionState(statePath, state)
	})
	return err
}

func objectiveReminderPartsFromMeta(meta sessionMeta) []string {
	parts := []string{}
	hasObjectiveContext := false
	if v := strings.TrimSpace(meta.ObjectiveID); v != "" {
		parts = append(parts, "id="+v)
		hasObjectiveContext = true
	}
	if v := strings.TrimSpace(meta.ObjectiveGoal); v != "" {
		parts = append(parts, "goal="+v)
		hasObjectiveContext = true
	}
	if v := strings.TrimSpace(meta.ObjectiveAcceptance); v != "" {
		parts = append(parts, "acceptance="+v)
		hasObjectiveContext = true
	}
	if meta.ObjectiveBudget > 0 {
		parts = append(parts, fmt.Sprintf("budget=%d", meta.ObjectiveBudget))
		hasObjectiveContext = true
	}
	if !hasObjectiveContext {
		return nil
	}
	if v := strings.TrimSpace(meta.Lane); v != "" {
		parts = append(parts, "lane="+v)
	}
	return parts
}

func buildObjectiveReminderPrefixFromMeta(meta sessionMeta) string {
	parts := objectiveReminderPartsFromMeta(meta)
	if len(parts) == 0 {
		return ""
	}
	return "Objective reminder: " + strings.Join(parts, " | ")
}

func objectiveReminderAlreadyPresent(text, prefix string, meta sessionMeta) bool {
	lowerText := strings.ToLower(strings.TrimSpace(text))
	if lowerText == "" {
		return false
	}
	lowerPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if lowerPrefix != "" && strings.Contains(lowerText, lowerPrefix) {
		return true
	}
	if !strings.Contains(lowerText, "objective reminder:") {
		return false
	}
	for _, part := range objectiveReminderPartsFromMeta(meta) {
		if !strings.Contains(lowerText, strings.ToLower(part)) {
			return false
		}
	}
	return true
}

func cmdSessionSend(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	text := ""
	keys := ""
	enter := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session send")
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
	if text == "" && keys == "" {
		return commandError(jsonOut, "missing_send_payload", "provide --text or --keys")
	}
	if text != "" && keys != "" {
		return commandError(jsonOut, "send_payload_conflict", "use either --text or --keys, not both")
	}
	keyList := []string(nil)
	if keys != "" {
		keyList = strings.Fields(keys)
		if len(keyList) == 0 {
			return commandError(jsonOut, "empty_keys", "empty --keys")
		}
	}

	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	meta, metaErr := loadSessionMeta(projectRoot, session)
	if metaErr != nil {
		meta = sessionMeta{Session: session, ProjectRoot: projectRoot}
	}
	if text != "" {
		prefix := buildObjectiveSendPrefix(meta)
		if prefix == "" {
			prefix = buildObjectiveReminderPrefixFromMeta(meta)
		}
		if prefix != "" && !objectiveReminderAlreadyPresent(text, prefix, meta) {
			text = prefix + "\n" + text
		}
	}
	if !tmuxHasSessionFn(session) {
		if jsonOut {
			writeJSONError("session_not_found", "session not found", map[string]any{
				"session":     session,
				"projectRoot": projectRoot,
			})
		} else {
			fmt.Fprintln(os.Stderr, "session not found")
		}
		return 1
	}
	recordInputAt := shouldRecordInputTimestamp(text, keyList, enter)
	sendAt := time.Now()

	if text != "" {
		if err := tmuxSendTextFn(session, text, enter); err != nil {
			return commandErrorf(jsonOut, "send_text_failed", "failed sending text: %v", err)
		}
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "in_progress", "active", "send_text"); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
		}
	} else {
		if err := tmuxSendKeysFn(session, keyList, enter); err != nil {
			return commandErrorf(jsonOut, "send_keys_failed", "failed sending keys: %v", err)
		}
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "in_progress", "active", "send_keys"); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
		}
	}
	if recordInputAt {
		if err := recordSessionInputTimestamp(projectRoot, session, sendAt); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: failed to record input timestamp: %v\n", err)
		}
	}

	if jsonOut {
		objective := objectivePayloadFromMeta(meta)
		if jsonMin {
			payload := map[string]any{
				"session": session,
				"ok":      true,
			}
			if objective != nil {
				payload["objective"] = objective
			}
			writeJSON(payload)
			return 0
		}
		payload := map[string]any{
			"session": session,
			"ok":      true,
			"enter":   enter,
		}
		if strings.TrimSpace(meta.Lane) != "" {
			payload["lane"] = meta.Lane
		}
		if objective != nil {
			payload["objective"] = objective
		}
		writeJSON(payload)
		return 0
	}
	fmt.Println("ok")
	return 0
}
