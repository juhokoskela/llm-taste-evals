package harness

import (
	"testing"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

const codexFixture = `{"type":"thread.started","thread_id":"th-9"}
{"type":"turn.started"}
{"type":"item.completed","item":{"type":"reasoning","text":"thinking"}}
{"type":"item.completed","item":{"type":"command_execution","command":"cat pipedrive/v1/files.go","exit_code":0}}
{"type":"item.completed","item":{"type":"command_execution","command":"/bin/sh -lc 'go test ./pipedrive/...'","exit_code":0}}
{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"pipedrive/v1/files.go","kind":"update"},{"path":"pipedrive/v1/files_test.go","kind":"update"}]}}
{"type":"item.completed","item":{"type":"command_execution","command":"go test ./pipedrive/...","exit_code":0}}
{"type":"item.completed","item":{"type":"agent_message","text":"Implemented Upload."}}
{"type":"turn.completed","usage":{"input_tokens":900,"cached_input_tokens":700,"cache_write_input_tokens":50,"output_tokens":210}}
`

func TestParseCodexStream(t *testing.T) {
	t.Parallel()

	turn, err := parseCodexStream([]byte(codexFixture))
	if err != nil {
		t.Fatalf("parseCodexStream error: %v", err)
	}
	if turn.SessionID != "th-9" {
		t.Errorf("SessionID = %q, want th-9", turn.SessionID)
	}
	if turn.Final != "Implemented Upload." {
		t.Errorf("Final = %q", turn.Final)
	}
	if turn.Usage.InputTokens != 900 || turn.Usage.OutputTokens != 210 {
		t.Errorf("Usage = %+v", turn.Usage)
	}
	if turn.Usage.CacheReadTokens != 700 || turn.Usage.CacheCreationTokens != 50 {
		t.Errorf("cache usage = %+v", turn.Usage)
	}

	want := []events.Type{
		events.TypeShell,
		events.TypeTestRun,
		events.TypeEdit,
		events.TypeEdit,
		events.TypeTestRun,
		events.TypeAssistantMessage,
	}
	if len(turn.Events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(turn.Events), len(want), turn.Events)
	}
	for i, typ := range want {
		if turn.Events[i].Type != typ {
			t.Errorf("event %d type = %q, want %q", i, turn.Events[i].Type, typ)
		}
	}
	if turn.Events[2].Path != "pipedrive/v1/files.go" || turn.Events[3].Path != "pipedrive/v1/files_test.go" {
		t.Errorf("file_change paths = %q, %q", turn.Events[2].Path, turn.Events[3].Path)
	}
}
