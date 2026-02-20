package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/ashwch/ew/internal/config"
	"github.com/ashwch/ew/internal/history"
	"github.com/ashwch/ew/internal/hook"
	"github.com/ashwch/ew/internal/memory"
	"github.com/ashwch/ew/internal/router"
)

func TestParseArgsHelpReturnsFlagErrHelp(t *testing.T) {
	_, _, err := parseArgs([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestParseArgsVersionFlag(t *testing.T) {
	opts, prompt, err := parseArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !opts.Version {
		t.Fatalf("expected version flag to be true")
	}
	if prompt != "" {
		t.Fatalf("expected empty prompt, got %q", prompt)
	}
}

func TestParseArgsCopyFlag(t *testing.T) {
	opts, prompt, err := parseArgs([]string{"--copy", "logout", "from", "aws", "sso"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !opts.Copy {
		t.Fatalf("expected copy flag to be true")
	}
	if prompt != "logout from aws sso" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestParseArgsQuietFlag(t *testing.T) {
	opts, prompt, err := parseArgs([]string{"--quiet", "fetch", "unshallow", "git", "origin"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !opts.Quiet {
		t.Fatalf("expected quiet flag to be true")
	}
	if prompt != "fetch unshallow git origin" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestParseArgsUIFlag(t *testing.T) {
	opts, prompt, err := parseArgs([]string{"--ui", "tview", "how", "to", "git", "push"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if opts.UI != "tview" {
		t.Fatalf("expected ui=tview, got %q", opts.UI)
	}
	if prompt != "how to git push" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestParseArgsExecuteAndUtilityFlags(t *testing.T) {
	opts, prompt, err := parseArgs([]string{"--execute", "--show-config", "--doctor", "--setup-hooks", "--intent", "find", "logout", "from", "aws", "sso"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !opts.Execute {
		t.Fatalf("expected execute flag to be true")
	}
	if !opts.ShowConfig {
		t.Fatalf("expected show-config flag to be true")
	}
	if !opts.Doctor {
		t.Fatalf("expected doctor flag to be true")
	}
	if !opts.SetupHooks {
		t.Fatalf("expected setup-hooks flag to be true")
	}
	if opts.Intent != "find" {
		t.Fatalf("expected intent=find, got %q", opts.Intent)
	}
	if opts.Locale != "" {
		t.Fatalf("expected locale to be empty unless explicitly set, got %q", opts.Locale)
	}
	if prompt != "logout from aws sso" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestParseArgsLocaleFlag(t *testing.T) {
	opts, prompt, err := parseArgs([]string{"--locale", "hi-IN", "show", "config"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if opts.Locale != "hi-IN" {
		t.Fatalf("expected locale=hi-IN, got %q", opts.Locale)
	}
	if prompt != "show config" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestParseArgsRejectsInvalidIntentFlag(t *testing.T) {
	_, _, err := parseArgs([]string{"--intent", "run", "logout", "from", "aws", "sso"})
	if err == nil {
		t.Fatalf("expected invalid --intent value to fail")
	}
}

func TestFlagOverrideIntent(t *testing.T) {
	if got := flagOverrideIntent("", false); got != router.IntentFix {
		t.Fatalf("expected empty prompt to map to fix intent, got %q", got)
	}
	if got := flagOverrideIntent("fix using profile staging", false); got != router.IntentFix {
		t.Fatalf("expected fix-style prompt to map to fix intent, got %q", got)
	}
	if got := flagOverrideIntent("fix using profile staging", true); got != router.IntentFind {
		t.Fatalf("expected --execute to keep run/find intent targeting, got %q", got)
	}
	if got := flagOverrideIntent("logout from aws sso", false); got != router.IntentFind {
		t.Fatalf("expected normal prompt to map to find intent, got %q", got)
	}
}

func TestMergeFlagOverridesTargetsFixForFixPromptByDefault(t *testing.T) {
	changes := map[string]string{}
	opts := options{Model: "gpt-5-mini"}
	mergeFlagOverrides(opts, changes, flagOverrideIntent("fix using profile staging", false))
	if got := changes["fix.model"]; got != "gpt-5-mini" {
		t.Fatalf("expected fix.model override, got %q", got)
	}
	if _, exists := changes["find.model"]; exists {
		t.Fatalf("did not expect find.model override for fix prompt")
	}
}

func TestIsVersionPrompt(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{in: "version", want: true},
		{in: "--version", want: true},
		{in: "-v", want: true},
		{in: "fix last command", want: false},
	}
	for _, tc := range cases {
		if got := isVersionPrompt(tc.in); got != tc.want {
			t.Fatalf("isVersionPrompt(%q)=%v want=%v", tc.in, got, tc.want)
		}
	}
}

func TestIsFixPrompt(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{in: "fix using profile staging", want: true},
		{in: "fix: aws auth issue", want: true},
		{in: "please fix last failed command", want: true},
		{in: "logout from aws sso", want: false},
		{in: "how to git push current branch", want: false},
	}
	for _, tc := range cases {
		if got := isFixPrompt(tc.in); got != tc.want {
			t.Fatalf("isFixPrompt(%q)=%v want=%v", tc.in, got, tc.want)
		}
	}
}

func TestFallbackHookSnippet(t *testing.T) {
	zsh := fallbackHookSnippet("zsh")
	if !strings.Contains(zsh, "add-zsh-hook preexec _ew_preexec") {
		t.Fatalf("expected zsh fallback snippet to contain zsh hook setup")
	}
	bash := fallbackHookSnippet("bash")
	if !strings.Contains(bash, "_EW_LAST_HISTCMD") {
		t.Fatalf("expected bash fallback snippet to contain HISTCMD guard")
	}
	fish := fallbackHookSnippet("fish")
	if !strings.Contains(fish, "function __ew_preexec --on-event fish_preexec") {
		t.Fatalf("expected fish fallback snippet to contain fish preexec hook")
	}
	if got := fallbackHookSnippet("powershell"); got != "" {
		t.Fatalf("expected unsupported shell fallback snippet to be empty, got %q", got)
	}
}

func TestLowSignalFindQuery(t *testing.T) {
	if !lowSignalFindQuery("push to gh") {
		t.Fatalf("expected push-to-gh query to be low signal")
	}
	if lowSignalFindQuery("git push current branch") {
		t.Fatalf("expected git push current branch to be sufficiently specific")
	}
}

func TestFilterFindMatchesRemovesHighRiskForNonDestructiveQuery(t *testing.T) {
	matches := []history.Match{
		{Command: "rm -rf /tmp/foo", Score: 10},
		{Command: "git worktree add ../my-wt -b feat/new", Score: 9},
	}
	filtered := filterFindMatches("command to create new worktree", matches)
	if len(filtered) != 1 {
		t.Fatalf("expected one filtered match, got %d", len(filtered))
	}
	if filtered[0].Command != "git worktree add ../my-wt -b feat/new" {
		t.Fatalf("unexpected command in filtered list: %q", filtered[0].Command)
	}
}

func TestFilterFindMatchesRemovesDestructiveRmForNonDestructiveQuery(t *testing.T) {
	matches := []history.Match{
		{Command: "rm /tmp/foo", Score: 11},
		{Command: "uv run scripts/create_worktree.py", Score: 10},
	}
	filtered := filterFindMatches("command to create new worktree", matches)
	if len(filtered) != 1 {
		t.Fatalf("expected one non-destructive command, got %d", len(filtered))
	}
	if filtered[0].Command != "uv run scripts/create_worktree.py" {
		t.Fatalf("unexpected command in filtered list: %q", filtered[0].Command)
	}
}

func TestFilterFindMatchesKeepsHighRiskForExplicitHighRiskQuery(t *testing.T) {
	matches := []history.Match{
		{Command: "rm -rf /tmp/foo", Score: 10},
	}
	filtered := filterFindMatches("wipe disk with rm -rf", matches)
	if len(filtered) != 1 {
		t.Fatalf("expected high-risk match to remain for explicit high-risk query")
	}
}

func TestFilterFindMatchesReturnsEmptyWhenOnlyDestructiveMatchesForSafeQuery(t *testing.T) {
	matches := []history.Match{
		{Command: "rm -rf /tmp/foo", Score: 10},
		{Command: "git checkout -- .", Score: 9},
	}
	filtered := filterFindMatches("command to create new worktree", matches)
	if len(filtered) != 0 {
		t.Fatalf("expected no safe matches, got %d", len(filtered))
	}
}

func TestFilterFindMatchesDropsLowQualityMatches(t *testing.T) {
	matches := []history.Match{
		{Command: "poetry run some-heavy-script --input global-data-file.csv", Score: 2.7},
		{Command: "another unrelated command with global file text", Score: 3.1},
	}
	filtered := filterFindMatches("find my global gitignore file", matches)
	if len(filtered) != 0 {
		t.Fatalf("expected weak lexical overlaps to be removed, got %d matches", len(filtered))
	}
}

func TestFilterFindMatchesDropsMutatingForReadOnlyQuery(t *testing.T) {
	matches := []history.Match{
		{Command: `echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc`, Score: 10.0},
		{Command: "echo ~/.zshrc", Score: 9.0},
	}
	filtered := filterFindMatches("path to .zshrc", matches)
	if len(filtered) != 1 {
		t.Fatalf("expected only read-only command to remain, got %d", len(filtered))
	}
	if filtered[0].Command != "echo ~/.zshrc" {
		t.Fatalf("unexpected command retained: %q", filtered[0].Command)
	}
}

func TestMinimumHistoryMatchScore(t *testing.T) {
	if got := minimumHistoryMatchScore("find my global gitignore file"); got != 7.0 {
		t.Fatalf("expected score threshold 7.0 for 2 signal tokens, got %.1f", got)
	}
	if got := minimumHistoryMatchScore("push"); got != 6.0 {
		t.Fatalf("expected score threshold 6.0 for short query, got %.1f", got)
	}
	if got := minimumHistoryMatchScore("find command to unshallow fetch origin branch"); got != 8.0 {
		t.Fatalf("expected score threshold 8.0 for high-signal query, got %.1f", got)
	}
}

func TestProviderFallbackMessage(t *testing.T) {
	if got := providerFallbackMessage("suggest", "claude"); got != "no local history match; suggestion from claude" {
		t.Fatalf("unexpected suggest message: %q", got)
	}
	if got := providerFallbackMessage("run", "codex"); got != "no local history match; command from codex" {
		t.Fatalf("unexpected run message: %q", got)
	}
	if got := providerFallbackMessage("ask", ""); got != "no local history match; follow-up requested by provider" {
		t.Fatalf("unexpected ask message: %q", got)
	}
}

func TestQueryPrefersReadOnly(t *testing.T) {
	if !queryPrefersReadOnly("path to .zshrc") {
		t.Fatalf("expected read-only intent for path query")
	}
	if queryPrefersReadOnly("add alias to .zshrc") {
		t.Fatalf("did not expect read-only intent for mutation query")
	}
	if queryPrefersReadOnly("find command to copy file into backup directory") {
		t.Fatalf("did not expect read-only intent for copy query")
	}
}

func TestIsMutatingCommand(t *testing.T) {
	if !isMutatingCommand(`echo "x" >> ~/.zshrc`) {
		t.Fatalf("expected redirection append to be mutating")
	}
	if !isMutatingCommand(`echo hi >/tmp/demo-file`) {
		t.Fatalf("expected compact write redirection to be mutating")
	}
	if !isMutatingCommand(`echo hi 1>/tmp/demo-file`) {
		t.Fatalf("expected fd-prefixed write redirection to be mutating")
	}
	if !isMutatingCommand(`echo "x" | tee -a ~/.zshrc`) {
		t.Fatalf("expected tee append to be mutating")
	}
	if !isMutatingCommand(`source ~/.zshrc`) {
		t.Fatalf("expected source command to be treated as state-changing")
	}
	if !isMutatingCommand(`. ~/.zshrc`) {
		t.Fatalf("expected dot-source command to be treated as state-changing")
	}
	if isMutatingCommand(`echo hi 2>&1`) {
		t.Fatalf("did not expect fd-dup redirection to be treated as mutating")
	}
	if isMutatingCommand(`echo "a>b"`) {
		t.Fatalf("did not expect quoted > operator to be treated as mutating")
	}
	if isMutatingCommand("echo ~/.zshrc") {
		t.Fatalf("did not expect simple echo path to be mutating")
	}
}

func TestCommandAllowedForQueryBlocksMutatingReadOnly(t *testing.T) {
	if commandAllowedForQuery("path to .zshrc", `echo "x" >> ~/.zshrc`) {
		t.Fatalf("expected mutating command to be blocked for read-only query")
	}
	if commandAllowedForQuery("path to /tmp/demo-file", "echo hi >/tmp/demo-file") {
		t.Fatalf("expected compact write redirection to be blocked for read-only query")
	}
	if !commandAllowedForQuery("path to .zshrc", "echo ~/.zshrc") {
		t.Fatalf("expected read-only command to be allowed")
	}
}

func TestCommandAllowedForQueryAllowsCopyIntent(t *testing.T) {
	if !commandAllowedForQuery("find command to copy file into backup directory", "cp ./a ./backup/a") {
		t.Fatalf("expected copy command to be allowed for copy-intent query")
	}
}

func TestIsDestructiveCommand(t *testing.T) {
	if !isDestructiveCommand("rm /tmp/foo") {
		t.Fatalf("expected rm command to be treated as destructive")
	}
	if !isDestructiveCommand("kubectl delete pod foo") {
		t.Fatalf("expected kubectl delete to be treated as destructive")
	}
	if !isDestructiveCommand("git worktree remove ../repo-wt") {
		t.Fatalf("expected git worktree remove to be treated as destructive")
	}
	if isDestructiveCommand("git worktree add ../repo-wt -b feat/new") {
		t.Fatalf("did not expect worktree add to be treated as destructive")
	}
}

func TestCommandAllowedForQuery(t *testing.T) {
	if commandAllowedForQuery("command to create new worktree", "rm -rf /tmp/foo") {
		t.Fatalf("expected destructive command to be blocked for non-destructive query")
	}
	if !commandAllowedForQuery("remove temp directory", "rm /tmp/foo") {
		t.Fatalf("expected destructive command to be allowed for destructive-intent query")
	}
	if !commandAllowedForQuery("command to create new worktree", "uv run scripts/create_worktree.py") {
		t.Fatalf("expected safe command to be allowed")
	}
}

func TestCommandAllowedForQueryResetPhraseDoesNotBypassDestructivePolicy(t *testing.T) {
	if commandAllowedForQuery("run reset aws profile", "git reset --hard") {
		t.Fatalf("expected broad reset phrase to not authorize destructive command")
	}
}

func TestQueryAllowsDestructiveAndHighRisk(t *testing.T) {
	if !queryAllowsDestructive("remove temp directory") {
		t.Fatalf("expected remove query to allow destructive commands")
	}
	if queryAllowsHighRisk("remove temp directory") {
		t.Fatalf("did not expect generic remove query to allow high-risk commands")
	}
	if !queryAllowsHighRisk("wipe disk with rm -rf") {
		t.Fatalf("expected explicit high-risk phrase to allow high-risk commands")
	}
}

func TestApplyExecutionRiskPolicyForcesConfirmForDestructiveYolo(t *testing.T) {
	cfg := config.Default()
	mode, risk := applyExecutionRiskPolicy(cfg, "yolo", "git reset --hard", "low")
	if mode != "confirm" {
		t.Fatalf("expected destructive yolo command to downgrade to confirm, got %q", mode)
	}
	if risk != "high" {
		t.Fatalf("expected destructive command risk to be high, got %q", risk)
	}
}

func TestApplyExecutionRiskPolicyProviderHighRiskForcesConfirm(t *testing.T) {
	cfg := config.Default()
	mode, risk := applyExecutionRiskPolicy(cfg, "yolo", "git status", "high")
	if mode != "confirm" {
		t.Fatalf("expected provider high risk to force confirm in yolo mode, got %q", mode)
	}
	if risk != "high" {
		t.Fatalf("expected normalized risk high, got %q", risk)
	}
}

func TestApplyExecutionRiskPolicyAllowYoloHighRiskBypassesDowngrade(t *testing.T) {
	cfg := config.Default()
	cfg.Safety.AllowYoloHighRisk = true
	mode, risk := applyExecutionRiskPolicy(cfg, "yolo", "git reset --hard", "low")
	if mode != "yolo" {
		t.Fatalf("expected yolo mode to remain when allow_yolo_high_risk=true, got %q", mode)
	}
	if risk != "high" {
		t.Fatalf("expected destructive command risk to still be high, got %q", risk)
	}
}

func TestApplyExecutionRiskPolicyRespectsBlockHighRiskFalse(t *testing.T) {
	cfg := config.Default()
	cfg.Safety.BlockHighRisk = false
	mode, risk := applyExecutionRiskPolicy(cfg, "yolo", "git reset --hard", "low")
	if mode != "yolo" {
		t.Fatalf("expected yolo mode to remain when block_high_risk=false, got %q", mode)
	}
	if risk != "medium" {
		t.Fatalf("expected downgraded medium risk when block_high_risk=false, got %q", risk)
	}
}

func TestApplyExecutionRiskPolicyElevatesMutatingLowRisk(t *testing.T) {
	cfg := config.Default()
	mode, risk := applyExecutionRiskPolicy(cfg, "confirm", "echo hi >/tmp/demo-file", "low")
	if mode != "confirm" {
		t.Fatalf("expected confirm mode to remain, got %q", mode)
	}
	if risk != "medium" {
		t.Fatalf("expected mutating command low risk to be elevated to medium, got %q", risk)
	}
}

func TestAISuggestionMatchesTopHistory(t *testing.T) {
	matches := []history.Match{
		{Command: "aws sso logout\\", Score: 12},
	}
	if !aiSuggestionMatchesTopHistory("aws sso logout", matches) {
		t.Fatalf("expected normalized commands to match")
	}
}

func TestCompactReasonPrefersFirstSentenceWhenLong(t *testing.T) {
	input := "This is the first sentence. This is a second sentence with extra detail."
	got := compactReason(input, 40)
	if got != "This is the first sentence." {
		t.Fatalf("expected first sentence truncation, got %q", got)
	}
}

func TestCompactReasonTruncatesWhenNoSentenceBoundary(t *testing.T) {
	input := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	got := compactReason(input, 20)
	if got == input || len(got) <= 20 {
		t.Fatalf("expected truncated reason with ellipsis, got %q", got)
	}
}

func TestIsCleanInferredCommand(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{command: "aws sso logout", want: true},
		{command: "git fetch --unshallow origin", want: true},
		{command: "type rtp 2>/dev/null || history | tail -5", want: false},
		{command: "which rtp && echo nope", want: false},
		{command: "echo hi > /tmp/x", want: false},
	}

	for _, tc := range cases {
		got := isCleanInferredCommand(tc.command)
		if got != tc.want {
			t.Fatalf("isCleanInferredCommand(%q)=%v want=%v", tc.command, got, tc.want)
		}
	}
}

func TestFallbackFixContextAddsSingleCommandConstraint(t *testing.T) {
	context := fallbackFixContext("logout from aws sso")
	if !strings.Contains(context, "direct replacement command") {
		t.Fatalf("expected guidance for direct replacement command, got %q", context)
	}
	if strings.Contains(context, "diagnose and fix") {
		t.Fatalf("unexpected prompt text leakage in fallback context")
	}
}

func TestStaleFailureDetail(t *testing.T) {
	now := time.Date(2026, 2, 20, 18, 0, 0, 0, time.UTC)
	ev := &hook.Event{
		Command:   "foobarcmd --badflag",
		SessionID: "ew-prov-test",
		Timestamp: "2026-02-20T10:49:17Z",
	}
	stale, detail := staleFailureDetail(ev, now)
	if !stale {
		t.Fatalf("expected stale failure event")
	}
	if detail == "" {
		t.Fatalf("expected stale detail message")
	}
}

func TestStaleFailureDetailFreshEvent(t *testing.T) {
	now := time.Date(2026, 2, 20, 18, 0, 0, 0, time.UTC)
	ev := &hook.Event{
		Command:   "aws llo sso",
		Timestamp: "2026-02-20T17:50:00Z",
	}
	stale, detail := staleFailureDetail(ev, now)
	if stale {
		t.Fatalf("expected fresh event to not be stale: %s", detail)
	}
}

func TestStaleFailureDetailInvalidTimestampIsStale(t *testing.T) {
	now := time.Date(2026, 2, 20, 18, 0, 0, 0, time.UTC)
	ev := &hook.Event{
		Command:   "aws llo sso",
		Timestamp: "not-a-timestamp",
	}
	stale, detail := staleFailureDetail(ev, now)
	if !stale {
		t.Fatalf("expected invalid timestamp event to be treated as stale")
	}
	if !strings.Contains(detail, "invalid timestamp") {
		t.Fatalf("expected invalid timestamp detail, got %q", detail)
	}
}

func TestEWLoaderMessagesThinkingHasLargeCreativeRotation(t *testing.T) {
	messages := ewLoaderMessages("thinking of a command that fits")
	if len(messages) < 20 {
		t.Fatalf("expected large message rotation, got %d", len(messages))
	}
	if messages[0] != "thinking of a command that fits" {
		t.Fatalf("expected first canonical message, got %q", messages[0])
	}
}

func TestEWLoaderFramesUseLogoMotif(t *testing.T) {
	frames := ewLoaderFrames()
	if len(frames) != 4 {
		t.Fatalf("expected 4 loader frames, got %d", len(frames))
	}
	if !strings.Contains(frames[0], "ew") {
		t.Fatalf("expected first frame to include ew motif, got %q", frames[0])
	}
	if !strings.Contains(frames[1], "we") {
		t.Fatalf("expected second frame to include we motif, got %q", frames[1])
	}
}

func TestEWLoaderMessagesContextSpecificRotations(t *testing.T) {
	cases := []struct {
		label string
		min   int
	}{
		{label: "ranking the best command", min: 8},
		{label: "scouting your history", min: 7},
		{label: "debugging the failed command", min: 7},
		{label: "something else", min: 5},
	}

	for _, tc := range cases {
		messages := ewLoaderMessages(tc.label)
		if len(messages) < tc.min {
			t.Fatalf("expected at least %d messages for %q, got %d", tc.min, tc.label, len(messages))
		}
		if messages[0] != tc.label {
			t.Fatalf("expected first message to echo label %q, got %q", tc.label, messages[0])
		}
	}
}

func TestParseSelfPromptActionSwitchBetterUI(t *testing.T) {
	action, ok := parseSelfPromptAction("switch to a better ui")
	if !ok {
		t.Fatalf("expected self action")
	}
	if action.Kind != selfActionConfigSet {
		t.Fatalf("expected config_set action, got %q", action.Kind)
	}
	if got := action.Changes["ui.backend"]; got != "bubbletea" {
		t.Fatalf("expected ui.backend=bubbletea, got %q", got)
	}
	if !action.Persist {
		t.Fatalf("expected switch command to persist config")
	}
}

func TestParseSelfPromptActionSetHindiLocale(t *testing.T) {
	action, ok := parseSelfPromptAction("set language hindi and save")
	if !ok {
		t.Fatalf("expected self action")
	}
	if action.Kind != selfActionConfigSet {
		t.Fatalf("expected config_set action, got %q", action.Kind)
	}
	if got := action.Changes["locale"]; got != "hi" {
		t.Fatalf("expected locale=hi, got %q", got)
	}
	if !action.Persist {
		t.Fatalf("expected save phrasing to persist locale")
	}
}

func TestParseSelfPromptActionSuggestExecutionToggle(t *testing.T) {
	action, ok := parseSelfPromptAction("allow suggest execution for ew and save")
	if !ok {
		t.Fatalf("expected self action")
	}
	if action.Kind != selfActionConfigSet {
		t.Fatalf("expected config_set action, got %q", action.Kind)
	}
	if got := action.Changes["ai.allow_suggest_execution"]; got != "true" {
		t.Fatalf("expected ai.allow_suggest_execution=true, got %q", got)
	}
	if !action.Persist {
		t.Fatalf("expected save phrasing to persist config")
	}

	action, ok = parseSelfPromptAction("disable suggest execution for ew and save")
	if !ok {
		t.Fatalf("expected self action")
	}
	if got := action.Changes["ai.allow_suggest_execution"]; got != "false" {
		t.Fatalf("expected ai.allow_suggest_execution=false, got %q", got)
	}
}

func TestParseSelfPromptActionSuggestExecutionQuestionDoesNotToggle(t *testing.T) {
	action, ok := parseSelfPromptAction("what is suggest execution policy for ew")
	if !ok {
		return
	}
	if _, exists := action.Changes["ai.allow_suggest_execution"]; exists {
		t.Fatalf("did not expect question to toggle ai.allow_suggest_execution: %+v", action)
	}
}

func TestParseSelfPromptActionAvoidsExternalConfigFalsePositive(t *testing.T) {
	if action, ok := parseSelfPromptAction("show config for nginx"); ok {
		t.Fatalf("did not expect self action for external config query: %+v", action)
	}
	if action, ok := parseSelfPromptAction("find command for systemctl status docker"); ok {
		t.Fatalf("did not expect self action for external system command query: %+v", action)
	}
	if action, ok := parseSelfPromptAction("switch interface for my app"); ok {
		t.Fatalf("did not expect self action for external interface query: %+v", action)
	}
	if action, ok := parseSelfPromptAction("switch interface in my app"); ok {
		t.Fatalf("did not expect self action for external in-scope query: %+v", action)
	}
}

func TestParseSelfPromptActionSystemContextToggles(t *testing.T) {
	action, ok := parseSelfPromptAction("disable system context for ew and save")
	if !ok {
		t.Fatalf("expected self action")
	}
	if action.Kind != selfActionConfigSet {
		t.Fatalf("expected config_set action, got %q", action.Kind)
	}
	if got := action.Changes["system.enable_context"]; got != "false" {
		t.Fatalf("expected system.enable_context=false, got %q", got)
	}
	if !action.Persist {
		t.Fatalf("expected disable/save phrasing to persist")
	}

	action, ok = parseSelfPromptAction("enable auto train for system profile and save")
	if !ok {
		t.Fatalf("expected self action")
	}
	if got := action.Changes["system.auto_train"]; got != "true" {
		t.Fatalf("expected system.auto_train=true, got %q", got)
	}
}

func TestExtractPromptRefreshHours(t *testing.T) {
	if got := extractPromptRefreshHours("refresh every 72 hours for system context"); got != 72 {
		t.Fatalf("expected 72, got %d", got)
	}
	if got := extractPromptRefreshHours("ttl 24 for profile"); got != 24 {
		t.Fatalf("expected 24, got %d", got)
	}
	if got := extractPromptRefreshHours("no refresh value here"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestParseMemoryPromptAction(t *testing.T) {
	action, ok := parseMemoryPromptAction("remember push current branch means git push origin HEAD")
	if !ok {
		t.Fatalf("expected remember action")
	}
	if action.Kind != memoryActionSave {
		t.Fatalf("expected remember kind, got %q", action.Kind)
	}
	if action.Query != "push current branch" {
		t.Fatalf("unexpected query: %q", action.Query)
	}
	if action.Command != "git push origin HEAD" {
		t.Fatalf("unexpected command: %q", action.Command)
	}

	action, ok = parseMemoryPromptAction("prefer git push origin HEAD for push current branch")
	if !ok || action.Kind != memoryActionBoost {
		t.Fatalf("expected promote action, got %+v", action)
	}
	action, ok = parseMemoryPromptAction("demote git push origin master for push current branch")
	if !ok || action.Kind != memoryActionDrop {
		t.Fatalf("expected demote action, got %+v", action)
	}
	action, ok = parseMemoryPromptAction("forget memory for push current branch")
	if !ok || action.Kind != memoryActionForget {
		t.Fatalf("expected forget action, got %+v", action)
	}
	action, ok = parseMemoryPromptAction("show memory for push current branch")
	if !ok || action.Kind != memoryActionShow {
		t.Fatalf("expected show action, got %+v", action)
	}
}

func TestParseMemoryPromptActionAvoidsForgetFalsePositives(t *testing.T) {
	if action, ok := parseMemoryPromptAction("forget about this deploy plan"); ok {
		t.Fatalf("did not expect memory action for generic forget prompt: %+v", action)
	}
	if action, ok := parseMemoryPromptAction("forget memory leaks in go runtime"); ok {
		t.Fatalf("did not expect memory action for non-ew memory phrase: %+v", action)
	}
}

func TestParseMemoryPromptActionHindiRemember(t *testing.T) {
	action, ok := parseMemoryPromptAction("याद रखो push current branch का मतलब git push origin HEAD")
	if !ok {
		t.Fatalf("expected hindi remember action")
	}
	if action.Kind != memoryActionSave {
		t.Fatalf("expected remember kind, got %q", action.Kind)
	}
	if action.Query != "push current branch" {
		t.Fatalf("unexpected query: %q", action.Query)
	}
	if action.Command != "git push origin HEAD" {
		t.Fatalf("unexpected command: %q", action.Command)
	}
}

func TestPreferredMemoryMatch(t *testing.T) {
	matches := []memory.Match{
		{Query: "push current branch", Command: "git push origin HEAD", Score: 19, Uses: 3, Exact: false},
	}
	match, ok := preferredMemoryMatch("push current branch", matches)
	if !ok {
		t.Fatalf("expected preferred memory match")
	}
	if match.Command != "git push origin HEAD" {
		t.Fatalf("unexpected preferred command: %q", match.Command)
	}
}

func TestPreferredMemoryMatchRejectsIncompatibleQuery(t *testing.T) {
	matches := []memory.Match{
		{Query: "where is node installed", Command: "which node", Score: 40.6, Uses: 2, Exact: false},
	}
	if _, ok := preferredMemoryMatch("where is rustc installed", matches); ok {
		t.Fatalf("expected incompatible memory query to be rejected")
	}
}

func TestPreferredMemoryMatchSkipsIncompatibleTopCandidate(t *testing.T) {
	matches := []memory.Match{
		{Query: "find which process is using port 3000", Command: "lsof -i :3000", Score: 45, Uses: 4, Exact: false},
		{Query: "find which process is using port 8000", Command: "lsof -i :8000", Score: 28, Uses: 1, Exact: false},
	}
	match, ok := preferredMemoryMatch("find which process is using port 8000", matches)
	if !ok {
		t.Fatalf("expected compatible memory match")
	}
	if match.Command != "lsof -i :8000" {
		t.Fatalf("unexpected preferred command: %q", match.Command)
	}
}

func TestMemoryQueryCompatible(t *testing.T) {
	if !memoryQueryCompatible("where is go installed", "where is go installed") {
		t.Fatalf("expected exact query compatibility")
	}
	if !memoryQueryCompatible("logout from aws sso", "aws sso logout command") {
		t.Fatalf("expected compatible semantic query")
	}
	if memoryQueryCompatible("where is rustc installed", "where is node installed") {
		t.Fatalf("did not expect incompatible semantic query")
	}
	if memoryQueryCompatible("where is node installed", "node version") {
		t.Fatalf("did not expect location query to match version query")
	}
	if memoryQueryCompatible("find which process is using port 8000", "find which process is using port 3000") {
		t.Fatalf("did not expect different numeric port query to match")
	}
	if !memoryQueryCompatible("find which process is using port 3000", "which process is using port 3000") {
		t.Fatalf("expected same numeric port query to match")
	}
	if memoryQueryCompatible("find process using port", "find process using port 3000") {
		t.Fatalf("did not expect unspecified numeric query to match numeric-specific query")
	}
}

func TestShouldPersistFindSuggestion(t *testing.T) {
	if !shouldPersistFindSuggestion("where is go installed", "which go", "claude", "low") {
		t.Fatalf("expected safe provider suggestion to be persisted")
	}
	if shouldPersistFindSuggestion("where is go installed", "which go", "memory", "low") {
		t.Fatalf("did not expect memory-sourced suggestion to be persisted again")
	}
	if shouldPersistFindSuggestion("path to .zshrc", `echo "x" >> ~/.zshrc`, "claude", "low") {
		t.Fatalf("did not expect mutating command for read-only query to be persisted")
	}
	if shouldPersistFindSuggestion("reset repo hard", "git reset --hard", "claude", "high") {
		t.Fatalf("did not expect high-risk suggestion to be persisted")
	}
	if shouldPersistFindSuggestion("", "which go", "claude", "low") {
		t.Fatalf("did not expect empty query to be persisted")
	}
}

func TestWrapWithSelfKnowledgeIncludesSystemProfileBlock(t *testing.T) {
	previous := runtimeSystemContext
	runtimeSystemContext = "os=darwin arch=arm64\ntools=git, go, uv"
	t.Cleanup(func() {
		runtimeSystemContext = previous
	})

	wrapped := wrapWithSelfKnowledge("find the right command")
	if !strings.Contains(wrapped, "EW_SYSTEM_PROFILE:\nos=darwin arch=arm64\ntools=git, go, uv") {
		t.Fatalf("expected system profile block, got: %q", wrapped)
	}
	if !strings.Contains(wrapped, "TASK:\nfind the right command") {
		t.Fatalf("expected task block, got: %q", wrapped)
	}
}
