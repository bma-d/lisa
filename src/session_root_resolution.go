package app

import (
	"strings"
)

func resolveSessionProjectRootChecked(session, projectRoot string, projectRootExplicit bool) (string, error) {
	root := canonicalProjectRoot(projectRoot)
	if projectRootExplicit {
		return root, nil
	}
	if strings.TrimSpace(session) == "" {
		return root, nil
	}
	if localMeta, err := loadSessionMeta(root, session); err == nil {
		if strings.TrimSpace(localMeta.ProjectRoot) != "" {
			return canonicalProjectRoot(localMeta.ProjectRoot), nil
		}
		return root, nil
	}
	meta, err := loadSessionMetaByGlobFn(session)
	if err != nil {
		if isSessionMetaAmbiguousError(err) {
			return "", err
		}
		return root, nil
	}
	if strings.TrimSpace(meta.ProjectRoot) == "" {
		return root, nil
	}
	return canonicalProjectRoot(meta.ProjectRoot), nil
}

func resolveSessionProjectRoot(session, projectRoot string, projectRootExplicit bool) string {
	resolved, err := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if err != nil {
		return canonicalProjectRoot(projectRoot)
	}
	return resolved
}
