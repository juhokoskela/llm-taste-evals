package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

// OpenCode drives the `opencode` CLI headlessly (`opencode run --format json`).
// Models are named provider/model (e.g. anthropic/claude-sonnet-5); provider
// auth comes from the usual env keys. This is the neutral-arm harness: one
// scaffold across vendors.
type OpenCode struct {
	// Effort maps to `opencode run --variant`, whose accepted values are
	// provider-specific (e.g. high, max, minimal).
	Effort string
}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) Start(ctx context.Context, workdir, model, prompt string) (*Turn, error) {
	return o.run(ctx, workdir, model, prompt, "")
}

func (o *OpenCode) Resume(ctx context.Context, workdir, model, sessionID, message string) (*Turn, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("opencode: resume requires a session id")
	}
	return o.run(ctx, workdir, model, message, sessionID)
}

func (o *OpenCode) run(ctx context.Context, workdir, model, message, sessionID string) (*Turn, error) {
	args := o.commandArgs(workdir, model, message, sessionID)

	out, err := runCLI(ctx, workdir, nil, "opencode", args...)
	if err != nil {
		return nil, err
	}
	turn, err := parseOpenCodeStream(out)
	if err != nil {
		return nil, fmt.Errorf("opencode: parse transcript: %w", err)
	}
	return turn, nil
}

func (o *OpenCode) commandArgs(workdir, model, message, sessionID string) []string {
	args := []string{"run", "--format", "json", "--auto", "--dir", workdir, "-m", model}
	if o.Effort != "" {
		args = append(args, "--variant", o.Effort)
	}
	if sessionID != "" {
		args = append(args, "-s", sessionID)
	}
	args = append(args, message)
	return args
}

type opencodeLine struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Error     struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	} `json:"error"`
	Part struct {
		Type  string `json:"type"`
		Tool  string `json:"tool"`
		Text  string `json:"text"`
		State struct {
			Input struct {
				Command  string `json:"command"`
				FilePath string `json:"filePath"`
				Path     string `json:"path"`
				Pattern  string `json:"pattern"`
			} `json:"input"`
		} `json:"state"`
		Tokens struct {
			Input  int `json:"input"`
			Output int `json:"output"`
			Cache  struct {
				Write int `json:"write"`
				Read  int `json:"read"`
			} `json:"cache"`
		} `json:"tokens"`
		Cost float64 `json:"cost"`
	} `json:"part"`
}

func parseOpenCodeStream(raw []byte) (*Turn, error) {
	turn := &Turn{Raw: raw}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var l opencodeLine
		if err := json.Unmarshal(line, &l); err != nil {
			continue
		}
		if l.SessionID != "" {
			turn.SessionID = l.SessionID
		}

		switch l.Type {
		case "error":
			turn.Events = append(turn.Events, events.Event{
				Type: events.TypeOther,
				Tool: "error",
				Text: truncate(l.Error.Name+": "+l.Error.Data.Message, 500),
			})
		case "text":
			turn.Final = l.Part.Text
			turn.Events = append(turn.Events, events.Event{
				Type: events.TypeAssistantMessage,
				Text: truncate(l.Part.Text, 2000),
			})
		case "tool_use":
			turn.Events = append(turn.Events, opencodeToolEvent(l))
		case "step_finish":
			// opencode reports cache reads/writes separately from input;
			// normalize to the total-input convention (see Usage).
			t := l.Part.Tokens
			turn.Usage.InputTokens += t.Input + t.Cache.Read + t.Cache.Write
			turn.Usage.CacheReadTokens += t.Cache.Read
			turn.Usage.CacheCreationTokens += t.Cache.Write
			turn.Usage.OutputTokens += t.Output
			turn.Usage.CostUSD += l.Part.Cost
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if turn.SessionID == "" {
		return nil, fmt.Errorf("no sessionID in transcript")
	}
	return turn, nil
}

func opencodeToolEvent(l opencodeLine) events.Event {
	in := l.Part.State.Input
	path := in.FilePath
	if path == "" {
		path = in.Path
	}

	ev := events.Event{Tool: l.Part.Tool, Path: path}
	switch l.Part.Tool {
	case "read", "grep", "glob", "list":
		ev.Type = events.TypeRead
	case "edit", "write", "patch":
		ev.Type = events.TypeEdit
	case "bash":
		ev.Type = events.ClassifyCommand(in.Command)
		ev.Command = in.Command
	default:
		ev.Type = events.TypeOther
	}
	return ev
}
