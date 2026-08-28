package main

import (
	"strings"
	"testing"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/simulator"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/task"
)

func TestBuildSimulatorOpenRouter(t *testing.T) {
	clearProviderCredentials(t)
	t.Setenv("OPENROUTER_API_KEY", "fixture-key")

	tests := []struct {
		name      string
		model     string
		wantModel string
	}{
		{
			name:      "default model",
			wantModel: "openai/gpt-5-mini",
		},
		{
			name:      "explicit model",
			model:     "openrouter/anthropic/claude-sonnet-4",
			wantModel: "anthropic/claude-sonnet-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSimulator(tt.model, &task.Task{})
			if err != nil {
				t.Fatalf("buildSimulator() error = %v", err)
			}

			sim, ok := got.(*simulator.OpenAI)
			if !ok {
				t.Fatalf("buildSimulator() type = %T, want *simulator.OpenAI", got)
			}
			if sim.APIKey != "fixture-key" {
				t.Errorf("APIKey = %q, want fixture key", sim.APIKey)
			}
			if sim.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", sim.Model, tt.wantModel)
			}
			if sim.BaseURL != "https://openrouter.ai/api" {
				t.Errorf("BaseURL = %q, want OpenRouter API base URL", sim.BaseURL)
			}
			if sim.ReasoningEffort != "minimal" {
				t.Errorf("ReasoningEffort = %q, want minimal", sim.ReasoningEffort)
			}
		})
	}
}

func TestBuildSimulatorOpenRouterRequiresKey(t *testing.T) {
	clearProviderCredentials(t)

	_, err := buildSimulator("openrouter/openai/gpt-5-mini", &task.Task{})
	if err == nil || !strings.Contains(err.Error(), "needs OPENROUTER_API_KEY") {
		t.Fatalf("buildSimulator() error = %v, want missing OPENROUTER_API_KEY error", err)
	}
}

func TestBuildSimulatorOpenRouterRequiresModelID(t *testing.T) {
	clearProviderCredentials(t)
	t.Setenv("OPENROUTER_API_KEY", "fixture-key")

	_, err := buildSimulator("openrouter/", &task.Task{})
	if err == nil || !strings.Contains(err.Error(), "must include an OpenRouter model id") {
		t.Fatalf("buildSimulator() error = %v, want missing model id error", err)
	}
}

func clearProviderCredentials(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"OPENAI_API_KEY",
		"FIREWORKS_API_KEY",
		"OPENROUTER_API_KEY",
	} {
		t.Setenv(name, "")
	}
}
