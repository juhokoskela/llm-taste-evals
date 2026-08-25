package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

// Codex drives the `codex` CLI headlessly (`codex exec --json`).
//
// Home, when set, is passed as CODEX_HOME so eval runs do not pick up the
// operator's global ~/.codex/AGENTS.md or config. Copy auth.json into the
// isolated home once beforehand.
//
// The --json event shape (thread/item events) is tied to the pinned CLI
// version; unknown items normalize to "other" and the raw transcript is
// always archived, so a version bump degrades gracefully instead of failing.
type Codex struct {
	Home string
	// Effort maps to codex's model_reasoning_effort config
	// (minimal, low, medium, high, xhigh).
	Effort string
}

func (c *Codex) Name() string { return "codex" }

// bypassFlag disables codex's own Landlock/seatbelt sandbox, which is
// unreliable inside containers; the eval's isolation comes from the container
// and egress proxy instead. Do not use this adapter on a bare host.
const bypassFlag = "--dangerously-bypass-approvals-and-sandbox"

func (c *Codex) Start(ctx context.Context, workdir, model, prompt string) (*Turn, error) {
	args := append([]string{"exec", "--json", bypassFlag, "-m", model}, c.effortArgs()...)
	return c.run(ctx, workdir, append(args, prompt))
}

func (c *Codex) Resume(ctx context.Context, workdir, model, sessionID, message string) (*Turn, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("codex: resume requires a thread id")
	}
	args := append([]string{"exec", "resume", sessionID, "--json", bypassFlag, "-m", model}, c.effortArgs()...)
	return c.run(ctx, workdir, append(args, message))
}

func (c *Codex) effortArgs() []string {
	if c.Effort == "" {
		return nil
	}
	return []string{"-c", "model_reasoning_effort=" + c.Effort}
}

func (c *Codex) run(ctx context.Context, workdir string, args []string) (*Turn, error) {
	var env []string
	if c.Home != "" {
		env = append(env, "CODEX_HOME="+c.Home)
	}

	out, err := runCLI(ctx, workdir, env, "codex", args...)
	if err != nil {
		return nil, err
	}
	turn, err := parseCodexStream(out)
	if err != nil {
		return nil, fmt.Errorf("codex: parse transcript: %w", err)
	}
	return turn, nil
}

type codexLine struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	ThreadID string `json:"thread_id"`
	Item     struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Command string `json:"command"`
		Changes []struct {
			Path string `json:"path"`
		} `json:"changes"`
	} `json:"item"`
	Usage struct {
		InputTokens      int `json:"input_tokens"`
		CachedInput      int `json:"cached_input_tokens"`
		CacheWriteTokens int `json:"cache_write_input_tokens"`
		OutputTokens     int `json:"output_tokens"`
	} `json:"usage"`
}

func parseCodexStream(raw []byte) (*Turn, error) {
	turn := &Turn{Raw: raw}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var l codexLine
		if err := json.Unmarshal(line, &l); err != nil {
			continue // codex prints some human-readable noise on stdout
		}

		switch l.Type {
		case "error":
			turn.Events = append(turn.Events, events.Event{
				Type: events.TypeOther,
				Tool: "error",
				Text: truncate(l.Message, 500),
			})
		case "thread.started":
			turn.SessionID = l.ThreadID
		case "turn.completed":
			// codex's input_tokens already includes cached reads/writes,
			// matching the normalized Usage convention.
			turn.Usage.InputTokens += l.Usage.InputTokens
			turn.Usage.CacheReadTokens += l.Usage.CachedInput
			turn.Usage.CacheCreationTokens += l.Usage.CacheWriteTokens
			turn.Usage.OutputTokens += l.Usage.OutputTokens
		case "item.completed":
			switch l.Item.Type {
			case "agent_message":
				turn.Final = l.Item.Text
				turn.Events = append(turn.Events, events.Event{
					Type: events.TypeAssistantMessage,
					Text: truncate(l.Item.Text, 2000),
				})
			case "command_execution":
				turn.Events = append(turn.Events, events.Event{
					Type:    events.ClassifyCommand(l.Item.Command),
					Tool:    "command_execution",
					Command: l.Item.Command,
				})
			case "file_change":
				for _, ch := range l.Item.Changes {
					turn.Events = append(turn.Events, events.Event{
						Type: events.TypeEdit,
						Tool: "file_change",
						Path: ch.Path,
					})
				}
			case "reasoning":
				// internal deliberation, not an action; skip
			default:
				turn.Events = append(turn.Events, events.Event{
					Type: events.TypeOther,
					Tool: l.Item.Type,
				})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if turn.SessionID == "" {
		return nil, fmt.Errorf("no thread_id in transcript")
	}
	return turn, nil
}
