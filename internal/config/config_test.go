package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSetGetAIConfidenceAndSuggestPolicy(t *testing.T) {
	cfg := Default()

	if err := cfg.Set("ai.min_confidence", "0.75"); err != nil {
		t.Fatalf("set ai.min_confidence failed: %v", err)
	}
	if err := cfg.Set("find.min_confidence", "0.65"); err != nil {
		t.Fatalf("set find.min_confidence failed: %v", err)
	}
	if err := cfg.Set("ai.allow_suggest_execution", "true"); err != nil {
		t.Fatalf("set ai.allow_suggest_execution failed: %v", err)
	}
	if err := cfg.Set("ui.backend", "bubbletea"); err != nil {
		t.Fatalf("set ui.backend failed: %v", err)
	}
	if err := cfg.Set("locale", "hi-in"); err != nil {
		t.Fatalf("set locale failed: %v", err)
	}
	if err := cfg.Set("system.enable_context", "false"); err != nil {
		t.Fatalf("set system.enable_context failed: %v", err)
	}
	if err := cfg.Set("system.auto_train", "false"); err != nil {
		t.Fatalf("set system.auto_train failed: %v", err)
	}
	if err := cfg.Set("system.refresh_hours", "24"); err != nil {
		t.Fatalf("set system.refresh_hours failed: %v", err)
	}
	if err := cfg.Set("system.max_prompt_items", "12"); err != nil {
		t.Fatalf("set system.max_prompt_items failed: %v", err)
	}

	gotGlobal, err := cfg.Get("ai.min_confidence")
	if err != nil {
		t.Fatalf("get ai.min_confidence failed: %v", err)
	}
	if gotGlobal != "0.75" {
		t.Fatalf("expected 0.75, got %q", gotGlobal)
	}

	gotFind, err := cfg.Get("find.min_confidence")
	if err != nil {
		t.Fatalf("get find.min_confidence failed: %v", err)
	}
	if gotFind != "0.65" {
		t.Fatalf("expected 0.65, got %q", gotFind)
	}

	gotSuggest, err := cfg.Get("ai.allow_suggest_execution")
	if err != nil {
		t.Fatalf("get ai.allow_suggest_execution failed: %v", err)
	}
	if gotSuggest != "true" {
		t.Fatalf("expected true, got %q", gotSuggest)
	}

	gotUI, err := cfg.Get("ui.backend")
	if err != nil {
		t.Fatalf("get ui.backend failed: %v", err)
	}
	if gotUI != "bubbletea" {
		t.Fatalf("expected bubbletea, got %q", gotUI)
	}
	gotLocale, err := cfg.Get("locale")
	if err != nil {
		t.Fatalf("get locale failed: %v", err)
	}
	if gotLocale != "hi-IN" {
		t.Fatalf("expected hi-IN locale normalization, got %q", gotLocale)
	}

	gotEnable, err := cfg.Get("system.enable_context")
	if err != nil {
		t.Fatalf("get system.enable_context failed: %v", err)
	}
	if gotEnable != "false" {
		t.Fatalf("expected false, got %q", gotEnable)
	}
	gotTrain, err := cfg.Get("system.auto_train")
	if err != nil {
		t.Fatalf("get system.auto_train failed: %v", err)
	}
	if gotTrain != "false" {
		t.Fatalf("expected false, got %q", gotTrain)
	}
	gotRefresh, err := cfg.Get("system.refresh_hours")
	if err != nil {
		t.Fatalf("get system.refresh_hours failed: %v", err)
	}
	if gotRefresh != "24" {
		t.Fatalf("expected 24, got %q", gotRefresh)
	}
	gotMax, err := cfg.Get("system.max_prompt_items")
	if err != nil {
		t.Fatalf("get system.max_prompt_items failed: %v", err)
	}
	if gotMax != "12" {
		t.Fatalf("expected 12, got %q", gotMax)
	}
}

func TestSetRejectsInvalidConfidence(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("fix.min_confidence", "1.2"); err == nil {
		t.Fatalf("expected error for invalid confidence >1")
	}
}

func TestSetRejectsInvalidUIBackend(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("ui.backend", "neon-ui"); err == nil {
		t.Fatalf("expected invalid ui.backend to be rejected")
	}
}

func TestDefaultUIBackendIsBubbleTea(t *testing.T) {
	cfg := Default()
	if cfg.UI.Backend != "bubbletea" {
		t.Fatalf("expected default ui backend bubbletea, got %q", cfg.UI.Backend)
	}
	if cfg.Locale != "auto" {
		t.Fatalf("expected default locale auto, got %q", cfg.Locale)
	}
	if !cfg.System.EnableContext {
		t.Fatalf("expected default system context enabled")
	}
	if !cfg.System.AutoTrain {
		t.Fatalf("expected default system auto-train enabled")
	}
	if cfg.System.RefreshHours <= 0 {
		t.Fatalf("expected positive default system refresh hours")
	}
	if cfg.System.MaxPromptItems <= 0 {
		t.Fatalf("expected positive default system prompt items")
	}
}

func TestSetRejectsInvalidLocale(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("locale", "%%bad-locale"); err == nil {
		t.Fatalf("expected invalid locale to be rejected")
	}
}

func TestSetRejectsInvalidSystemConfig(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("system.enable_context", "notabool"); err == nil {
		t.Fatalf("expected invalid bool to be rejected")
	}
	if err := cfg.Set("system.refresh_hours", "0"); err == nil {
		t.Fatalf("expected non-positive refresh hours to be rejected")
	}
	if err := cfg.Set("system.max_prompt_items", "-1"); err == nil {
		t.Fatalf("expected non-positive max_prompt_items to be rejected")
	}
}

func TestNormalizePreservesExplicitSafetyFalseValues(t *testing.T) {
	cfg := Default()
	cfg.Safety.RedactSecrets = false
	cfg.Safety.BlockHighRisk = false
	cfg.Safety.AllowYoloHighRisk = false

	cfg.normalize()

	if cfg.Safety.RedactSecrets {
		t.Fatalf("expected redact_secrets=false to be preserved")
	}
	if cfg.Safety.BlockHighRisk {
		t.Fatalf("expected block_high_risk=false to be preserved")
	}
	if cfg.Safety.AllowYoloHighRisk {
		t.Fatalf("expected allow_yolo_high_risk=false to be preserved")
	}
}

func TestSaveUsesPrivateFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	cfg := Default()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config failed: %v", err)
	}
	if perms := info.Mode().Perm(); perms&0o077 != 0 {
		t.Fatalf("expected private permissions, got %o", perms)
	}
}

func TestSaveAtomicWriteProducesParseableConfigUnderConcurrentSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg := Default()
			if idx%2 == 0 {
				cfg.Provider = "claude"
			} else {
				cfg.Provider = "codex"
			}
			if err := Save(path, cfg); err != nil {
				t.Errorf("save failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	var parsed Config
	if err := toml.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("expected final config to be parseable TOML, got error: %v\ncontent:\n%s", err, string(bytes))
	}
}
