package provider

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/ashwch/ew/internal/config"
	ewrt "github.com/ashwch/ew/internal/runtime"
)

var requestPattern = regexp.MustCompile(`(?is)request:\s*"([^"]+)"`)
var failedCommandPattern = regexp.MustCompile(`(?is)failed command:\s*"([^"]+)"`)

//go:embed builtin_rules.json
var builtinRulesJSON []byte

var builtinRulesOnce sync.Once
var builtinRulesCache []builtinRule
var builtinRulesErr error

type builtinRule struct {
	ID                string   `json:"id"`
	Intent            string   `json:"intent"`
	MatchAny          []string `json:"match_any"`
	MatchAll          []string `json:"match_all"`
	Action            string   `json:"action"`
	Command           string   `json:"command"`
	Reason            string   `json:"reason"`
	Risk              string   `json:"risk"`
	Confidence        float64  `json:"confidence"`
	NeedsConfirmation bool     `json:"needs_confirmation"`
}

type BuiltinAdapter struct {
	name string
}

func NewBuiltinAdapter(name string, _ config.ProviderConfig) (Adapter, error) {
	if strings.TrimSpace(name) == "" {
		name = "ew"
	}
	return &BuiltinAdapter{name: name}, nil
}

func (a *BuiltinAdapter) Name() string {
	return a.name
}

func (a *BuiltinAdapter) Type() string {
	return "builtin"
}

func (a *BuiltinAdapter) BuildInvocation(_ Request) ([]string, error) {
	return nil, fmt.Errorf("builtin adapter has no external invocation")
}

func (a *BuiltinAdapter) Resolve(_ context.Context, req Request) (Resolution, error) {
	switch req.Intent {
	case IntentFix:
		return a.resolveFix(req)
	case IntentFind:
		return a.resolveFind(req)
	default:
		return Resolution{}, fmt.Errorf("unsupported builtin intent: %s", req.Intent)
	}
}

func (a *BuiltinAdapter) resolveFix(req Request) (Resolution, error) {
	command := extractCapture(req.Prompt, failedCommandPattern)
	if command == "" {
		return Resolution{}, fmt.Errorf("builtin provider: no failed command to fix")
	}

	if suggested, reason := ewrt.SuggestFix(command); suggested != "" {
		return Resolution{
			Action:            "run",
			Command:           suggested,
			Reason:            reason,
			Risk:              "low",
			Confidence:        0.98,
			NeedsConfirmation: true,
		}, nil
	}

	return Resolution{}, fmt.Errorf("builtin provider: no deterministic fix for %q", command)
}

func (a *BuiltinAdapter) resolveFind(req Request) (Resolution, error) {
	query := extractCapture(req.Prompt, requestPattern)
	if query == "" {
		query = req.Prompt
	}
	low := strings.ToLower(strings.TrimSpace(query))

	rules, err := loadBuiltinRules()
	if err != nil {
		return Resolution{}, err
	}
	for _, rule := range rules {
		if rule.Intent != string(IntentFind) {
			continue
		}
		if !builtinRuleMatchesQuery(rule, low) {
			continue
		}
		return Resolution{
			Action:            rule.Action,
			Command:           rule.Command,
			Reason:            rule.Reason,
			Risk:              rule.Risk,
			Confidence:        rule.Confidence,
			NeedsConfirmation: rule.NeedsConfirmation,
		}, nil
	}
	return Resolution{}, fmt.Errorf("builtin provider: no deterministic command for query")
}

func extractCapture(input string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(input)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func loadBuiltinRules() ([]builtinRule, error) {
	builtinRulesOnce.Do(func() {
		rules, err := parseBuiltinRules(builtinRulesJSON)
		if err != nil {
			builtinRulesErr = err
			return
		}

		overridePath := strings.TrimSpace(os.Getenv("EW_BUILTIN_RULES_FILE"))
		if overridePath != "" {
			overrideBytes, err := os.ReadFile(overridePath)
			if err != nil {
				builtinRulesErr = fmt.Errorf("builtin provider: could not read EW_BUILTIN_RULES_FILE: %w", err)
				return
			}
			overrideRules, err := parseBuiltinRules(overrideBytes)
			if err != nil {
				builtinRulesErr = fmt.Errorf("builtin provider: invalid EW_BUILTIN_RULES_FILE: %w", err)
				return
			}
			rules = append(rules, overrideRules...)
		}

		builtinRulesCache = rules
	})

	if builtinRulesErr != nil {
		return nil, builtinRulesErr
	}
	return builtinRulesCache, nil
}

func parseBuiltinRules(payload []byte) ([]builtinRule, error) {
	var rules []builtinRule
	if err := json.Unmarshal(payload, &rules); err != nil {
		return nil, fmt.Errorf("could not parse builtin rules JSON: %w", err)
	}
	out := make([]builtinRule, 0, len(rules))
	for _, rule := range rules {
		normalized, err := normalizeBuiltinRule(rule)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeBuiltinRule(in builtinRule) (builtinRule, error) {
	rule := in
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Intent = strings.ToLower(strings.TrimSpace(rule.Intent))
	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	rule.Command = strings.TrimSpace(rule.Command)
	rule.Reason = strings.TrimSpace(rule.Reason)
	rule.Risk = strings.ToLower(strings.TrimSpace(rule.Risk))

	if rule.ID == "" {
		return builtinRule{}, fmt.Errorf("builtin rule missing id")
	}
	if rule.Intent == "" {
		return builtinRule{}, fmt.Errorf("builtin rule %q missing intent", rule.ID)
	}
	if rule.Action == "" {
		rule.Action = "run"
	}
	if rule.Command == "" {
		return builtinRule{}, fmt.Errorf("builtin rule %q missing command", rule.ID)
	}
	if rule.Reason == "" {
		rule.Reason = "builtin rule match"
	}
	if rule.Risk == "" {
		rule.Risk = "low"
	}
	if rule.Confidence <= 0 || rule.Confidence > 1 {
		rule.Confidence = 0.95
	}

	rule.MatchAny = normalizePatternList(rule.MatchAny)
	rule.MatchAll = normalizePatternList(rule.MatchAll)
	if len(rule.MatchAny) == 0 && len(rule.MatchAll) == 0 {
		return builtinRule{}, fmt.Errorf("builtin rule %q has no match patterns", rule.ID)
	}

	return rule, nil
}

func normalizePatternList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func builtinRuleMatchesQuery(rule builtinRule, queryLower string) bool {
	if len(rule.MatchAny) > 0 {
		anyMatched := false
		for _, pattern := range rule.MatchAny {
			if strings.Contains(queryLower, pattern) {
				anyMatched = true
				break
			}
		}
		if !anyMatched {
			return false
		}
	}
	for _, pattern := range rule.MatchAll {
		if !strings.Contains(queryLower, pattern) {
			return false
		}
	}
	return true
}
