// Package harness adapts vendor agent CLIs behind one interface and
// normalizes their transcripts into the common event schema.
package harness

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/agentexec"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

// Usage is normalized across harnesses: InputTokens is TOTAL input including
// cache reads and writes (vendors disagree — Claude Code reports uncached
// input separately from cache fields, codex folds cache into input_tokens;
// adapters convert to this convention). The cache fields are informational
// subsets of InputTokens.
type Usage struct {
	InputTokens         int     `json:"input_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	OutputTokens        int     `json:"output_tokens"`
	CostUSD             float64 `json:"cost_usd,omitempty"`
}

// Turn is one agent turn: everything between handing the agent a message and
// the agent yielding back to the user.
type Turn struct {
	SessionID string
	Final     string
	Events    []events.Event
	Raw       []byte
	Usage     Usage
}

type Harness interface {
	Name() string
	// Start begins a fresh session in workdir with the task prompt.
	Start(ctx context.Context, workdir, model, prompt string) (*Turn, error)
	// Resume continues an existing session with a user message.
	Resume(ctx context.Context, workdir, model, sessionID, message string) (*Turn, error)
}

// New builds a harness adapter. effort is passed through verbatim in the
// harness's own vocabulary (claudecode: low..max, codex: minimal..xhigh);
// empty means the harness default.
func New(name, effort string) (Harness, error) {
	switch name {
	case "claudecode":
		return &ClaudeCode{ConfigDir: os.Getenv("EVAL_CLAUDE_CONFIG_DIR"), Effort: effort}, nil
	case "codex":
		return &Codex{Home: os.Getenv("EVAL_CODEX_HOME"), Effort: effort}, nil
	case "opencode":
		return &OpenCode{Effort: effort}, nil
	default:
		return nil, fmt.Errorf("unknown harness %q (want claudecode, codex, or opencode)", name)
	}
}

// runCLI executes an agent CLI in workdir and returns its stdout. Stderr is
// included in the error because agent CLIs report auth and usage problems
// there.
func runCLI(ctx context.Context, workdir string, env []string, name string, args ...string) ([]byte, error) {
	cmd, err := agentexec.Command(ctx, workdir, env, name, args...)
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	cleanupErr := agentexec.KillProcessGroup(cmd)
	if runErr != nil {
		if cleanupErr != nil {
			return stdout.Bytes(), fmt.Errorf("%s %v: %w; process cleanup: %v\nstderr: %s\nstdout: %s", name, args, runErr, cleanupErr, truncate(stderr.String(), 2000), truncate(stdout.String(), 2000))
		}
		return stdout.Bytes(), fmt.Errorf("%s %v: %w\nstderr: %s\nstdout: %s", name, args, runErr, truncate(stderr.String(), 2000), truncate(stdout.String(), 2000))
	}
	if cleanupErr != nil {
		return stdout.Bytes(), cleanupErr
	}
	return stdout.Bytes(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
