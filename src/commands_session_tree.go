package app

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type sessionTreeNode struct {
	Session       string            `json:"session"`
	ParentSession string            `json:"parentSession,omitempty"`
	Agent         string            `json:"agent,omitempty"`
	Mode          string            `json:"mode,omitempty"`
	ProjectRoot   string            `json:"projectRoot,omitempty"`
	CreatedAt     string            `json:"createdAt,omitempty"`
	Children      []sessionTreeNode `json:"children,omitempty"`
}

type sessionTreeResult struct {
	Session     string            `json:"session,omitempty"`
	ProjectRoot string            `json:"projectRoot"`
	AllHashes   bool              `json:"allHashes"`
	Flat        bool              `json:"flat,omitempty"`
	NodeCount   int               `json:"nodeCount"`
	Rows        []sessionTreeRow  `json:"rows,omitempty"`
	Roots       []sessionTreeNode `json:"roots"`
}

type sessionTreeRow struct {
	Session       string `json:"session"`
	ParentSession string `json:"parentSession,omitempty"`
	Agent         string `json:"agent,omitempty"`
	Mode          string `json:"mode,omitempty"`
	ProjectRoot   string `json:"projectRoot,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
}

func cmdSessionTree(args []string) int {
	projectRoot := getPWD()
	sessionFilter := ""
	allHashes := false
	flat := false
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session tree")
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			sessionFilter = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--all-hashes":
			allHashes = true
		case "--flat":
			flat = true
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	metas, err := loadSessionMetasForProject(projectRoot, allHashes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build session tree: %v\n", err)
		return 1
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
			fmt.Fprintf(os.Stderr, "session not found in metadata: %s\n", sessionFilter)
			return 1
		}
		rootSessions = []string{sessionFilter}
	} else {
		rootSessions = findTreeRoots(nodesBySession)
	}

	roots := make([]sessionTreeNode, 0, len(rootSessions))
	for _, root := range rootSessions {
		roots = append(roots, buildSessionTreeNode(root, nodesBySession, childrenByParent, map[string]bool{}))
	}

	result := sessionTreeResult{
		Session:     sessionFilter,
		ProjectRoot: projectRoot,
		AllHashes:   allHashes,
		Flat:        flat,
		NodeCount:   len(nodesBySession),
		Roots:       roots,
	}
	if flat {
		result.Rows = flattenSessionTreeRows(roots)
	}

	if jsonOut {
		writeJSON(result)
		return 0
	}
	if flat {
		fmt.Println("session\tparentSession\tagent\tmode\tprojectRoot\tcreatedAt")
		for _, row := range result.Rows {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n",
				row.Session,
				row.ParentSession,
				row.Agent,
				row.Mode,
				row.ProjectRoot,
				row.CreatedAt,
			)
		}
		return 0
	}

	for _, root := range roots {
		printSessionTreeNode(root, "")
	}
	return 0
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
