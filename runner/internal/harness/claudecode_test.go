package harness

import (
	"testing"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

const claudeFixture = `{"type":"system","subtype":"init","session_id":"sess-1"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Let me look around."},{"type":"tool_use","name":"Read","input":{"file_path":"pipedrive/v1/files.go"}}]},"session_id":"sess-1"}
{"type":"user","message":{"content":[{"type":"tool_result","content":"..."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Grep","input":{"pattern":"multipart","path":"."}}]},"session_id":"sess-1"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"pipedrive/v1/files.go","old_string":"x","new_string":"y"}}]},"session_id":"sess-1"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./pipedrive/v1/"}}]},"session_id":"sess-1"}
{"type":"result","subtype":"success","result":"Done. Should Update get the same change?","session_id":"sess-1","usage":{"input_tokens":1200,"cache_read_input_tokens":50000,"cache_creation_input_tokens":9000,"output_tokens":340},"total_cost_usd":0.05}
`

func TestParseClaudeStream(t *testing.T) {
	t.Parallel()

	turn, err := parseClaudeStream([]byte(claudeFixture))
	if err != nil {
		t.Fatalf("parseClaudeStream error: %v", err)
	}
	if turn.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", turn.SessionID)
	}
	if turn.Final != "Done. Should Update get the same change?" {
		t.Errorf("Final = %q", turn.Final)
	}
	if turn.Usage.InputTokens != 60200 || turn.Usage.OutputTokens != 340 || turn.Usage.CostUSD != 0.05 {
		t.Errorf("Usage = %+v", turn.Usage)
	}
	if turn.Usage.CacheReadTokens != 50000 || turn.Usage.CacheCreationTokens != 9000 {
		t.Errorf("cache usage = %+v", turn.Usage)
	}

	want := []events.Type{
		events.TypeAssistantMessage,
		events.TypeRead,
		events.TypeRead,
		events.TypeEdit,
		events.TypeTestRun,
	}
	if len(turn.Events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(turn.Events), len(want), turn.Events)
	}
	for i, typ := range want {
		if turn.Events[i].Type != typ {
			t.Errorf("event %d type = %q, want %q", i, turn.Events[i].Type, typ)
		}
	}
	if turn.Events[1].Path != "pipedrive/v1/files.go" {
		t.Errorf("read path = %q", turn.Events[1].Path)
	}
	if turn.Events[3].Path != "pipedrive/v1/files.go" {
		t.Errorf("edit path = %q", turn.Events[3].Path)
	}
}

func TestParseClaudeStream_NoSession(t *testing.T) {
	t.Parallel()

	if _, err := parseClaudeStream([]byte(`{"type":"result","result":"x"}`)); err == nil {
		t.Fatal("expected error for missing session_id")
	}
}
