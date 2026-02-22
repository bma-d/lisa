package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const lisaSkillName = "lisa"
const lisaRepoOwner = "bma-d"
const lisaRepoName = "lisa"

var osUserHomeDirFn = os.UserHomeDir
var fetchReleaseSkillToTempDirFn = fetchReleaseSkillToTempDir
var skillsHTTPClient = &http.Client{Timeout: 20 * time.Second}

type skillsCopySummary struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Files       int    `json:"files"`
	Directories int    `json:"directories"`
	Symlinks    int    `json:"symlinks"`
	Noop        bool   `json:"noop,omitempty"`
}

type skillsInstallBatchSummary struct {
	Installs []skillsCopySummary `json:"installs"`
}

type skillsDoctorTarget struct {
	Target          string   `json:"target"`
	Path            string   `json:"path"`
	Exists          bool     `json:"exists"`
	Version         string   `json:"version,omitempty"`
	ContentHash     string   `json:"contentHash,omitempty"`
	RepoContentHash string   `json:"repoContentHash,omitempty"`
	Status          string   `json:"status"`
	MissingCommands []string `json:"missingCommands,omitempty"`
	Detail          string   `json:"detail,omitempty"`
	Remediation     []string `json:"remediation,omitempty"`
}

type skillsDoctorSummary struct {
	OK             bool                 `json:"ok"`
	Deep           bool                 `json:"deep,omitempty"`
	RepoRoot       string               `json:"repoRoot"`
	RepoSkillPath  string               `json:"repoSkillPath"`
	RepoVersion    string               `json:"repoVersion,omitempty"`
	CapabilityHash string               `json:"capabilityHash"`
	Targets        []skillsDoctorTarget `json:"targets"`
	ErrorCode      string               `json:"errorCode,omitempty"`
}

func cmdSkills(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lisa skills <subcommand>")
		return 1
	}
	if args[0] == "--help" || args[0] == "-h" {
		return showHelp("skills")
	}
	if args[0] == "help" {
		if len(args) > 1 {
			return showHelp("skills " + args[1])
		}
		return showHelp("skills")
	}
	switch args[0] {
	case "sync":
		return cmdSkillsSync(args[1:])
	case "doctor":
		return cmdSkillsDoctor(args[1:])
	case "install":
		return cmdSkillsInstall(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown skills subcommand: %s\n", args[0])
		return 1
	}
}

func cmdSkillsSync(args []string) int {
	from := "codex"
	fromPath := ""
	repoRoot := getPWD()
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("skills sync")
		case "--from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --from")
			}
			from = args[i+1]
			i++
		case "--path":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --path")
			}
			fromPath = args[i+1]
			i++
		case "--repo-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --repo-root")
			}
			repoRoot = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	sourcePath, err := resolveSkillsSourcePath(from, fromPath)
	if err != nil {
		return commandError(jsonOut, "skills_source_resolve_failed", err.Error())
	}
	destinationPath, err := repoSkillPath(repoRoot)
	if err != nil {
		return commandError(jsonOut, "skills_destination_resolve_failed", err.Error())
	}

	summary, err := copyDirReplace(sourcePath, destinationPath)
	if err != nil {
		return commandErrorf(jsonOut, "skills_sync_failed", "skills sync failed: %v", err)
	}

	if jsonOut {
		writeJSON(summary)
		return 0
	}
	fmt.Printf("skills sync ok: %s -> %s (%d files)\n", summary.Source, summary.Destination, summary.Files)
	return 0
}

func cmdSkillsInstall(args []string) int {
	to := ""
	toExplicit := false
	installPath := ""
	projectPath := ""
	repoRoot := getPWD()
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("skills install")
		case "--to":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --to")
			}
			to = args[i+1]
			toExplicit = true
			i++
		case "--path":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --path")
			}
			installPath = args[i+1]
			i++
		case "--project-path":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-path")
			}
			projectPath = args[i+1]
			i++
		case "--repo-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --repo-root")
			}
			repoRoot = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	sourcePath, cleanupSource, err := resolveSkillsInstallSource(repoRoot)
	if err != nil {
		return commandError(jsonOut, "skills_source_resolve_failed", err.Error())
	}
	defer cleanupSource()
	destinationPaths, err := resolveSkillsInstallDestinations(to, toExplicit, projectPath, installPath)
	if err != nil {
		return commandError(jsonOut, "skills_destination_resolve_failed", err.Error())
	}

	summaries := make([]skillsCopySummary, 0, len(destinationPaths))
	for _, destinationPath := range destinationPaths {
		summary, err := copyDirReplace(sourcePath, destinationPath)
		if err != nil {
			return commandErrorf(jsonOut, "skills_install_failed", "skills install failed: %v", err)
		}
		summaries = append(summaries, summary)
	}

	if jsonOut {
		if len(summaries) == 1 {
			writeJSON(summaries[0])
		} else {
			writeJSON(skillsInstallBatchSummary{Installs: summaries})
		}
		return 0
	}
	for _, summary := range summaries {
		fmt.Printf("skills install ok: %s -> %s (%d files)\n", summary.Source, summary.Destination, summary.Files)
	}
	return 0
}

func cmdSkillsDoctor(args []string) int {
	repoRoot := getPWD()
	jsonOut := hasJSONFlag(args)
	deep := false
	explainDrift := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("skills doctor")
		case "--repo-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --repo-root")
			}
			repoRoot = args[i+1]
			i++
		case "--deep":
			deep = true
		case "--explain-drift":
			explainDrift = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	repoSkillPath, err := repoSkillPath(repoRoot)
	if err != nil {
		return commandError(jsonOut, "skills_doctor_repo_path_failed", err.Error())
	}
	repoVersion, repoErr := readSkillVersion(filepath.Join(repoSkillPath, "SKILL.md"))
	if repoErr != nil {
		return commandErrorf(jsonOut, "skills_doctor_repo_read_failed", "failed reading repo skill: %v", repoErr)
	}
	requiredCommands := requiredSkillCommandNames()
	capHash := capabilityContractHash()
	repoContentHash := ""
	if deep {
		repoContentHash, err = hashDirectoryContents(repoSkillPath)
		if err != nil {
			return commandErrorf(jsonOut, "skills_doctor_repo_hash_failed", "failed hashing repo skill: %v", err)
		}
	}
	targets := make([]skillsDoctorTarget, 0, 2)

	for _, target := range []string{"codex", "claude"} {
		path, pathErr := defaultSkillInstallPath(target)
		if pathErr != nil {
			targetErr := skillsDoctorTarget{
				Target: target,
				Path:   "",
				Status: "error",
				Detail: pathErr.Error(),
			}
			if explainDrift {
				targetErr.Remediation = skillsDoctorRemediation(targetErr, canonicalProjectRoot(repoRoot))
			}
			targets = append(targets, targetErr)
			continue
		}
		info := skillsDoctorTarget{
			Target: target,
			Path:   path,
		}
		if !pathExists(path) {
			info.Status = "missing"
			info.Detail = "skill directory not found"
			if explainDrift {
				info.Remediation = skillsDoctorRemediation(info, canonicalProjectRoot(repoRoot))
			}
			targets = append(targets, info)
			continue
		}
		info.Exists = true
		version, versionErr := readSkillVersion(filepath.Join(path, "SKILL.md"))
		if versionErr != nil {
			info.Status = "error"
			info.Detail = versionErr.Error()
			if explainDrift {
				info.Remediation = skillsDoctorRemediation(info, canonicalProjectRoot(repoRoot))
			}
			targets = append(targets, info)
			continue
		}
		info.Version = version
		info.MissingCommands = detectMissingSkillCommands(filepath.Join(path, "data", "commands.md"), requiredCommands)
		if deep {
			info.RepoContentHash = repoContentHash
			contentHash, hashErr := hashDirectoryContents(path)
			if hashErr != nil {
				info.Status = "error"
				info.Detail = fmt.Sprintf("content hash failed: %v", hashErr)
				if explainDrift {
					info.Remediation = skillsDoctorRemediation(info, canonicalProjectRoot(repoRoot))
				}
				targets = append(targets, info)
				continue
			}
			info.ContentHash = contentHash
		}
		if version == repoVersion && len(info.MissingCommands) == 0 {
			info.Status = "up_to_date"
		} else {
			info.Status = "outdated"
			if version != repoVersion {
				info.Detail = fmt.Sprintf("version drift: repo=%s installed=%s", repoVersion, version)
			}
			if len(info.MissingCommands) > 0 {
				if info.Detail != "" {
					info.Detail += "; "
				}
				info.Detail += "command contract drift"
			}
		}
		if deep && info.Status == "up_to_date" && info.ContentHash != info.RepoContentHash {
			info.Status = "outdated"
			if info.Detail != "" {
				info.Detail += "; "
			}
			info.Detail += "content drift"
		}
		if explainDrift {
			info.Remediation = skillsDoctorRemediation(info, canonicalProjectRoot(repoRoot))
		}
		targets = append(targets, info)
	}

	ok := true
	for _, target := range targets {
		if target.Status != "up_to_date" && target.Status != "missing" {
			ok = false
			break
		}
	}
	summary := skillsDoctorSummary{
		OK:             ok,
		Deep:           deep,
		RepoRoot:       canonicalProjectRoot(repoRoot),
		RepoSkillPath:  repoSkillPath,
		RepoVersion:    repoVersion,
		CapabilityHash: capHash,
		Targets:        targets,
	}
	if !ok {
		summary.ErrorCode = "skills_doctor_drift_detected"
	}

	if jsonOut {
		writeJSON(summary)
		return boolExit(ok)
	}
	fmt.Printf("repo_version=%s capability_hash=%s ok=%t\n", summary.RepoVersion, summary.CapabilityHash, summary.OK)
	for _, target := range summary.Targets {
		fmt.Printf("- %s status=%s version=%s path=%s\n", target.Target, target.Status, target.Version, target.Path)
		if deep {
			fmt.Printf("  content_hash=%s repo_content_hash=%s\n", target.ContentHash, target.RepoContentHash)
		}
		if target.Detail != "" {
			fmt.Printf("  detail: %s\n", target.Detail)
		}
		if len(target.MissingCommands) > 0 {
			fmt.Printf("  missing: %s\n", strings.Join(target.MissingCommands, ", "))
		}
		if len(target.Remediation) > 0 {
			for _, hint := range target.Remediation {
				fmt.Printf("  remediation: %s\n", hint)
			}
		}
	}
	return boolExit(ok)
}

func skillsDoctorRemediation(target skillsDoctorTarget, repoRoot string) []string {
	hints := []string{}
	baseInstall := fmt.Sprintf("./lisa skills install --to %s --repo-root %s --json", target.Target, shellQuote(repoRoot))
	baseSync := fmt.Sprintf("./lisa skills sync --from %s --repo-root %s --json", target.Target, shellQuote(repoRoot))

	switch target.Status {
	case "up_to_date":
		return hints
	case "missing":
		hints = append(hints, baseInstall)
		hints = append(hints, "rerun doctor after install: ./lisa skills doctor --repo-root "+shellQuote(repoRoot)+" --json")
	case "outdated":
		hints = append(hints, baseInstall)
		if len(target.MissingCommands) > 0 {
			hints = append(hints, "command drift detected; verify commands.md contract after install")
		}
		hints = append(hints, "if local customizations are required, sync first: "+baseSync)
		hints = append(hints, "rerun with deep hash check: ./lisa skills doctor --repo-root "+shellQuote(repoRoot)+" --deep --json")
	default:
		hints = append(hints, "inspect target path and permissions: "+target.Path)
		hints = append(hints, baseInstall)
	}
	return hints
}

func resolveSkillsSourcePath(from, fromPath string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "codex":
		return defaultSkillInstallPath("codex")
	case "claude":
		return defaultSkillInstallPath("claude")
	case "path":
		if strings.TrimSpace(fromPath) == "" {
			return "", fmt.Errorf("--path is required when --from path")
		}
		return expandAndCleanPath(fromPath)
	default:
		return "", fmt.Errorf("invalid --from: %s (expected codex|claude|path)", from)
	}
}

func resolveSkillsInstallPath(to, projectPath, installPath string) (string, error) {
	if strings.TrimSpace(installPath) != "" {
		return expandAndCleanPath(installPath)
	}

	switch strings.ToLower(strings.TrimSpace(to)) {
	case "codex":
		return defaultSkillInstallPath("codex")
	case "claude":
		return defaultSkillInstallPath("claude")
	case "project":
		root := strings.TrimSpace(projectPath)
		if root == "" {
			return "", fmt.Errorf("--project-path is required when --to project")
		}
		expanded, err := expandAndCleanPath(root)
		if err != nil {
			return "", err
		}
		return filepath.Join(expanded, "skills", lisaSkillName), nil
	default:
		return "", fmt.Errorf("invalid --to: %s (expected codex|claude|project)", to)
	}
}

func resolveSkillsInstallDestinations(to string, toExplicit bool, projectPath, installPath string) ([]string, error) {
	if strings.TrimSpace(installPath) != "" {
		path, err := resolveSkillsInstallPath("", projectPath, installPath)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	if toExplicit {
		path, err := resolveSkillsInstallPath(to, projectPath, "")
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}

	targets, err := discoverDefaultInstallTargets()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		path, err := defaultSkillInstallPath(target)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func resolveSkillsInstallSource(repoRoot string) (string, func(), error) {
	if isTaggedReleaseBuild() {
		path, err := fetchReleaseSkillToTempDirFn(BuildVersion)
		if err != nil {
			return "", nil, err
		}
		return path, func() { _ = os.RemoveAll(filepath.Dir(filepath.Dir(path))) }, nil
	}
	path, err := repoSkillPath(repoRoot)
	if err != nil {
		return "", nil, err
	}
	return path, func() {}, nil
}

func isTaggedReleaseBuild() bool {
	version := strings.ToLower(strings.TrimSpace(BuildVersion))
	return version != "" && version != "dev"
}

func repoSkillPath(repoRoot string) (string, error) {
	root, err := expandAndCleanPath(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "skills", lisaSkillName), nil
}

func defaultSkillInstallPath(target string) (string, error) {
	home, err := osUserHomeDirFn()
	if err != nil {
		return "", err
	}
	switch target {
	case "codex":
		return filepath.Join(home, ".codex", "skills", lisaSkillName), nil
	case "claude":
		return filepath.Join(home, ".claude", "skills", lisaSkillName), nil
	default:
		return "", fmt.Errorf("invalid target: %s", target)
	}
}

func discoverDefaultInstallTargets() ([]string, error) {
	home, err := osUserHomeDirFn()
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0, 2)
	if pathExists(filepath.Join(home, ".codex")) {
		targets = append(targets, "codex")
	}
	if pathExists(filepath.Join(home, ".claude")) {
		targets = append(targets, "claude")
	}
	if len(targets) == 0 {
		return nil, errors.New("no default install targets found (expected ~/.codex and/or ~/.claude); pass --to or --path")
	}
	return targets, nil
}

func readSkillVersion(skillPath string) (string, error) {
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "version:")), nil
		}
	}
	return "", fmt.Errorf("version field not found in %s", skillPath)
}

func requiredSkillCommandNames() []string {
	names := make([]string, 0, len(commandCapabilities))
	for _, cmd := range commandCapabilities {
		names = append(names, cmd.Name)
	}
	return names
}

func capabilityContractHash() string {
	builder := strings.Builder{}
	for _, cmd := range commandCapabilities {
		builder.WriteString(cmd.Name)
		builder.WriteString("|")
		builder.WriteString(strings.Join(cmd.Flags, ","))
		builder.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:12]
}

func detectMissingSkillCommands(commandsPath string, required []string) []string {
	raw, err := os.ReadFile(commandsPath)
	if err != nil {
		return append([]string{}, required...)
	}
	text := string(raw)
	missing := []string{}
	for _, name := range required {
		if !strings.Contains(text, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func hashDirectoryContents(root string) (string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return "", fmt.Errorf("directory path is required")
	}
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, _ = hash.Write([]byte("L|" + rel + "|" + target + "\n"))
			return nil
		}
		if d.IsDir() {
			_, _ = hash.Write([]byte("D|" + rel + "\n"))
			return nil
		}
		_, _ = hash.Write([]byte("F|" + rel + "\n"))
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := io.Copy(hash, file); err != nil {
			return err
		}
		_, _ = hash.Write([]byte("\n"))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func expandAndCleanPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := osUserHomeDirFn()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func copyDirReplace(sourcePath, destinationPath string) (skillsCopySummary, error) {
	sourcePath = filepath.Clean(sourcePath)
	destinationPath = filepath.Clean(destinationPath)
	if sourcePath == destinationPath {
		return skillsCopySummary{
			Source:      sourcePath,
			Destination: destinationPath,
			Noop:        true,
		}, nil
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return skillsCopySummary{}, err
	}
	if !info.IsDir() {
		return skillsCopySummary{}, fmt.Errorf("source is not a directory: %s", sourcePath)
	}

	if err := os.RemoveAll(destinationPath); err != nil {
		return skillsCopySummary{}, err
	}
	if err := os.MkdirAll(destinationPath, info.Mode().Perm()); err != nil {
		return skillsCopySummary{}, err
	}

	summary := skillsCopySummary{
		Source:      sourcePath,
		Destination: destinationPath,
	}

	err = filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(destinationPath, rel)

		switch {
		case d.Type()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(linkTarget, dest); err != nil {
				return err
			}
			summary.Symlinks++
		case d.IsDir():
			dirInfo, err := d.Info()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dest, dirInfo.Mode().Perm()); err != nil {
				return err
			}
			summary.Directories++
		default:
			fileInfo, err := d.Info()
			if err != nil {
				return err
			}
			if err := copyFile(path, dest, fileInfo.Mode().Perm()); err != nil {
				return err
			}
			summary.Files++
		}
		return nil
	})
	if err != nil {
		return skillsCopySummary{}, err
	}

	return summary, nil
}

func copyFile(sourcePath, destinationPath string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	in, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func fetchReleaseSkillToTempDir(version string) (string, error) {
	refCandidates := releaseRefCandidates(version)
	if len(refCandidates) == 0 {
		return "", fmt.Errorf("invalid build version for release skill fetch: %q", version)
	}

	var lastErr error
	for _, ref := range refCandidates {
		skillBody, err := fetchReleaseSkillFile(ref)
		if err != nil {
			lastErr = err
			continue
		}

		tmpRoot, err := os.MkdirTemp("", "lisa-skill-release-*")
		if err != nil {
			return "", err
		}
		skillDir := filepath.Join(tmpRoot, "skills", lisaSkillName)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			_ = os.RemoveAll(tmpRoot)
			return "", err
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(skillBody), 0o644); err != nil {
			_ = os.RemoveAll(tmpRoot)
			return "", err
		}
		return skillDir, nil
	}

	if lastErr == nil {
		lastErr = errors.New("release skill fetch failed")
	}
	return "", fmt.Errorf("failed fetching release skill from GitHub for version %q: %w", version, lastErr)
}

func releaseRefCandidates(version string) []string {
	v := strings.TrimSpace(version)
	if v == "" || strings.EqualFold(v, "dev") {
		return nil
	}
	candidates := []string{v}
	if strings.HasPrefix(strings.ToLower(v), "v") {
		candidates = append(candidates, strings.TrimPrefix(v, "v"))
	} else {
		candidates = append(candidates, "v"+v)
	}
	candidates = append(candidates, "main")
	return dedupeStrings(candidates)
}

func fetchReleaseSkillFile(tag string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/skills/%s/SKILL.md", lisaRepoOwner, lisaRepoName, tag, lisaSkillName)
	resp, err := skillsHTTPClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("GET %s: %s (%s)", url, resp.Status, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
