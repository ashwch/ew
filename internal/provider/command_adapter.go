package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ashwch/ew/internal/config"
)

var placeholderRegex = regexp.MustCompile(`\{([a-z_]+)\}`)

type CommandAdapter struct {
	name string
	cfg  config.ProviderConfig
}

func NewCommandAdapter(name string, cfg config.ProviderConfig) (Adapter, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		cfg.Command = name
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = name
	}
	if strings.TrimSpace(cfg.Thinking) == "" {
		cfg.Thinking = "medium"
	}
	if strings.TrimSpace(cfg.ModelFlag) == "" {
		cfg.ModelFlag = "--model"
	}

	return &CommandAdapter{name: name, cfg: cfg}, nil
}

func (a *CommandAdapter) Name() string {
	return a.name
}

func (a *CommandAdapter) Type() string {
	return "command"
}

func (a *CommandAdapter) Resolve(ctx context.Context, req Request) (Resolution, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	workingReq, cleanup, err := a.prepareRequest(req)
	if err != nil {
		return Resolution{}, err
	}
	defer cleanup()

	invocation, err := a.BuildInvocation(workingReq)
	if err != nil {
		return Resolution{}, err
	}

	cmd := exec.CommandContext(ctx, invocation[0], invocation[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	raw := strings.TrimSpace(readPreferredOutput(workingReq, stdout.String()))
	if raw == "" {
		raw = strings.TrimSpace(stdout.String())
	}
	if runErr != nil {
		return Resolution{}, fmt.Errorf("provider command failed (%s): %w; stderr=%s", a.cfg.Command, runErr, truncate(stderr.String(), 800))
	}

	resolution, parseErr := parseResolution(raw)
	if parseErr == nil {
		return normalizeResolution(resolution), nil
	}

	combined := strings.TrimSpace(strings.TrimSpace(stdout.String()) + "\n" + strings.TrimSpace(stderr.String()))
	if combined != "" {
		if extracted, ok := extractJSONObject(combined); ok {
			if parsed, err := parseResolution(extracted); err == nil {
				return normalizeResolution(parsed), nil
			}
		}
	}

	return Resolution{}, fmt.Errorf("provider returned unparseable output: %s", truncate(raw, 800))
}

func (a *CommandAdapter) BuildInvocation(req Request) ([]string, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = a.cfg.Model
	}
	if model == "" {
		return nil, fmt.Errorf("model cannot be empty")
	}

	thinking := strings.TrimSpace(req.Thinking)
	if thinking == "" {
		thinking = a.cfg.Thinking
	}

	values := templateValues(req, model, thinking)

	if len(a.cfg.Args) > 0 {
		args := make([]string, 0, len(a.cfg.Args)+2)
		hasPromptPlaceholder := false
		for _, templateArg := range a.cfg.Args {
			if strings.Contains(templateArg, "{prompt}") {
				hasPromptPlaceholder = true
			}
			rendered, ok := renderTemplateArg(templateArg, values)
			if !ok {
				continue
			}
			args = append(args, rendered)
		}
		if !hasPromptPlaceholder {
			args = append(args, req.Prompt)
		}
		return append([]string{a.cfg.Command}, args...), nil
	}

	args := make([]string, 0, 8)
	if a.cfg.ModelFlag != "" {
		args = append(args, a.cfg.ModelFlag, model)
	}
	if a.cfg.ThinkingFlag != "" && thinking != "" {
		args = append(args, expandThinkingFlag(a.cfg.ThinkingFlag, thinking)...)
	}
	args = append(args, req.Prompt)
	return append([]string{a.cfg.Command}, args...), nil
}

func (a *CommandAdapter) HealthCheck() error {
	if _, err := exec.LookPath(a.cfg.Command); err != nil {
		return fmt.Errorf("command not found in PATH: %s", a.cfg.Command)
	}
	return nil
}

func (a *CommandAdapter) prepareRequest(req Request) (Request, func(), error) {
	working := req
	if working.Context == nil {
		working.Context = map[string]any{}
	}

	tmpDir, err := os.MkdirTemp("", "ew-provider-")
	if err != nil {
		return Request{}, nil, fmt.Errorf("could not create provider temp dir: %w", err)
	}

	schemaFile := filepath.Join(tmpDir, "resolution.schema.json")
	if err := os.WriteFile(schemaFile, []byte(resolutionJSONSchema), 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return Request{}, nil, fmt.Errorf("could not write schema file: %w", err)
	}

	working.Context["schema_file"] = schemaFile
	working.Context["output_file"] = filepath.Join(tmpDir, "resolution.output.json")
	working.Context["schema_json"] = compactSchema(resolutionJSONSchema)

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return working, cleanup, nil
}

func templateValues(req Request, model, thinking string) map[string]string {
	values := map[string]string{
		"model":    model,
		"thinking": thinking,
		"prompt":   req.Prompt,
		"mode":     req.Mode,
	}
	for key, value := range req.Context {
		if str, ok := value.(string); ok {
			values[key] = str
		}
	}
	return values
}

func renderTemplateArg(template string, values map[string]string) (string, bool) {
	matches := placeholderRegex.FindAllStringSubmatch(template, -1)
	rendered := template
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := match[1]
		value, ok := values[key]
		if !ok || strings.TrimSpace(value) == "" {
			return "", false
		}
		rendered = strings.ReplaceAll(rendered, "{"+key+"}", value)
	}
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return "", false
	}
	return rendered, true
}

func parseResolution(raw string) (Resolution, error) {
	trimmed := preprocessStructuredText(raw)
	if trimmed == "" {
		return Resolution{}, fmt.Errorf("empty response")
	}

	if parsed, err := decodeResolutionJSON(trimmed); err == nil {
		return parsed, nil
	}

	var wrapper map[string]any
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil {
		if value, ok := wrapper["result"]; ok {
			switch result := value.(type) {
			case string:
				if parsed, err := parseResolution(result); err == nil {
					return parsed, nil
				}
			case map[string]any:
				if bytes, err := json.Marshal(result); err == nil {
					if parsed, err := parseResolution(string(bytes)); err == nil {
						return parsed, nil
					}
				}
			}
		}
		if value, ok := wrapper["content"]; ok {
			switch content := value.(type) {
			case string:
				if parsed, err := parseResolution(content); err == nil {
					return parsed, nil
				}
			case []any:
				for _, item := range content {
					obj, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if text, ok := obj["text"].(string); ok {
						if parsed, err := parseResolution(text); err == nil {
							return parsed, nil
						}
					}
				}
			}
		}
	}

	if extracted, ok := extractJSONObject(trimmed); ok {
		if parsed, err := decodeResolutionJSON(extracted); err == nil {
			return parsed, nil
		}
	}

	return Resolution{}, fmt.Errorf("could not parse structured resolution")
}

func decodeResolutionJSON(raw string) (Resolution, error) {
	var result Resolution
	trimmed := preprocessStructuredText(raw)
	if err := json.Unmarshal([]byte(trimmed), &result); err == nil {
		if strings.TrimSpace(result.Action) != "" || strings.TrimSpace(result.Reason) != "" {
			return result, nil
		}
	}

	var generic map[string]any
	if err := json.Unmarshal([]byte(trimmed), &generic); err != nil {
		if extracted, ok := extractJSONObject(trimmed); ok && strings.TrimSpace(extracted) != strings.TrimSpace(trimmed) {
			return decodeResolutionJSON(extracted)
		}
		return Resolution{}, err
	}
	if adapted, ok := adaptLooseResolution(generic); ok {
		return adapted, nil
	}
	return Resolution{}, fmt.Errorf("missing action/reason fields")
}

func normalizeResolution(in Resolution) Resolution {
	out := in
	out.Action = normalizeAction(out.Action)
	out.Risk = strings.ToLower(strings.TrimSpace(out.Risk))
	if out.Risk != "low" && out.Risk != "medium" && out.Risk != "high" {
		out.Risk = "low"
	}
	if strings.TrimSpace(out.Reason) == "" {
		out.Reason = "provider suggestion"
	}
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	if out.Action == "run" && out.NeedsConfirmation {
		out.Action = "suggest"
	}
	return out
}

func normalizeAction(raw string) string {
	action := strings.ToLower(strings.TrimSpace(raw))
	switch action {
	case "run", "execute", "fix", "apply", "do":
		return "run"
	case "suggest", "recommend", "recommendation", "propose", "proposal", "resolve", "answer":
		return "suggest"
	case "ask", "confirm", "question", "clarify":
		return "ask"
	case "":
		return "ask"
	default:
		return "ask"
	}
}

func readPreferredOutput(req Request, stdout string) string {
	if req.Context != nil {
		if outputFile, ok := req.Context["output_file"].(string); ok && outputFile != "" {
			if bytes, err := os.ReadFile(outputFile); err == nil {
				content := strings.TrimSpace(string(bytes))
				if content != "" {
					return content
				}
			}
		}
	}
	return stdout
}

func compactSchema(schema string) string {
	var generic map[string]any
	if err := json.Unmarshal([]byte(schema), &generic); err != nil {
		return strings.TrimSpace(schema)
	}
	bytes, err := json.Marshal(generic)
	if err != nil {
		return strings.TrimSpace(schema)
	}
	return string(bytes)
}

func expandThinkingFlag(template, thinking string) []string {
	filled := strings.TrimSpace(strings.ReplaceAll(template, "{thinking}", thinking))
	if filled == "" {
		return nil
	}
	return strings.Fields(filled)
}

func extractJSONObject(raw string) (string, bool) {
	inString := false
	escape := false
	depth := 0
	start := -1
	for i, r := range raw {
		if escape {
			escape = false
			continue
		}
		if r == '\\' {
			escape = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if r == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if r == '}' {
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					return raw[start : i+1], true
				}
			}
		}
	}
	return "", false
}

func preprocessStructuredText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}

	withoutFence := strings.TrimPrefix(trimmed, "```")
	withoutFence = strings.TrimSpace(withoutFence)

	if idx := strings.IndexRune(withoutFence, '\n'); idx >= 0 {
		firstLine := strings.TrimSpace(withoutFence[:idx])
		if !strings.HasPrefix(firstLine, "{") && !strings.HasPrefix(firstLine, "[") {
			withoutFence = withoutFence[idx+1:]
		}
	}

	if idx := strings.LastIndex(withoutFence, "```"); idx >= 0 {
		withoutFence = withoutFence[:idx]
	}
	return strings.TrimSpace(withoutFence)
}

func truncate(text string, max int) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}

func adaptLooseResolution(payload map[string]any) (Resolution, bool) {
	if len(payload) == 0 {
		return Resolution{}, false
	}

	command := stringValue(payload["command"])
	reason := firstNonEmpty(
		stringValue(payload["reason"]),
		stringValue(payload["rationale"]),
		stringValue(payload["explanation"]),
		stringValue(payload["message"]),
	)
	if command == "" && reason == "" {
		return Resolution{}, false
	}

	action := strings.ToLower(strings.TrimSpace(stringValue(payload["action"])))
	if action == "" {
		action = "suggest"
	}
	risk := strings.ToLower(strings.TrimSpace(stringValue(payload["risk"])))
	if risk == "" {
		risk = "low"
	}

	confidence := 0.45
	if v, ok := numericValue(payload["confidence"]); ok {
		confidence = v
	} else if command != "" && reason != "" {
		switch action {
		case "run", "fix", "execute":
			confidence = 0.85
		case "suggest", "recommend", "recommendation":
			confidence = 0.75
		default:
			confidence = 0.60
		}
	}

	needsConfirmation := true
	if v, ok := boolValue(payload["needs_confirmation"]); ok {
		needsConfirmation = v
	}

	return Resolution{
		Action:            action,
		Command:           command,
		Reason:            reason,
		Risk:              risk,
		Confidence:        confidence,
		NeedsConfirmation: needsConfirmation,
	}, true
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		if parsed, err := v.Float64(); err == nil {
			return parsed, true
		}
	case string:
		if parsed, err := json.Number(strings.TrimSpace(v)).Float64(); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func boolValue(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		low := strings.ToLower(strings.TrimSpace(v))
		if low == "true" || low == "yes" || low == "1" {
			return true, true
		}
		if low == "false" || low == "no" || low == "0" {
			return false, true
		}
	}
	return false, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

const resolutionJSONSchema = `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["action", "command", "reason", "risk", "confidence", "needs_confirmation"],
  "properties": {
    "action": { "type": "string", "enum": ["ask", "suggest", "run"] },
    "command": { "type": "string" },
    "reason": { "type": "string" },
    "risk": { "type": "string", "enum": ["low", "medium", "high"] },
    "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
    "needs_confirmation": { "type": "boolean" }
  },
  "additionalProperties": false
}
`

func timeoutContext(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 120 * time.Second
	}
	return context.WithTimeout(parent, d)
}
