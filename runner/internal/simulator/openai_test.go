package simulator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIDecide(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"action\":\"nudge\",\"reply\":\"Please continue.\"}"}}]}`))
	}))
	defer srv.Close()

	sim := &OpenAI{
		APIKey:     "test-key",
		Model:      "gpt-5-mini",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		TaskPrompt: "prompt",
		FactSheet:  "Q: x\nA: y",
	}
	d, err := sim.Decide(context.Background(), "I refactored half of it so far.")
	if err != nil {
		t.Fatalf("Decide error: %v", err)
	}
	if d.Action != ActionNudge || d.Reply != "Please continue." {
		t.Fatalf("unexpected decision: %+v", d)
	}
}
