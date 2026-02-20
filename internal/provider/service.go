package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ashwch/ew/internal/config"
)

type Service struct {
	registry *Registry
}

func NewService(registry *Registry) *Service {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Service{registry: registry}
}

func (s *Service) Resolve(ctx context.Context, cfg config.Config, req Request, preferredProvider string) (Resolution, string, error) {
	order := providerOrder(cfg, preferredProvider)
	if len(order) == 0 {
		return Resolution{}, "", fmt.Errorf("no providers configured")
	}

	issues := make([]string, 0, len(order))
	for _, name := range order {
		providerCfg, ok := cfg.Providers[name]
		if !ok {
			continue
		}
		if providerCfg.Enabled != nil && !*providerCfg.Enabled {
			continue
		}

		adapter, err := s.registry.Build(name, providerCfg)
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if checker, ok := adapter.(HealthChecker); ok {
			if err := checker.HealthCheck(); err != nil {
				issues = append(issues, fmt.Sprintf("%s: %v", name, err))
				continue
			}
		}

		providerReq := req
		providerReq.Model = resolveModel(providerCfg, req.Model)
		providerReq.Thinking = resolveThinking(name, providerCfg, providerReq.Model, req.Thinking)
		providerReq.Context = cloneContext(req.Context)
		providerReq.Context["permission_mode"] = permissionModeFor(providerReq.Mode)

		providerCtx, cancel := timeoutContext(ctx, 90*time.Second)
		resolution, err := adapter.Resolve(providerCtx, providerReq)
		cancel()
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		return normalizeResolution(resolution), name, nil
	}

	if len(issues) == 0 {
		return Resolution{}, "", fmt.Errorf("no enabled provider was available")
	}
	return Resolution{}, "", fmt.Errorf("all providers failed: %s", strings.Join(issues, " | "))
}

func providerOrder(cfg config.Config, preferredProvider string) []string {
	seen := map[string]struct{}{}
	order := make([]string, 0, len(cfg.Providers))

	add := func(name string) {
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" || name == "auto" {
			return
		}
		if _, ok := cfg.Providers[name]; !ok {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		order = append(order, name)
	}

	add(preferredProvider)
	add(cfg.Provider)
	add("codex")
	add("claude")
	add("ew")

	names := cfg.ProviderNames()
	sort.Strings(names)
	for _, name := range names {
		add(name)
	}
	return order
}

func resolveModel(providerCfg config.ProviderConfig, requested string) string {
	model := strings.TrimSpace(requested)
	explicitRequested := model != ""
	if model == "" {
		model = strings.TrimSpace(providerCfg.Model)
	}
	switch model {
	case "auto-fast":
		model = pickModelAliasBySpeed(providerCfg, []string{"fast", "balanced"})
	case "auto-main":
		model = pickModelAliasBySpeed(providerCfg, []string{"quality", "balanced", "fast"})
	default:
		if strings.HasPrefix(model, "auto-") {
			model = strings.TrimSpace(providerCfg.Model)
		}
	}

	if explicitRequested && providerModelIsUnknown(providerCfg, model) {
		model = strings.TrimSpace(providerCfg.Model)
	}
	if providerModelIsUnknown(providerCfg, model) {
		model = fallbackKnownModel(providerCfg)
	}
	if strings.TrimSpace(model) == "" {
		return ""
	}
	if def, ok := providerCfg.Models[model]; ok {
		if strings.TrimSpace(def.ProviderModel) != "" {
			return strings.TrimSpace(def.ProviderModel)
		}
	}
	return model
}

func fallbackKnownModel(providerCfg config.ProviderConfig) string {
	if len(providerCfg.Models) == 0 {
		return strings.TrimSpace(providerCfg.Model)
	}
	if alias := pickModelAliasBySpeed(providerCfg, []string{"quality", "balanced", "fast"}); alias != "" {
		if !providerModelIsUnknown(providerCfg, alias) {
			return alias
		}
	}
	aliases := make([]string, 0, len(providerCfg.Models))
	for alias := range providerCfg.Models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	if len(aliases) == 0 {
		return ""
	}
	return aliases[0]
}

func providerModelIsUnknown(providerCfg config.ProviderConfig, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if len(providerCfg.Models) == 0 {
		return false
	}
	if _, ok := providerCfg.Models[model]; ok {
		return false
	}
	for _, details := range providerCfg.Models {
		if strings.EqualFold(strings.TrimSpace(details.ProviderModel), model) {
			return false
		}
	}
	return true
}

func pickModelAliasBySpeed(providerCfg config.ProviderConfig, speedOrder []string) string {
	models := providerCfg.Models
	if len(models) == 0 {
		return strings.TrimSpace(providerCfg.Model)
	}

	speedRank := map[string]int{}
	for idx, speed := range speedOrder {
		speedRank[strings.ToLower(strings.TrimSpace(speed))] = idx
	}

	aliases := make([]string, 0, len(models))
	for alias := range models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	bestAlias := ""
	bestRank := len(speedOrder) + 1
	for _, alias := range aliases {
		model := models[alias]
		rank, ok := speedRank[strings.ToLower(strings.TrimSpace(model.Speed))]
		if !ok {
			continue
		}
		if rank < bestRank {
			bestAlias = alias
			bestRank = rank
		}
	}
	if bestAlias != "" {
		return bestAlias
	}

	return strings.TrimSpace(providerCfg.Model)
}

func resolveThinking(providerName string, providerCfg config.ProviderConfig, resolvedModel string, requested string) string {
	thinking := strings.TrimSpace(requested)
	if thinking == "" {
		thinking = strings.TrimSpace(providerCfg.Thinking)
	}
	if thinking == "" {
		thinking = "medium"
	}

	for alias, details := range providerCfg.Models {
		if alias == resolvedModel || strings.EqualFold(details.ProviderModel, resolvedModel) {
			if strings.TrimSpace(requested) == "" && strings.TrimSpace(details.Thinking) != "" {
				return normalizeThinkingForProvider(providerName, strings.TrimSpace(details.Thinking))
			}
		}
	}
	return normalizeThinkingForProvider(providerName, thinking)
}

func cloneContext(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func permissionModeFor(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "yolo":
		return "bypassPermissions"
	case "suggest":
		return "plan"
	default:
		return "default"
	}
}

func normalizeThinkingForProvider(providerName, level string) string {
	normalized := strings.ToLower(strings.TrimSpace(level))
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "claude":
		switch normalized {
		case "off", "minimal", "low":
			return "low"
		case "medium":
			return "medium"
		case "high":
			return "high"
		case "xhigh", "max":
			return "max"
		default:
			return "medium"
		}
	case "codex":
		switch normalized {
		case "off", "minimal", "low":
			return "low"
		case "medium":
			return "medium"
		case "high", "xhigh", "max":
			return "high"
		default:
			return "medium"
		}
	default:
		if normalized == "" {
			return "medium"
		}
		return normalized
	}
}
