package safety

import (
	"strings"
	"testing"
)

func TestRedactTextRedactsAssignments(t *testing.T) {
	input := "AWS_SECRET_ACCESS_KEY=abc123 token: xyz password='hunter2'"
	got := RedactText(input)

	if strings.Contains(strings.ToLower(got), "abc123") || strings.Contains(strings.ToLower(got), "xyz") || strings.Contains(strings.ToLower(got), "hunter2") {
		t.Fatalf("expected secrets to be redacted, got %q", got)
	}
	if !strings.Contains(got, "AWS_SECRET_ACCESS_KEY=<redacted>") {
		t.Fatalf("expected AWS secret assignment to be redacted, got %q", got)
	}
}

func TestRedactTextRedactsBearerToken(t *testing.T) {
	input := "Authorization: Bearer verysecrettoken"
	got := RedactText(input)
	if strings.Contains(got, "verysecrettoken") {
		t.Fatalf("expected bearer token to be redacted, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "authorization: bearer <redacted>") {
		t.Fatalf("expected normalized bearer redaction, got %q", got)
	}
}

func TestRedactTextLeavesRegularCommands(t *testing.T) {
	input := "git status && ls -la"
	got := RedactText(input)
	if got != input {
		t.Fatalf("expected non-secret text unchanged, got %q", got)
	}
}

func TestRedactTextRedactsFlagStyleSecrets(t *testing.T) {
	input := "mycli --password hunter2 --token=abc123 --api-key \"xyz\" --user bob"
	got := RedactText(input)

	if strings.Contains(strings.ToLower(got), "hunter2") || strings.Contains(strings.ToLower(got), "abc123") || strings.Contains(strings.ToLower(got), "xyz") {
		t.Fatalf("expected flag-style secrets to be redacted, got %q", got)
	}
	if !strings.Contains(got, "--password <redacted>") {
		t.Fatalf("expected --password redaction, got %q", got)
	}
	if !strings.Contains(got, "--token=<redacted>") {
		t.Fatalf("expected --token= redaction, got %q", got)
	}
	if !strings.Contains(got, "--api-key <redacted>") {
		t.Fatalf("expected --api-key redaction, got %q", got)
	}
	if !strings.Contains(got, "--user bob") {
		t.Fatalf("expected non-secret flags to remain unchanged, got %q", got)
	}
}

func TestRedactTextRedactsShortSecretFlags(t *testing.T) {
	input := "mycli login -p hunter2 -k=abc123 -t \"tok-xyz\" -s 's3cr3t' --port 5432"
	got := RedactText(input)

	if strings.Contains(strings.ToLower(got), "hunter2") ||
		strings.Contains(strings.ToLower(got), "abc123") ||
		strings.Contains(strings.ToLower(got), "tok-xyz") ||
		strings.Contains(strings.ToLower(got), "s3cr3t") {
		t.Fatalf("expected short-flag secrets to be redacted, got %q", got)
	}
	if !strings.Contains(got, "-p <redacted>") {
		t.Fatalf("expected -p redaction, got %q", got)
	}
	if !strings.Contains(got, "-k=<redacted>") {
		t.Fatalf("expected -k= redaction, got %q", got)
	}
	if !strings.Contains(got, "-t <redacted>") {
		t.Fatalf("expected -t redaction, got %q", got)
	}
	if !strings.Contains(got, "-s <redacted>") {
		t.Fatalf("expected -s redaction, got %q", got)
	}
	if !strings.Contains(got, "--port 5432") {
		t.Fatalf("expected non-secret args to remain unchanged, got %q", got)
	}
}

func TestRedactTextRedactsPositionalSecretKeywords(t *testing.T) {
	input := "mycli login token abc123 password \"hunter2\" --profile dev"
	got := RedactText(input)

	if strings.Contains(strings.ToLower(got), "abc123") || strings.Contains(strings.ToLower(got), "hunter2") {
		t.Fatalf("expected positional keyword secrets to be redacted, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "token <redacted>") {
		t.Fatalf("expected positional token redaction, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "password <redacted>") {
		t.Fatalf("expected positional password redaction, got %q", got)
	}
	if !strings.Contains(got, "--profile dev") {
		t.Fatalf("expected non-secret args to remain unchanged, got %q", got)
	}
}

func TestRedactTextRedactsPrefixedPositionalSecretKeyNames(t *testing.T) {
	input := "aws configure set aws_secret_access_key ABC123 --profile dev"
	got := RedactText(input)

	if strings.Contains(got, "ABC123") {
		t.Fatalf("expected prefixed positional secret value to be redacted, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "aws_secret_access_key <redacted>") {
		t.Fatalf("expected aws_secret_access_key positional redaction, got %q", got)
	}
	if !strings.Contains(got, "--profile dev") {
		t.Fatalf("expected non-secret args to remain unchanged, got %q", got)
	}
}
