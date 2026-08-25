package judge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCodexUsage(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"thread.started","thread_id":"x"}
{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":80,"cache_write_input_tokens":5,"output_tokens":20}}
`)
	usage := parseCodexUsage(raw)
	if usage.InputTokens != 100 || usage.CacheReadTokens != 80 || usage.CacheCreationTokens != 5 || usage.OutputTokens != 20 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestJudgeEnvironmentDoesNotForwardSecrets(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	env := strings.Join(judgeEnvironment("/codex", "/runtime"), "\n")
	if strings.Contains(env, "secret") || strings.Contains(env, "OPENAI_API_KEY") || strings.Contains(env, "ANTHROPIC_API_KEY") {
		t.Fatalf("judge environment leaked provider credentials:\n%s", env)
	}
	for _, want := range []string{"CODEX_HOME=/codex", "HOME=/runtime", "TMPDIR=/runtime"} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q", want)
		}
	}
}

func TestValidateDecision(t *testing.T) {
	t.Parallel()
	valid := Decision{Winner: "tie", Confidence: "low", Summary: "equivalent", Reasons: []string{"same shape"}}
	if err := validateDecision(valid); err != nil {
		t.Fatalf("valid decision: %v", err)
	}
	valid.Winner = "reference"
	if err := validateDecision(valid); err == nil {
		t.Fatal("accepted invalid winner")
	}
}

func TestCodexVoterClearsEarlierAttemptErrorAfterSuccessfulRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	script := filepath.Join(dir, "fake-codex")
	source := `#!/bin/sh
state='` + state + `'
if [ ! -f "$state" ]; then
  touch "$state"
  exit 1
fi
previous=''
last=''
for argument in "$@"; do
  if [ "$previous" = '--output-last-message' ]; then
    last="$argument"
  fi
  previous="$argument"
done
printf '%s' '{"winner":"A","confidence":"high","summary":"A wins","reasons":["better"],"risks_a":[],"risks_b":[]}' > "$last"
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}'
`
	if err := os.WriteFile(script, []byte(source), 0o755); err != nil {
		t.Fatal(err)
	}
	auth := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(auth, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	voter, err := NewCodexVoter(CodexOptions{
		Command: script, Model: "test", Effort: "xhigh", Timeout: time.Second,
		MaxAttempts: 2, AuthFile: auth,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer voter.Close()
	result := voter.Vote(context.Background(), Job{Key: "job", prompt: "judge"})
	if result.Err != nil {
		t.Fatalf("successful retry retained earlier error: %v", result.Err)
	}
	if len(result.Attempts) != 2 || result.Attempts[0].Err == nil || result.Attempts[1].Err != nil {
		t.Fatalf("attempts = %+v", result.Attempts)
	}
}
