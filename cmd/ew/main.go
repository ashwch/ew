package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ashwch/ew/internal/appdirs"
	"github.com/ashwch/ew/internal/config"
	"github.com/ashwch/ew/internal/history"
	"github.com/ashwch/ew/internal/hook"
	"github.com/ashwch/ew/internal/i18n"
	"github.com/ashwch/ew/internal/knowledge"
	"github.com/ashwch/ew/internal/memory"
	"github.com/ashwch/ew/internal/provider"
	"github.com/ashwch/ew/internal/router"
	ewrt "github.com/ashwch/ew/internal/runtime"
	"github.com/ashwch/ew/internal/safety"
	"github.com/ashwch/ew/internal/systemprofile"
	"github.com/ashwch/ew/internal/ui"
)

var version = "dev"

const maxFixFailureAge = 60 * time.Minute
const maxInferredHistoryAge = 90 * time.Second

var localeCatalog = i18n.LoadCatalog("")
var runtimeSystemContext = ""

type options struct {
	Model      string
	Thinking   string
	Provider   string
	Locale     string
	Mode       string
	UI         string
	Intent     string
	Save       bool
	Yes        bool
	JSON       bool
	DryRun     bool
	Offline    bool
	Version    bool
	Copy       bool
	Quiet      bool
	Execute    bool
	ShowConfig bool
	Doctor     bool
	SetupHooks bool
}

type response struct {
	Intent      string      `json:"intent"`
	Message     string      `json:"message,omitempty"`
	Command     string      `json:"command,omitempty"`
	Results     interface{} `json:"results,omitempty"`
	Risk        string      `json:"risk,omitempty"`
	Executed    bool        `json:"executed,omitempty"`
	ConfigPath  string      `json:"config_path,omitempty"`
	Suggestions []string    `json:"suggestions,omitempty"`
}

type selfPromptActionKind string

const (
	selfActionNone       selfPromptActionKind = ""
	selfActionConfigShow selfPromptActionKind = "config_show"
	selfActionSetupHooks selfPromptActionKind = "setup_hooks"
	selfActionDiagnose   selfPromptActionKind = "diagnose"
	selfActionConfigSet  selfPromptActionKind = "config_set"
)

type selfPromptAction struct {
	Kind    selfPromptActionKind
	Changes map[string]string
	Persist bool
}

type memoryPromptActionKind string

const (
	memoryActionNone   memoryPromptActionKind = ""
	memoryActionShow   memoryPromptActionKind = "show"
	memoryActionSave   memoryPromptActionKind = "remember"
	memoryActionForget memoryPromptActionKind = "forget"
	memoryActionBoost  memoryPromptActionKind = "promote"
	memoryActionDrop   memoryPromptActionKind = "demote"
)

type memoryPromptAction struct {
	Kind    memoryPromptActionKind
	Query   string
	Command string
}

type executionOutcome struct {
	Command  string
	Executed bool
	Success  bool
}

func main() {
	opts, prompt, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if opts.Version || isVersionPrompt(prompt) {
		fmt.Println(version)
		return
	}

	cfg, cfgPath, err := config.LoadOrCreate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ew: could not load config: %v\n", err)
		os.Exit(1)
	}

	changes := map[string]string{}
	trimmedPrompt := strings.TrimSpace(prompt)
	targetIntent := flagOverrideIntent(trimmedPrompt, opts.Execute)
	mergeFlagOverrides(opts, changes, targetIntent)

	if len(changes) > 0 {
		for key, value := range changes {
			if err := cfg.Set(key, value); err != nil {
				fmt.Fprintf(os.Stderr, "ew: invalid config change %s=%s: %v\n", key, value, err)
				os.Exit(1)
			}
		}
	}

	persist := opts.Save
	if persist && len(changes) > 0 {
		if err := config.Save(cfgPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "ew: could not save config: %v\n", err)
			os.Exit(1)
		}
	}

	applyRuntimeLocale(cfg, opts)
	initializeSystemProfileContext(&cfg, cfgPath, opts)

	if opts.ShowConfig {
		handleConfigShow(cfg, cfgPath, opts)
		return
	}
	if opts.Doctor {
		handleDiagnose(cfg, opts)
		return
	}
	if opts.SetupHooks {
		handleSetupHooks(opts)
		return
	}

	if len(changes) > 0 && opts.Save && trimmedPrompt == "" {
		handleConfigSet(cfgPath, changes, opts)
		return
	}

	prompt = trimmedPrompt
	if prompt == "" {
		if opts.Execute {
			payload := response{Intent: string(router.IntentRun), Message: "add a query to execute, e.g. ew --execute clear aws vault"}
			printResponse(payload, opts.JSON)
			return
		}
		handleFix("", cfg, opts)
		return
	}
	if !opts.Execute {
		if handled := maybeHandleMemoryPrompt(prompt, opts); handled {
			return
		}
		if handled := maybeHandleSelfAwarePrompt(prompt, cfg, cfgPath, opts); handled {
			return
		}
	}
	if !opts.Execute && isFixPrompt(prompt) {
		handleFix(prompt, cfg, opts)
		return
	}
	if opts.Execute {
		handleRun(prompt, cfg, opts)
		return
	}
	handleFind(prompt, cfg, opts)
}

func parseArgs(args []string) (options, string, error) {
	fs := flag.NewFlagSet("ew", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts options
	fs.StringVar(&opts.Model, "model", "", "override model for this invocation")
	fs.StringVar(&opts.Thinking, "thinking", "", "override thinking level")
	fs.StringVar(&opts.Provider, "provider", "", "override provider: auto|codex|claude")
	fs.StringVar(&opts.Locale, "locale", "", "override locale: auto|en|en-US|hi|hi-IN")
	fs.StringVar(&opts.Mode, "mode", "", "override mode: suggest|confirm|yolo")
	fs.StringVar(&opts.UI, "ui", "", "override ui backend: auto|bubbletea|huh|tview|plain")
	fs.StringVar(&opts.Intent, "intent", "", "target config for --model/--thinking: fix|find")
	fs.BoolVar(&opts.Save, "save", false, "persist overrides")
	fs.BoolVar(&opts.Yes, "yes", false, "auto-confirm execution prompts")
	fs.BoolVar(&opts.JSON, "json", false, "output JSON")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "do not execute commands")
	fs.BoolVar(&opts.Offline, "offline", false, "skip AI provider fallback")
	fs.BoolVar(&opts.Version, "version", false, "print version")
	fs.BoolVar(&opts.Copy, "copy", false, "copy suggested command to clipboard when possible")
	fs.BoolVar(&opts.Quiet, "quiet", false, "print only the suggested command")
	fs.BoolVar(&opts.Execute, "execute", false, "execute selected command instead of only suggesting")
	fs.BoolVar(&opts.ShowConfig, "show-config", false, "show effective settings and exit")
	fs.BoolVar(&opts.Doctor, "doctor", false, "run diagnostic checks and exit")
	fs.BoolVar(&opts.SetupHooks, "setup-hooks", false, "print shell hook snippet and exit")

	if err := fs.Parse(args); err != nil {
		return options{}, "", err
	}
	opts.Intent = strings.ToLower(strings.TrimSpace(opts.Intent))
	if opts.Intent != "" && opts.Intent != "fix" && opts.Intent != "find" {
		return options{}, "", fmt.Errorf("--intent must be one of: fix, find")
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	return opts, prompt, nil
}

func isVersionPrompt(prompt string) bool {
	switch strings.ToLower(strings.TrimSpace(prompt)) {
	case "version", "--version", "-v":
		return true
	default:
		return false
	}
}

func isFixPrompt(prompt string) bool {
	low := strings.ToLower(strings.TrimSpace(prompt))
	if low == "" {
		return false
	}
	if strings.HasPrefix(low, "fix ") || strings.HasPrefix(low, "fix:") {
		return true
	}
	if strings.Contains(low, "last failed") || strings.Contains(low, "failed command") {
		return true
	}
	return false
}

func mergeFlagOverrides(opts options, changes map[string]string, intent router.Intent) {
	target := "fix"
	if intent == router.IntentFind || intent == router.IntentRun {
		target = "find"
	}
	if opts.Intent == "fix" || opts.Intent == "find" {
		target = opts.Intent
	}

	if strings.TrimSpace(opts.Provider) != "" {
		changes["provider"] = strings.TrimSpace(opts.Provider)
	}
	if strings.TrimSpace(opts.Locale) != "" {
		changes["locale"] = strings.TrimSpace(opts.Locale)
	}
	if strings.TrimSpace(opts.Mode) != "" {
		changes["mode"] = strings.TrimSpace(opts.Mode)
	}
	if strings.TrimSpace(opts.UI) != "" {
		changes["ui.backend"] = strings.TrimSpace(opts.UI)
	}
	if strings.TrimSpace(opts.Model) != "" {
		changes[target+".model"] = strings.TrimSpace(opts.Model)
	}
	if strings.TrimSpace(opts.Thinking) != "" {
		changes[target+".thinking"] = strings.TrimSpace(opts.Thinking)
	}
}

func flagOverrideIntent(prompt string, execute bool) router.Intent {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return router.IntentFix
	}
	if !execute && isFixPrompt(trimmedPrompt) {
		return router.IntentFix
	}
	return router.IntentFind
}

func applyRuntimeLocale(cfg config.Config, opts options) {
	locale := strings.TrimSpace(opts.Locale)
	if locale == "" {
		locale = strings.TrimSpace(cfg.Locale)
	}
	if strings.EqualFold(locale, "auto") {
		locale = ""
	}
	localeCatalog = i18n.LoadCatalog(locale)
}

func initializeSystemProfileContext(cfg *config.Config, cfgPath string, opts options) {
	runtimeSystemContext = ""
	if cfg == nil {
		return
	}

	options := systemprofile.Options{
		AutoTrain:    cfg.System.AutoTrain,
		RefreshHours: cfg.System.RefreshHours,
	}

	var (
		profile systemprofile.Profile
		status  systemprofile.Status
		err     error
	)
	withEWLoader(opts, "learning your system", func() {
		profile, status, err = systemprofile.Ensure(options)
	})
	if err != nil {
		if !opts.JSON {
			fmt.Fprintf(os.Stderr, "ew: system training skipped: %v\n", err)
		}
		return
	}

	if status.Created {
		confirmFirstRunSystemProfile(cfg, cfgPath, &profile, opts)
	}

	if !cfg.System.EnableContext {
		return
	}
	runtimeSystemContext = profile.PromptContext(cfg.System.MaxPromptItems)
}

func confirmFirstRunSystemProfile(cfg *config.Config, cfgPath string, profile *systemprofile.Profile, opts options) {
	if cfg == nil || profile == nil {
		return
	}
	if opts.JSON || opts.Quiet {
		return
	}
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return
	}

	summary := strings.TrimSpace(profile.HumanSummary(cfg.System.MaxPromptItems))
	if summary == "" {
		return
	}

	backend := effectiveUIBackend(*cfg, opts)
	decision, used, err := ui.SystemProfileOnboarding(backend, summary, profile.UserNote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ew: onboarding ui failed (%v); falling back to plain prompt\n", err)
	}
	if !used {
		var ok bool
		decision, ok = confirmFirstRunSystemProfilePlain(summary)
		if !ok {
			return
		}
	}
	applySystemProfileDecision(cfg, cfgPath, profile, decision)
}

func confirmFirstRunSystemProfilePlain(summary string) (ui.SystemProfileDecision, bool) {
	fmt.Println("ew initialized system context from your machine:")
	fmt.Println(summary)
	fmt.Print("Use this context? [Y]es / [N]o / [E]dit note: ")

	reader := bufio.NewReader(os.Stdin)
	choice, err := reader.ReadString('\n')
	if err != nil {
		return ui.SystemProfileDecision{}, false
	}
	choice = strings.ToLower(strings.TrimSpace(choice))
	switch choice {
	case "", "y", "yes":
		return ui.SystemProfileDecision{}, true
	case "n", "no":
		return ui.SystemProfileDecision{DisableContext: true}, true
	case "e", "edit":
		fmt.Print("Add a short correction note (optional): ")
		note, err := reader.ReadString('\n')
		if err != nil {
			return ui.SystemProfileDecision{}, false
		}
		return ui.SystemProfileDecision{
			SetUserNote: true,
			UserNote:    strings.TrimSpace(note),
		}, true
	default:
		return ui.SystemProfileDecision{}, false
	}
}

func applySystemProfileDecision(cfg *config.Config, cfgPath string, profile *systemprofile.Profile, decision ui.SystemProfileDecision) {
	if cfg == nil || profile == nil {
		return
	}

	if decision.DisableContext {
		cfg.System.EnableContext = false
		if err := config.Save(cfgPath, *cfg); err != nil {
			fmt.Fprintf(os.Stderr, "ew: could not save system context preference: %v\n", err)
			return
		}
		fmt.Println("System context disabled.")
		return
	}

	if decision.SetUserNote {
		profile.UserNote = strings.TrimSpace(decision.UserNote)
		if err := systemprofile.Save(*profile); err != nil {
			fmt.Fprintf(os.Stderr, "ew: could not save system note: %v\n", err)
			return
		}
		if profile.UserNote == "" {
			fmt.Println("Cleared system note.")
			return
		}
		fmt.Println("Saved system note.")
	}
}

func maybeHandleSelfAwarePrompt(prompt string, cfg config.Config, cfgPath string, opts options) bool {
	action, ok := parseSelfPromptAction(prompt)
	if !ok || action.Kind == selfActionNone {
		return false
	}

	switch action.Kind {
	case selfActionConfigShow:
		handleConfigShow(cfg, cfgPath, opts)
		return true
	case selfActionSetupHooks:
		handleSetupHooks(opts)
		return true
	case selfActionDiagnose:
		handleDiagnose(cfg, opts)
		return true
	case selfActionConfigSet:
		if len(action.Changes) == 0 {
			return false
		}
		if !action.Persist {
			suggestions := sortedChangeSuggestions(action.Changes)
			suggestions = append(suggestions, "add 'save' (or 'persist'/'remember'/'default') in your prompt to persist these changes")
			payload := response{
				Intent:      string(router.IntentConfigSet),
				Message:     "parsed self-config request",
				Suggestions: suggestions,
			}
			printResponse(payload, opts.JSON)
			return true
		}
		for key, value := range action.Changes {
			if err := cfg.Set(key, value); err != nil {
				payload := response{
					Intent:      string(router.IntentConfigSet),
					Message:     fmt.Sprintf("invalid self-config change %s=%s: %v", key, value, err),
					Suggestions: sortedChangeSuggestions(action.Changes),
				}
				printResponse(payload, opts.JSON)
				return true
			}
		}
		if err := config.Save(cfgPath, cfg); err != nil {
			payload := response{
				Intent:      string(router.IntentConfigSet),
				Message:     fmt.Sprintf("could not save self-config changes: %v", err),
				Suggestions: sortedChangeSuggestions(action.Changes),
			}
			printResponse(payload, opts.JSON)
			return true
		}
		handleConfigSet(cfgPath, action.Changes, opts)
		return true
	default:
		return false
	}
}

func parseSelfPromptAction(prompt string) (selfPromptAction, bool) {
	low := strings.ToLower(strings.TrimSpace(prompt))
	if low == "" {
		return selfPromptAction{}, false
	}
	catalog := localeCatalog
	questionLike := isQuestionLikePrompt(low, catalog.Self.Question)
	selfReferenced := promptHasSelfReference(low)
	implicitConfigAllowed := selfReferenced || !looksLikeExternalScopedPrompt(low)

	switch {
	case matchesSelfUtilityPrompt(low, catalog.Self.ShowConfig, selfReferenced):
		return selfPromptAction{Kind: selfActionConfigShow}, true
	case matchesSelfUtilityPrompt(low, catalog.Self.SetupHooks, selfReferenced):
		return selfPromptAction{Kind: selfActionSetupHooks}, true
	case matchesSelfUtilityPrompt(low, catalog.Self.Diagnose, selfReferenced):
		return selfPromptAction{Kind: selfActionDiagnose}, true
	}
	if !implicitConfigAllowed {
		return selfPromptAction{}, false
	}

	changes := map[string]string{}
	tokens := promptTokenSet(low)
	providers := []string{"auto", "codex", "claude", "ew", "openrouter"}
	modes := []string{"suggest", "confirm", "yolo"}
	uiBackends := []string{"auto", "bubbletea", "huh", "tview", "plain"}

	if containsAny(low, catalog.Self.Provider...) {
		if providerName := firstTokenMatch(tokens, providers); providerName != "" {
			changes["provider"] = providerName
		}
	}
	if containsAny(low, catalog.Self.Mode...) {
		if modeName := firstTokenMatch(tokens, modes); modeName != "" {
			changes["mode"] = modeName
		}
	}
	if selfReferenced &&
		strings.Contains(low, "suggest") &&
		containsAny(low, "execute", "execution") &&
		containsAny(low, "allow", "enable", "disable", "block", "turn on", "turn off") {
		switch {
		case containsAny(low, "disable", "block", "turn off", "dont", "do not"):
			changes["ai.allow_suggest_execution"] = "false"
		case containsAny(low, "enable", "allow", "turn on"):
			changes["ai.allow_suggest_execution"] = "true"
		}
	}
	if containsAny(low, catalog.Self.UI...) {
		if backend := firstTokenMatch(tokens, uiBackends); backend != "" {
			changes["ui.backend"] = backend
		} else if containsAny(low, catalog.Self.UIUpgrade...) {
			// Opinionated default for vague UI upgrade asks.
			changes["ui.backend"] = "bubbletea"
		}
	}
	if containsAny(low, "locale", "language", "lang", "भाषा") {
		if locale := extractPromptLocaleChoice(low); locale != "" {
			changes["locale"] = locale
		}
	}
	if containsAny(low, "system context", "system profile", "machine context", "machine profile", "ew context") {
		switch {
		case containsAny(low, "disable", "turn off", "off", "dont use", "do not use"):
			changes["system.enable_context"] = "false"
		case containsAny(low, "enable", "turn on", "on", "use"):
			changes["system.enable_context"] = "true"
		}
	}
	if containsAny(low, "auto train", "auto-train", "autotrain", "auto training") && containsAny(low, "system", "context", "profile") {
		switch {
		case containsAny(low, "disable", "turn off", "off", "stop"):
			changes["system.auto_train"] = "false"
		case containsAny(low, "enable", "turn on", "on", "start"):
			changes["system.auto_train"] = "true"
		}
	}
	if containsAny(low, "system", "context", "profile") {
		if refresh := extractPromptRefreshHours(low); refresh > 0 {
			changes["system.refresh_hours"] = fmt.Sprintf("%d", refresh)
		}
	}

	intentTarget := "fix"
	if strings.Contains(low, " for find") || strings.Contains(low, "find model") || strings.Contains(low, "find thinking") {
		intentTarget = "find"
	}
	if model := extractPromptModel(low); model != "" {
		changes[intentTarget+".model"] = model
	}
	if thinking := extractPromptThinking(low); thinking != "" {
		changes[intentTarget+".thinking"] = thinking
	}

	if len(changes) == 0 {
		return selfPromptAction{}, false
	}
	persist := containsAny(low, catalog.Self.Persist...)
	if !persist && !questionLike && containsAny(low, catalog.Self.Imperative...) {
		persist = true
	}
	return selfPromptAction{
		Kind:    selfActionConfigSet,
		Changes: changes,
		Persist: persist,
	}, true
}

func promptHasSelfReference(low string) bool {
	tokens := promptTokenSet(low)
	if _, ok := tokens["ew"]; ok {
		return true
	}
	return containsAny(
		low,
		"this tool",
		"this cli",
		"your config",
		"your settings",
		"ew config",
		"ew settings",
		"for ew",
		"of ew",
		"ew itself",
		"about ew",
		"ew ui",
		"ew mode",
		"ew provider",
		"ew locale",
		"ew language",
	)
}

func looksLikeExternalScopedPrompt(low string) bool {
	trimmed := strings.TrimSpace(low)
	if !containsAny(trimmed, " for ", " in ", " on ") {
		return false
	}
	if containsAny(
		trimmed,
		" for ew",
		" for this tool",
		" for this cli",
		" for fix",
		" for find",
		" for me",
		" for system profile",
		" for machine profile",
		" for system context",
		" for machine context",
		" in ew",
		" in this tool",
		" in this cli",
		" in fix",
		" in find",
		" on ew",
		" on this tool",
		" on this cli",
	) {
		return false
	}
	return true
}

func matchesSelfUtilityPrompt(low string, patterns []string, selfReferenced bool) bool {
	trimmed := strings.TrimSpace(low)
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if trimmed == pattern || trimmed == "ew "+pattern {
			return true
		}
	}
	if !selfReferenced {
		return false
	}
	return containsAny(trimmed, patterns...)
}

func sortedChangeSuggestions(changes map[string]string) []string {
	keys := make([]string, 0, len(changes))
	for key := range changes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	suggestions := make([]string, 0, len(keys))
	for _, key := range keys {
		suggestions = append(suggestions, fmt.Sprintf("%s=%s", key, changes[key]))
	}
	return suggestions
}

func containsAny(low string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(low, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func promptTokenSet(low string) map[string]struct{} {
	parts := strings.FieldsFunc(low, func(r rune) bool {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= '0' && r <= '9' {
			return false
		}
		switch r {
		case '-', '_', '.':
			return false
		default:
			return true
		}
	})
	tokens := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens[part] = struct{}{}
	}
	return tokens
}

func firstTokenMatch(tokens map[string]struct{}, allowed []string) string {
	for _, candidate := range allowed {
		if _, ok := tokens[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func extractPromptModel(low string) string {
	re := regexp.MustCompile(`\bmodel\s+([a-z0-9._-]+)\b`)
	matches := re.FindStringSubmatch(low)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func extractPromptThinking(low string) string {
	re := regexp.MustCompile(`\bthinking\s+(minimal|low|medium|high)\b`)
	matches := re.FindStringSubmatch(low)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func extractPromptLocaleChoice(low string) string {
	trimmed := strings.TrimSpace(low)
	switch {
	case strings.Contains(trimmed, "hindi"), strings.Contains(trimmed, "हिंदी"), strings.Contains(trimmed, "हिन्दी"):
		return "hi"
	case strings.Contains(trimmed, "english"), strings.Contains(trimmed, "अंग्रेज़ी"), strings.Contains(trimmed, "अंग्रेजी"):
		return "en"
	case strings.Contains(trimmed, "auto locale"), strings.Contains(trimmed, "locale auto"), strings.Contains(trimmed, "language auto"), strings.Contains(trimmed, "auto language"):
		return "auto"
	}

	re := regexp.MustCompile(`(?:locale|language|lang|भाषा)\s+([a-z0-9._-]+)`)
	matches := re.FindStringSubmatch(trimmed)
	if len(matches) >= 2 {
		candidate := strings.TrimSpace(matches[1])
		if strings.EqualFold(candidate, "auto") {
			return "auto"
		}
		if normalized := i18n.NormalizeLocale(candidate); normalized != "" {
			return normalized
		}
	}

	return ""
}

func extractPromptRefreshHours(low string) int {
	re := regexp.MustCompile(`(?i)(?:refresh(?:_hours)?|refresh every|ttl)\s+(\d{1,4})`)
	matches := re.FindStringSubmatch(low)
	if len(matches) < 2 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func isQuestionLikePrompt(low string, patterns []string) bool {
	trimmed := strings.TrimSpace(low)
	if len(patterns) > 0 {
		return containsAny(trimmed, patterns...)
	}
	switch {
	case strings.Contains(trimmed, "?"):
		return true
	case strings.HasPrefix(trimmed, "how "):
		return true
	case strings.HasPrefix(trimmed, "what "):
		return true
	case strings.HasPrefix(trimmed, "which "):
		return true
	case strings.HasPrefix(trimmed, "why "):
		return true
	case strings.HasPrefix(trimmed, "can "):
		return true
	default:
		return false
	}
}

var (
	reMemoryRemember = regexp.MustCompile(`(?i)^(?:remember|learn)\s+(?:that\s+)?(.+?)\s+(?:=>|->|as|means|is)\s+(.+)$`)
	reMemoryPrefer   = regexp.MustCompile(`(?i)^(?:prefer|promote|boost)\s+(.+?)\s+(?:for|when i say)\s+(.+)$`)
	reMemoryDemote   = regexp.MustCompile(`(?i)^(?:demote|downrank|deprioritize)\s+(.+?)\s+(?:for|when i say)\s+(.+)$`)
	reMemoryForget   = regexp.MustCompile(`(?i)^(?:forget|remove)\s+(?:memory|memories)\s+for\s+(.+)$`)
	reMemoryShowFor  = regexp.MustCompile(`(?i)^(?:show|list)\s+(?:memory|memories)(?:\s+for\s+(.+))?$`)
	reDigits         = regexp.MustCompile(`\d+`)
)

func parseMemoryPromptAction(prompt string) (memoryPromptAction, bool) {
	trimmed := strings.TrimSpace(prompt)
	low := strings.ToLower(trimmed)
	if trimmed == "" {
		return memoryPromptAction{}, false
	}

	if matches := reMemoryRemember.FindStringSubmatch(trimmed); len(matches) >= 3 {
		return memoryPromptAction{
			Kind:    memoryActionSave,
			Query:   strings.TrimSpace(matches[1]),
			Command: strings.TrimSpace(matches[2]),
		}, true
	}
	if matches := reMemoryPrefer.FindStringSubmatch(trimmed); len(matches) >= 3 {
		return memoryPromptAction{
			Kind:    memoryActionBoost,
			Query:   strings.TrimSpace(matches[2]),
			Command: strings.TrimSpace(matches[1]),
		}, true
	}
	if matches := reMemoryDemote.FindStringSubmatch(trimmed); len(matches) >= 3 {
		return memoryPromptAction{
			Kind:    memoryActionDrop,
			Query:   strings.TrimSpace(matches[2]),
			Command: strings.TrimSpace(matches[1]),
		}, true
	}
	if matches := reMemoryForget.FindStringSubmatch(trimmed); len(matches) >= 2 {
		return memoryPromptAction{
			Kind:  memoryActionForget,
			Query: strings.TrimSpace(matches[1]),
		}, true
	}
	if matches := reMemoryShowFor.FindStringSubmatch(trimmed); len(matches) >= 1 {
		if containsAny(low, "memory", "memories") {
			query := ""
			if len(matches) >= 2 {
				query = strings.TrimSpace(matches[1])
			}
			return memoryPromptAction{
				Kind:  memoryActionShow,
				Query: query,
			}, true
		}
	}
	if containsAny(low, "what do you remember", "memory for", "show memory", "list memories") {
		query := ""
		if idx := strings.Index(low, "for "); idx >= 0 && idx+4 < len(trimmed) {
			query = strings.TrimSpace(trimmed[idx+4:])
		}
		return memoryPromptAction{
			Kind:  memoryActionShow,
			Query: query,
		}, true
	}

	if containsAny(low, "याद रख", "सीख") {
		if query, command, ok := splitPromptPair(trimmed, []string{" का मतलब ", " मतलब ", " means ", " is ", " => ", " -> "}); ok {
			query = stripLeadingMemoryVerb(query)
			return memoryPromptAction{
				Kind:    memoryActionSave,
				Query:   query,
				Command: command,
			}, true
		}
	}
	if containsAny(low, "याद", "memory") && containsAny(low, "दिख", "show", "list") {
		query := ""
		if idx := strings.Index(low, "के लिए "); idx >= 0 && idx+len("के लिए ") < len(trimmed) {
			query = strings.TrimSpace(trimmed[idx+len("के लिए "):])
		}
		return memoryPromptAction{
			Kind:  memoryActionShow,
			Query: query,
		}, true
	}
	if containsAny(low, "भूल", "हटा") && containsAny(low, "याद", "memory") {
		query := strings.TrimSpace(trimmed)
		query = strings.TrimPrefix(strings.TrimPrefix(query, "memory"), "याद")
		query = strings.TrimPrefix(strings.TrimPrefix(query, "for"), "के लिए")
		query = strings.TrimSpace(query)
		if query == "" {
			query = trimmed
		}
		return memoryPromptAction{
			Kind:  memoryActionForget,
			Query: query,
		}, true
	}

	return memoryPromptAction{}, false
}

func splitPromptPair(input string, separators []string) (string, string, bool) {
	low := strings.ToLower(input)
	for _, separator := range separators {
		sep := strings.ToLower(separator)
		idx := strings.Index(low, sep)
		if idx <= 0 {
			continue
		}
		left := strings.TrimSpace(input[:idx])
		right := strings.TrimSpace(input[idx+len(sep):])
		if left == "" || right == "" {
			continue
		}
		return left, right, true
	}
	return "", "", false
}

func stripLeadingMemoryVerb(query string) string {
	trimmed := strings.TrimSpace(query)
	low := strings.ToLower(trimmed)
	prefixes := []string{
		"remember ",
		"learn ",
		"याद रखो ",
		"याद रख ",
		"सीखो ",
		"सीख ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(low, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return trimmed
}

func maybeHandleMemoryPrompt(prompt string, opts options) bool {
	action, ok := parseMemoryPromptAction(prompt)
	if !ok || action.Kind == memoryActionNone {
		return false
	}

	store, path, err := memory.Load()
	if err != nil {
		payload := response{
			Intent:      string(router.IntentFind),
			Message:     fmt.Sprintf("memory load failed: %v", err),
			Suggestions: []string{"continue with normal search by rephrasing your request"},
		}
		printResponse(payload, opts.JSON)
		return true
	}

	switch action.Kind {
	case memoryActionShow:
		var matches []memory.Match
		if strings.TrimSpace(action.Query) == "" {
			matches = store.Top(8)
		} else {
			matches = store.Search(action.Query, 8)
		}
		if opts.JSON {
			payload := response{
				Intent:  string(router.IntentFind),
				Message: "memory matches",
				Results: matches,
			}
			printResponse(payload, true)
			return true
		}
		if len(matches) == 0 {
			fmt.Println("No memory entries found.")
			return true
		}
		if strings.TrimSpace(action.Query) != "" {
			fmt.Printf("Memory matches for %q:\n", action.Query)
		} else {
			fmt.Println("Top memory entries:")
		}
		for idx, match := range matches {
			fmt.Printf("%d. %s\n", idx+1, match.Command)
			fmt.Printf("   query: %s\n", match.Query)
			fmt.Printf("   score: %.2f | uses: %d\n", match.Score, match.Uses)
		}
		return true

	case memoryActionSave:
		if err := store.Remember(action.Query, action.Command); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory update failed: %v", err),
			}, opts.JSON)
			return true
		}
		if err := memory.Save(path, store); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory save failed: %v", err),
			}, opts.JSON)
			return true
		}
		printResponse(response{
			Intent:      string(router.IntentFind),
			Message:     "saved memory",
			Command:     action.Command,
			Suggestions: []string{fmt.Sprintf("query=%s", action.Query)},
		}, opts.JSON)
		return true

	case memoryActionBoost:
		if err := store.Promote(action.Query, action.Command); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory promote failed: %v", err),
			}, opts.JSON)
			return true
		}
		if err := memory.Save(path, store); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory save failed: %v", err),
			}, opts.JSON)
			return true
		}
		printResponse(response{
			Intent:      string(router.IntentFind),
			Message:     "promoted memory ranking",
			Command:     action.Command,
			Suggestions: []string{fmt.Sprintf("query=%s", action.Query)},
		}, opts.JSON)
		return true

	case memoryActionDrop:
		if err := store.Demote(action.Query, action.Command); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory demote failed: %v", err),
			}, opts.JSON)
			return true
		}
		if err := memory.Save(path, store); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory save failed: %v", err),
			}, opts.JSON)
			return true
		}
		printResponse(response{
			Intent:      string(router.IntentFind),
			Message:     "demoted memory ranking",
			Command:     action.Command,
			Suggestions: []string{fmt.Sprintf("query=%s", action.Query)},
		}, opts.JSON)
		return true

	case memoryActionForget:
		removed := store.ForgetQuery(action.Query)
		if err := memory.Save(path, store); err != nil {
			printResponse(response{
				Intent:  string(router.IntentFind),
				Message: fmt.Sprintf("memory save failed: %v", err),
			}, opts.JSON)
			return true
		}
		msg := "no memory entries removed"
		if removed > 0 {
			msg = fmt.Sprintf("removed %d memory entrie(s)", removed)
		}
		printResponse(response{
			Intent:      string(router.IntentFind),
			Message:     msg,
			Suggestions: []string{fmt.Sprintf("query=%s", action.Query)},
		}, opts.JSON)
		return true

	default:
		return false
	}
}

func handleConfigShow(cfg config.Config, cfgPath string, opts options) {
	payload := response{
		Intent:     string(router.IntentConfigShow),
		Message:    "effective settings",
		Results:    cfg,
		ConfigPath: cfgPath,
	}
	printResponse(payload, opts.JSON)
}

func handleConfigSet(cfgPath string, changes map[string]string, opts options) {
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	suggestions := make([]string, 0, len(keys))
	for _, key := range keys {
		suggestions = append(suggestions, fmt.Sprintf("%s=%s", key, changes[key]))
	}

	payload := response{
		Intent:      string(router.IntentConfigSet),
		Message:     "saved settings",
		ConfigPath:  cfgPath,
		Suggestions: suggestions,
	}
	printResponse(payload, opts.JSON)
}

func handleDiagnose(cfg config.Config, opts options) {
	output, err := runInternal("doctor")
	if err != nil {
		output, err = fallbackDoctorOutput(cfg)
		if err != nil {
			payload := response{Intent: string(router.IntentDiagnose), Message: "doctor check failed", Suggestions: []string{string(output)}}
			if strings.TrimSpace(err.Error()) != "" {
				payload.Suggestions = append(payload.Suggestions, err.Error())
			}
			printResponse(payload, opts.JSON)
			return
		}
	}

	if opts.JSON {
		fmt.Println(string(output))
		return
	}
	fmt.Println("doctor checks:")
	fmt.Println(string(output))
}

func fallbackDoctorOutput(cfg config.Config) ([]byte, error) {
	type check struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Status string `json:"status"`
	}

	cfgPath, err := appdirs.ConfigFilePath()
	if err != nil {
		return nil, err
	}
	statePath, err := appdirs.StateDir()
	if err != nil {
		return nil, err
	}

	checks := []check{
		{Key: "os", Value: goruntime.GOOS, Status: "ok"},
		{Key: "config_path", Value: cfgPath, Status: statusFile(cfgPath)},
		{Key: "state_dir", Value: statePath, Status: statusDir(statePath)},
		{Key: "codex", Value: pathOrMissing("codex"), Status: statusBinary("codex")},
		{Key: "claude", Value: pathOrMissing("claude"), Status: statusBinary("claude")},
	}

	registry := provider.NewRegistry()
	issues := registry.Validate(cfg)
	if len(issues) == 0 {
		checks = append(checks, check{Key: "providers", Value: fmt.Sprintf("%d configured", len(cfg.Providers)), Status: "ok"})
	} else {
		checks = append(checks, check{Key: "providers", Value: fmt.Sprintf("%d issue(s)", len(issues)), Status: "error"})
		for _, issue := range issues {
			checks = append(checks, check{Key: "provider_issue", Value: issue.Error(), Status: "error"})
		}
	}

	names := cfg.ProviderNames()
	sort.Strings(names)
	for _, name := range names {
		providerCfg := cfg.Providers[name]
		status := "ok"
		if providerCfg.Enabled != nil && !*providerCfg.Enabled {
			status = "disabled"
		}
		checks = append(checks, check{
			Key:    "provider." + name,
			Value:  fmt.Sprintf("type=%s command=%s model=%s", providerCfg.Type, providerCfg.Command, providerCfg.Model),
			Status: status,
		})
	}

	return json.MarshalIndent(checks, "", "  ")
}

func statusFile(path string) string {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	return "ok"
}

func statusDir(path string) string {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	return "ok"
}

func statusBinary(name string) string {
	if _, err := exec.LookPath(name); err != nil {
		return "missing"
	}
	return "ok"
}

func pathOrMissing(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "not found"
	}
	return path
}

func handleSetupHooks(opts options) {
	shell := detectShell()
	output, err := runInternal("hook-snippet", "--shell", shell)
	if err != nil {
		if fallback := fallbackHookSnippet(shell); fallback != "" {
			output = []byte(fallback)
		} else {
			payload := response{
				Intent:  string(router.IntentSetupHooks),
				Message: "could not generate hook snippet",
				Suggestions: []string{
					"Build _ew and ensure it is available in PATH",
					"Then run: _ew hook-snippet --shell zsh|bash|fish",
				},
			}
			printResponse(payload, opts.JSON)
			return
		}
	}

	if opts.JSON {
		payload := response{
			Intent:  string(router.IntentSetupHooks),
			Message: "hook snippet generated",
			Results: map[string]string{"shell": shell, "snippet": string(output)},
		}
		printResponse(payload, true)
		return
	}

	fmt.Printf("Add this %s snippet to your shell rc file:\n\n", shell)
	fmt.Println(string(output))
}

func fallbackHookSnippet(shell string) string {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "zsh":
		return `export EW_SESSION_ID=${EW_SESSION_ID:-"$$.$(date +%s)"}
function _ew_preexec() {
  EW_LAST_COMMAND="$1"
}
function _ew_precmd() {
  local exit_code=$?
  if [ -n "$EW_LAST_COMMAND" ]; then
    _ew hook-record --command "$EW_LAST_COMMAND" --exit-code "$exit_code" --cwd "$PWD" --shell "zsh" --session-id "$EW_SESSION_ID" >/dev/null 2>&1
    EW_LAST_COMMAND=""
  fi
}
autoload -Uz add-zsh-hook
add-zsh-hook preexec _ew_preexec
add-zsh-hook precmd _ew_precmd`
	case "bash":
		return `export EW_SESSION_ID=${EW_SESSION_ID:-"$$.$(date +%s)"}
_EW_LAST_HISTCMD="$HISTCMD"
_ew_prompt() {
  local exit_code=$?
  if [ "$HISTCMD" = "$_EW_LAST_HISTCMD" ]; then
    return
  fi
  _EW_LAST_HISTCMD="$HISTCMD"
  local last_command
  last_command=$(fc -ln -1 2>/dev/null)
  if [ -n "$last_command" ]; then
    _ew hook-record --command "$last_command" --exit-code "$exit_code" --cwd "$PWD" --shell "bash" --session-id "$EW_SESSION_ID" >/dev/null 2>&1
  fi
}
case ";$PROMPT_COMMAND;" in
  *";_ew_prompt;"*) ;;
  *) PROMPT_COMMAND="_ew_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac`
	case "fish":
		return `set -q EW_SESSION_ID; or set -gx EW_SESSION_ID "$fish_pid".(date +%s)
function __ew_preexec --on-event fish_preexec
  set -g EW_LAST_COMMAND $argv[1]
end
function __ew_postexec --on-event fish_postexec
  set -l exit_code $status
  if test -n "$EW_LAST_COMMAND"
    _ew hook-record --command "$EW_LAST_COMMAND" --exit-code "$exit_code" --cwd "$PWD" --shell "fish" --session-id "$EW_SESSION_ID" >/dev/null 2>&1
    set -e EW_LAST_COMMAND
  end
end`
	default:
		return ""
	}
}

func handleFind(query string, cfg config.Config, opts options) {
	query = strings.TrimSpace(query)
	if query == "" {
		payload := response{Intent: string(router.IntentFind), Message: "add a query, e.g. ew command to clear aws vault"}
		printResponse(payload, opts.JSON)
		return
	}

	memoryMatches, _ := searchMemoryWithLoader(query, cfg.Find.MaxResults, opts, "checking what you've used before")
	if top, ok := preferredMemoryMatch(query, memoryMatches); ok {
		reason := compactReason(fmt.Sprintf("learned from memory for %q (uses: %d)", top.Query, top.Uses), 120)
		if opts.JSON {
			payload := response{
				Intent:      string(router.IntentFind),
				Message:     "memory match",
				Command:     top.Command,
				Risk:        "low",
				Executed:    false,
				Suggestions: []string{reason},
			}
			printResponse(payload, true)
			return
		}
		printSuggestedCommandBlock(top.Command, reason, "memory", opts)
		return
	}

	matches, err := searchHistoryWithLoader(query, cfg.Find.MaxResults, opts, "scouting your history")
	if err != nil {
		payload := response{Intent: string(router.IntentFind), Message: fmt.Sprintf("search failed: %v", err)}
		printResponse(payload, opts.JSON)
		return
	}
	matches = filterFindMatches(query, matches)
	if len(matches) == 0 {
		if opts.Offline {
			payload := response{Intent: string(router.IntentFind), Message: "no safe matching history entries found"}
			printResponse(payload, opts.JSON)
			return
		}

		prompt := buildFindPrompt(query, nil)
		resolution, providerName, resolveErr := resolveProviderWithLoader(
			context.Background(),
			cfg,
			opts,
			provider.IntentFind,
			prompt,
			"thinking of a command that fits",
		)
		if resolveErr != nil {
			payload := response{
				Intent:  string(router.IntentFind),
				Message: "no local history match and provider fallback failed",
				Suggestions: []string{
					resolveErr.Error(),
				},
			}
			printResponse(payload, opts.JSON)
			return
		}
		if !commandAllowedForQuery(query, resolution.Command) {
			payload := response{
				Intent:  string(router.IntentFind),
				Message: "no safe suggestion found for this query",
			}
			if opts.JSON {
				payload.Suggestions = []string{
					"provider suggestion was filtered as destructive for a non-destructive query",
				}
			}
			printResponse(payload, opts.JSON)
			return
		}
		if !opts.JSON {
			printSuggestedCommandBlock(resolution.Command, compactReason(resolution.Reason, 120), providerName, opts)
			persistFindSuggestionMemory(query, resolution.Command, providerName, resolution.Risk)
			return
		}

		payload := response{
			Intent:  string(router.IntentFind),
			Message: providerFallbackMessage(resolution.Action, providerName),
			Command: resolution.Command,
			Risk:    resolution.Risk,
			Suggestions: []string{
				resolution.Reason,
			},
		}
		printResponse(payload, opts.JSON)
		persistFindSuggestionMemory(query, resolution.Command, providerName, resolution.Risk)
		return
	}

	if opts.JSON {
		payload := response{Intent: string(router.IntentFind), Message: "top history matches", Results: matches}
		printResponse(payload, true)
		return
	}

	aiCommand := ""
	aiReason := ""
	aiSource := ""
	aiRisk := ""
	if len(memoryMatches) > 0 {
		top := memoryMatches[0]
		if commandAllowedForQuery(query, top.Command) && memoryQueryCompatible(query, top.Query) {
			aiCommand = strings.TrimSpace(top.Command)
			aiReason = fmt.Sprintf("learned from memory for %q (uses: %d)", top.Query, top.Uses)
			aiSource = "memory"
			aiRisk = "low"
		}
	}
	if shouldAIRerank(cfg.Find.AIRerank, matches) && !opts.Offline {
		prompt := buildFindPrompt(query, matches)
		if resolution, providerName, err := resolveProviderWithLoader(
			context.Background(),
			cfg,
			opts,
			provider.IntentFind,
			prompt,
			"ranking the best command",
		); err == nil && strings.TrimSpace(resolution.Command) != "" {
			if commandAllowedForQuery(query, resolution.Command) {
				aiCommand = strings.TrimSpace(resolution.Command)
				aiReason = strings.TrimSpace(resolution.Reason)
				aiSource = providerName
				aiRisk = strings.TrimSpace(resolution.Risk)
				if aiReason == "" {
					aiReason = fmt.Sprintf("suggested by %s", providerName)
				}
			}
		}
	}
	aiReason = compactReason(aiReason, 120)

	if lowSignalFindQuery(query) && aiCommand != "" {
		printSuggestedCommandBlock(aiCommand, aiReason, aiSource, opts)
		persistFindSuggestionMemory(query, aiCommand, aiSource, aiRisk)
		return
	}
	if aiSuggestionMatchesTopHistory(aiCommand, matches) {
		printSuggestedCommandBlock(aiCommand, aiReason, aiSource, opts)
		persistFindSuggestionMemory(query, aiCommand, aiSource, aiRisk)
		return
	}
	if opts.Quiet {
		if aiCommand != "" {
			persistFindSuggestionMemory(query, aiCommand, aiSource, aiRisk)
			fmt.Println(aiCommand)
			return
		}
		if len(matches) > 0 {
			fmt.Println(matches[0].Command)
			return
		}
	}

	if aiCommand != "" {
		if matches == nil {
			matches = []history.Match{}
		}
		backend := effectiveUIBackend(cfg, opts)
		if canUseInteractiveUI(opts, backend) {
			selected, used, selectErr := ui.SelectSuggestedCommand(backend, query, ui.Selection{
				Command: aiCommand,
				Reason:  aiReason,
				Source:  aiSource,
			}, matches)
			if selectErr == nil && used {
				if strings.TrimSpace(selected.Command) == "" {
					fmt.Println("Cancelled.")
					return
				}
				printSuggestedCommandBlock(selected.Command, compactReason(selected.Reason, 120), selected.Source, opts)
				selectedRisk := ""
				if normalizeComparableCommand(selected.Command) == normalizeComparableCommand(aiCommand) {
					selectedRisk = aiRisk
				}
				persistFindSuggestionMemory(query, selected.Command, selected.Source, selectedRisk)
				return
			}
			if selectErr != nil {
				fmt.Fprintf(os.Stderr, "ew: ui picker failed (%v); falling back to plain output\n", selectErr)
			}
		}

		fmt.Println("Suggested command:")
		fmt.Println(aiCommand)
		if aiReason != "" {
			fmt.Printf("reason: %s\n", aiReason)
		}
		if aiSource != "" {
			fmt.Printf("source: %s\n", aiSource)
		}
		persistFindSuggestionMemory(query, aiCommand, aiSource, aiRisk)
		if copySuggestedCommand(aiCommand, opts) {
			fmt.Println("copied: yes")
		}
		if len(matches) > 0 {
			fmt.Println("Tip: add `--json` to inspect ranked history matches")
		}
		return
	}

	fmt.Printf("Top matches for: %q\n", query)
	for idx, match := range matches {
		fmt.Printf("%d. %s\n", idx+1, match.Command)
	}
	fmt.Println("Tip: use `ew --execute <query>` to execute the top match")
}

func handleRun(query string, cfg config.Config, opts options) {
	query = strings.TrimSpace(query)
	if query == "" {
		payload := response{Intent: string(router.IntentRun), Message: "add a query to run, e.g. ew --execute clear aws vault"}
		printResponse(payload, opts.JSON)
		return
	}

	memoryMatches, _ := searchMemoryWithLoader(query, cfg.Find.MaxResults, opts, "checking what you've used before")
	if top, ok := preferredMemoryMatch(query, memoryMatches); ok {
		outcome := executeSuggested(top.Command, fmt.Sprintf("learned from memory for %q (uses: %d)", top.Query, top.Uses), "", cfg, opts, router.IntentRun)
		persistExecutionMemory(query, outcome)
		return
	}

	matches, err := searchHistoryWithLoader(query, cfg.Find.MaxResults, opts, "scouting your history")
	if err != nil {
		payload := response{Intent: string(router.IntentRun), Message: fmt.Sprintf("search failed: %v", err)}
		printResponse(payload, opts.JSON)
		return
	}
	matches = filterFindMatches(query, matches)
	if len(matches) == 0 {
		if opts.Offline {
			payload := response{Intent: string(router.IntentRun), Message: "no safe matching history entries found"}
			printResponse(payload, opts.JSON)
			return
		}

		prompt := buildFindPrompt(query, nil)
		resolution, providerName, resolveErr := resolveProviderWithLoader(
			context.Background(),
			cfg,
			opts,
			provider.IntentFind,
			prompt,
			"thinking of an executable command",
		)
		if resolveErr != nil {
			payload := response{
				Intent:  string(router.IntentRun),
				Message: "no local history match and provider fallback failed",
				Suggestions: []string{
					resolveErr.Error(),
				},
			}
			printResponse(payload, opts.JSON)
			return
		}
		decision := evaluateAIResolution(router.IntentRun, cfg, resolution)
		if !decision.Allowed {
			if !opts.JSON && strings.TrimSpace(decision.Command) != "" && commandAllowedForQuery(query, decision.Command) {
				if strings.TrimSpace(decision.Message) != "" {
					fmt.Printf("Not executed automatically: %s\n", decision.Message)
				}
				printSuggestedCommandBlock(decision.Command, compactReason(resolution.Reason, 120), providerName, opts)
				return
			}
			payload := response{
				Intent:   string(router.IntentRun),
				Message:  decision.Message,
				Command:  decision.Command,
				Risk:     normalizeRiskHint(resolution.Risk),
				Executed: false,
			}
			if strings.TrimSpace(resolution.Reason) != "" {
				payload.Suggestions = []string{resolution.Reason}
			}
			printResponse(payload, opts.JSON)
			return
		}
		if decision.ModeOverride != "" {
			opts.Mode = decision.ModeOverride
		}
		if !commandAllowedForQuery(query, decision.Command) {
			payload := response{
				Intent:   string(router.IntentRun),
				Message:  "provider suggested a destructive command for a non-destructive query",
				Command:  strings.TrimSpace(decision.Command),
				Risk:     "high",
				Executed: false,
			}
			printResponse(payload, opts.JSON)
			return
		}
		outcome := executeSuggested(decision.Command, decision.Reason, decision.RiskHint, cfg, opts, router.IntentRun)
		persistExecutionMemory(query, outcome)
		return
	}

	command := matches[0].Command
	reason := "selected from history"
	if shouldAIRerank(cfg.Find.AIRerank, matches) && !opts.Offline {
		prompt := buildFindPrompt(query, matches)
		if resolution, providerName, err := resolveProviderWithLoader(
			context.Background(),
			cfg,
			opts,
			provider.IntentFind,
			prompt,
			"ranking the safest executable command",
		); err == nil && strings.TrimSpace(resolution.Command) != "" {
			decision := evaluateAIResolution(router.IntentRun, cfg, resolution)
			if decision.Allowed && commandAllowedForQuery(query, decision.Command) {
				command = decision.Command
				reason = fmt.Sprintf("%s (via %s)", decision.Reason, providerName)
				if decision.ModeOverride != "" {
					opts.Mode = decision.ModeOverride
				}
			}
		}
	}
	outcome := executeSuggested(command, reason, "", cfg, opts, router.IntentRun)
	persistExecutionMemory(query, outcome)
}

func handleFix(userContext string, cfg config.Config, opts options) {
	sessionID := strings.TrimSpace(os.Getenv("EW_SESSION_ID"))
	ev, err := hook.LatestFailure(sessionID)
	if err != nil {
		payload := response{Intent: string(router.IntentFix), Message: fmt.Sprintf("could not read latest failure: %v", err)}
		printResponse(payload, opts.JSON)
		return
	}
	if ev == nil {
		if tryInferredFixFromRecentHistory(userContext, cfg, opts) {
			return
		}
		printNoCapturedFailureMessage(opts, "")
		return
	}
	if stale, detail := staleFailureDetail(ev, time.Now().UTC()); stale {
		if tryInferredFixFromRecentHistory(userContext, cfg, opts) {
			return
		}
		printNoCapturedFailureMessage(opts, detail)
		return
	}

	suggested, reason := ewrt.SuggestFix(ev.Command)
	if suggested == "" {
		if opts.Offline {
			payload := response{
				Intent:  string(router.IntentFix),
				Message: "no deterministic fix found yet",
				Suggestions: []string{
					fmt.Sprintf("Failed command: %s", ev.Command),
				},
			}
			printResponse(payload, opts.JSON)
			return
		}

		prompt := buildFixPrompt(ev.Command, ev.ExitCode, ev.CWD, userContext)
		resolution, providerName, resolveErr := resolveProviderWithLoader(
			context.Background(),
			cfg,
			opts,
			provider.IntentFix,
			prompt,
			"debugging the failed command",
		)
		if resolveErr != nil {
			payload := response{
				Intent:  string(router.IntentFix),
				Message: "no deterministic fix found and provider fallback failed",
				Suggestions: []string{
					fmt.Sprintf("Failed command: %s", ev.Command),
					resolveErr.Error(),
				},
			}
			printResponse(payload, opts.JSON)
			return
		}
		decision := evaluateAIResolution(router.IntentFix, cfg, resolution)
		if !decision.Allowed {
			if !opts.JSON && strings.TrimSpace(decision.Command) != "" {
				if strings.TrimSpace(decision.Message) != "" {
					fmt.Printf("Not executed automatically: %s\n", decision.Message)
				}
				printSuggestedCommandBlock(decision.Command, compactReason(resolution.Reason, 120), providerName, opts)
				return
			}
			payload := response{
				Intent:   string(router.IntentFix),
				Message:  decision.Message,
				Command:  decision.Command,
				Risk:     normalizeRiskHint(resolution.Risk),
				Executed: false,
			}
			if strings.TrimSpace(resolution.Reason) != "" {
				payload.Suggestions = []string{resolution.Reason}
			}
			printResponse(payload, opts.JSON)
			return
		}
		if decision.ModeOverride != "" {
			opts.Mode = decision.ModeOverride
		}
		executeSuggested(decision.Command, decision.Reason, decision.RiskHint, cfg, opts, router.IntentFix)
		return
	}

	executeSuggested(suggested, reason, "", cfg, opts, router.IntentFix)
}

func printNoCapturedFailureMessage(opts options, detail string) {
	if opts.JSON {
		suggestions := []string{
			"Try `ew <what you want>`, e.g. `ew logout from aws sso`",
			"Optional once: run `ew --setup-hooks` for automatic failure capture",
		}
		if strings.TrimSpace(detail) != "" {
			suggestions = append(suggestions, "debug: "+detail)
		}
		payload := response{
			Intent:      string(router.IntentFix),
			Message:     "could not infer a recent failed command",
			Suggestions: suggestions,
		}
		printResponse(payload, true)
		return
	}

	fmt.Println("Couldn't infer a recent failed command.")
	fmt.Println("Try: `ew <what you want>` (example: `ew logout from aws sso`)")
	fmt.Println("Optional once: `ew --setup-hooks` for automatic failure capture")
}

func staleFailureDetail(ev *hook.Event, now time.Time) (bool, string) {
	if ev == nil {
		return true, "no captured failure event"
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(ev.Timestamp))
	if err != nil {
		detail := "captured failure has invalid timestamp"
		if strings.TrimSpace(ev.Command) != "" {
			detail += fmt.Sprintf(": %s", strings.TrimSpace(ev.Command))
		}
		return true, detail
	}
	age := now.Sub(ts)
	if age <= maxFixFailureAge {
		return false, ""
	}
	detail := fmt.Sprintf("captured %s ago: %s", age.Round(time.Minute), strings.TrimSpace(ev.Command))
	if strings.TrimSpace(ev.SessionID) != "" {
		detail += fmt.Sprintf(" (session: %s)", strings.TrimSpace(ev.SessionID))
	}
	return true, detail
}

func tryInferredFixFromRecentHistory(userContext string, cfg config.Config, opts options) bool {
	recent, err := latestHistoryEntryWithLoader(maxInferredHistoryAge, opts)
	if err != nil || recent == nil {
		return false
	}
	failedCommand := strings.TrimSpace(recent.Command)
	if failedCommand == "" {
		return false
	}

	if suggested, reason := ewrt.SuggestFix(failedCommand); suggested != "" {
		printSuggestedCommandBlock(
			suggested,
			compactReason("inferred from your latest shell command; "+reason, 120),
			"ew",
			opts,
		)
		return true
	}

	if opts.Offline {
		return false
	}

	cwd, cwdErr := os.Getwd()
	if cwdErr != nil || strings.TrimSpace(cwd) == "" {
		cwd = "."
	}

	prompt := buildFixPrompt(
		failedCommand,
		1,
		cwd,
		fallbackFixContext(userContext),
	)
	resolution, providerName, resolveErr := resolveProviderWithLoader(
		context.Background(),
		cfg,
		opts,
		provider.IntentFix,
		prompt,
		"inferring intent from your latest command",
	)
	if resolveErr != nil {
		return false
	}

	decision := evaluateAIResolution(router.IntentFix, cfg, resolution)
	command := strings.TrimSpace(decision.Command)
	if command == "" {
		command = strings.TrimSpace(resolution.Command)
	}
	if command == "" {
		return false
	}

	normalized, normalizeErr := ewrt.NormalizeCommand(command)
	if normalizeErr != nil {
		return false
	}
	if !isCleanInferredCommand(normalized) {
		return false
	}

	reason := strings.TrimSpace(resolution.Reason)
	if reason == "" {
		reason = strings.TrimSpace(decision.Reason)
	}
	if reason == "" {
		reason = "best correction inferred from your latest shell command"
	} else {
		reason = "inferred from your latest shell command; " + reason
	}
	reason = compactReason(reason, 120)

	if opts.JSON {
		payload := response{
			Intent:   string(router.IntentFix),
			Message:  "suggestion inferred from latest shell command history",
			Command:  normalized,
			Risk:     normalizeRiskHint(resolution.Risk),
			Executed: false,
			Suggestions: []string{
				fmt.Sprintf("latest shell command: %s", failedCommand),
				reason,
			},
		}
		printResponse(payload, true)
		return true
	}

	printSuggestedCommandBlock(normalized, reason, providerName, opts)
	return true
}

func fallbackFixContext(userContext string) string {
	trimmed := strings.TrimSpace(userContext)
	if trimmed != "" {
		return trimmed + " Return one direct replacement command only; avoid shell chaining, pipes, or diagnostic command bundles."
	}
	return "Infer the intended command from this recently executed shell command. Return one direct replacement command only; avoid shell chaining, pipes, or diagnostic command bundles."
}

func isCleanInferredCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return false
	}
	blockedTokens := []string{
		"&&",
		"||",
		"|",
		";",
		"\n",
		"\r",
		"`",
		"$(",
	}
	for _, token := range blockedTokens {
		if strings.Contains(trimmed, token) {
			return false
		}
	}
	if strings.ContainsAny(trimmed, "<>") {
		return false
	}
	return true
}

func executeSuggested(command, reason, riskHint string, cfg config.Config, opts options, intent router.Intent) executionOutcome {
	normalizedCommand, normalizeErr := ewrt.NormalizeCommand(command)
	if normalizeErr != nil {
		payload := response{
			Intent:   string(intent),
			Message:  fmt.Sprintf("command rejected: %v", normalizeErr),
			Command:  strings.TrimSpace(command),
			Risk:     "high",
			Executed: false,
		}
		printResponse(payload, opts.JSON)
		return executionOutcome{Command: strings.TrimSpace(command), Executed: false, Success: false}
	}
	command = normalizedCommand

	mode := cfg.Mode
	if strings.TrimSpace(opts.Mode) != "" {
		mode = strings.TrimSpace(opts.Mode)
	}

	mode, risk := applyExecutionRiskPolicy(cfg, mode, command, riskHint)

	if opts.DryRun {
		payload := response{Intent: string(intent), Message: reason, Command: command, Risk: risk, Executed: false}
		printResponse(payload, opts.JSON)
		return executionOutcome{Command: command, Executed: false, Success: false}
	}

	if opts.JSON && isConfirmMode(mode) && !opts.Yes {
		payload := response{
			Intent:   string(intent),
			Message:  "confirmation required; rerun with --yes or --mode yolo",
			Command:  command,
			Risk:     risk,
			Executed: false,
		}
		printResponse(payload, true)
		return executionOutcome{Command: command, Executed: false, Success: false}
	}

	if isConfirmMode(mode) && !opts.Yes && !opts.JSON {
		backend := effectiveUIBackend(cfg, opts)
		if canUseInteractiveUI(opts, backend) {
			approved, used, uiErr := ui.ConfirmExecution(backend, command, risk)
			if uiErr == nil && used {
				if !approved {
					printConfirmCancelled(command, risk)
					return executionOutcome{Command: command, Executed: false, Success: false}
				}
				if err := ewrt.RunCommand(command); err != nil {
					payload := response{Intent: string(intent), Message: fmt.Sprintf("execution failed: %v", err), Command: command, Risk: risk, Executed: true}
					printResponse(payload, opts.JSON)
					return executionOutcome{Command: command, Executed: true, Success: false}
				}
				payload := response{Intent: string(intent), Message: reason, Command: command, Risk: risk, Executed: true}
				printResponse(payload, opts.JSON)
				return executionOutcome{Command: command, Executed: true, Success: true}
			}
			if uiErr != nil {
				fmt.Fprintf(os.Stderr, "ew: ui confirmation failed (%v); falling back to plain prompt\n", uiErr)
			}
		}

		fmt.Println("Command to run:")
		fmt.Println(command)
	}

	shouldRun, err := ewrt.ShouldExecute(mode, opts.Yes)
	if err != nil {
		payload := response{Intent: string(intent), Message: err.Error(), Command: command, Risk: risk}
		printResponse(payload, opts.JSON)
		return executionOutcome{Command: command, Executed: false, Success: false}
	}

	if !shouldRun {
		if isConfirmMode(mode) && !opts.Yes && !opts.JSON {
			printConfirmCancelled(command, risk)
			return executionOutcome{Command: command, Executed: false, Success: false}
		}
		payload := response{Intent: string(intent), Message: reason, Command: command, Risk: risk, Executed: false}
		printResponse(payload, opts.JSON)
		return executionOutcome{Command: command, Executed: false, Success: false}
	}

	if err := ewrt.RunCommand(command); err != nil {
		payload := response{Intent: string(intent), Message: fmt.Sprintf("execution failed: %v", err), Command: command, Risk: risk, Executed: true}
		printResponse(payload, opts.JSON)
		return executionOutcome{Command: command, Executed: true, Success: false}
	}

	payload := response{Intent: string(intent), Message: reason, Command: command, Risk: risk, Executed: true}
	printResponse(payload, opts.JSON)
	return executionOutcome{Command: command, Executed: true, Success: true}
}

func printConfirmCancelled(command string, risk string) {
	fmt.Println("Cancelled. Command not executed.")
	fmt.Printf("command: %s\n", command)
	if risk != "" {
		fmt.Printf("risk: %s\n", risk)
	}
}

func printResponse(payload response, asJSON bool) {
	if asJSON {
		encoded, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	if payload.Message != "" {
		fmt.Println(payload.Message)
	}
	if payload.Command != "" {
		fmt.Printf("command: %s\n", payload.Command)
	}
	if payload.Risk != "" {
		fmt.Printf("risk: %s\n", payload.Risk)
	}
	if len(payload.Suggestions) > 0 {
		for _, suggestion := range payload.Suggestions {
			fmt.Printf("- %s\n", suggestion)
		}
	}
	if payload.Results != nil {
		encoded, _ := json.MarshalIndent(payload.Results, "", "  ")
		fmt.Println(string(encoded))
	}
	if payload.ConfigPath != "" {
		fmt.Printf("config: %s\n", payload.ConfigPath)
	}
}

func isConfirmMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "confirm":
		return true
	default:
		return false
	}
}

func init() {
	_, _ = knowledge.CorePrompt()
}

func runInternal(args ...string) ([]byte, error) {
	seen := map[string]struct{}{}
	candidates := make([]string, 0, 2)

	if path, err := exec.LookPath("_ew"); err == nil {
		candidates = append(candidates, path)
		seen[path] = struct{}{}
	}
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), "_ew")
		if _, exists := seen[sibling]; !exists {
			if _, err := os.Stat(sibling); err == nil {
				candidates = append(candidates, sibling)
			}
		}
	}

	var lastErr error
	var lastOut []byte
	for _, bin := range candidates {
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return out, nil
		}
		lastErr = err
		lastOut = out
	}

	if lastErr == nil {
		return nil, fmt.Errorf("_ew executable not found")
	}
	return lastOut, lastErr
}

func detectShell() string {
	shellPath := strings.TrimSpace(os.Getenv("SHELL"))
	if shellPath == "" {
		return "zsh"
	}
	base := filepath.Base(shellPath)
	switch base {
	case "zsh", "bash", "fish":
		return base
	default:
		return "zsh"
	}
}

func resolveProvider(ctx context.Context, cfg config.Config, opts options, intent provider.Intent, prompt string) (provider.Resolution, string, error) {
	registry := provider.NewRegistry()
	service := provider.NewService(registry)
	model, thinking, mode := intentSettings(cfg, opts, intent)
	if cfg.Safety.RedactSecrets {
		prompt = safety.RedactText(prompt)
	}

	req := provider.Request{
		Intent:   intent,
		Prompt:   prompt,
		Model:    model,
		Thinking: thinking,
		Mode:     mode,
		Context:  map[string]any{},
	}
	return service.Resolve(ctx, cfg, req, strings.TrimSpace(opts.Provider))
}

func intentSettings(cfg config.Config, opts options, intent provider.Intent) (string, string, string) {
	var model string
	var thinking string
	switch intent {
	case provider.IntentFind:
		model = cfg.Find.Model
		thinking = cfg.Find.Thinking
	default:
		model = cfg.Fix.Model
		thinking = cfg.Fix.Thinking
	}
	if strings.TrimSpace(opts.Model) != "" {
		model = strings.TrimSpace(opts.Model)
	}
	if strings.TrimSpace(opts.Thinking) != "" {
		thinking = strings.TrimSpace(opts.Thinking)
	}

	mode := cfg.Mode
	if strings.TrimSpace(opts.Mode) != "" {
		mode = strings.TrimSpace(opts.Mode)
	}
	return model, thinking, mode
}

func buildFixPrompt(command string, exitCode int, cwd string, userContext string) string {
	base := fmt.Sprintf(
		"Return only JSON matching schema. Diagnose and fix this failed shell command. Failed command: %q. Exit code: %d. Working directory: %q. Output one safest next command.",
		command,
		exitCode,
		cwd,
	)
	contextNote := strings.TrimSpace(userContext)
	lower := strings.ToLower(contextNote)
	if contextNote != "" && !isTrivialFixContext(lower) {
		base += fmt.Sprintf(" Additional user context: %q.", contextNote)
	}
	return wrapWithSelfKnowledge(base)
}

func buildFindPrompt(query string, candidates []history.Match) string {
	base := fmt.Sprintf("Return only JSON matching schema. Find the best shell command for this request: %q.", query)
	if len(candidates) == 0 {
		return wrapWithSelfKnowledge(base + " There were no local history matches.")
	}
	lines := make([]string, 0, len(candidates))
	for idx, candidate := range candidates {
		lines = append(lines, fmt.Sprintf("%d) %s (score=%.2f)", idx+1, candidate.Command, candidate.Score))
	}
	return wrapWithSelfKnowledge(base + " Rank these candidate commands and pick the best one:\n" + strings.Join(lines, "\n"))
}

func wrapWithSelfKnowledge(prompt string) string {
	core, err := knowledge.CorePrompt()
	core = strings.TrimSpace(core)
	systemContext := strings.TrimSpace(runtimeSystemContext)

	parts := make([]string, 0, 2)
	if err == nil && core != "" {
		parts = append(parts, "EW_SELF_KNOWLEDGE_JSON:\n"+core)
	}
	if systemContext != "" {
		parts = append(parts, "EW_SYSTEM_PROFILE:\n"+systemContext)
	}
	if len(parts) == 0 {
		return strings.TrimSpace(prompt)
	}
	parts = append(parts, "TASK:\n"+prompt)
	return strings.Join(parts, "\n\n")
}

func isTrivialFixContext(lower string) bool {
	switch strings.TrimSpace(lower) {
	case "", "fix", "ew", "last failed", "fix last failed command", "fix the last failed command":
		return true
	default:
		return false
	}
}

func shouldAIRerank(mode string, matches []history.Match) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "off", "false", "never":
		return false
	case "always", "on", "true":
		return len(matches) > 0
	default:
		if len(matches) == 0 {
			return false
		}
		return len(matches) > 1 && matches[0].Score < 24
	}
}

func lowSignalFindQuery(query string) bool {
	return countSignalTokens(query) < 2
}

func filterFindMatches(query string, matches []history.Match) []history.Match {
	if len(matches) == 0 {
		return matches
	}
	allowDestructive := queryAllowsDestructive(query)
	allowHighRisk := queryAllowsHighRisk(query)
	readOnly := queryPrefersReadOnly(query)
	minScore := minimumHistoryMatchScore(query)
	filtered := make([]history.Match, 0, len(matches))
	for _, match := range matches {
		command := strings.TrimSpace(match.Command)
		if command == "" {
			continue
		}
		if match.Score < minScore {
			continue
		}
		if readOnly && isMutatingCommand(command) {
			continue
		}
		if ewrt.HighRisk(command) && !allowHighRisk {
			continue
		}
		if isDestructiveCommand(command) && !allowDestructive {
			continue
		}
		filtered = append(filtered, match)
	}
	return filtered
}

func queryAllowsDestructive(query string) bool {
	low := strings.ToLower(strings.TrimSpace(query))
	keywords := []string{
		"delete",
		"remove",
		"destroy",
		"drop",
		"purge",
		"wipe",
		"uninstall",
		"terminate",
		"kill",
		"prune",
	}
	for _, keyword := range keywords {
		if strings.Contains(low, keyword) {
			return true
		}
	}
	explicitPhrases := []string{
		"hard reset",
		"reset --hard",
		"delete all",
		"remove all",
		"clean branch completely",
	}
	for _, phrase := range explicitPhrases {
		if strings.Contains(low, phrase) {
			return true
		}
	}
	return false
}

func queryAllowsHighRisk(query string) bool {
	low := strings.ToLower(strings.TrimSpace(query))
	explicitHighRiskPhrases := []string{
		"rm -rf",
		"mkfs",
		"dd if=",
		"format disk",
		"wipe disk",
		"destroy all data",
		"delete everything",
		"factory reset",
		"chmod 777 /",
	}
	for _, phrase := range explicitHighRiskPhrases {
		if strings.Contains(low, phrase) {
			return true
		}
	}
	return false
}

func isDestructiveCommand(command string) bool {
	low := strings.ToLower(strings.TrimSpace(command))
	patterns := []string{
		"rm ",
		"rmdir ",
		"git clean ",
		"git reset --hard",
		"git checkout --",
		"git worktree remove",
		"dropdb ",
		"kubectl delete ",
		"terraform destroy",
		"docker system prune",
	}
	for _, pattern := range patterns {
		if strings.Contains(low, pattern) {
			return true
		}
	}
	return false
}

func commandAllowedForQuery(query string, command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return false
	}
	if queryPrefersReadOnly(query) && isMutatingCommand(trimmed) {
		return false
	}
	allowDestructive := queryAllowsDestructive(query)
	allowHighRisk := queryAllowsHighRisk(query)
	if ewrt.HighRisk(trimmed) && !allowHighRisk {
		return false
	}
	if isDestructiveCommand(trimmed) && !allowDestructive {
		return false
	}
	return true
}

func queryPrefersReadOnly(query string) bool {
	low := strings.ToLower(strings.TrimSpace(query))
	if low == "" {
		return false
	}
	if containsAny(
		low,
		"set ",
		"change ",
		"update ",
		"write ",
		"append ",
		"add ",
		"create ",
		"edit ",
		"modify ",
		"remove ",
		"delete ",
		"install ",
		"enable ",
		"disable ",
		"export ",
		"replace ",
		"fix ",
		"run ",
		"execute ",
		"copy ",
		"move ",
		"rename ",
		"clone ",
		"download ",
		"upload ",
	) {
		return false
	}
	return containsAny(
		low,
		"path ",
		"path to",
		"where is",
		"where's",
		"locate ",
		"show ",
		"list ",
		"print ",
		"display ",
		"what is",
		"check ",
		"view ",
		"find ",
	)
}

func isMutatingCommand(command string) bool {
	low := strings.ToLower(strings.TrimSpace(command))
	if low == "" {
		return false
	}
	if strings.HasPrefix(low, ". ") {
		return true
	}
	if strings.Contains(low, ">>") || strings.Contains(low, ">|") || strings.Contains(low, " > ") || hasWriteRedirection(low) {
		return true
	}
	if strings.HasPrefix(low, "tee ") || strings.Contains(low, "| tee ") || strings.Contains(low, " tee -a ") {
		return true
	}
	patterns := []string{
		"sed -i",
		"perl -i",
		"truncate ",
		"rm ",
		"rmdir ",
		"mv ",
		"cp ",
		"touch ",
		"chmod ",
		"chown ",
		"mkdir ",
		"ln -s ",
		"ln ",
		"source ",
		"export ",
		"alias ",
		"unalias ",
		"cd ",
		"pushd ",
		"popd ",
		"git commit",
		"git push",
		"git reset",
		"git checkout -b",
		"git branch -d",
		"git branch -D",
	}
	for _, pattern := range patterns {
		if strings.Contains(low, pattern) {
			return true
		}
	}
	return false
}

func hasWriteRedirection(command string) bool {
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if ch != '>' {
			continue
		}
		next := byte(0)
		if i+1 < len(command) {
			next = command[i+1]
		}
		// fd duplication like 2>&1 is not file mutation.
		if next == '&' {
			continue
		}
		return true
	}
	return false
}

func applyExecutionRiskPolicy(cfg config.Config, mode string, command string, riskHint string) (string, string) {
	effectiveMode := strings.ToLower(strings.TrimSpace(mode))
	if effectiveMode == "" {
		effectiveMode = "confirm"
	}

	risk := normalizeRiskHint(riskHint)
	isHighRiskCommand := ewrt.HighRisk(command)
	isDestructive := isDestructiveCommand(command)
	if (isHighRiskCommand || isDestructive) && cfg.Safety.BlockHighRisk {
		risk = "high"
	} else if (isHighRiskCommand || isDestructive) && risk == "low" {
		risk = "medium"
	} else if isMutatingCommand(command) && risk == "low" {
		risk = "medium"
	}

	if effectiveMode == "yolo" && !cfg.Safety.AllowYoloHighRisk && (risk == "high" || (cfg.Safety.BlockHighRisk && (isHighRiskCommand || isDestructive))) {
		effectiveMode = "confirm"
	}
	return effectiveMode, risk
}

func countSignalTokens(query string) int {
	return len(queryRelevanceTokens(query))
}

func minimumHistoryMatchScore(query string) float64 {
	tokenCount := len(queryRelevanceTokens(query))
	switch {
	case tokenCount >= 4:
		return 8.0
	case tokenCount >= 2:
		return 7.0
	default:
		return 6.0
	}
}

func queryRelevanceTokens(query string) []string {
	stopwords := map[string]struct{}{
		"the": {}, "for": {}, "and": {}, "with": {}, "from": {}, "into": {}, "onto": {}, "that": {}, "this": {},
		"you": {}, "your": {}, "can": {}, "could": {}, "how": {}, "what": {}, "when": {}, "where": {}, "why": {},
		"are": {}, "is": {}, "to": {}, "me": {}, "my": {}, "find": {}, "search": {}, "show": {}, "list": {},
		"please": {}, "help": {}, "command": {}, "commands": {}, "run": {}, "execute": {}, "file": {}, "files": {},
		"path": {}, "paths": {}, "location": {}, "locate": {},
	}
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(query)), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '_' || r == ':' || r == '/'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		token := strings.Trim(strings.TrimSpace(part), `"'.,!?;:()[]{}<>`)
		if len(token) < 3 {
			continue
		}
		if _, blocked := stopwords[token]; blocked {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func aiSuggestionMatchesTopHistory(aiCommand string, matches []history.Match) bool {
	if strings.TrimSpace(aiCommand) == "" || len(matches) == 0 {
		return false
	}
	top := strings.TrimSpace(matches[0].Command)
	return normalizeComparableCommand(aiCommand) == normalizeComparableCommand(top)
}

func normalizeComparableCommand(command string) string {
	normalized := strings.TrimSpace(strings.ToLower(command))
	for strings.HasSuffix(normalized, `\`) {
		normalized = strings.TrimSpace(strings.TrimSuffix(normalized, `\`))
	}
	return normalized
}

func compactReason(reason string, max int) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ""
	}
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}

	for _, sep := range []string{". ", "; ", "\n"} {
		if idx := strings.Index(trimmed, sep); idx > 0 && idx < max {
			return strings.TrimSpace(trimmed[:idx+1])
		}
	}
	return strings.TrimSpace(trimmed[:max]) + "..."
}

func providerFallbackMessage(action string, providerName string) string {
	name := strings.TrimSpace(providerName)
	if name == "" {
		name = "provider"
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "run", "fix":
		return fmt.Sprintf("no local history match; command from %s", name)
	case "ask":
		return fmt.Sprintf("no local history match; follow-up requested by %s", name)
	case "suggest":
		fallthrough
	default:
		return fmt.Sprintf("no local history match; suggestion from %s", name)
	}
}

func printSuggestedCommandBlock(command, reason, source string, opts options) {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		fmt.Println("No suggested command available")
		return
	}
	if opts.Quiet {
		if copySuggestedCommand(normalized, opts) {
			// quiet mode intentionally emits only the command on stdout.
		}
		fmt.Println(normalized)
		return
	}

	fmt.Println("Suggested command:")
	fmt.Println(normalized)
	if reason != "" {
		fmt.Printf("reason: %s\n", reason)
	}
	if source != "" {
		fmt.Printf("source: %s\n", source)
	}
	if copySuggestedCommand(normalized, opts) {
		fmt.Println("copied: yes")
	}
}

func searchHistoryWithLoader(query string, limit int, opts options, label string) ([]history.Match, error) {
	var (
		matches []history.Match
		err     error
	)
	withEWLoader(opts, label, func() {
		matches, err = history.Search(query, limit)
	})
	return matches, err
}

func searchMemoryWithLoader(query string, limit int, opts options, label string) ([]memory.Match, error) {
	var (
		matches []memory.Match
		err     error
	)
	withEWLoader(opts, label, func() {
		var store memory.Store
		store, _, err = memory.Load()
		if err != nil {
			return
		}
		matches = store.Search(query, limit)
	})
	return matches, err
}

func preferredMemoryMatch(query string, matches []memory.Match) (memory.Match, bool) {
	for _, candidate := range matches {
		if strings.TrimSpace(candidate.Command) == "" {
			continue
		}
		if !commandAllowedForQuery(query, candidate.Command) {
			continue
		}
		if !memoryQueryCompatible(query, candidate.Query) {
			continue
		}
		if candidate.Exact || candidate.Score >= 26 || (candidate.Uses >= 2 && candidate.Score >= 18) {
			return candidate, true
		}
	}
	return memory.Match{}, false
}

func memoryQueryCompatible(query string, storedQuery string) bool {
	nq := normalizeComparableCommand(query)
	ns := normalizeComparableCommand(storedQuery)
	if nq == "" || ns == "" {
		return false
	}
	if nq == ns {
		return true
	}

	queryNums := memoryNumericTokens(query)
	storedNums := memoryNumericTokens(storedQuery)
	if len(queryNums) > 0 || len(storedNums) > 0 {
		if !sameStringSet(queryNums, storedNums) {
			return false
		}
	}

	queryTokens := memorySignalTokens(query)
	storedTokens := memorySignalTokens(storedQuery)
	if len(queryTokens) == 0 || len(storedTokens) == 0 {
		return false
	}

	storedSet := map[string]struct{}{}
	for _, token := range storedTokens {
		storedSet[token] = struct{}{}
	}

	shared := 0
	for _, token := range queryTokens {
		if _, ok := storedSet[token]; ok {
			shared++
		}
	}
	if shared == 0 {
		return false
	}
	if shared >= 2 {
		return true
	}
	if len(queryTokens) == 1 && len(storedTokens) == 1 && shared == 1 {
		return true
	}
	return strings.Contains(ns, nq) || strings.Contains(nq, ns)
}

func memoryNumericTokens(input string) []string {
	matches := reDigits.FindAllString(strings.ToLower(strings.TrimSpace(input)), -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, token := range matches {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}

func sameStringSet(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	set := map[string]struct{}{}
	for _, item := range a {
		set[item] = struct{}{}
	}
	for _, item := range b {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func memorySignalTokens(input string) []string {
	stopwords := map[string]struct{}{
		"the": {}, "for": {}, "and": {}, "with": {}, "from": {}, "into": {}, "onto": {}, "that": {}, "this": {},
		"you": {}, "your": {}, "can": {}, "could": {}, "how": {}, "what": {}, "when": {}, "where": {}, "why": {},
		"are": {}, "is": {}, "to": {}, "me": {}, "my": {}, "find": {}, "search": {}, "show": {}, "list": {},
		"please": {}, "help": {}, "command": {}, "commands": {}, "run": {}, "execute": {}, "file": {}, "files": {},
		"path": {}, "paths": {}, "location": {}, "locate": {}, "installed": {}, "install": {}, "current": {},
		"global": {}, "local": {}, "all": {},
	}
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(input)), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '_' || r == ':' || r == '/'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		token := strings.Trim(strings.TrimSpace(part), `"'.,!?;:()[]{}<>`)
		if len(token) < 2 {
			continue
		}
		if _, blocked := stopwords[token]; blocked {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func latestHistoryEntryWithLoader(maxAge time.Duration, opts options) (*history.Entry, error) {
	var (
		entry *history.Entry
		err   error
	)
	withEWLoader(opts, "checking your latest shell command", func() {
		entry, err = history.LatestEntry(maxAge)
	})
	return entry, err
}

func resolveProviderWithLoader(
	ctx context.Context,
	cfg config.Config,
	opts options,
	intent provider.Intent,
	prompt string,
	label string,
) (provider.Resolution, string, error) {
	var (
		resolution   provider.Resolution
		providerName string
		err          error
	)
	withEWLoader(opts, label, func() {
		resolution, providerName, err = resolveProvider(ctx, cfg, opts, intent, prompt)
	})
	return resolution, providerName, err
}

func persistExecutionMemory(query string, outcome executionOutcome) {
	if !outcome.Executed || !outcome.Success {
		return
	}
	query = strings.TrimSpace(query)
	command := strings.TrimSpace(outcome.Command)
	if query == "" || command == "" {
		return
	}
	store, path, err := memory.Load()
	if err != nil {
		return
	}
	if err := store.Learn(query, command, true); err != nil {
		return
	}
	_ = memory.Save(path, store)
}

func shouldPersistFindSuggestion(query string, command string, source string, risk string) bool {
	query = strings.TrimSpace(query)
	command = strings.TrimSpace(command)
	if query == "" || command == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(source), "memory") {
		return false
	}
	if normalizeRiskHint(risk) == "high" {
		return false
	}
	if !commandAllowedForQuery(query, command) {
		return false
	}
	return true
}

func persistFindSuggestionMemory(query string, command string, source string, risk string) {
	if !shouldPersistFindSuggestion(query, command, source, risk) {
		return
	}
	store, path, err := memory.Load()
	if err != nil {
		return
	}
	if err := store.Learn(query, command, true); err != nil {
		return
	}
	_ = memory.Save(path, store)
}

func withEWLoader(opts options, label string, run func()) {
	if run == nil {
		return
	}
	if !loaderEnabled(opts) {
		run()
		return
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		renderEWLoader(label, done)
	}()

	run()
	close(done)
	wg.Wait()
}

func loaderEnabled(opts options) bool {
	if opts.JSON {
		return false
	}
	override := strings.ToLower(strings.TrimSpace(os.Getenv("EW_LOADER")))
	switch override {
	case "0", "off", "false", "no":
		return false
	}
	return isTerminal(os.Stderr)
}

func renderEWLoader(label string, done <-chan struct{}) {
	delay := time.NewTimer(180 * time.Millisecond)
	defer delay.Stop()
	select {
	case <-done:
		return
	case <-delay.C:
	}

	messages := ewLoaderMessages(label)
	frames := ewLoaderFrames()
	ticker := time.NewTicker(260 * time.Millisecond)
	defer ticker.Stop()

	index := 0
	messageIndex := 0
	for {
		line := fmt.Sprintf("%s %s", frames[index], messages[messageIndex])
		fmt.Fprintf(os.Stderr, "\r%s\x1b[K", line)
		index = (index + 1) % len(frames)
		if index == 0 {
			messageIndex = (messageIndex + 1) % len(messages)
		}

		select {
		case <-done:
			fmt.Fprint(os.Stderr, "\r\x1b[K")
			return
		case <-ticker.C:
		}
	}
}

func ewLoaderFrames() []string {
	return []string{
		"ew   ",
		"we.  ",
		"EW.. ",
		"WE...",
	}
}

func ewLoaderMessages(label string) []string {
	base := strings.TrimSpace(label)
	low := strings.ToLower(base)
	catalog := localeCatalog
	switch {
	case low == "thinking of a command that fits":
		if len(catalog.Loader.ThinkingFit) > 0 {
			return catalog.Loader.ThinkingFit
		}
		return []string{"thinking of a command that fits"}
	case strings.Contains(low, "ranking"):
		return loaderCategoryMessages(base, catalog.Loader.Ranking)
	case strings.Contains(low, "history"):
		return loaderCategoryMessages(base, catalog.Loader.History)
	case strings.Contains(low, "debugging"):
		return loaderCategoryMessages(base, catalog.Loader.Debugging)
	}
	if base == "" {
		base = "working"
	}
	defaultMessages := loaderDefaultMessages(base, catalog.Loader.Default)
	if len(defaultMessages) > 0 {
		return defaultMessages
	}
	return []string{base}
}

func loaderCategoryMessages(base string, messages []string) []string {
	trimmedBase := strings.TrimSpace(base)
	if trimmedBase == "" {
		trimmedBase = "working"
	}
	if len(messages) == 0 {
		return []string{trimmedBase}
	}
	trimmedFirst := strings.TrimSpace(messages[0])
	if strings.EqualFold(trimmedFirst, trimmedBase) {
		return messages
	}
	out := make([]string, 0, len(messages)+1)
	out = append(out, trimmedBase)
	out = append(out, messages...)
	return out
}

func loaderDefaultMessages(base string, templates []string) []string {
	trimmedBase := strings.TrimSpace(base)
	if trimmedBase == "" {
		trimmedBase = "working"
	}
	if len(templates) == 0 {
		return nil
	}
	out := make([]string, 0, len(templates))
	for _, template := range templates {
		trimmed := strings.TrimSpace(template)
		if trimmed == "" {
			continue
		}
		out = append(out, strings.ReplaceAll(trimmed, "{label}", trimmedBase))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func canUseInteractiveUI(opts options, backend string) bool {
	if opts.JSON || opts.Quiet {
		return false
	}
	if !ui.IsInteractiveBackend(backend) {
		return false
	}
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func effectiveUIBackend(cfg config.Config, opts options) string {
	if strings.TrimSpace(opts.UI) != "" {
		return ui.NormalizeBackend(strings.TrimSpace(opts.UI))
	}
	return ui.NormalizeBackend(cfg.UI.Backend)
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func copySuggestedCommand(command string, opts options) bool {
	if !opts.Copy {
		return false
	}
	if err := copyToClipboard(command); err != nil {
		fmt.Fprintf(os.Stderr, "ew: could not copy command: %v\n", err)
		return false
	}
	return true
}

func copyToClipboard(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fmt.Errorf("empty command")
	}

	try := func(bin string, args ...string) error {
		path, err := exec.LookPath(bin)
		if err != nil {
			return err
		}
		cmd := exec.Command(path, args...)
		cmd.Stdin = strings.NewReader(trimmed)
		return cmd.Run()
	}

	switch goruntime.GOOS {
	case "darwin":
		if err := try("pbcopy"); err == nil {
			return nil
		}
	case "windows":
		if err := try("clip"); err == nil {
			return nil
		}
	default:
		if err := try("wl-copy"); err == nil {
			return nil
		}
		if err := try("xclip", "-selection", "clipboard"); err == nil {
			return nil
		}
		if err := try("xsel", "--clipboard", "--input"); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no supported clipboard tool found")
}
