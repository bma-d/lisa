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
	lisaTmuxSocketDir  = "LISA_TMUX_SOCKET_DIR"
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
	return filepath.Join(preferredTmuxSocketDir(), fmt.Sprintf("lisa-tmux-%s-%s.sock", projectSlug(root), projectHash(root)))
}

func tmuxLegacySocketPathForProjectRoot(projectRoot string) string {
	root := canonicalProjectRoot(projectRoot)
	return filepath.Join(os.TempDir(), fmt.Sprintf("lisa-tmux-%s-%s.sock", projectSlug(root), projectHash(root)))
}

func currentTmuxSocketPath() string {
	if explicit := strings.TrimSpace(os.Getenv(lisaTmuxSocketEnv)); explicit != "" {
		return filepath.Clean(explicit)
	}
	return tmuxSocketPathForProjectRoot(currentProjectRootForTmux())
}

func currentTmuxSocketCandidates() []string {
	explicit := strings.TrimSpace(os.Getenv(lisaTmuxSocketEnv))
	if explicit != "" {
		out := []string{filepath.Clean(explicit)}
		root := currentProjectRootForTmux()
		primary := tmuxSocketPathForProjectRoot(root)
		legacy := tmuxLegacySocketPathForProjectRoot(root)
		if out[0] == primary && legacy != primary {
			out = append(out, legacy)
		}
		return dedupeSocketPaths(out)
	}
	root := currentProjectRootForTmux()
	primary := tmuxSocketPathForProjectRoot(root)
	legacy := tmuxLegacySocketPathForProjectRoot(root)
	if legacy == primary {
		return []string{primary}
	}
	return []string{primary, legacy}
}

func dedupeSocketPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		clean := filepath.Clean(strings.TrimSpace(p))
		if clean == "" || clean == "." {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func preferredTmuxSocketDir() string {
	if override := strings.TrimSpace(os.Getenv(lisaTmuxSocketDir)); override != "" {
		return filepath.Clean(override)
	}
	// Prefer /tmp for nested codex exec sandbox compatibility.
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		return "/tmp"
	}
	tmp := strings.TrimSpace(os.TempDir())
	if tmp == "" {
		return "/tmp"
	}
	return filepath.Clean(tmp)
}

func ensureTmuxSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}
