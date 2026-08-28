package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAI is a Simulator backed by an OpenAI-compatible Chat Completions API.
type OpenAI struct {
	APIKey     string
	Model      string
	BaseURL    string // defaults to https://api.openai.com
	HTTPClient *http.Client
	TaskPrompt string
	FactSheet  string
	// ReasoningEffort caps hidden reasoning on reasoning models ("minimal"
	// recommended); leave empty for endpoints that reject the parameter.
	ReasoningEffort string
}

func (o *OpenAI) Decide(ctx context.Context, agentMessage string) (Decision, error) {
	baseURL := o.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	// No completion-token cap: reasoning models spend completion tokens on
	// hidden reasoning first, and a cap can silently yield empty content.
	payload := map[string]any{
		"model": o.Model,
		"messages": []map[string]string{
			{"role": "system", "content": fmt.Sprintf(systemTemplate, o.TaskPrompt, o.FactSheet)},
			{"role": "user", "content": agentMessage},
		},
	}
	if o.ReasoningEffort != "" {
		payload["reasoning_effort"] = o.ReasoningEffort
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

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
		return Decision{}, fmt.Errorf("chat completions api: http %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Decision{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Decision{}, fmt.Errorf("empty response choices")
	}
	return parseDecision(parsed.Choices[0].Message.Content)
}
