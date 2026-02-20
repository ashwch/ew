package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ashwch/ew/internal/appdirs"
	"github.com/ashwch/ew/internal/i18n"
	"github.com/pelletier/go-toml/v2"
)

type IntentConfig struct {
	Model         string  `toml:"model" json:"model"`
	Thinking      string  `toml:"thinking" json:"thinking"`
	MinConfidence float64 `toml:"min_confidence,omitempty" json:"min_confidence,omitempty"`
	MaxResults    int     `toml:"max_results,omitempty" json:"max_results,omitempty"`
	AIRerank      string  `toml:"ai_rerank,omitempty" json:"ai_rerank,omitempty"`
	AutoRun       bool    `toml:"auto_run,omitempty" json:"auto_run,omitempty"`
}

type ModelConfig struct {
	ProviderModel string            `toml:"provider_model,omitempty" json:"provider_model,omitempty"`
	Thinking      string            `toml:"thinking,omitempty" json:"thinking,omitempty"`
	Speed         string            `toml:"speed,omitempty" json:"speed,omitempty"`
	Description   string            `toml:"description,omitempty" json:"description,omitempty"`
	Metadata      map[string]string `toml:"metadata,omitempty" json:"metadata,omitempty"`
}

type ProviderConfig struct {
	Type         string                 `toml:"type,omitempty" json:"type,omitempty"`
	Command      string                 `toml:"command,omitempty" json:"command,omitempty"`
	Enabled      *bool                  `toml:"enabled,omitempty" json:"enabled,omitempty"`
	Model        string                 `toml:"model" json:"model"`
	Thinking     string                 `toml:"thinking" json:"thinking"`
	ModelFlag    string                 `toml:"model_flag,omitempty" json:"model_flag,omitempty"`
	ThinkingFlag string                 `toml:"thinking_flag,omitempty" json:"thinking_flag,omitempty"`
	Args         []string               `toml:"args,omitempty" json:"args,omitempty"`
	Models       map[string]ModelConfig `toml:"models,omitempty" json:"models,omitempty"`
}

type SafetyConfig struct {
	RedactSecrets     bool `toml:"redact_secrets" json:"redact_secrets"`
	BlockHighRisk     bool `toml:"block_high_risk" json:"block_high_risk"`
	AllowYoloHighRisk bool `toml:"allow_yolo_high_risk" json:"allow_yolo_high_risk"`
}

type PromptConfig struct {
	SelfKnowledge string `toml:"self_knowledge" json:"self_knowledge"`
	StrictJSON    bool   `toml:"strict_json" json:"strict_json"`
}

type AIConfig struct {
	MinConfidence         float64 `toml:"min_confidence" json:"min_confidence"`
	AllowSuggestExecution bool    `toml:"allow_suggest_execution" json:"allow_suggest_execution"`
}

type UIConfig struct {
	Backend string `toml:"backend" json:"backend"`
}

type SystemConfig struct {
	EnableContext  bool `toml:"enable_context" json:"enable_context"`
	AutoTrain      bool `toml:"auto_train" json:"auto_train"`
	RefreshHours   int  `toml:"refresh_hours" json:"refresh_hours"`
	MaxPromptItems int  `toml:"max_prompt_items" json:"max_prompt_items"`
}

type Config struct {
	Version   int                       `toml:"version" json:"version"`
	Locale    string                    `toml:"locale" json:"locale"`
	Provider  string                    `toml:"provider" json:"provider"`
	Mode      string                    `toml:"mode" json:"mode"`
	Fix       IntentConfig              `toml:"fix" json:"fix"`
	Find      IntentConfig              `toml:"find" json:"find"`
	Providers map[string]ProviderConfig `toml:"providers" json:"providers"`
	Safety    SafetyConfig              `toml:"safety" json:"safety"`
	Prompt    PromptConfig              `toml:"prompt" json:"prompt"`
	AI        AIConfig                  `toml:"ai" json:"ai"`
	UI        UIConfig                  `toml:"ui" json:"ui"`
	System    SystemConfig              `toml:"system" json:"system"`
}

func Default() Config {
	return Config{
		Version:  1,
		Locale:   "auto",
		Provider: "auto",
		Mode:     "confirm",
		Fix: IntentConfig{
			Model:         "auto-main",
			Thinking:      "medium",
			MinConfidence: 0.70,
		},
		Find: IntentConfig{
			Model:         "auto-fast",
			Thinking:      "minimal",
			MinConfidence: 0.60,
			MaxResults:    8,
			AIRerank:      "auto",
			AutoRun:       false,
		},
		Providers: defaultProviderCatalog(),
		Safety: SafetyConfig{
			RedactSecrets:     true,
			BlockHighRisk:     true,
			AllowYoloHighRisk: false,
		},
		Prompt: PromptConfig{SelfKnowledge: "compiled", StrictJSON: true},
		AI: AIConfig{
			MinConfidence:         0.60,
			AllowSuggestExecution: false,
		},
		UI: UIConfig{
			Backend: "bubbletea",
		},
		System: SystemConfig{
			EnableContext:  true,
			AutoTrain:      true,
			RefreshHours:   168,
			MaxPromptItems: 16,
		},
	}
}

func defaultProviderCatalog() map[string]ProviderConfig {
	codexEnabled := true
	claudeEnabled := true
	ewEnabled := true

	return map[string]ProviderConfig{
		"ew": {
			Type:     "builtin",
			Command:  "ew",
			Enabled:  &ewEnabled,
			Model:    "ew-core",
			Thinking: "minimal",
			Models: map[string]ModelConfig{
				"ew-core": {
					ProviderModel: "ew-core",
					Thinking:      "minimal",
					Speed:         "fast",
					Description:   "Local deterministic command suggestions",
				},
			},
		},
		"codex": {
			Type:         "command",
			Command:      "codex",
			Enabled:      &codexEnabled,
			Model:        "gpt-5-codex",
			Thinking:     "medium",
			ModelFlag:    "--model",
			ThinkingFlag: "-c model_reasoning_effort={thinking}",
			Args: []string{
				"exec",
				"--skip-git-repo-check",
				"--sandbox",
				"read-only",
				"--output-schema",
				"{schema_file}",
				"--output-last-message",
				"{output_file}",
				"--model",
				"{model}",
				"-c",
				"model_reasoning_effort={thinking}",
				"-c",
				"web_search='disabled'",
				"{prompt}",
			},
			Models: map[string]ModelConfig{
				"gpt-5-codex": {
					ProviderModel: "gpt-5-codex",
					Thinking:      "medium",
					Speed:         "quality",
					Description:   "Best default for command fixing",
				},
				"gpt-5-mini": {
					ProviderModel: "gpt-5-mini",
					Thinking:      "minimal",
					Speed:         "fast",
					Description:   "Fast/low-cost search and rerank",
				},
			},
		},
		"claude": {
			Type:         "command",
			Command:      "claude",
			Enabled:      &claudeEnabled,
			Model:        "sonnet",
			Thinking:     "medium",
			ModelFlag:    "--model",
			ThinkingFlag: "--thinking {thinking}",
			Args: []string{
				"-p",
				"--output-format",
				"json",
				"--json-schema",
				"{schema_json}",
				"--model",
				"{model}",
				"--effort",
				"{thinking}",
				"--permission-mode",
				"{permission_mode}",
				"{prompt}",
			},
			Models: map[string]ModelConfig{
				"sonnet": {
					ProviderModel: "sonnet",
					Thinking:      "medium",
					Speed:         "balanced",
					Description:   "Balanced default",
				},
				"haiku": {
					ProviderModel: "haiku",
					Thinking:      "minimal",
					Speed:         "fast",
					Description:   "Fast/low-cost search and rerank",
				},
			},
		},
	}
}

func LoadOrCreate() (Config, string, error) {
	path, err := appdirs.ConfigFilePath()
	if err != nil {
		return Config{}, "", err
	}

	cfg := Default()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if _, err := appdirs.EnsureConfigDir(); err != nil {
			return Config{}, "", err
		}
		if err := Save(path, cfg); err != nil {
			return Config{}, "", err
		}
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, "", fmt.Errorf("could not stat config path: %w", err)
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return Config{}, "", fmt.Errorf("could not read config file: %w", err)
	}

	if err := toml.Unmarshal(bytes, &cfg); err != nil {
		return Config{}, "", fmt.Errorf("could not parse config file: %w", err)
	}
	cfg.normalize()
	return cfg, path, nil
}

func Save(path string, cfg Config) error {
	cfg.normalize()
	payload, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("could not serialize config: %w", err)
	}
	if _, err := appdirs.EnsureConfigDir(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, ".ew-config-*.toml")
	if err != nil {
		return fmt.Errorf("could not create temp config file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
	}

	if _, err := tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("could not write temp config file: %w", err)
	}
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("could not secure temp config file permissions: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("could not close temp config file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return fmt.Errorf("could not atomically replace config file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("could not secure config file permissions: %w", err)
	}
	return nil
}

func (c *Config) normalize() {
	defaults := Default()
	if c.Version == 0 {
		c.Version = defaults.Version
	}
	if c.Provider == "" {
		c.Provider = defaults.Provider
	}
	c.Locale = normalizeLocaleSetting(c.Locale, defaults.Locale)
	if c.Mode == "" {
		c.Mode = defaults.Mode
	}
	if c.Fix.Model == "" {
		c.Fix.Model = defaults.Fix.Model
	}
	if c.Fix.Thinking == "" {
		c.Fix.Thinking = defaults.Fix.Thinking
	}
	if c.Fix.MinConfidence <= 0 || c.Fix.MinConfidence > 1 {
		c.Fix.MinConfidence = defaults.Fix.MinConfidence
	}
	if c.Find.Model == "" {
		c.Find.Model = defaults.Find.Model
	}
	if c.Find.Thinking == "" {
		c.Find.Thinking = defaults.Find.Thinking
	}
	if c.Find.MinConfidence <= 0 || c.Find.MinConfidence > 1 {
		c.Find.MinConfidence = defaults.Find.MinConfidence
	}
	if c.Find.MaxResults <= 0 {
		c.Find.MaxResults = defaults.Find.MaxResults
	}
	if c.Find.AIRerank == "" {
		c.Find.AIRerank = defaults.Find.AIRerank
	}
	if c.Prompt.SelfKnowledge == "" {
		c.Prompt.SelfKnowledge = defaults.Prompt.SelfKnowledge
	}
	if c.AI.MinConfidence <= 0 || c.AI.MinConfidence > 1 {
		c.AI.MinConfidence = defaults.AI.MinConfidence
	}
	c.UI.Backend = normalizeUIBackend(c.UI.Backend, defaults.UI.Backend)
	if c.System.RefreshHours <= 0 {
		c.System.RefreshHours = defaults.System.RefreshHours
	}
	if c.System.MaxPromptItems <= 0 {
		c.System.MaxPromptItems = defaults.System.MaxPromptItems
	}
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}

	defaultProviders := defaultProviderCatalog()
	for name, def := range defaultProviders {
		current, ok := c.Providers[name]
		if !ok {
			c.Providers[name] = def
			continue
		}
		mergeProviderDefaults(&current, def)
		c.Providers[name] = current
	}

	for name, provider := range c.Providers {
		if provider.Type == "" {
			provider.Type = "command"
		}
		if provider.Command == "" {
			provider.Command = name
		}
		if provider.Enabled == nil {
			provider.Enabled = boolPtr(true)
		}
		if provider.Models == nil {
			provider.Models = map[string]ModelConfig{}
		}
		if provider.Model == "" {
			provider.Model = pickFirstModelAlias(provider.Models)
		}
		if provider.Thinking == "" {
			provider.Thinking = defaults.Fix.Thinking
		}
		if provider.ModelFlag == "" {
			provider.ModelFlag = "--model"
		}
		c.Providers[name] = provider
	}

	if c.Provider != "auto" {
		if _, ok := c.Providers[c.Provider]; !ok {
			c.Providers[c.Provider] = ProviderConfig{
				Type:     "command",
				Command:  c.Provider,
				Enabled:  boolPtr(true),
				Model:    c.Fix.Model,
				Thinking: c.Fix.Thinking,
				Models:   map[string]ModelConfig{},
			}
		}
	}
}

func mergeProviderDefaults(target *ProviderConfig, defaults ProviderConfig) {
	if target.Type == "" {
		target.Type = defaults.Type
	}
	if target.Command == "" {
		target.Command = defaults.Command
	}
	if target.Enabled == nil {
		target.Enabled = defaults.Enabled
	}
	if target.Model == "" {
		target.Model = defaults.Model
	}
	if target.Thinking == "" {
		target.Thinking = defaults.Thinking
	}
	if target.ModelFlag == "" {
		target.ModelFlag = defaults.ModelFlag
	}
	if target.ThinkingFlag == "" {
		target.ThinkingFlag = defaults.ThinkingFlag
	}
	if len(target.Args) == 0 {
		target.Args = append([]string(nil), defaults.Args...)
	}
	if target.Models == nil {
		target.Models = map[string]ModelConfig{}
	}
	for alias, defModel := range defaults.Models {
		if _, ok := target.Models[alias]; !ok {
			target.Models[alias] = defModel
		}
	}
}

func (c *Config) Set(key, value string) error {
	key = strings.TrimSpace(strings.ToLower(key))
	value = strings.TrimSpace(value)

	if strings.HasPrefix(key, "providers.") {
		if err := c.setProviderKey(key, value); err != nil {
			return err
		}
		c.normalize()
		return nil
	}

	switch key {
	case "locale":
		c.Locale = normalizeLocaleSetting(value, "")
		if c.Locale == "" {
			return fmt.Errorf("locale must be 'auto' or a locale like en, en-US, hi, hi-IN")
		}
	case "provider":
		c.Provider = value
	case "mode":
		c.Mode = value
	case "ui.backend":
		c.UI.Backend = normalizeUIBackend(value, "")
		if c.UI.Backend == "" {
			return fmt.Errorf("ui.backend must be one of auto|bubbletea|huh|tview|plain")
		}
	case "system.enable_context":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("system.enable_context must be boolean")
		}
		c.System.EnableContext = b
	case "system.auto_train":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("system.auto_train must be boolean")
		}
		c.System.AutoTrain = b
	case "system.refresh_hours":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("system.refresh_hours must be a positive number")
		}
		c.System.RefreshHours = n
	case "system.max_prompt_items":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("system.max_prompt_items must be a positive number")
		}
		c.System.MaxPromptItems = n
	case "fix.model":
		c.Fix.Model = value
	case "fix.thinking":
		c.Fix.Thinking = value
	case "fix.min_confidence":
		n, err := parseConfidence(value)
		if err != nil {
			return fmt.Errorf("fix.min_confidence must be between 0 and 1")
		}
		c.Fix.MinConfidence = n
	case "find.model":
		c.Find.Model = value
	case "find.thinking":
		c.Find.Thinking = value
	case "find.min_confidence":
		n, err := parseConfidence(value)
		if err != nil {
			return fmt.Errorf("find.min_confidence must be between 0 and 1")
		}
		c.Find.MinConfidence = n
	case "find.max_results":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("find.max_results must be a number")
		}
		if n <= 0 {
			return fmt.Errorf("find.max_results must be positive")
		}
		c.Find.MaxResults = n
	case "ai.min_confidence":
		n, err := parseConfidence(value)
		if err != nil {
			return fmt.Errorf("ai.min_confidence must be between 0 and 1")
		}
		c.AI.MinConfidence = n
	case "ai.allow_suggest_execution":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("ai.allow_suggest_execution must be boolean")
		}
		c.AI.AllowSuggestExecution = b
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	c.normalize()
	return nil
}

func (c *Config) setProviderKey(key, value string) error {
	parts := strings.Split(key, ".")
	if len(parts) < 3 {
		return fmt.Errorf("invalid provider key: %s", key)
	}
	providerName := parts[1]
	provider := c.ensureProvider(providerName)

	if len(parts) == 3 {
		switch parts[2] {
		case "model":
			provider.Model = value
		case "thinking":
			provider.Thinking = value
		case "type":
			provider.Type = value
		case "command":
			provider.Command = value
		case "model_flag":
			provider.ModelFlag = value
		case "thinking_flag":
			provider.ThinkingFlag = value
		case "enabled":
			b, err := parseBool(value)
			if err != nil {
				return fmt.Errorf("providers.%s.enabled must be boolean", providerName)
			}
			provider.Enabled = boolPtr(b)
		case "args":
			provider.Args = splitCommaList(value)
		default:
			return fmt.Errorf("unknown provider field: %s", parts[2])
		}
		c.Providers[providerName] = provider
		return nil
	}

	if len(parts) == 5 && parts[2] == "models" {
		alias := parts[3]
		field := parts[4]
		if provider.Models == nil {
			provider.Models = map[string]ModelConfig{}
		}
		model := provider.Models[alias]
		switch field {
		case "provider_model":
			model.ProviderModel = value
		case "thinking":
			model.Thinking = value
		case "speed":
			model.Speed = value
		case "description":
			model.Description = value
		default:
			return fmt.Errorf("unknown model field: %s", field)
		}
		provider.Models[alias] = model
		c.Providers[providerName] = provider
		return nil
	}

	return fmt.Errorf("unsupported provider key path: %s", key)
}

func (c *Config) ensureProvider(name string) ProviderConfig {
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}
	provider, ok := c.Providers[name]
	if !ok {
		provider = ProviderConfig{
			Type:     "command",
			Command:  name,
			Enabled:  boolPtr(true),
			Model:    c.Fix.Model,
			Thinking: c.Fix.Thinking,
			Models:   map[string]ModelConfig{},
		}
		c.Providers[name] = provider
	}
	if provider.Models == nil {
		provider.Models = map[string]ModelConfig{}
	}
	return provider
}

func (c Config) Get(key string) (string, error) {
	key = strings.TrimSpace(strings.ToLower(key))

	if strings.HasPrefix(key, "providers.") {
		return c.getProviderKey(key)
	}

	switch key {
	case "locale":
		return c.Locale, nil
	case "provider":
		return c.Provider, nil
	case "mode":
		return c.Mode, nil
	case "ui.backend":
		return c.UI.Backend, nil
	case "system.enable_context":
		return strconv.FormatBool(c.System.EnableContext), nil
	case "system.auto_train":
		return strconv.FormatBool(c.System.AutoTrain), nil
	case "system.refresh_hours":
		return fmt.Sprintf("%d", c.System.RefreshHours), nil
	case "system.max_prompt_items":
		return fmt.Sprintf("%d", c.System.MaxPromptItems), nil
	case "fix.model":
		return c.Fix.Model, nil
	case "fix.thinking":
		return c.Fix.Thinking, nil
	case "fix.min_confidence":
		return fmt.Sprintf("%g", c.Fix.MinConfidence), nil
	case "find.model":
		return c.Find.Model, nil
	case "find.thinking":
		return c.Find.Thinking, nil
	case "find.min_confidence":
		return fmt.Sprintf("%g", c.Find.MinConfidence), nil
	case "find.max_results":
		return fmt.Sprintf("%d", c.Find.MaxResults), nil
	case "ai.min_confidence":
		return fmt.Sprintf("%g", c.AI.MinConfidence), nil
	case "ai.allow_suggest_execution":
		return strconv.FormatBool(c.AI.AllowSuggestExecution), nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func (c Config) getProviderKey(key string) (string, error) {
	parts := strings.Split(key, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid provider key: %s", key)
	}
	providerName := parts[1]
	provider, ok := c.Providers[providerName]
	if !ok {
		return "", fmt.Errorf("unknown provider: %s", providerName)
	}

	if len(parts) == 3 {
		switch parts[2] {
		case "model":
			return provider.Model, nil
		case "thinking":
			return provider.Thinking, nil
		case "type":
			return provider.Type, nil
		case "command":
			return provider.Command, nil
		case "model_flag":
			return provider.ModelFlag, nil
		case "thinking_flag":
			return provider.ThinkingFlag, nil
		case "enabled":
			return strconv.FormatBool(provider.Enabled == nil || *provider.Enabled), nil
		case "args":
			return strings.Join(provider.Args, ","), nil
		default:
			return "", fmt.Errorf("unknown provider field: %s", parts[2])
		}
	}

	if len(parts) == 5 && parts[2] == "models" {
		alias := parts[3]
		field := parts[4]
		model, ok := provider.Models[alias]
		if !ok {
			return "", fmt.Errorf("unknown model alias: %s", alias)
		}
		switch field {
		case "provider_model":
			return model.ProviderModel, nil
		case "thinking":
			return model.Thinking, nil
		case "speed":
			return model.Speed, nil
		case "description":
			return model.Description, nil
		default:
			return "", fmt.Errorf("unknown model field: %s", field)
		}
	}

	return "", fmt.Errorf("unsupported provider key path: %s", key)
}

func (c Config) ProviderNames() []string {
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool: %s", value)
	}
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func parseConfidence(value string) (float64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	if n <= 0 || n > 1 {
		return 0, fmt.Errorf("confidence must be between 0 and 1")
	}
	return n, nil
}

func pickFirstModelAlias(models map[string]ModelConfig) string {
	if len(models) == 0 {
		return ""
	}
	aliases := make([]string, 0, len(models))
	for alias := range models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases[0]
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}

func normalizeUIBackend(value string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "auto", "bubbletea", "huh", "tview", "plain":
		return normalized
	case "":
		return strings.ToLower(strings.TrimSpace(fallback))
	default:
		return strings.ToLower(strings.TrimSpace(fallback))
	}
}

func normalizeLocaleSetting(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = strings.TrimSpace(fallback)
	}
	if strings.EqualFold(trimmed, "auto") {
		return "auto"
	}
	normalized := i18n.NormalizeLocale(trimmed)
	if normalized == "" {
		return ""
	}
	return normalized
}
