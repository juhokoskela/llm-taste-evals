package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
)

type CodexOptions struct {
	Command     string
	Model       string
	Effort      string
	Timeout     time.Duration
	MaxAttempts int
	AuthFile    string
}

type CodexVoter struct {
	command     string
	model       string
	effort      string
	timeout     time.Duration
	maxAttempts int
	root        string
	home        string
	work        string
	schema      string
}

func NewCodexVoter(opts CodexOptions) (*CodexVoter, error) {
	if opts.Command == "" {
		opts.Command = "codex"
	}
	if opts.Model == "" || opts.Effort == "" {
		return nil, fmt.Errorf("codex model and effort are required")
	}
	if opts.Timeout <= 0 {
		return nil, fmt.Errorf("codex timeout must be positive")
	}
	if opts.MaxAttempts < 1 {
		return nil, fmt.Errorf("codex max attempts must be positive")
	}
	if opts.AuthFile == "" {
		base := os.Getenv("CODEX_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("find user home: %w", err)
			}
			base = filepath.Join(home, ".codex")
		}
		opts.AuthFile = filepath.Join(base, "auth.json")
	}

	root, err := os.MkdirTemp("", "llm-taste-judge-")
	if err != nil {
		return nil, fmt.Errorf("create judge runtime: %w", err)
	}
	v := &CodexVoter{
		command:     opts.Command,
		model:       opts.Model,
		effort:      opts.Effort,
		timeout:     opts.Timeout,
		maxAttempts: opts.MaxAttempts,
		root:        root,
		home:        filepath.Join(root, "codex-home"),
		work:        filepath.Join(root, "work"),
		schema:      filepath.Join(root, "decision.schema.json"),
	}
	if err := v.prepare(opts.AuthFile); err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

func (v *CodexVoter) prepare(authFile string) error {
	for _, dir := range []string{v.home, v.work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create judge runtime directory: %w", err)
		}
	}
	auth, err := os.ReadFile(authFile)
	if err != nil {
		return fmt.Errorf("read Codex auth from %s: %w", authFile, err)
	}
	if err := os.WriteFile(filepath.Join(v.home, "auth.json"), auth, 0o600); err != nil {
		return fmt.Errorf("seed isolated Codex auth: %w", err)
	}
	if err := os.WriteFile(v.schema, []byte(decisionSchema), 0o600); err != nil {
		return fmt.Errorf("write decision schema: %w", err)
	}
	return nil
}

func (v *CodexVoter) Close() {
	if v == nil || v.root == "" {
		return
	}
	_ = os.RemoveAll(v.root)
	v.root = ""
}

func (v *CodexVoter) Vote(ctx context.Context, job Job) VoteResult {
	result := VoteResult{Job: job}
	for attemptNo := 1; attemptNo <= v.maxAttempts; attemptNo++ {
		attempt := v.attempt(ctx, job, attemptNo)
		result.Attempts = append(result.Attempts, attempt)
		if attempt.Err == nil {
			if err := json.Unmarshal(attempt.Response, &result.Decision); err != nil {
				attempt.Err = fmt.Errorf("parse decision: %w", err)
				result.Attempts[len(result.Attempts)-1] = attempt
			} else if err := validateDecision(result.Decision); err != nil {
				attempt.Err = err
				result.Attempts[len(result.Attempts)-1] = attempt
			} else {
				result.Err = nil
				return result
			}
		}
		result.Err = attempt.Err
		if attemptNo < v.maxAttempts && waitErr(ctx, time.Duration(attemptNo)*5*time.Second) != nil {
			result.Err = context.Cause(ctx)
			return result
		}
	}
	return result
}

func (v *CodexVoter) attempt(parent context.Context, job Job, attemptNo int) Attempt {
	started := time.Now().UTC()
	attempt := Attempt{Number: attemptNo, StartedAt: started}
	ctx, cancel := context.WithTimeout(parent, v.timeout)
	defer cancel()

	lastMessage := filepath.Join(v.root, fmt.Sprintf("%s-attempt-%d.json", job.Key, attemptNo))
	args := []string{
		"exec",
		"--json",
		"--ephemeral",
		"--ignore-user-config",
		"--ignore-rules",
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"--disable", "shell_tool",
		"-C", v.work,
		"-m", v.model,
		"-c", "model_reasoning_effort=" + v.effort,
		"-c", "approval_policy=never",
		"-c", "web_search=disabled",
		"-c", "tools.web_search=false",
		"-c", "tools.view_image=false",
		"-c", "agents.enabled=false",
		"-c", "apps._default.enabled=false",
		"-c", "project_doc_max_bytes=0",
		"-c", "history.persistence=none",
		"-c", "check_for_update_on_startup=false",
		"--output-schema", v.schema,
		"--output-last-message", lastMessage,
		"-",
	}
	cmd := exec.CommandContext(ctx, v.command, args...)
	cmd.Dir = v.work
	cmd.Env = judgeEnvironment(v.home, v.root)
	cmd.Stdin = strings.NewReader(job.prompt)
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	attempt.Duration = time.Since(started)
	attempt.Raw = stdout.Bytes()
	attempt.Usage = parseCodexUsage(attempt.Raw)
	attempt.Response, _ = os.ReadFile(lastMessage)
	_ = os.Remove(lastMessage)

	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			attempt.Err = cause
		} else {
			attempt.Err = fmt.Errorf("codex exec: %w: %s", err, truncate(stderr.String(), 2000))
		}
		return attempt
	}
	if len(attempt.Response) == 0 {
		attempt.Err = fmt.Errorf("codex returned no final message: %s", truncate(stderr.String(), 2000))
	}
	return attempt
}

func judgeEnvironment(codexHome, runtimeRoot string) []string {
	env := []string{
		"CODEX_HOME=" + codexHome,
		"HOME=" + runtimeRoot,
		"TMPDIR=" + runtimeRoot,
	}
	allowed := map[string]bool{
		"PATH": true, "LANG": true, "LC_ALL": true, "TERM": true,
		"SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
		"http_proxy": true, "https_proxy": true, "no_proxy": true,
	}
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if allowed[name] {
			env = append(env, item)
		}
	}
	return env
}

func validateDecision(decision Decision) error {
	if decision.Winner != "A" && decision.Winner != "B" && decision.Winner != "tie" {
		return fmt.Errorf("invalid winner %q", decision.Winner)
	}
	if decision.Confidence != "low" && decision.Confidence != "medium" && decision.Confidence != "high" {
		return fmt.Errorf("invalid confidence %q", decision.Confidence)
	}
	if strings.TrimSpace(decision.Summary) == "" || len(decision.Reasons) == 0 {
		return fmt.Errorf("decision lacks a summary or reasons")
	}
	return nil
}

func parseCodexUsage(raw []byte) harness.Usage {
	var usage harness.Usage
	for line := range bytes.Lines(raw) {
		var event struct {
			Type  string `json:"type"`
			Usage struct {
				InputTokens      int `json:"input_tokens"`
				CachedInput      int `json:"cached_input_tokens"`
				CacheWriteTokens int `json:"cache_write_input_tokens"`
				OutputTokens     int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &event) == nil && event.Type == "turn.completed" {
			usage.InputTokens += event.Usage.InputTokens
			usage.CacheReadTokens += event.Usage.CachedInput
			usage.CacheCreationTokens += event.Usage.CacheWriteTokens
			usage.OutputTokens += event.Usage.OutputTokens
		}
	}
	return usage
}

func waitErr(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

const decisionSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["winner", "confidence", "summary", "reasons", "risks_a", "risks_b"],
  "properties": {
    "winner": {"type": "string", "enum": ["A", "B", "tie"]},
    "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
    "summary": {"type": "string"},
    "reasons": {
      "type": "array",
      "minItems": 1,
      "maxItems": 4,
      "items": {"type": "string"}
    },
    "risks_a": {
      "type": "array",
      "maxItems": 3,
      "items": {"type": "string"}
    },
    "risks_b": {
      "type": "array",
      "maxItems": 3,
      "items": {"type": "string"}
    }
  }
}`
