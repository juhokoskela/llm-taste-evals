package harness

import (
	"context"
	"strings"
	"testing"
)

func TestRunCLIIncludesOutputOnFailure(t *testing.T) {
	_, err := runCLI(
		context.Background(),
		t.TempDir(),
		nil,
		"sh",
		"-c",
		"printf stdout-detail; printf stderr-detail >&2; exit 1",
	)
	if err == nil {
		t.Fatal("runCLI succeeded")
	}
	for _, want := range []string{"stdout: stdout-detail", "stderr: stderr-detail"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
