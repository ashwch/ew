package provider

import (
	"context"
	"fmt"

	"github.com/ashwch/ew/internal/config"
)

type Intent string

const (
	IntentFix  Intent = "fix"
	IntentFind Intent = "find"
)

type Request struct {
	Intent   Intent
	Prompt   string
	Mode     string
	Model    string
	Thinking string
	Context  map[string]any
}

type Resolution struct {
	Action            string  `json:"action"`
	Command           string  `json:"command,omitempty"`
	Reason            string  `json:"reason"`
	Risk              string  `json:"risk"`
	Confidence        float64 `json:"confidence"`
	NeedsConfirmation bool    `json:"needs_confirmation"`
}

type Adapter interface {
	Name() string
	Type() string
	Resolve(ctx context.Context, req Request) (Resolution, error)
	BuildInvocation(req Request) ([]string, error)
}

type HealthChecker interface {
	HealthCheck() error
}

type Factory func(name string, cfg config.ProviderConfig) (Adapter, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	r := &Registry{factories: map[string]Factory{}}
	r.Register("command", NewCommandAdapter)
	r.Register("builtin", NewBuiltinAdapter)
	return r
}

func (r *Registry) Register(providerType string, factory Factory) {
	if r.factories == nil {
		r.factories = map[string]Factory{}
	}
	r.factories[providerType] = factory
}

func (r *Registry) Build(name string, cfg config.ProviderConfig) (Adapter, error) {
	providerType := cfg.Type
	if providerType == "" {
		providerType = "command"
	}
	factory, ok := r.factories[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
	return factory(name, cfg)
}

func (r *Registry) Validate(cfg config.Config) []error {
	issues := []error{}
	for name, providerCfg := range cfg.Providers {
		if providerCfg.Enabled != nil && !*providerCfg.Enabled {
			continue
		}
		adapter, err := r.Build(name, providerCfg)
		if err != nil {
			issues = append(issues, fmt.Errorf("provider %q invalid: %w", name, err))
			continue
		}
		if checker, ok := adapter.(HealthChecker); ok {
			if err := checker.HealthCheck(); err != nil {
				issues = append(issues, fmt.Errorf("provider %q health check failed: %w", name, err))
			}
		}
	}
	return issues
}
