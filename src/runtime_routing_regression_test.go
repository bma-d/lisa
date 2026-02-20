package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSessionProjectRootExplicitBypassesMetadata(t *testing.T) {
	origMetaGlob := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMetaGlob
	})

	called := false
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		called = true
		return sessionMeta{ProjectRoot: t.TempDir()}, nil
	}

	explicitRoot := t.TempDir()
	got := resolveSessionProjectRoot("lisa-explicit", explicitRoot, true)
	if got != canonicalProjectRoot(explicitRoot) {
		t.Fatalf("expected explicit root %q, got %q", canonicalProjectRoot(explicitRoot), got)
	}
	if called {
		t.Fatalf("expected metadata glob lookup to be skipped for explicit project root")
	}
}

func TestResolveSessionProjectRootImplicitUsesMetadata(t *testing.T) {
	origMetaGlob := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMetaGlob
	})

	metaRoot := t.TempDir()
	calls := 0
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		calls++
		if session != "lisa-implicit" {
			t.Fatalf("unexpected session lookup: %q", session)
		}
		return sessionMeta{ProjectRoot: metaRoot + "/./"}, nil
	}

	fallbackRoot := t.TempDir()
	got := resolveSessionProjectRoot("lisa-implicit", fallbackRoot, false)
	if got != canonicalProjectRoot(metaRoot) {
		t.Fatalf("expected metadata root %q, got %q", canonicalProjectRoot(metaRoot), got)
	}
	if calls != 1 {
		t.Fatalf("expected one metadata lookup, got %d", calls)
	}
}

func TestResolveSessionProjectRootFallbackOnLookupFailure(t *testing.T) {
	origMetaGlob := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMetaGlob
	})

	fallbackRoot := t.TempDir()
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{}, errors.New("lookup failed")
	}

	got := resolveSessionProjectRoot("lisa-fallback", fallbackRoot, false)
	if got != canonicalProjectRoot(fallbackRoot) {
		t.Fatalf("expected fallback root %q, got %q", canonicalProjectRoot(fallbackRoot), got)
	}
}

func TestResolveSessionProjectRootFallbackOnBlankMetadataRoot(t *testing.T) {
	origMetaGlob := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMetaGlob
	})

	fallbackRoot := t.TempDir()
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{ProjectRoot: "  "}, nil
	}

	got := resolveSessionProjectRoot("lisa-blank-meta", fallbackRoot, false)
	if got != canonicalProjectRoot(fallbackRoot) {
		t.Fatalf("expected fallback root %q, got %q", canonicalProjectRoot(fallbackRoot), got)
	}
}

func TestTmuxSocketPathForProjectRootIsShortStableAndDistinct(t *testing.T) {
	longRootA := filepath.Join(t.TempDir(), strings.Repeat("very-long-segment-", 40), "project-a")
	longRootB := filepath.Join(t.TempDir(), strings.Repeat("very-long-segment-", 40), "project-b")

	sockA1 := tmuxSocketPathForProjectRoot(longRootA)
	sockA2 := tmuxSocketPathForProjectRoot(longRootA + "/./")
	sockB := tmuxSocketPathForProjectRoot(longRootB)

	if sockA1 != sockA2 {
		t.Fatalf("expected canonicalized roots to map to same socket path, got %q vs %q", sockA1, sockA2)
	}
	if sockA1 == sockB {
		t.Fatalf("expected different project roots to map to different socket paths, got %q", sockA1)
	}
	if !strings.HasPrefix(sockA1, "/tmp/lisa-tmux-") {
		t.Fatalf("expected /tmp socket path, got %q", sockA1)
	}
	if len(sockA1) > 100 {
		t.Fatalf("expected unix-safe short socket path, got len=%d path=%q", len(sockA1), sockA1)
	}
}

func TestCurrentTmuxSocketCandidatesIncludesLegacyFallbackForDefaultRuntimeSocket(t *testing.T) {
	projectRoot := t.TempDir()
	legacy := tmuxLegacySocketPathForProjectRoot(projectRoot)
	primary := tmuxSocketPathForProjectRoot(projectRoot)

	origRoot, hadRoot := os.LookupEnv(lisaProjectRootEnv)
	origSocket, hadSocket := os.LookupEnv(lisaTmuxSocketEnv)
	t.Cleanup(func() {
		if hadRoot {
			_ = os.Setenv(lisaProjectRootEnv, origRoot)
		} else {
			_ = os.Unsetenv(lisaProjectRootEnv)
		}
		if hadSocket {
			_ = os.Setenv(lisaTmuxSocketEnv, origSocket)
		} else {
			_ = os.Unsetenv(lisaTmuxSocketEnv)
		}
	})

	restore := withProjectRuntimeEnv(projectRoot)
	t.Cleanup(restore)

	candidates := currentTmuxSocketCandidates()
	if len(candidates) == 0 {
		t.Fatal("expected at least one tmux socket candidate")
	}
	if candidates[0] != primary {
		t.Fatalf("expected primary socket first, got %q (want %q)", candidates[0], primary)
	}
	if legacy != primary {
		foundLegacy := false
		for _, c := range candidates {
			if c == legacy {
				foundLegacy = true
				break
			}
		}
		if !foundLegacy {
			t.Fatalf("expected legacy socket fallback %q in %v", legacy, candidates)
		}
	}
}

func TestCmdSessionSendUsesMetadataProjectRootSocketWhenImplicit(t *testing.T) {
	projectRoot := t.TempDir()
	expectedSocket := tmuxSocketPathForProjectRoot(projectRoot)

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "tmux.log")
	tmuxPath := filepath.Join(binDir, "tmux")
	script := strings.Join([]string{
		"#!/usr/bin/env sh",
		`log="${TMUX_LOG_FILE:-/tmp/tmux.log}"`,
		`sock=""`,
		`if [ "$1" = "-S" ]; then`,
		`  sock="$2"`,
		`  echo "sock:$sock cmd:$3" >> "$log"`,
		`  shift 2`,
		`fi`,
		`if [ "$sock" != "` + expectedSocket + `" ]; then`,
		`  echo "bad_socket:$sock args:$@" >> "$log"`,
		`  exit 1`,
		`fi`,
		`cmd="$1"`,
		`case "$cmd" in`,
		`  has-session)`,
		`    if [ "$2" = "-t" ] && [ "$3" = "lisa-send-implicit-root" ]; then exit 0; fi`,
		`    exit 1 ;;`,
		`  load-buffer) cat >/dev/null; exit 0 ;;`,
		`  paste-buffer|delete-buffer|send-keys) exit 0 ;;`,
		`  *) exit 0 ;;`,
		`esac`,
		"",
	}, "\n")
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake tmux: %v", err)
	}

	origPath := os.Getenv("PATH")
	origLog := os.Getenv("TMUX_LOG_FILE")
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read cwd: %v", err)
	}
	origMetaGlob := loadSessionMetaByGlobFn
	origRuntimeRoot, hadRuntimeRoot := os.LookupEnv(lisaProjectRootEnv)
	origRuntimeSocket, hadRuntimeSocket := os.LookupEnv(lisaTmuxSocketEnv)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Setenv("TMUX_LOG_FILE", origLog)
		_ = os.Chdir(origWD)
		loadSessionMetaByGlobFn = origMetaGlob
		if hadRuntimeRoot {
			_ = os.Setenv(lisaProjectRootEnv, origRuntimeRoot)
		} else {
			_ = os.Unsetenv(lisaProjectRootEnv)
		}
		if hadRuntimeSocket {
			_ = os.Setenv(lisaTmuxSocketEnv, origRuntimeSocket)
		} else {
			_ = os.Unsetenv(lisaTmuxSocketEnv)
		}
	})
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG_FILE", logPath); err != nil {
		t.Fatalf("failed to set TMUX_LOG_FILE: %v", err)
	}

	otherRoot := t.TempDir()
	if err := os.Chdir(otherRoot); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	_ = os.Setenv(lisaProjectRootEnv, "/tmp/wrong-root")
	_ = os.Setenv(lisaTmuxSocketEnv, "/tmp/wrong.sock")
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		if session != "lisa-send-implicit-root" {
			t.Fatalf("unexpected session lookup: %q", session)
		}
		return sessionMeta{Session: session, ProjectRoot: projectRoot}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionSend([]string{
			"--session", "lisa-send-implicit-root",
			"--text", "echo hi",
			"--enter",
		}); code != 0 {
			t.Fatalf("expected send success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if stdout != "ok" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}

	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux log: %v", err)
	}
	logText := string(logRaw)
	if strings.Contains(logText, "bad_socket:") {
		t.Fatalf("expected all tmux calls to use resolved metadata socket, log: %s", logText)
	}
	if !strings.Contains(logText, "sock:"+expectedSocket+" cmd:has-session") {
		t.Fatalf("expected has-session routed through metadata socket, log: %s", logText)
	}
}
