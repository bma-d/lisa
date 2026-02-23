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

type sessionTreeNode struct {
	Session       string            `json:"session"`
	ParentSession string            `json:"parentSession,omitempty"`
	Agent         string            `json:"agent,omitempty"`
	Mode          string            `json:"mode,omitempty"`
	Status        string            `json:"status,omitempty"`
	SessionState  string            `json:"sessionState,omitempty"`
	ProjectRoot   string            `json:"projectRoot,omitempty"`
	CreatedAt     string            `json:"createdAt,omitempty"`
	Children      []sessionTreeNode `json:"children,omitempty"`
}

type sessionTreeResult struct {
	Session           string            `json:"session,omitempty"`
	ProjectRoot       string            `json:"projectRoot"`
	AllHashes         bool              `json:"allHashes"`
	ActiveOnly        bool              `json:"activeOnly,omitempty"`
	DeltaOnly         bool              `json:"delta,omitempty"`
	Flat              bool              `json:"flat,omitempty"`
	WithState         bool              `json:"withState,omitempty"`
	NodeCount         int               `json:"nodeCount"`
	TotalNodeCount    int               `json:"totalNodeCount,omitempty"`
	FilteredNodeCount int               `json:"filteredNodeCount,omitempty"`
	Rows              []sessionTreeRow  `json:"rows,omitempty"`
	Roots             []sessionTreeNode `json:"roots"`
	Delta             *sessionTreeDelta `json:"deltaResult,omitempty"`
}

type sessionTreeRow struct {
	Session       string `json:"session"`
	ParentSession string `json:"parentSession,omitempty"`
	Agent         string `json:"agent,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Status        string `json:"status,omitempty"`
	SessionState  string `json:"sessionState,omitempty"`
	ProjectRoot   string `json:"projectRoot,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
}

type sessionTreeDelta struct {
	Added   []sessionTreeRow `json:"added,omitempty"`
	Removed []sessionTreeRow `json:"removed,omitempty"`
	Count   int              `json:"count"`
}

type sessionTreeDeltaCursor struct {
	UpdatedAt string                    `json:"updatedAt"`
	Rows      map[string]sessionTreeRow `json:"rows"`
}

func cmdSessionTree(args []string) int {
	projectRoot := getPWD()
	sessionFilter := ""
	allHashes := false
	activeOnly := false
	delta := false
	deltaJSON := false
	flat := false
	withState := false
	cursorFile := ""
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session tree")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			sessionFilter = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--all-hashes":
			allHashes = true
		case "--active-only":
			activeOnly = true
		case "--delta":
			delta = true
		case "--delta-json":
			deltaJSON = true
			jsonOut = true
		case "--flat":
			flat = true
		case "--with-state":
			withState = true
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
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

	if delta && deltaJSON {
		return commandError(jsonOut, "delta_mode_conflict", "--delta cannot be combined with --delta-json")
	}
	if cursorFile != "" && !deltaJSON {
		return commandError(jsonOut, "cursor_file_requires_delta_json", "--cursor-file requires --delta-json")
	}
	projectRoot = canonicalProjectRoot(projectRoot)
	if deltaJSON {
		if cursorFile == "" {
			cursorFile = sessionTreeDeltaCursorFile(projectRoot, sessionFilter)
		}
		resolvedCursor, err := expandAndCleanPath(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
		cursorFile = resolvedCursor
	}
	metas, err := loadSessionMetasForProject(projectRoot, allHashes)
	if err != nil {
		return commandErrorf(jsonOut, "session_tree_build_failed", "failed to build session tree: %v", err)
	}
	totalSessions := map[string]struct{}{}
	for _, meta := range metas {
		session := strings.TrimSpace(meta.Session)
		if session != "" {
			totalSessions[session] = struct{}{}
		}
	}
	totalNodeCount := len(totalSessions)
	if activeOnly {
		metas = filterActiveTreeMetas(projectRoot, metas)
	}

	nodesBySession := make(map[string]*sessionTreeNode, len(metas))
	childrenByParent := make(map[string][]string)

	for _, meta := range metas {
		session := strings.TrimSpace(meta.Session)
		if session == "" {
			continue
		}
		node := &sessionTreeNode{
			Session:       session,
			ParentSession: strings.TrimSpace(meta.ParentSession),
			Agent:         strings.TrimSpace(meta.Agent),
			Mode:          strings.TrimSpace(meta.Mode),
			ProjectRoot:   strings.TrimSpace(meta.ProjectRoot),
			CreatedAt:     strings.TrimSpace(meta.CreatedAt),
		}
		nodesBySession[session] = node
		if node.ParentSession != "" && node.ParentSession != session {
			childrenByParent[node.ParentSession] = append(childrenByParent[node.ParentSession], session)
		}
	}
	for parent := range childrenByParent {
		sort.Strings(childrenByParent[parent])
	}

	var rootSessions []string
	if sessionFilter != "" {
		if _, ok := nodesBySession[sessionFilter]; !ok {
			return commandErrorf(jsonOut, "session_not_found_in_tree", "session not found in metadata: %s", sessionFilter)
		}
		rootSessions = []string{sessionFilter}
	} else {
		rootSessions = findTreeRoots(nodesBySession)
	}

	roots := make([]sessionTreeNode, 0, len(rootSessions))
	for _, root := range rootSessions {
		roots = append(roots, buildSessionTreeNode(root, nodesBySession, childrenByParent, map[string]bool{}))
	}

	rows := flattenSessionTreeRows(roots)
	if withState {
		rows = enrichSessionTreeRowsWithState(projectRoot, rows)
		roots = applyRowsToTreeRoots(roots, rows)
	}
	nodeCount := len(rows)
	filteredNodeCount := totalNodeCount - nodeCount
	if filteredNodeCount < 0 {
		filteredNodeCount = 0
	}
	result := sessionTreeResult{
		Session:           sessionFilter,
		ProjectRoot:       projectRoot,
		AllHashes:         allHashes,
		ActiveOnly:        activeOnly,
		DeltaOnly:         delta,
		Flat:              flat,
		WithState:         withState,
		NodeCount:         nodeCount,
		TotalNodeCount:    totalNodeCount,
		FilteredNodeCount: filteredNodeCount,
		Roots:             roots,
	}
	if flat || withState {
		result.Rows = rows
	}
	if delta {
		deltaResult, deltaErr := computeSessionTreeDelta(projectRoot, rows)
		if deltaErr != nil {
			return commandErrorf(jsonOut, "session_tree_delta_failed", "failed to compute --delta: %v", deltaErr)
		}
		result.Delta = deltaResult
	}
	deltaAdded := []sessionTreeRow{}
	deltaRemoved := []sessionTreeRow{}
	deltaChanged := []sessionTreeRow{}
	if deltaJSON {
		current := make(map[string]sessionTreeRow, len(rows))
		for _, row := range rows {
			current[row.Session] = row
		}
		prev, prevErr := loadSessionTreeDeltaCursor(cursorFile)
		if prevErr != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", prevErr)
		}
		for session, row := range current {
			prevRow, ok := prev.Rows[session]
			if !ok {
				deltaAdded = append(deltaAdded, row)
				continue
			}
			if !sessionTreeRowsEqual(row, prevRow) {
				deltaChanged = append(deltaChanged, row)
			}
		}
		for session, prevRow := range prev.Rows {
			if _, ok := current[session]; !ok {
				deltaRemoved = append(deltaRemoved, prevRow)
			}
		}
		sort.Slice(deltaAdded, func(i, j int) bool { return deltaAdded[i].Session < deltaAdded[j].Session })
		sort.Slice(deltaRemoved, func(i, j int) bool { return deltaRemoved[i].Session < deltaRemoved[j].Session })
		sort.Slice(deltaChanged, func(i, j int) bool { return deltaChanged[i].Session < deltaChanged[j].Session })
		if err := saveSessionTreeDeltaCursor(cursorFile, sessionTreeDeltaCursor{
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Rows:      current,
		}); err != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
		}
	}

	if jsonOut {
		if jsonMin {
			payload := map[string]any{
				"nodeCount":         result.NodeCount,
				"totalNodeCount":    result.TotalNodeCount,
				"filteredNodeCount": result.FilteredNodeCount,
			}
			if delta && result.Delta != nil {
				payload["added"] = flattenSessionTreeRowsMin(result.Delta.Added)
				payload["removed"] = flattenSessionTreeRowsMin(result.Delta.Removed)
				payload["deltaCount"] = result.Delta.Count
			} else if deltaJSON {
				payload["delta"] = map[string]any{
					"added":   flattenSessionTreeRowsMin(deltaAdded),
					"removed": flattenSessionTreeRowsMin(deltaRemoved),
					"changed": flattenSessionTreeRowsMin(deltaChanged),
					"count":   len(deltaAdded) + len(deltaRemoved) + len(deltaChanged),
				}
				payload["cursorFile"] = cursorFile
			} else if flat || withState {
				payload["rows"] = flattenSessionTreeRowsMin(result.Rows)
			} else {
				payload["roots"] = minimizeSessionTreeRoots(result.Roots)
			}
			writeJSON(payload)
			return 0
		}
		if deltaJSON {
			resultPayload := map[string]any{
				"session":           result.Session,
				"projectRoot":       result.ProjectRoot,
				"allHashes":         result.AllHashes,
				"activeOnly":        result.ActiveOnly,
				"flat":              result.Flat,
				"withState":         result.WithState,
				"nodeCount":         result.NodeCount,
				"totalNodeCount":    result.TotalNodeCount,
				"filteredNodeCount": result.FilteredNodeCount,
				"delta": map[string]any{
					"added":   deltaAdded,
					"removed": deltaRemoved,
					"changed": deltaChanged,
					"count":   len(deltaAdded) + len(deltaRemoved) + len(deltaChanged),
				},
				"cursorFile": cursorFile,
			}
			if flat || withState {
				resultPayload["rows"] = result.Rows
			} else {
				resultPayload["roots"] = result.Roots
			}
			writeJSON(resultPayload)
			return 0
		}
		writeJSON(result)
		return 0
	}
	if deltaJSON {
		fmt.Printf("delta added=%d removed=%d changed=%d cursor=%s\n", len(deltaAdded), len(deltaRemoved), len(deltaChanged), cursorFile)
		return 0
	}
	if flat {
		if withState {
			fmt.Println("session\tparentSession\tagent\tmode\tstatus\tsessionState\tprojectRoot\tcreatedAt")
		} else {
			fmt.Println("session\tparentSession\tagent\tmode\tprojectRoot\tcreatedAt")
		}
		for _, row := range result.Rows {
			if withState {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					row.Session,
					row.ParentSession,
					row.Agent,
					row.Mode,
					row.Status,
					row.SessionState,
					row.ProjectRoot,
					row.CreatedAt,
				)
			} else {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n",
					row.Session,
					row.ParentSession,
					row.Agent,
					row.Mode,
					row.ProjectRoot,
					row.CreatedAt,
				)
			}
		}
		return 0
	}
	if delta && result.Delta != nil {
		fmt.Printf("delta_count=%d\n", result.Delta.Count)
		for _, row := range result.Delta.Added {
			fmt.Printf("+\t%s\t%s\n", row.Session, row.ParentSession)
		}
		for _, row := range result.Delta.Removed {
			fmt.Printf("-\t%s\t%s\n", row.Session, row.ParentSession)
		}
		return 0
	}

	for _, root := range result.Roots {
		printSessionTreeNode(root, "")
	}
	return 0
}

func computeSessionTreeDelta(projectRoot string, rows []sessionTreeRow) (*sessionTreeDelta, error) {
	path := sessionTreeDeltaStateFile(projectRoot)
	prev := sessionTreeDeltaState{Rows: map[string]string{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &prev)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if prev.Rows == nil {
		prev.Rows = map[string]string{}
	}

	currentRows := make(map[string]string, len(rows))
	currentByKey := make(map[string]sessionTreeRow, len(rows))
	for _, row := range rows {
		parent := strings.TrimSpace(row.ParentSession)
		currentRows[row.Session] = parent
		key := row.Session + "|" + parent
		currentByKey[key] = row
	}
	prevByKey := make(map[string]sessionTreeRow, len(prev.Rows))
	for session, parent := range prev.Rows {
		prevByKey[session+"|"+parent] = sessionTreeRow{
			Session:       session,
			ParentSession: parent,
		}
	}

	added := make([]sessionTreeRow, 0)
	removed := make([]sessionTreeRow, 0)
	for key, row := range currentByKey {
		if _, ok := prevByKey[key]; !ok {
			added = append(added, row)
		}
	}
	for key, row := range prevByKey {
		if _, ok := currentByKey[key]; !ok {
			removed = append(removed, row)
		}
	}
	sort.Slice(added, func(i, j int) bool {
		return added[i].Session < added[j].Session
	})
	sort.Slice(removed, func(i, j int) bool {
		return removed[i].Session < removed[j].Session
	})

	state := sessionTreeDeltaState{Rows: currentRows}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return nil, err
	}

	return &sessionTreeDelta{
		Added:   added,
		Removed: removed,
		Count:   len(added) + len(removed),
	}, nil
}

func sessionTreeDeltaStateFile(projectRoot string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-tree-delta.json", projectHash(projectRoot))
}

func sessionTreeDeltaCursorFile(projectRoot, sessionFilter string) string {
	scope := strings.TrimSpace(sessionFilter)
	if scope == "" {
		scope = "all"
	}
	return fmt.Sprintf("/tmp/.lisa-%s-tree-delta-cursor-%s.json", projectHash(projectRoot), sessionArtifactID(scope))
}

func loadSessionTreeDeltaCursor(path string) (sessionTreeDeltaCursor, error) {
	out := sessionTreeDeltaCursor{
		Rows: map[string]sessionTreeRow{},
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
	if out.Rows == nil {
		out.Rows = map[string]sessionTreeRow{}
	}
	return out, nil
}

func saveSessionTreeDeltaCursor(path string, cursor sessionTreeDeltaCursor) error {
	if cursor.Rows == nil {
		cursor.Rows = map[string]sessionTreeRow{}
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

func sessionTreeRowsEqual(a, b sessionTreeRow) bool {
	return a.Session == b.Session &&
		a.ParentSession == b.ParentSession &&
		a.Agent == b.Agent &&
		a.Mode == b.Mode &&
		a.Status == b.Status &&
		a.SessionState == b.SessionState &&
		a.ProjectRoot == b.ProjectRoot &&
		a.CreatedAt == b.CreatedAt
}

func flattenSessionTreeRowsMin(rows []sessionTreeRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"session": row.Session,
		}
		if strings.TrimSpace(row.ParentSession) != "" {
			item["parentSession"] = row.ParentSession
		}
		if strings.TrimSpace(row.Status) != "" {
			item["status"] = row.Status
		}
		if strings.TrimSpace(row.SessionState) != "" {
			item["sessionState"] = row.SessionState
		}
		out = append(out, item)
	}
	return out
}

func minimizeSessionTreeRoots(roots []sessionTreeNode) []map[string]any {
	out := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		out = append(out, minimizeSessionTreeNode(root))
	}
	return out
}

func minimizeSessionTreeNode(node sessionTreeNode) map[string]any {
	item := map[string]any{
		"session": node.Session,
	}
	if strings.TrimSpace(node.ParentSession) != "" {
		item["parentSession"] = node.ParentSession
	}
	if strings.TrimSpace(node.Status) != "" {
		item["status"] = node.Status
	}
	if strings.TrimSpace(node.SessionState) != "" {
		item["sessionState"] = node.SessionState
	}
	if len(node.Children) > 0 {
		children := make([]map[string]any, 0, len(node.Children))
		for _, child := range node.Children {
			children = append(children, minimizeSessionTreeNode(child))
		}
		item["children"] = children
	}
	return item
}

func filterActiveTreeMetas(defaultProjectRoot string, metas []sessionMeta) []sessionMeta {
	filtered := make([]sessionMeta, 0, len(metas))
	cache := make(map[string]bool, len(metas))
	for _, meta := range metas {
		session := strings.TrimSpace(meta.Session)
		if session == "" {
			continue
		}
		root := strings.TrimSpace(meta.ProjectRoot)
		if root == "" {
			root = defaultProjectRoot
		}
		root = canonicalProjectRoot(root)
		cacheKey := root + "|" + session
		active, ok := cache[cacheKey]
		if !ok {
			restoreRuntime := withProjectRuntimeEnv(root)
			active = tmuxHasSessionFn(session)
			restoreRuntime()
			cache[cacheKey] = active
		}
		if active {
			filtered = append(filtered, meta)
		}
	}
	return filtered
}

func findTreeRoots(nodesBySession map[string]*sessionTreeNode) []string {
	roots := make([]string, 0, len(nodesBySession))
	for session, node := range nodesBySession {
		parent := strings.TrimSpace(node.ParentSession)
		if parent == "" {
			roots = append(roots, session)
			continue
		}
		if _, ok := nodesBySession[parent]; !ok {
			roots = append(roots, session)
		}
	}
	if len(roots) == 0 {
		for session := range nodesBySession {
			roots = append(roots, session)
		}
	}
	sort.Strings(roots)
	return roots
}

func buildSessionTreeNode(session string, nodesBySession map[string]*sessionTreeNode, childrenByParent map[string][]string, path map[string]bool) sessionTreeNode {
	nodePtr, ok := nodesBySession[session]
	if !ok {
		return sessionTreeNode{Session: session}
	}
	node := *nodePtr
	if path[session] {
		return node
	}
	path[session] = true
	for _, child := range childrenByParent[session] {
		if child == session {
			continue
		}
		node.Children = append(node.Children, buildSessionTreeNode(child, nodesBySession, childrenByParent, path))
	}
	delete(path, session)
	return node
}

func printSessionTreeNode(node sessionTreeNode, indent string) {
	descriptor := strings.TrimSpace(node.Agent + "/" + node.Mode)
	if descriptor == "/" || descriptor == "" {
		fmt.Printf("%s%s\n", indent, node.Session)
	} else {
		fmt.Printf("%s%s (%s)\n", indent, node.Session, descriptor)
	}
	for _, child := range node.Children {
		printSessionTreeNode(child, indent+"  ")
	}
}

func flattenSessionTreeRows(roots []sessionTreeNode) []sessionTreeRow {
	rows := make([]sessionTreeRow, 0)
	var walk func(node sessionTreeNode)
	walk = func(node sessionTreeNode) {
		rows = append(rows, sessionTreeRow{
			Session:       node.Session,
			ParentSession: node.ParentSession,
			Agent:         node.Agent,
			Mode:          node.Mode,
			Status:        node.Status,
			SessionState:  node.SessionState,
			ProjectRoot:   node.ProjectRoot,
			CreatedAt:     node.CreatedAt,
		})
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return rows
}

func enrichSessionTreeRowsWithState(defaultProjectRoot string, rows []sessionTreeRow) []sessionTreeRow {
	out := make([]sessionTreeRow, 0, len(rows))
	for _, row := range rows {
		updated := row
		root := strings.TrimSpace(row.ProjectRoot)
		if root == "" {
			root = defaultProjectRoot
		}
		root = canonicalProjectRoot(root)
		restoreRuntime := withProjectRuntimeEnv(root)
		status, err := computeSessionStatusFn(row.Session, root, "auto", "auto", false, 0)
		restoreRuntime()
		if err == nil {
			status = normalizeStatusForSessionStatusOutput(status)
			updated.Status = status.Status
			updated.SessionState = status.SessionState
		}
		out = append(out, updated)
	}
	return out
}

func applyRowsToTreeRoots(roots []sessionTreeNode, rows []sessionTreeRow) []sessionTreeNode {
	statusBySession := make(map[string]sessionTreeRow, len(rows))
	for _, row := range rows {
		statusBySession[row.Session] = row
	}
	var walk func(node sessionTreeNode) sessionTreeNode
	walk = func(node sessionTreeNode) sessionTreeNode {
		if row, ok := statusBySession[node.Session]; ok {
			node.Status = row.Status
			node.SessionState = row.SessionState
		}
		for i := range node.Children {
			node.Children[i] = walk(node.Children[i])
		}
		return node
	}
	out := make([]sessionTreeNode, 0, len(roots))
	for _, root := range roots {
		out = append(out, walk(root))
	}
	return out
}
