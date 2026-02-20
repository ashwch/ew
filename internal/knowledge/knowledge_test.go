package knowledge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCorePromptNotEmpty(t *testing.T) {
	core, err := CorePrompt()
	if err != nil {
		t.Fatalf("CorePrompt returned error: %v", err)
	}
	if strings.TrimSpace(core) == "" {
		t.Fatalf("CorePrompt returned empty content")
	}
}

func TestCorePromptContract(t *testing.T) {
	core, err := CorePrompt()
	if err != nil {
		t.Fatalf("CorePrompt returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(core), &payload); err != nil {
		t.Fatalf("self knowledge must be valid JSON: %v", err)
	}

	assertStringField(t, payload, "name", "ew")
	assertStringField(t, payload, "public_command", "ew")
	assertStringField(t, payload, "internal_command", "_ew")

	publicCLI := mustObjectField(t, payload, "public_cli_contract")
	assertStringField(t, publicCLI, "user_subcommands", "none")

	flags := mustObjectField(t, payload, "flags")
	for _, requiredFlag := range []string{"--execute", "--save", "--show-config", "--doctor", "--setup-hooks", "--intent", "--json", "--quiet"} {
		if _, ok := flags[requiredFlag]; !ok {
			t.Fatalf("flags section missing %q", requiredFlag)
		}
	}

	outputContract := mustObjectField(t, payload, "output_contract")
	jsonFields := mustStringSliceField(t, outputContract, "json_fields")
	for _, requiredField := range []string{"action", "command", "reason", "risk", "confidence", "needs_confirmation"} {
		if !containsString(jsonFields, requiredField) {
			t.Fatalf("output_contract.json_fields missing %q", requiredField)
		}
	}

	intents := mustStringSliceField(t, payload, "intents")
	for _, requiredIntent := range []string{"fix", "find", "run", "config_show", "config_set", "diagnose", "setup_hooks"} {
		if !containsString(intents, requiredIntent) {
			t.Fatalf("intents missing %q", requiredIntent)
		}
	}

	rules := mustStringSliceField(t, payload, "anti_hallucination_rules")
	if !containsString(rules, "Never invent user-facing subcommands.") {
		t.Fatalf("anti_hallucination_rules must explicitly forbid subcommand hallucination")
	}
}

func assertStringField(t *testing.T, payload map[string]any, key string, want string) {
	t.Helper()
	gotRaw, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	got, ok := gotRaw.(string)
	if !ok {
		t.Fatalf("key %q must be a string", key)
	}
	if got != want {
		t.Fatalf("key %q mismatch: got %q want %q", key, got, want)
	}
}

func mustObjectField(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()
	raw, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("key %q must be an object", key)
	}
	return obj
}

func mustStringSliceField(t *testing.T, payload map[string]any, key string) []string {
	t.Helper()
	raw, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("key %q must be an array", key)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("key %q must contain only strings", key)
		}
		out = append(out, text)
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
