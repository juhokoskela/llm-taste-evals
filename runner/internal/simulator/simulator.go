// Package simulator implements the turn-boundary user simulator: after each
// agent turn it decides whether to answer a question, nudge the agent to
// continue, or end the run.
package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Action string

const (
	ActionAnswer Action = "answer"
	ActionNudge  Action = "nudge"
	ActionEnd    Action = "end"
)

type Decision struct {
	Action Action `json:"action"`
	Reply  string `json:"reply"`
}

type Simulator interface {
	Decide(ctx context.Context, agentMessage string) (Decision, error)
}

// Anthropic is a Simulator backed by the Anthropic Messages API. Use a model
// from a different family than the contestant where possible; a small model
// is sufficient for fact-sheet lookups.
type Anthropic struct {
	APIKey     string
	Model      string
	BaseURL    string // defaults to https://api.anthropic.com
	HTTPClient *http.Client
	TaskPrompt string
	FactSheet  string
}

const systemTemplate = `You simulate the human user who filed the following request to a coding agent:

<request>
%s
</request>

You hold this fact sheet. It is the ONLY source of answers to the agent's questions:

<fact_sheet>
%s
</fact_sheet>

The agent has just ended a turn. Classify its message and respond with a single JSON object, nothing else:
{"action": "answer" | "nudge" | "end", "reply": "<message to send the agent, empty when action is end>"}

Rules:
- "answer": the agent asked you something. Answer in one or two sentences using ONLY the fact sheet. If the fact sheet does not cover it, reply exactly "Use your judgment."
- "end": the agent states the work is complete (or asks for nothing and clearly has nothing left to do).
- "nudge": anything else — the agent stopped mid-task without a question. Reply with a short prompt to continue, e.g. "Please continue. Say DONE when you are finished."
- Never volunteer fact-sheet content the agent did not ask about. Never give implementation hints.`

func (a *Anthropic) Decide(ctx context.Context, agentMessage string) (Decision, error) {
	baseURL := a.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	// The Messages API requires max_tokens; keep it far above any realistic
	// decision size so it never truncates.
	body, err := json.Marshal(map[string]any{
		"model":      a.Model,
		"max_tokens": 8192,
		"system":     fmt.Sprintf(systemTemplate, a.TaskPrompt, a.FactSheet),
		"messages": []map[string]string{
			{"role": "user", "content": agentMessage},
		},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return Decision{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Decision{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Decision{}, fmt.Errorf("anthropic api: http %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Decision{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Content) == 0 {
		return Decision{}, fmt.Errorf("empty response content")
	}
	return parseDecision(parsed.Content[0].Text)
}

func parseDecision(text string) (Decision, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return Decision{}, fmt.Errorf("no JSON object in simulator output: %q", text)
	}
	var d Decision
	if err := json.Unmarshal([]byte(text[start:end+1]), &d); err != nil {
		return Decision{}, fmt.Errorf("decode simulator decision: %w", err)
	}
	switch d.Action {
	case ActionAnswer, ActionNudge, ActionEnd:
		return d, nil
	default:
		return Decision{}, fmt.Errorf("invalid simulator action %q", d.Action)
	}
}

// Static replays a fixed decision sequence; it exists for tests and dry runs.
type Static struct {
	Decisions []Decision
	next      int
}

func (s *Static) Decide(context.Context, string) (Decision, error) {
	if s.next >= len(s.Decisions) {
		return Decision{Action: ActionEnd}, nil
	}
	d := s.Decisions[s.next]
	s.next++
	return d, nil
}
