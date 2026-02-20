package safety

import "regexp"

type redactionRule struct {
	pattern     *regexp.Regexp
	replacement string
}

var secretRedactionRules = []redactionRule{
	{
		pattern:     regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:token|secret|password|passwd|api[_-]?key|access[_-]?key)[a-z0-9_]*)\s*=\s*([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1=<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:token|secret|password|passwd|api[_-]?key|access[_-]?key)[a-z0-9_]*)\s*:\s*([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1=<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer)\s+([^\s"']+)`),
		replacement: `$1 <redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b([a-z0-9_-]*(?:token|secret|password|passwd|api[_-]?key|access[_-]?key)[a-z0-9_-]*)\b\s+([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1 <redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)(--[a-z0-9_-]*(?:token|secret|password|passwd|api[_-]?key|access[_-]?key|authorization)[a-z0-9_-]*)\s*=\s*([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1=<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)(--[a-z0-9_-]*(?:token|secret|password|passwd|api[_-]?key|access[_-]?key|authorization)[a-z0-9_-]*)\s+([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1 <redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)(^|\s)(-(?:p|k|t|s))\s*=\s*([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1$2=<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)(^|\s)(-(?:p|k|t|s))\s+([^\s"']+|"[^"]*"|'[^']*')`),
		replacement: `$1$2 <redacted>`,
	},
}

// RedactText scrubs common secret/token/password patterns from free-form text.
func RedactText(input string) string {
	redacted := input
	for _, rule := range secretRedactionRules {
		redacted = rule.pattern.ReplaceAllString(redacted, rule.replacement)
	}
	return redacted
}
