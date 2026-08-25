// Package events defines the harness-agnostic transcript event schema and
// the trajectory metrics computed from it.
package events

import (
	"regexp"
	"strings"
)

type Type string

const (
	TypeAssistantMessage Type = "assistant_message"
	TypeRead             Type = "read"
	TypeEdit             Type = "edit"
	TypeShell            Type = "shell"
	TypeTestRun          Type = "test_run"
	TypeQuestion         Type = "question"
	TypeUserMessage      Type = "user_message"
	TypeTurnEnd          Type = "turn_end"
	TypeOther            Type = "other"
)

type Event struct {
	Seq     int    `json:"seq"`
	Turn    int    `json:"turn"`
	Type    Type   `json:"type"`
	Tool    string `json:"tool,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Text    string `json:"text,omitempty"`
}

// The boundary is "any non-word character" rather than whitespace: harnesses
// wrap commands in shells (`/bin/sh -lc 'go test ./...'`), so quotes and
// slashes commonly precede the verb.
var testCommandRE = regexp.MustCompile(`(^|[^\w-])(go test|go vet|gofmt|golangci-lint|pytest|npm (run )?test|make test|cargo test)(\s|$)`)

// ClassifyCommand distinguishes verification commands from other shell use.
func ClassifyCommand(cmd string) Type {
	if testCommandRE.MatchString(cmd) {
		return TypeTestRun
	}
	return TypeShell
}

var readCommandRE = regexp.MustCompile(`(^|[^\w-])(cat|less|head|tail|sed -n|rg|grep|ls|find|git (log|show|diff|grep|blame))\b`)

// IsReadCommand reports whether a shell command is plausibly a file read.
// Used for best-effort read tracking on harnesses that read via the shell.
func IsReadCommand(cmd string) bool {
	return readCommandRE.MatchString(cmd)
}

type Metrics struct {
	Turns             int      `json:"turns"`
	ToolCalls         int      `json:"tool_calls"`
	Reads             int      `json:"reads"`
	Edits             int      `json:"edits"`
	ShellCommands     int      `json:"shell_commands"`
	TestRuns          int      `json:"test_runs"`
	QuestionsAsked    int      `json:"questions_asked"`
	EditedWithoutRead []string `json:"edited_without_read"`
}

// Compute derives trajectory metrics from a normalized event stream.
//
// EditedWithoutRead is best-effort: it only sees reads that carry an explicit
// path (dedicated read tools, or shell reads whose command mentions the path),
// so treat it as a signal to inspect the transcript, not a verdict.
func Compute(evs []Event) Metrics {
	var m Metrics
	read := make(map[string]bool)
	editedUnread := make(map[string]bool)

	for _, ev := range evs {
		if ev.Turn > m.Turns {
			m.Turns = ev.Turn
		}
		switch ev.Type {
		case TypeRead:
			m.ToolCalls++
			m.Reads++
			if ev.Path != "" {
				read[ev.Path] = true
			}
		case TypeEdit:
			m.ToolCalls++
			m.Edits++
			if ev.Path != "" && !read[ev.Path] && !wasMentioned(ev.Path, evs, ev.Seq) {
				editedUnread[ev.Path] = true
			}
		case TypeShell:
			m.ToolCalls++
			m.ShellCommands++
		case TypeTestRun:
			m.ToolCalls++
			m.TestRuns++
		case TypeQuestion:
			m.QuestionsAsked++
		case TypeOther:
			m.ToolCalls++
		}
	}

	for path := range editedUnread {
		m.EditedWithoutRead = append(m.EditedWithoutRead, path)
	}
	return m
}

// wasMentioned reports whether path appeared in any earlier shell command,
// which counts as a best-effort read for harnesses that read via the shell.
func wasMentioned(path string, evs []Event, before int) bool {
	base := path[strings.LastIndex(path, "/")+1:]
	for _, ev := range evs {
		if ev.Seq >= before {
			break
		}
		if (ev.Type == TypeShell || ev.Type == TypeTestRun) && strings.Contains(ev.Command, base) {
			return true
		}
	}
	return false
}
