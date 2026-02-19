package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	lisaProjectRootEnv = "LISA_PROJECT_ROOT"
	lisaTmuxSocketEnv  = "LISA_TMUX_SOCKET"
)

func withProjectRuntimeEnv(projectRoot string) func() {
	root := canonicalProjectRoot(projectRoot)
	socket := tmuxSocketPathForProjectRoot(root)

	restoreRoot := setEnvScoped(lisaProjectRootEnv, root)
	restoreSocket := setEnvScoped(lisaTmuxSocketEnv, socket)
	return func() {
		restoreSocket()
		restoreRoot()
	}
}

func setEnvScoped(key, value string) func() {
	prev, hadPrev := os.LookupEnv(key)
	_ = os.Setenv(key, value)
	return func() {
		if hadPrev {
			_ = os.Setenv(key, prev)
			return
		}
		_ = os.Unsetenv(key)
	}
}

func currentProjectRootForTmux() string {
	root := strings.TrimSpace(os.Getenv(lisaProjectRootEnv))
	if root == "" {
		root = getPWD()
	}
	return canonicalProjectRoot(root)
}

func tmuxSocketPathForProjectRoot(projectRoot string) string {
	root := canonicalProjectRoot(projectRoot)
	return filepath.Join(os.TempDir(), fmt.Sprintf("lisa-tmux-%s-%s.sock", projectSlug(root), projectHash(root)))
}

func currentTmuxSocketPath() string {
	if explicit := strings.TrimSpace(os.Getenv(lisaTmuxSocketEnv)); explicit != "" {
		return filepath.Clean(explicit)
	}
	return tmuxSocketPathForProjectRoot(currentProjectRootForTmux())
}

func ensureTmuxSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}
