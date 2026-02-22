package app

import (
	"fmt"
	"sort"
	"strings"
)

func cmdSessionDetectNested(args []string) int {
	agent := "codex"
	mode := "exec"
	nestedPolicy := "auto"
	nestingIntent := "auto"
	prompt := ""
	agentArgs := ""
	model := ""
	projectRoot := getPWD()
	rewrite := false
	why := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session detect-nested")
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
		case "--nested-policy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nested-policy")
			}
			nestedPolicy = args[i+1]
			i++
		case "--nesting-intent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nesting-intent")
			}
			nestingIntent = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --prompt")
			}
			prompt = args[i+1]
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
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--rewrite":
			rewrite = true
		case "--why":
			why = true
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
	nestedPolicy, err = parseNestedPolicy(nestedPolicy)
	if err != nil {
		return commandError(jsonOut, "invalid_nested_policy", err.Error())
	}
	nestingIntent, err = parseNestingIntent(nestingIntent)
	if err != nil {
		return commandError(jsonOut, "invalid_nesting_intent", err.Error())
	}
	model, err = parseModel(model)
	if err != nil {
		return commandError(jsonOut, "invalid_model", err.Error())
	}
	agentArgs, err = applyModelToAgentArgs(agent, agentArgs, model)
	if err != nil {
		return commandError(jsonOut, "invalid_model_configuration", err.Error())
	}
	projectRoot = canonicalProjectRoot(projectRoot)

	detection, effectiveAgentArgs, err := applyNestedPolicyToAgentArgs(agent, mode, prompt, agentArgs, nestedPolicy, nestingIntent)
	if err != nil {
		return commandError(jsonOut, "invalid_nested_policy_combination", err.Error())
	}

	payload := map[string]any{
		"agent":             agent,
		"mode":              mode,
		"nestedPolicy":      nestedPolicy,
		"nestingIntent":     nestingIntent,
		"prompt":            prompt,
		"projectRoot":       projectRoot,
		"agentArgs":         agentArgs,
		"effectiveAgentArgs": effectiveAgentArgs,
		"nestedDetection":   detection,
	}
	if model != "" {
		payload["model"] = model
	}
	if rewrite {
		payload["rewrites"] = nestedRewriteSuggestions(prompt, detection)
	}
	if why {
		payload["why"] = buildNestedDetectionWhy(prompt, detection)
	}
	if command, buildErr := buildAgentCommandWithOptions(agent, mode, prompt, effectiveAgentArgs, true); buildErr == nil {
		payload["command"] = command
	}

	if jsonOut {
		writeJSON(payload)
		return 0
	}

	fmt.Printf("eligible=%t autoBypass=%t reason=%s matchedHint=%s\n", detection.Eligible, detection.AutoBypass, detection.Reason, detection.MatchedHint)
	if cmd, ok := payload["command"].(string); ok && cmd != "" {
		fmt.Println(cmd)
	}
	if rewrite {
		if suggestions, ok := payload["rewrites"].([]string); ok && len(suggestions) > 0 {
			fmt.Println("rewrites:")
			for _, line := range suggestions {
				fmt.Printf("- %s\n", line)
			}
		}
	}
	if why {
		fmt.Printf("why: %v\n", payload["why"])
	}
	return 0
}

func nestedRewriteSuggestions(prompt string, detection nestedCodexDetection) []string {
	base := strings.TrimSpace(prompt)
	if base == "" {
		base = "Run nested child orchestration and report markers."
	}
	prefixes := []string{
		"Use ./lisa for child orchestration.",
		"Run lisa session spawn for child workers.",
		"Create nested lisa inside lisa inside lisa and report markers.",
	}
	out := make([]string, 0, len(prefixes))
	seen := map[string]struct{}{}

	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	if detection.AutoBypass {
		add(base)
		add("Use ./lisa for child orchestration. Continue with the existing task.")
		return out
	}
	for _, prefix := range prefixes {
		if strings.EqualFold(base, prefix) {
			add(prefix)
			continue
		}
		add(prefix + " " + base)
	}
	return out
}

type nestedDetectionWhySpan struct {
	Hint         string `json:"hint"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	Context      string `json:"context"`
	DocContext   bool   `json:"docContext"`
	ActionCtx    bool   `json:"actionContext"`
	NonExec      bool   `json:"nonExecutable"`
	MatchedHint  bool   `json:"matchedHint"`
}

func buildNestedDetectionWhy(prompt string, detection nestedCodexDetection) map[string]any {
	lower := strings.ToLower(prompt)
	hints := []string{"./lisa", "lisa session spawn", "nested lisa"}
	spans := make([]nestedDetectionWhySpan, 0, 4)
	for _, hint := range hints {
		for _, idx := range findAllSubstringIndices(lower, hint) {
			start := idx - 48
			if start < 0 {
				start = 0
			}
			end := idx + len(hint) + 48
			if end > len(lower) {
				end = len(lower)
			}
			window := lower[start:end]
			docCtx := containsAnyKeyword(window, []string{
				"docs", "documentation", "readme", "string", "literal", "quote", "quoted",
				"mention", "mentions", "example", "examples", "appears", "appear", "shown", "text",
			})
			actionCtx := containsAnyKeyword(window, []string{
				"run", "use", "invoke", "spawn", "execute", "launch", "call",
			})
			spans = append(spans, nestedDetectionWhySpan{
				Hint:        hint,
				Start:       idx,
				End:         idx + len(hint),
				Context:     strings.TrimSpace(prompt[start:end]),
				DocContext:  docCtx,
				ActionCtx:   actionCtx,
				NonExec:     docCtx && !actionCtx,
				MatchedHint: strings.EqualFold(strings.TrimSpace(detection.MatchedHint), strings.TrimSpace(hint)),
			})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start == spans[j].Start {
			return spans[i].Hint < spans[j].Hint
		}
		return spans[i].Start < spans[j].Start
	})
	return map[string]any{
		"eligible":      detection.Eligible,
		"reason":        detection.Reason,
		"autoBypass":    detection.AutoBypass,
		"matchedHint":   detection.MatchedHint,
		"hasBypassArg":  detection.HasBypassArg,
		"hasFullAutoArg": detection.HasFullAutoArg,
		"spans":         spans,
	}
}
