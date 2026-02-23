package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type staleSessionInfo struct {
	Session     string `json:"session"`
	ProjectRoot string `json:"projectRoot,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	MetaPath    string `json:"metaPath,omitempty"`
	PruneCmd    string `json:"pruneCmd,omitempty"`
}

type sessionListItem struct {
	Session       string `json:"session"`
	Status        string `json:"status,omitempty"`
	SessionState  string `json:"sessionState,omitempty"`
	NextAction    string `json:"nextAction,omitempty"`
	PriorityScore int    `json:"priorityScore,omitempty"`
	PriorityLabel string `json:"priorityLabel,omitempty"`
	ProjectRoot   string `json:"projectRoot,omitempty"`
	SocketPath    string `json:"socketPath,omitempty"`
}

type sessionListDeltaCursor struct {
	UpdatedAt string                     `json:"updatedAt"`
	Items     map[string]sessionListItem `json:"items"`
}

func cmdSessionList(args []string) int {
	projectOnly := false
	allSockets := false
	activeOnly := false
	withNextAction := false
	priority := false
	stale := false
	prunePreview := false
	deltaJSON := false
	watchJSON := false
	watchInterval := 2
	watchCycles := 0
	cursorFile := ""
	projectRoot := getPWD()
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session list")
		case "--project-only":
			projectOnly = true
		case "--all-sockets":
			allSockets = true
		case "--active-only":
			activeOnly = true
		case "--with-next-action":
			withNextAction = true
		case "--priority":
			priority = true
			withNextAction = true
		case "--stale":
			stale = true
		case "--prune-preview":
			prunePreview = true
		case "--delta-json":
			deltaJSON = true
			jsonOut = true
		case "--watch-json":
			watchJSON = true
			jsonOut = true
		case "--watch-interval":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --watch-interval")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--watch-interval")
			if err != nil {
				return commandError(jsonOut, "invalid_watch_interval", err.Error())
			}
			watchInterval = n
			i++
		case "--watch-cycles":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --watch-cycles")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--watch-cycles")
			if err != nil {
				return commandError(jsonOut, "invalid_watch_cycles", err.Error())
			}
			watchCycles = n
			i++
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
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

	projectRoot = canonicalProjectRoot(projectRoot)
	if watchJSON {
		deltaJSON = true
		jsonOut = true
		if !jsonMin {
			jsonMin = true
		}
	}
	if prunePreview && !stale {
		return commandError(jsonOut, "prune_preview_requires_stale", "--prune-preview requires --stale")
	}
	if deltaJSON && stale {
		return commandError(jsonOut, "delta_json_incompatible_with_stale", "--delta-json cannot be combined with --stale")
	}
	if stale && activeOnly {
		return commandError(jsonOut, "active_only_incompatible_with_stale", "--active-only cannot be combined with --stale")
	}
	if cursorFile != "" && !deltaJSON {
		return commandError(jsonOut, "cursor_file_requires_delta_json", "--cursor-file requires --delta-json")
	}
	if deltaJSON && cursorFile != "" {
		cursorFileResolved, err := expandAndCleanPath(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
		cursorFile = cursorFileResolved
	}
	if watchJSON {
		return cmdSessionListWatch(args, watchInterval, watchCycles)
	}
	list := []string{}
	var err error
	if allSockets {
		list, err = listSessionsAcrossSockets(projectRoot, projectOnly)
	} else {
		restoreRuntime := withProjectRuntimeEnv(projectRoot)
		defer restoreRuntime()
		list, err = tmuxListSessionsFn(projectOnly, projectRoot)
	}
	if err != nil {
		return commandErrorf(jsonOut, "session_list_failed", "failed to list sessions: %v", err)
	}
	items := []sessionListItem{}
	if withNextAction || activeOnly || priority {
		filtered := make([]string, 0, len(list))
		items = make([]sessionListItem, 0, len(list))
		for _, session := range list {
			resolvedRoot := resolveSessionProjectRoot(session, projectRoot, false)
			restoreRuntime := withProjectRuntimeEnv(resolvedRoot)
			status, statusErr := computeSessionStatusFn(session, resolvedRoot, "auto", "auto", false, 0)
			restoreRuntime()
			if statusErr != nil {
				if !activeOnly {
					filtered = append(filtered, session)
				}
				if withNextAction {
					items = append(items, sessionListItem{
						Session:     session,
						ProjectRoot: resolvedRoot,
						SocketPath:  resolveSessionSocketPath(session, resolvedRoot),
						NextAction:  "session status",
					})
				}
				continue
			}
			status = normalizeStatusForSessionStatusOutput(status)
			if activeOnly && status.SessionState == "not_found" {
				continue
			}
			filtered = append(filtered, session)
			if withNextAction {
				priorityScore, priorityLabel := computeSessionPriority(status)
				items = append(items, sessionListItem{
					Session:       session,
					Status:        status.Status,
					SessionState:  status.SessionState,
					NextAction:    nextActionForState(status.SessionState),
					PriorityScore: priorityScore,
					PriorityLabel: priorityLabel,
					ProjectRoot:   resolvedRoot,
					SocketPath:    resolveSessionSocketPath(session, resolvedRoot),
				})
			}
		}
		list = filtered
		if priority {
			sortSessionListByPriority(items)
			sorted := make([]string, 0, len(items))
			for _, item := range items {
				sorted = append(sorted, item.Session)
			}
			list = sorted
		}
	}
	staleSessions := []string{}
	staleInfos := []staleSessionInfo{}
	historicalCount := 0
	if stale {
		staleList, staleDetails, historyCount, staleErr := listStaleSessions(projectRoot, list)
		if staleErr != nil {
			return commandErrorf(jsonOut, "session_list_stale_failed", "failed to compute stale sessions: %v", staleErr)
		}
		staleSessions = staleList
		staleInfos = staleDetails
		historicalCount = historyCount
	}
	deltaAdded := []sessionListItem{}
	deltaRemoved := []sessionListItem{}
	deltaChanged := []sessionListItem{}
	if deltaJSON {
		if cursorFile == "" {
			cursorFile = sessionListDeltaCursorFile(projectRoot)
		}
		current := make(map[string]sessionListItem, len(list))
		if withNextAction {
			for _, item := range items {
				current[item.Session] = item
			}
		} else {
			for _, session := range list {
				resolvedRoot := resolveSessionProjectRoot(session, projectRoot, false)
				current[session] = sessionListItem{
					Session:     session,
					ProjectRoot: resolvedRoot,
					SocketPath:  resolveSessionSocketPath(session, resolvedRoot),
				}
			}
		}
		prev, prevErr := loadSessionListDeltaCursor(cursorFile)
		if prevErr != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", prevErr)
		}
		for session, nowItem := range current {
			prevItem, ok := prev.Items[session]
			if !ok {
				deltaAdded = append(deltaAdded, nowItem)
				continue
			}
			if !sessionListItemsEqual(nowItem, prevItem) {
				deltaChanged = append(deltaChanged, nowItem)
			}
		}
		for session, prevItem := range prev.Items {
			if _, ok := current[session]; !ok {
				deltaRemoved = append(deltaRemoved, prevItem)
			}
		}
		sort.Slice(deltaAdded, func(i, j int) bool { return deltaAdded[i].Session < deltaAdded[j].Session })
		sort.Slice(deltaRemoved, func(i, j int) bool { return deltaRemoved[i].Session < deltaRemoved[j].Session })
		sort.Slice(deltaChanged, func(i, j int) bool { return deltaChanged[i].Session < deltaChanged[j].Session })
		if err := saveSessionListDeltaCursor(cursorFile, sessionListDeltaCursor{
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Items:     current,
		}); err != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
		}
	}
	if jsonOut {
		payload := map[string]any{
			"sessions": list,
			"count":    len(list),
		}
		if withNextAction {
			payload["items"] = items
		}
		if stale {
			payload["historicalCount"] = historicalCount
			payload["staleCount"] = len(staleSessions)
			if prunePreview {
				payload["prunePreview"] = staleInfos
			}
			if !jsonMin {
				payload["staleSessions"] = staleSessions
			}
		}
		if deltaJSON {
			payload["delta"] = map[string]any{
				"added":   deltaAdded,
				"removed": deltaRemoved,
				"changed": deltaChanged,
				"count":   len(deltaAdded) + len(deltaRemoved) + len(deltaChanged),
			}
			payload["cursorFile"] = cursorFile
		}
		if !jsonMin {
			payload["projectOnly"] = projectOnly
			payload["allSockets"] = allSockets
			payload["activeOnly"] = activeOnly
			payload["withNextAction"] = withNextAction
			payload["priority"] = priority
			payload["projectRoot"] = projectRoot
		}
		writeJSON(payload)
		return 0
	}
	if deltaJSON {
		fmt.Printf("delta added=%d removed=%d changed=%d cursor=%s\n", len(deltaAdded), len(deltaRemoved), len(deltaChanged), cursorFile)
		return 0
	}
	if !stale {
		if withNextAction {
			for _, item := range items {
				if priority {
					fmt.Printf("%s,%s,%s,%s,%d,%s\n", item.Session, item.Status, item.SessionState, item.NextAction, item.PriorityScore, item.PriorityLabel)
				} else {
					fmt.Printf("%s,%s,%s,%s\n", item.Session, item.Status, item.SessionState, item.NextAction)
				}
			}
		} else {
			fmt.Println(strings.Join(list, "\n"))
		}
		return 0
	}
	fmt.Printf("active=%d historical=%d stale=%d\n", len(list), historicalCount, len(staleSessions))
	if len(list) > 0 {
		fmt.Println("active_sessions:")
		fmt.Println(strings.Join(list, "\n"))
	}
	if len(staleSessions) > 0 {
		fmt.Println("stale_sessions:")
		fmt.Println(strings.Join(staleSessions, "\n"))
	}
	if prunePreview && len(staleInfos) > 0 {
		fmt.Println("prune_preview:")
		for _, info := range staleInfos {
			fmt.Printf("- %s\n", info.PruneCmd)
		}
	}
	return 0
}

func cmdSessionListWatch(args []string, watchInterval, watchCycles int) int {
	binPath, err := osExecutableFn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve lisa binary path: %v\n", err)
		return 1
	}
	watchArgs := make([]string, 0, len(args)+4)
	haveDelta := false
	haveJSONMin := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--watch-json":
			continue
		case "--watch-interval", "--watch-cycles":
			i++
			continue
		}
		if arg == "--delta-json" {
			haveDelta = true
		}
		if arg == "--json-min" {
			haveJSONMin = true
		}
		watchArgs = append(watchArgs, arg)
	}
	if !haveDelta {
		watchArgs = append(watchArgs, "--delta-json")
	}
	if !haveJSONMin {
		watchArgs = append(watchArgs, "--json-min")
	}

	cycles := 0
	for {
		out, stderrText, runErr := runLisaSubcommandFn(binPath, append([]string{"session", "list"}, watchArgs...)...)
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		if runErr != nil {
			if strings.TrimSpace(stderrText) != "" {
				fmt.Fprintln(os.Stderr, strings.TrimSpace(stderrText))
			}
			return 1
		}
		cycles++
		if watchCycles > 0 && cycles >= watchCycles {
			return 0
		}
		time.Sleep(time.Duration(watchInterval) * time.Second)
	}
}

func listStaleSessions(projectRoot string, activeSessions []string) ([]string, []staleSessionInfo, int, error) {
	metas, err := loadSessionMetasForProject(projectRoot, true)
	if err != nil {
		return nil, nil, 0, err
	}
	activeSet := make(map[string]struct{}, len(activeSessions))
	for _, session := range activeSessions {
		activeSet[session] = struct{}{}
	}
	projectHashCurrent := projectHash(projectRoot)
	historicalSet := map[string]struct{}{}
	staleSet := map[string]struct{}{}
	metaBySession := map[string]sessionMeta{}
	for _, meta := range metas {
		session := strings.TrimSpace(meta.Session)
		if session == "" {
			continue
		}
		metaRoot := canonicalProjectRoot(meta.ProjectRoot)
		if metaRoot == "" {
			metaRoot = projectRoot
		}
		if projectHash(metaRoot) != projectHashCurrent {
			continue
		}
		historicalSet[session] = struct{}{}
		if _, ok := activeSet[session]; !ok {
			staleSet[session] = struct{}{}
			metaBySession[session] = meta
		}
	}
	stale := make([]string, 0, len(staleSet))
	for session := range staleSet {
		stale = append(stale, session)
	}
	sort.Strings(stale)
	staleInfos := make([]staleSessionInfo, 0, len(stale))
	for _, session := range stale {
		meta := metaBySession[session]
		metaRoot := canonicalProjectRoot(meta.ProjectRoot)
		if metaRoot == "" {
			metaRoot = projectRoot
		}
		staleInfos = append(staleInfos, staleSessionInfo{
			Session:     session,
			ProjectRoot: metaRoot,
			CreatedAt:   strings.TrimSpace(meta.CreatedAt),
			MetaPath:    filepath.Clean(sessionMetaFile(metaRoot, session)),
			PruneCmd:    fmt.Sprintf("./lisa session kill --session %s --project-root %s --json", shellQuote(session), shellQuote(metaRoot)),
		})
	}
	return stale, staleInfos, len(historicalSet), nil
}

func listSessionsAcrossSockets(projectRoot string, projectOnly bool) ([]string, error) {
	projectRoot = canonicalProjectRoot(projectRoot)
	rootHash := projectHash(projectRoot)

	outSet := map[string]struct{}{}

	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	currentSessions, err := tmuxListSessionsFn(projectOnly, projectRoot)
	restoreRuntime()
	if err != nil {
		return nil, err
	}
	for _, session := range currentSessions {
		outSet[session] = struct{}{}
	}

	metas, err := loadSessionMetasForProject(projectRoot, true)
	if err != nil {
		return nil, err
	}
	for _, meta := range metas {
		session := strings.TrimSpace(meta.Session)
		if session == "" {
			continue
		}
		metaRoot := canonicalProjectRoot(meta.ProjectRoot)
		if metaRoot == "" {
			metaRoot = projectRoot
		}
		if projectOnly && projectHash(metaRoot) != rootHash {
			continue
		}
		active := false
		socketPath := strings.TrimSpace(meta.SocketPath)
		if socketPath != "" {
			_, hasErr := runTmuxCmdWithSocket(socketPath, "has-session", "-t", session)
			active = hasErr == nil
		} else {
			restore := withProjectRuntimeEnv(metaRoot)
			active = tmuxHasSessionFn(session)
			restore()
		}
		if active {
			outSet[session] = struct{}{}
		}
	}

	out := make([]string, 0, len(outSet))
	for session := range outSet {
		out = append(out, session)
	}
	sort.Strings(out)
	return out, nil
}

func sessionListDeltaCursorFile(projectRoot string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-list-delta.json", projectHash(projectRoot))
}

func loadSessionListDeltaCursor(path string) (sessionListDeltaCursor, error) {
	out := sessionListDeltaCursor{
		Items: map[string]sessionListItem{},
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
		out.Items = map[string]sessionListItem{}
	}
	return out, nil
}

func saveSessionListDeltaCursor(path string, cursor sessionListDeltaCursor) error {
	if cursor.Items == nil {
		cursor.Items = map[string]sessionListItem{}
	}
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func sessionListItemsEqual(a, b sessionListItem) bool {
	return a.Session == b.Session &&
		a.Status == b.Status &&
		a.SessionState == b.SessionState &&
		a.NextAction == b.NextAction &&
		a.PriorityScore == b.PriorityScore &&
		a.PriorityLabel == b.PriorityLabel &&
		a.ProjectRoot == b.ProjectRoot &&
		a.SocketPath == b.SocketPath
}

func resolveSessionSocketPath(session, projectRoot string) string {
	root := canonicalProjectRoot(projectRoot)
	if meta, err := loadSessionMeta(root, session); err == nil {
		if socket := strings.TrimSpace(meta.SocketPath); socket != "" {
			return filepath.Clean(socket)
		}
	}
	return tmuxSocketPathForProjectRoot(root)
}

func cmdSessionExists(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session exists")
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
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	exists := tmuxHasSessionFn(session)
	if jsonOut {
		payload := map[string]any{
			"session":     session,
			"exists":      exists,
			"projectRoot": projectRoot,
		}
		if !exists {
			payload["errorCode"] = "session_not_found"
		}
		writeJSON(payload)
		if exists {
			return 0
		}
		return 1
	}
	if exists {
		fmt.Println("true")
		return 0
	}
	fmt.Println("false")
	return 1
}

func cmdSessionKill(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	cleanupAllHashes := false
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session kill")
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
		case "--cleanup-all-hashes":
			cleanupAllHashes = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	cleanupOpts := cleanupOptions{
		AllHashes:  cleanupAllHashes,
		KeepEvents: true,
	}
	descendants, descendantsErr := listSessionDescendants(projectRoot, session, cleanupOpts.AllHashes)
	if descendantsErr != nil {
		fmt.Fprintf(os.Stderr, "descendant lookup warning: %v\n", descendantsErr)
	}

	targets := make([]string, 0, len(descendants)+1)
	targets = append(targets, descendants...)
	targets = append(targets, session)

	rootFound := false
	var errs []string
	for _, target := range targets {
		isRoot := target == session
		reasonPrefix := "kill_descendant"
		if isRoot {
			reasonPrefix = "kill"
		}

		if !tmuxHasSessionFn(target) {
			if isRoot {
				if !jsonOut {
					fmt.Fprintln(os.Stderr, "session not found")
				}
			}
			if err := cleanupSessionArtifactsWithOptions(projectRoot, target, cleanupOpts); err != nil {
				errs = append(errs, fmt.Sprintf("%s cleanup: %v", target, err))
			}
			if err := appendLifecycleEvent(projectRoot, target, "lifecycle", "not_found", "idle", reasonPrefix+"_not_found"); err != nil {
				errs = append(errs, fmt.Sprintf("%s observability: %v", target, err))
			}
			continue
		}
		if isRoot {
			rootFound = true
		}

		killErr := tmuxKillSessionFn(target)
		cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, target, cleanupOpts)
		eventState := "terminated"
		eventReason := reasonPrefix + "_success"
		if killErr != nil {
			eventState = "degraded"
			eventReason = reasonPrefix + "_error"
		}
		if err := appendLifecycleEvent(projectRoot, target, "lifecycle", eventState, "idle", eventReason); err != nil {
			errs = append(errs, fmt.Sprintf("%s observability: %v", target, err))
		}
		if killErr != nil {
			errs = append(errs, fmt.Sprintf("%s kill: %v", target, killErr))
		}
		if cleanupErr != nil {
			errs = append(errs, fmt.Sprintf("%s cleanup: %v", target, cleanupErr))
		}
	}

	if err := pruneStaleSessionEventArtifactsFn(); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}

	if !rootFound {
		if jsonOut {
			writeJSON(map[string]any{
				"session":     session,
				"ok":          false,
				"found":       false,
				"errors":      errs,
				"errorCode":   "session_not_found",
				"projectRoot": projectRoot,
			})
		}
		if !jsonOut {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e)
			}
		}
		return 1
	}

	if len(errs) > 0 {
		if jsonOut {
			writeJSON(map[string]any{
				"session":     session,
				"ok":          false,
				"found":       true,
				"errors":      errs,
				"errorCode":   "session_kill_failed",
				"projectRoot": projectRoot,
			})
		}
		if !jsonOut {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e)
			}
		}
		return 1
	}
	if jsonOut {
		writeJSON(map[string]any{
			"session":     session,
			"ok":          true,
			"found":       true,
			"projectRoot": projectRoot,
		})
		return 0
	}
	fmt.Println("ok")
	return 0
}

func cmdSessionKillAll(args []string) int {
	projectOnly := false
	projectRoot := getPWD()
	cleanupAllHashes := false
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session kill-all")
		case "--project-only":
			projectOnly = true
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--cleanup-all-hashes":
			cleanupAllHashes = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	allHashes := cleanupAllHashes || !projectOnly
	cleanupOpts := cleanupOptions{
		AllHashes:  allHashes,
		KeepEvents: true,
	}
	sessions, err := tmuxListSessionsFn(projectOnly, projectRoot)
	if err != nil {
		return commandErrorf(jsonOut, "session_list_failed", "failed to list sessions: %v", err)
	}
	var errs []string
	killed := 0
	for _, s := range sessions {
		killErr := tmuxKillSessionFn(s)
		if killErr != nil {
			errs = append(errs, fmt.Sprintf("%s kill: %v", s, killErr))
		} else {
			killed++
		}
		cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, s, cleanupOpts)
		eventState := "terminated"
		eventReason := "kill_all_success"
		if killErr != nil {
			eventState = "degraded"
			eventReason = "kill_all_error"
		}
		if eventErr := appendLifecycleEvent(projectRoot, s, "lifecycle", eventState, "idle", eventReason); eventErr != nil {
			errs = append(errs, fmt.Sprintf("%s observability: %v", s, eventErr))
		}
		if cleanupErr != nil {
			errs = append(errs, fmt.Sprintf("%s cleanup: %v", s, cleanupErr))
		}
	}
	if pruneErr := pruneStaleSessionEventArtifactsFn(); pruneErr != nil {
		errs = append(errs, fmt.Sprintf("event retention cleanup: %v", pruneErr))
	}
	if len(errs) > 0 {
		if jsonOut {
			writeJSON(map[string]any{
				"ok":          false,
				"killed":      killed,
				"total":       len(sessions),
				"projectOnly": projectOnly,
				"projectRoot": projectRoot,
				"errors":      errs,
				"errorCode":   "session_kill_all_failed",
			})
		}
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "killed %d/%d sessions\n", killed, len(sessions))
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e)
			}
		}
		return 1
	}
	if jsonOut {
		writeJSON(map[string]any{
			"ok":          true,
			"killed":      killed,
			"total":       len(sessions),
			"projectOnly": projectOnly,
			"projectRoot": projectRoot,
		})
		return 0
	}
	fmt.Printf("killed %d sessions\n", killed)
	return 0
}
