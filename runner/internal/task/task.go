// Package task loads task definitions (task.json + prompt file) from a task
// directory.
package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type QA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Task struct {
	Name         string `json:"name"`
	Version      int    `json:"version"`
	Repo         string `json:"repo"`
	BaseCommit   string `json:"base_commit"`
	PromptFile   string `json:"prompt_file"`
	ChecksScript string `json:"checks_script"`
	FactSheet    []QA   `json:"fact_sheet"`
	MaxTurns     int    `json:"max_turns"`
	TurnTimeout  int    `json:"turn_timeout_minutes"`

	dir    string
	prompt string
}

func Load(dir string) (*Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "task.json"))
	if err != nil {
		return nil, fmt.Errorf("read task.json: %w", err)
	}
	var t Task
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("parse task.json: %w", err)
	}
	if t.Name == "" || t.Repo == "" || t.BaseCommit == "" {
		return nil, fmt.Errorf("task.json must set name, repo, and base_commit")
	}
	if t.PromptFile == "" {
		t.PromptFile = "task.md"
	}
	if t.ChecksScript == "" {
		t.ChecksScript = "checks.sh"
	}
	if t.MaxTurns == 0 {
		t.MaxTurns = 8
	}
	if t.TurnTimeout == 0 {
		t.TurnTimeout = 30
	}
	t.dir = dir

	prompt, err := os.ReadFile(filepath.Join(dir, t.PromptFile))
	if err != nil {
		return nil, fmt.Errorf("read prompt file: %w", err)
	}
	t.prompt = string(prompt)
	return &t, nil
}

func (t *Task) Dir() string        { return t.dir }
func (t *Task) Prompt() string     { return t.prompt }
func (t *Task) ChecksPath() string { return filepath.Join(t.dir, t.ChecksScript) }
func (t *Task) TurnTimeoutDur() time.Duration {
	return time.Duration(t.TurnTimeout) * time.Minute
}

// FactSheetText renders the fact sheet for the simulator's system prompt.
func (t *Task) FactSheetText() string {
	if len(t.FactSheet) == 0 {
		return "(empty — answer every question with \"Use your judgment.\")"
	}
	var b strings.Builder
	for _, qa := range t.FactSheet {
		fmt.Fprintf(&b, "Q: %s\nA: %s\n\n", qa.Question, qa.Answer)
	}
	return strings.TrimSpace(b.String())
}
