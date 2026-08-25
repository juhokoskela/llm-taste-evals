package simulator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeCLIDecide(t *testing.T) {
	dir := t.TempDir()
	stub := `#!/usr/bin/env bash
echo '{"type":"result","subtype":"success","result":"{\"action\":\"answer\",\"reply\":\"Keep Add as-is.\"}"}'
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	sim := &ClaudeCLI{Model: "claude-haiku-4-5", TaskPrompt: "prompt", FactSheet: "Q: x\nA: y"}
	d, err := sim.Decide(context.Background(), "Should Add be deprecated?")
	if err != nil {
		t.Fatalf("Decide error: %v", err)
	}
	if d.Action != ActionAnswer || d.Reply != "Keep Add as-is." {
		t.Fatalf("unexpected decision: %+v", d)
	}
}
