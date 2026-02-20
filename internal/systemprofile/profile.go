package systemprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ashwch/ew/internal/appdirs"
)

const (
	profileFileName = "system_profile.json"
	schemaVersion   = 1
)

type Options struct {
	AutoTrain    bool
	RefreshHours int
}

type Status struct {
	Created   bool
	Refreshed bool
}

type Profile struct {
	Version         int      `json:"version"`
	CapturedAt      string   `json:"captured_at"`
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	Shell           string   `json:"shell,omitempty"`
	Locale          string   `json:"locale,omitempty"`
	ConfigFiles     []string `json:"config_files,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	GitGlobalIgnore string   `json:"git_global_ignore,omitempty"`
	UserNote        string   `json:"user_note,omitempty"`
}

func Ensure(opts Options) (Profile, Status, error) {
	if opts.RefreshHours <= 0 {
		opts.RefreshHours = 24 * 7
	}

	path, err := appdirs.StateFilePath(profileFileName)
	if err != nil {
		return Profile{}, Status{}, err
	}

	current, exists, err := loadPath(path)
	if err == nil && exists && !current.IsStale(opts.RefreshHours) {
		current.normalize()
		return current, Status{}, nil
	}
	if exists && !opts.AutoTrain && err == nil {
		current.normalize()
		return current, Status{}, nil
	}
	if !exists && !opts.AutoTrain {
		return Profile{}, Status{}, nil
	}

	captured := Capture()
	if exists && strings.TrimSpace(current.UserNote) != "" {
		captured.UserNote = strings.TrimSpace(current.UserNote)
	}
	if saveErr := savePath(path, captured); saveErr != nil {
		if exists && err == nil {
			return current, Status{}, nil
		}
		return Profile{}, Status{}, saveErr
	}

	status := Status{Created: !exists}
	if exists {
		status.Refreshed = true
	}
	if err != nil && exists {
		status.Refreshed = true
	}
	return captured, status, nil
}

func Save(profile Profile) error {
	path, err := appdirs.StateFilePath(profileFileName)
	if err != nil {
		return err
	}
	return savePath(path, profile)
}

func Capture() Profile {
	profile := Profile{
		Version:    schemaVersion,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}
	profile.Shell = detectShell()
	profile.Locale = detectLocale()
	profile.ConfigFiles = detectConfigFiles()
	profile.Tools = detectTools()
	profile.GitGlobalIgnore = detectGitGlobalIgnore()
	profile.normalize()
	return profile
}

func (p Profile) IsStale(refreshHours int) bool {
	if refreshHours <= 0 {
		refreshHours = 24 * 7
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(p.CapturedAt))
	if err != nil {
		return true
	}
	age := time.Since(ts)
	if age < 0 {
		return false
	}
	return age > time.Duration(refreshHours)*time.Hour
}

func (p Profile) PromptContext(maxItems int) string {
	p.normalize()
	if maxItems <= 0 {
		maxItems = 16
	}
	lines := make([]string, 0, 5)

	base := []string{}
	if p.OS != "" {
		base = append(base, "os="+p.OS)
	}
	if p.Arch != "" {
		base = append(base, "arch="+p.Arch)
	}
	if p.Shell != "" {
		base = append(base, "shell="+p.Shell)
	}
	if p.Locale != "" {
		base = append(base, "locale="+p.Locale)
	}
	if len(base) > 0 {
		lines = append(lines, strings.Join(base, " "))
	}

	if len(p.ConfigFiles) > 0 {
		lines = append(lines, "config_files="+strings.Join(trimList(p.ConfigFiles, maxItems/2), ", "))
	}
	if len(p.Tools) > 0 {
		lines = append(lines, "tools="+strings.Join(trimList(p.Tools, maxItems), ", "))
	}
	if strings.TrimSpace(p.GitGlobalIgnore) != "" {
		lines = append(lines, "git_global_ignore="+strings.TrimSpace(p.GitGlobalIgnore))
	}
	if strings.TrimSpace(p.UserNote) != "" {
		lines = append(lines, "user_note="+strings.TrimSpace(p.UserNote))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (p Profile) HumanSummary(maxItems int) string {
	context := p.PromptContext(maxItems)
	if context == "" {
		return ""
	}
	lines := strings.Split(context, "\n")
	bullets := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		bullets = append(bullets, "- "+line)
	}
	return strings.Join(bullets, "\n")
}

func detectShell() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell != "" {
		return filepath.Base(shell)
	}
	if runtime.GOOS == "windows" {
		comspec := strings.TrimSpace(os.Getenv("COMSPEC"))
		if comspec != "" {
			return filepath.Base(comspec)
		}
	}
	return ""
}

func detectLocale() string {
	candidates := []string{
		os.Getenv("EW_LOCALE"),
		os.Getenv("LC_ALL"),
		os.Getenv("LC_MESSAGES"),
		os.Getenv("LANG"),
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func detectConfigFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	paths := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".config", "fish", "config.fish"),
		filepath.Join(home, ".gitconfig"),
		filepath.Join(home, ".gitignore_global"),
		filepath.Join(home, ".config", "git", "ignore"),
	}
	found := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			found = append(found, homeRelative(path, home))
		}
	}
	sort.Strings(found)
	return dedupeStrings(found)
}

func detectTools() []string {
	candidates := []string{
		"git", "gh", "aws", "docker", "kubectl", "terraform", "terragrunt",
		"uv", "python3", "python", "node", "npm", "pnpm", "yarn",
		"go", "rustc", "cargo", "brew", "jq", "rg", "fzf",
		"claude", "codex",
	}
	installed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate); err == nil {
			installed = append(installed, candidate)
		}
	}
	sort.Strings(installed)
	return dedupeStrings(installed)
}

func detectGitGlobalIgnore() string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "config", "--global", "core.excludesFile")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return value
	}
	return homeRelative(value, home)
}

func loadPath(path string) (Profile, bool, error) {
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, fmt.Errorf("could not read system profile: %w", err)
	}
	var profile Profile
	if err := json.Unmarshal(bytes, &profile); err != nil {
		return Profile{}, true, fmt.Errorf("could not parse system profile: %w", err)
	}
	profile.normalize()
	return profile, true, nil
}

func savePath(path string, profile Profile) error {
	profile.normalize()
	payload, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode system profile: %w", err)
	}
	if _, err := appdirs.EnsureStateDir(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, ".ew-system-profile-*.json")
	if err != nil {
		return fmt.Errorf("could not create temp profile file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if _, err := tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("could not write temp profile file: %w", err)
	}
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("could not secure temp profile file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("could not close temp profile file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return fmt.Errorf("could not atomically replace profile file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("could not secure profile file permissions: %w", err)
	}
	return nil
}

func (p *Profile) normalize() {
	if p == nil {
		return
	}
	if p.Version == 0 {
		p.Version = schemaVersion
	}
	p.CapturedAt = strings.TrimSpace(p.CapturedAt)
	p.OS = strings.TrimSpace(strings.ToLower(p.OS))
	p.Arch = strings.TrimSpace(strings.ToLower(p.Arch))
	p.Shell = strings.TrimSpace(strings.ToLower(filepath.Base(p.Shell)))
	p.Locale = strings.TrimSpace(p.Locale)
	p.GitGlobalIgnore = strings.TrimSpace(p.GitGlobalIgnore)
	p.UserNote = strings.TrimSpace(p.UserNote)
	p.ConfigFiles = normalizeStringList(p.ConfigFiles)
	p.Tools = normalizeStringList(p.Tools)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func trimList(values []string, limit int) []string {
	if len(values) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(values) {
		limit = len(values)
	}
	out := append([]string(nil), values[:limit]...)
	if len(values) > limit {
		out = append(out, fmt.Sprintf("+%d more", len(values)-limit))
	}
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func homeRelative(path string, home string) string {
	path = strings.TrimSpace(path)
	home = strings.TrimSpace(home)
	if path == "" || home == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		return path
	}
	cleanHome := filepath.Clean(home)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(os.PathSeparator)
	if strings.HasPrefix(cleanPath, prefix) {
		return "~" + string(os.PathSeparator) + strings.TrimPrefix(cleanPath, prefix)
	}
	return path
}
