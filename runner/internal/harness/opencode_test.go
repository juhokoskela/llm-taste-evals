package harness

import (
	"slices"
	"testing"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
)

func TestOpenCodeCommandArgsPinsWorkdir(t *testing.T) {
	t.Parallel()

	o := &OpenCode{Effort: "xhigh"}
	args := o.commandArgs("/agent/workspace", "provider/model", "continue", "session-1")
	want := []string{
		"run", "--format", "json", "--auto", "--dir", "/agent/workspace",
		"-m", "provider/model", "--variant", "xhigh", "-s", "session-1", "continue",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("commandArgs = %q, want %q", args, want)
	}
}

// Fixture lines captured from a real `opencode run --format json` session
// (opencode 1.18.x), trimmed to the fields the parser reads.
const opencodeFixture = `{"type":"step_start","timestamp":1787339384519,"sessionID":"ses_abc","part":{"type":"step-start"}}
{"type":"tool_use","sessionID":"ses_abc","part":{"type":"tool","tool":"read","state":{"status":"completed","input":{"filePath":"pipedrive/v1/files.go"}}}}
{"type":"tool_use","sessionID":"ses_abc","part":{"type":"tool","tool":"grep","state":{"status":"completed","input":{"pattern":"multipart","path":"."}}}}
{"type":"tool_use","sessionID":"ses_abc","part":{"type":"tool","tool":"edit","state":{"status":"completed","input":{"filePath":"pipedrive/v1/files.go"}}}}
{"type":"tool_use","sessionID":"ses_abc","part":{"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"go test ./pipedrive/v1/","timeout":120000},"output":"ok"}}}
{"type":"step_finish","sessionID":"ses_abc","part":{"type":"step-finish","reason":"tool-calls","tokens":{"total":10786,"input":10716,"output":65,"reasoning":0,"cache":{"write":100,"read":5}},"cost":0.000769795}}
{"type":"text","sessionID":"ses_abc","part":{"type":"text","text":"Implemented Upload."}}
{"type":"step_finish","sessionID":"ses_abc","part":{"type":"step-finish","reason":"stop","tokens":{"total":500,"input":400,"output":100,"reasoning":0,"cache":{"write":0,"read":200}},"cost":0.0001}}
`

func TestParseOpenCodeStream(t *testing.T) {
	t.Parallel()

	turn, err := parseOpenCodeStream([]byte(opencodeFixture))
	if err != nil {
		t.Fatalf("parseOpenCodeStream error: %v", err)
	}
	if turn.SessionID != "ses_abc" {
		t.Errorf("SessionID = %q, want ses_abc", turn.SessionID)
	}
	if turn.Final != "Implemented Upload." {
		t.Errorf("Final = %q", turn.Final)
	}

	// input normalized to total: (10716+5+100) + (400+200+0)
	if turn.Usage.InputTokens != 11421 {
		t.Errorf("InputTokens = %d, want 11421", turn.Usage.InputTokens)
	}
	if turn.Usage.CacheReadTokens != 205 || turn.Usage.CacheCreationTokens != 100 {
		t.Errorf("cache usage = %+v", turn.Usage)
	}
	if turn.Usage.OutputTokens != 165 {
		t.Errorf("OutputTokens = %d, want 165", turn.Usage.OutputTokens)
	}
	if turn.Usage.CostUSD < 0.00086 || turn.Usage.CostUSD > 0.00088 {
		t.Errorf("CostUSD = %v", turn.Usage.CostUSD)
	}

	want := []events.Type{
		events.TypeRead,
		events.TypeRead,
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
	if turn.Events[0].Path != "pipedrive/v1/files.go" {
		t.Errorf("read path = %q", turn.Events[0].Path)
	}
	if turn.Events[1].Path != "." {
		t.Errorf("grep path = %q", turn.Events[1].Path)
	}
}
