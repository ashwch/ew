package router

// Intent describes the high-level action ew should take.
// Router detection logic was intentionally removed from runtime flow;
// ew now uses explicit flags plus direct prompt handling.
type Intent string

const (
	IntentFix        Intent = "fix"
	IntentFind       Intent = "find"
	IntentRun        Intent = "run"
	IntentConfigShow Intent = "config_show"
	IntentConfigSet  Intent = "config_set"
	IntentDiagnose   Intent = "diagnose"
	IntentSetupHooks Intent = "setup_hooks"
)
