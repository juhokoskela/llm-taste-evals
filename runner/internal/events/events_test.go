package events

import (
	"testing"
)

func TestClassifyCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cmd  string
		want Type
	}{
		{"go test ./...", TypeTestRun},
		{"/bin/sh -lc 'go test ./pipedrive/v1 -count=1'", TypeTestRun},
		{`/bin/sh -lc "/usr/local/go/bin/go test ./..."`, TypeTestRun},
		{"cd /x && go test -run TestFoo ./pkg", TypeTestRun},
		{"go vet ./...", TypeTestRun},
		{"golangci-lint run", TypeTestRun},
		{"go build ./...", TypeShell},
		{"ls -la", TypeShell},
		{"echo gotestsum", TypeShell},
		{"cargo test --all", TypeTestRun},
	}
	for _, tt := range tests {
		if got := ClassifyCommand(tt.cmd); got != tt.want {
			t.Errorf("ClassifyCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestComputeMetrics(t *testing.T) {
	t.Parallel()

	evs := []Event{
		{Seq: 1, Turn: 1, Type: TypeRead, Path: "a.go"},
		{Seq: 2, Turn: 1, Type: TypeEdit, Path: "a.go"},
		{Seq: 3, Turn: 1, Type: TypeEdit, Path: "b.go"},
		{Seq: 4, Turn: 2, Type: TypeShell, Command: "ls"},
		{Seq: 5, Turn: 2, Type: TypeTestRun, Command: "go test ./..."},
		{Seq: 6, Turn: 2, Type: TypeQuestion, Text: "should I?"},
		{Seq: 7, Turn: 2, Type: TypeTurnEnd},
	}

	m := Compute(evs)
	if m.Turns != 2 {
		t.Errorf("Turns = %d, want 2", m.Turns)
	}
	if m.Reads != 1 || m.Edits != 2 || m.ShellCommands != 1 || m.TestRuns != 1 {
		t.Errorf("counts = %+v", m)
	}
	if m.QuestionsAsked != 1 {
		t.Errorf("QuestionsAsked = %d, want 1", m.QuestionsAsked)
	}
	if len(m.EditedWithoutRead) != 1 || m.EditedWithoutRead[0] != "b.go" {
		t.Errorf("EditedWithoutRead = %v, want [b.go]", m.EditedWithoutRead)
	}
}

func TestComputeMetrics_ShellReadCountsAsRead(t *testing.T) {
	t.Parallel()

	evs := []Event{
		{Seq: 1, Turn: 1, Type: TypeShell, Command: "sed -n 1,50p pkg/b.go"},
		{Seq: 2, Turn: 1, Type: TypeEdit, Path: "pkg/b.go"},
	}
	m := Compute(evs)
	if len(m.EditedWithoutRead) != 0 {
		t.Errorf("EditedWithoutRead = %v, want empty (shell read mentioned the file)", m.EditedWithoutRead)
	}
}
