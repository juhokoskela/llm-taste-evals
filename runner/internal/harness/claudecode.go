package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

// ClaudeCode drives the `claude` CLI in headless mode
// (`claude -p --output-format stream-json`).
//
// ConfigDir, when set, is passed as CLAUDE_CONFIG_DIR so eval runs do not pick
// up the operator's global CLAUDE.md, settings, or MCP servers. Authenticate
// the isolated dir once beforehand, or export ANTHROPIC_API_KEY.
type ClaudeCode struct {
	ConfigDir string
	// Effort maps to `claude --effort` (low, medium, high, xhigh, max).
	Effort string
}

func (c *ClaudeCode) Name() string { return "claudecode" }

func (c *ClaudeCode) Start(ctx context.Context, workdir, model, prompt string) (*Turn, error) {
	return c.run(ctx, workdir, model, prompt, "")
}

func (c *ClaudeCode) Resume(ctx context.Context, workdir, model, sessionID, message string) (*Turn, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("claudecode: resume requires a session id")
	}
	return c.run(ctx, workdir, model, message, sessionID)
}

func (c *ClaudeCode) run(ctx context.Context, workdir, model, message, resumeID string) (*Turn, error) {
	args := []string{
		"-p", message,
		"--output-format", "stream-json",
		"--verbose",
		"--model", model,
		"--dangerously-skip-permissions",
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
	if c.Effort != "" {
		args = append(args, "--effort", c.Effort)
	}

	var env []string
	if c.ConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+c.ConfigDir)
	}

	out, err := runCLI(ctx, workdir, env, "claude", args...)
	if err != nil {
		return nil, err
	}
	turn, err := parseClaudeStream(out)
	if err != nil {
		return nil, fmt.Errorf("claudecode: parse transcript: %w", err)
	}
	return turn, nil
}

type claudeLine struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
	Message   struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Usage struct {
		InputTokens         int `json:"input_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
		OutputTokens        int `json:"output_tokens"`
	} `json:"usage"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type claudeToolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`
	Command      string `json:"command"`
}

func parseClaudeStream(raw []byte) (*Turn, error) {
	turn := &Turn{Raw: raw}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var l claudeLine
		if err := json.Unmarshal(line, &l); err != nil {
			continue // tolerate non-JSON noise on stdout
		}
		if l.SessionID != "" {
			turn.SessionID = l.SessionID
		}

		switch l.Type {
		case "assistant":
			for _, block := range l.Message.Content {
				switch block.Type {
				case "text":
					turn.Events = append(turn.Events, events.Event{
						Type: events.TypeAssistantMessage,
						Text: truncate(block.Text, 2000),
					})
				case "tool_use":
					turn.Events = append(turn.Events, claudeToolEvent(block.Name, block.Input))
				}
			}
		case "result":
			turn.Final = l.Result
			turn.Usage = Usage{
				// Claude Code reports uncached input; normalize to total.
				InputTokens:         l.Usage.InputTokens + l.Usage.CacheReadTokens + l.Usage.CacheCreationTokens,
				CacheReadTokens:     l.Usage.CacheReadTokens,
				CacheCreationTokens: l.Usage.CacheCreationTokens,
				OutputTokens:        l.Usage.OutputTokens,
				CostUSD:             l.TotalCostUSD,
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if turn.SessionID == "" {
		return nil, fmt.Errorf("no session_id in transcript")
	}
	return turn, nil
}

func claudeToolEvent(name string, rawInput json.RawMessage) events.Event {
	var in claudeToolInput
	_ = json.Unmarshal(rawInput, &in)
	path := in.FilePath
	if path == "" {
		path = in.NotebookPath
	}

	ev := events.Event{Tool: name, Path: path}
	switch name {
	case "Read", "Grep", "Glob":
		ev.Type = events.TypeRead
		if ev.Path == "" {
			ev.Path = in.Path
		}
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		ev.Type = events.TypeEdit
	case "Bash":
		ev.Type = events.ClassifyCommand(in.Command)
		ev.Command = in.Command
	default:
		ev.Type = events.TypeOther
	}
	return ev
}
