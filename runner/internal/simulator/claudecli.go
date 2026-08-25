package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ClaudeCLI is a Simulator that runs `claude -p` headlessly, so subscription
// auth (CLAUDE_CODE_OAUTH_TOKEN) covers the simulator too and no vendor API
// key is needed. Slower than the HTTP simulators; prefer those when a key is
// available.
type ClaudeCLI struct {
	Model      string
	TaskPrompt string
	FactSheet  string
}

func (c *ClaudeCLI) Decide(ctx context.Context, agentMessage string) (Decision, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	prompt := fmt.Sprintf(systemTemplate, c.TaskPrompt, c.FactSheet) +
		"\n\nThe agent's message:\n\n" + agentMessage

	cmd := exec.CommandContext(ctx, "claude", "-p", prompt,
		"--model", c.Model, "--output-format", "json")
	// A neutral working directory: the simulator must not pick up the eval
	// workspace's repo context.
	cmd.Dir = os.TempDir()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Decision{}, fmt.Errorf("claude cli simulator: %w: %s", err, stderr.String())
	}

	var parsed struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return Decision{}, fmt.Errorf("decode claude cli output: %w", err)
	}
	return parseDecision(parsed.Result)
}
