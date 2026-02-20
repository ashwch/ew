package router

import "testing"

func TestIntentConstantValues(t *testing.T) {
	cases := []struct {
		name string
		got  Intent
		want string
	}{
		{name: "fix", got: IntentFix, want: "fix"},
		{name: "find", got: IntentFind, want: "find"},
		{name: "run", got: IntentRun, want: "run"},
		{name: "config_show", got: IntentConfigShow, want: "config_show"},
		{name: "config_set", got: IntentConfigSet, want: "config_set"},
		{name: "diagnose", got: IntentDiagnose, want: "diagnose"},
		{name: "setup_hooks", got: IntentSetupHooks, want: "setup_hooks"},
	}
	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Fatalf("%s intent value mismatch: got %q want %q", tc.name, tc.got, tc.want)
		}
	}
}
