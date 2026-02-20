package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/ashwch/ew/internal/appdirs"
	"github.com/ashwch/ew/internal/config"
	"github.com/ashwch/ew/internal/history"
	"github.com/ashwch/ew/internal/hook"
	"github.com/ashwch/ew/internal/provider"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	sub := os.Args[1]
	args := os.Args[2:]

	var err error
	switch sub {
	case "hook-record":
		err = hookRecord(args)
	case "latest-failure":
		err = latestFailure(args)
	case "history-search":
		err = historySearch(args)
	case "config-get":
		err = configGet(args)
	case "config-set":
		err = configSet(args)
	case "config-path":
		err = configPath()
	case "state-path":
		err = statePath()
	case "doctor":
		err = doctor()
	case "hook-snippet":
		err = hookSnippet(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown _ew subcommand: %s\n", sub)
		printUsage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "_ew error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("_ew <hook-record|latest-failure|history-search|config-get|config-set|config-path|state-path|doctor|hook-snippet>")
}

func hookRecord(args []string) error {
	fs := flag.NewFlagSet("hook-record", flag.ContinueOnError)
	command := fs.String("command", "", "command that was run")
	exitCode := fs.Int("exit-code", 1, "exit code")
	cwd := fs.String("cwd", "", "working directory")
	shell := fs.String("shell", "", "shell name")
	sessionID := fs.String("session-id", "", "shell session id")
	timestamp := fs.String("timestamp", "", "timestamp in RFC3339")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*command) == "" {
		return fmt.Errorf("--command is required")
	}

	ev := hook.Event{
		Command:   *command,
		ExitCode:  *exitCode,
		CWD:       *cwd,
		Shell:     *shell,
		SessionID: *sessionID,
		Timestamp: *timestamp,
	}
	return hook.RecordEvent(ev)
}

func latestFailure(args []string) error {
	fs := flag.NewFlagSet("latest-failure", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "shell session id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ev, err := hook.LatestFailure(*sessionID)
	if err != nil {
		return err
	}
	if ev == nil {
		fmt.Println("{}")
		return nil
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

func historySearch(args []string) error {
	fs := flag.NewFlagSet("history-search", flag.ContinueOnError)
	query := fs.String("query", "", "query text")
	limit := fs.Int("limit", 8, "max results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*query) == "" {
		return fmt.Errorf("--query is required")
	}

	matches, err := history.Search(*query, *limit)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(matches)
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

func configGet(args []string) error {
	fs := flag.NewFlagSet("config-get", flag.ContinueOnError)
	key := fs.String("key", "", "optional config key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, _, err := config.LoadOrCreate()
	if err != nil {
		return err
	}

	if strings.TrimSpace(*key) == "" {
		payload, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(payload))
		return nil
	}

	val, err := cfg.Get(*key)
	if err != nil {
		return err
	}
	fmt.Println(val)
	return nil
}

func configSet(args []string) error {
	fs := flag.NewFlagSet("config-set", flag.ContinueOnError)
	key := fs.String("key", "", "config key")
	value := fs.String("value", "", "config value")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*key) == "" {
		return fmt.Errorf("--key is required")
	}
	if strings.TrimSpace(*value) == "" {
		return fmt.Errorf("--value is required")
	}

	cfg, path, err := config.LoadOrCreate()
	if err != nil {
		return err
	}
	if err := cfg.Set(*key, *value); err != nil {
		return err
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("saved %s=%s\n", *key, *value)
	return nil
}

func configPath() error {
	path, err := appdirs.ConfigFilePath()
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func statePath() error {
	path, err := appdirs.StateDir()
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func doctor() error {
	type check struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Status string `json:"status"`
	}

	cfgPath, err := appdirs.ConfigFilePath()
	if err != nil {
		return err
	}
	statePath, err := appdirs.StateDir()
	if err != nil {
		return err
	}

	checks := []check{
		{Key: "os", Value: runtime.GOOS, Status: "ok"},
		{Key: "config_path", Value: cfgPath, Status: statusFile(cfgPath)},
		{Key: "state_dir", Value: statePath, Status: statusDir(statePath)},
		{Key: "codex", Value: pathOrMissing("codex"), Status: statusBinary("codex")},
		{Key: "claude", Value: pathOrMissing("claude"), Status: statusBinary("claude")},
	}

	cfg, _, err := config.LoadOrCreate()
	if err == nil {
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
	}

	payload, err := json.MarshalIndent(checks, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
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

func hookSnippet(args []string) error {
	fs := flag.NewFlagSet("hook-snippet", flag.ContinueOnError)
	shell := fs.String("shell", "zsh", "shell type: zsh|bash|fish")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch strings.ToLower(*shell) {
	case "zsh":
		fmt.Println(zshSnippet())
	case "bash":
		fmt.Println(bashSnippet())
	case "fish":
		fmt.Println(fishSnippet())
	default:
		return fmt.Errorf("unsupported shell: %s", *shell)
	}
	return nil
}

func zshSnippet() string {
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
}

func bashSnippet() string {
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
}

func fishSnippet() string {
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
}
