package app

import "strings"

func resolveSessionProjectRoot(session, projectRoot string, projectRootExplicit bool) string {
	root := canonicalProjectRoot(projectRoot)
	if projectRootExplicit {
		return root
	}
	if strings.TrimSpace(session) == "" {
		return root
	}
	meta, err := loadSessionMetaByGlobFn(session)
	if err != nil {
		return root
	}
	if strings.TrimSpace(meta.ProjectRoot) == "" {
		return root
	}
	return canonicalProjectRoot(meta.ProjectRoot)
}
