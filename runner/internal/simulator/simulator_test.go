package simulator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicDecide(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"action\":\"answer\",\"reply\":\"Out of scope for this change.\"}"}]}`))
	}))
	defer srv.Close()

	sim := &Anthropic{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		TaskPrompt: "prompt",
		FactSheet:  "Q: x\nA: y",
	}
	d, err := sim.Decide(context.Background(), "Should Update get the same treatment?")
	if err != nil {
		t.Fatalf("Decide error: %v", err)
	}
	if d.Action != ActionAnswer || d.Reply != "Out of scope for this change." {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestParseDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		want    Action
		wantErr bool
	}{
		{"bare json", `{"action":"end","reply":""}`, ActionEnd, false},
		{"fenced json", "Here you go:\n```json\n{\"action\":\"nudge\",\"reply\":\"continue\"}\n```", ActionNudge, false},
		{"invalid action", `{"action":"maybe","reply":""}`, "", true},
		{"no json", "I cannot decide", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, err := parseDecision(tt.text)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", d)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDecision error: %v", err)
			}
			if d.Action != tt.want {
				t.Errorf("action = %q, want %q", d.Action, tt.want)
			}
		})
	}
}

func TestStaticExhaustionEnds(t *testing.T) {
	t.Parallel()

	s := &Static{Decisions: []Decision{{Action: ActionNudge, Reply: "go on"}}}
	if d, _ := s.Decide(context.Background(), "x"); d.Action != ActionNudge {
		t.Fatalf("first decision = %+v", d)
	}
	if d, _ := s.Decide(context.Background(), "x"); d.Action != ActionEnd {
		t.Fatalf("exhausted static should end, got %+v", d)
	}
}
