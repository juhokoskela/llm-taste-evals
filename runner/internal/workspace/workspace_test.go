package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepare(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	mustGit(t, src, "init", "--quiet")
	mustGit(t, src, "config", "user.email", "test@example.com")
	mustGit(t, src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, src, "add", ".")
	mustGit(t, src, "commit", "--quiet", "-m", "first")
	base := strings.TrimSpace(mustGit(t, src, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, src, "commit", "--quiet", "-am", "second")
	solution := strings.TrimSpace(mustGit(t, src, "rev-parse", "HEAD"))
	mustGit(t, src, "tag", "v9.9.9")

	dst := filepath.Join(t.TempDir(), "ws")
	if err := Prepare(context.Background(), src, base, dst); err != nil {
		t.Fatalf("Prepare error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "one" {
		t.Errorf("workspace not at base commit: a.txt = %q", content)
	}
	if remotes := strings.TrimSpace(mustGit(t, dst, "remote")); remotes != "" {
		t.Errorf("origin remote should be removed, got %q", remotes)
	}
	if refs := strings.TrimSpace(mustGit(t, dst, "for-each-ref", "--format=%(refname)")); refs != "" {
		t.Errorf("all refs should be stripped, got %q", refs)
	}
	// The post-base "solution" commit must be unrecoverable, not merely
	// unreferenced.
	cmd := exec.Command("git", "cat-file", "-e", solution)
	cmd.Dir = dst
	if err := cmd.Run(); err == nil {
		t.Errorf("post-base commit %s is still recoverable in the workspace", solution)
	}
}

func TestPrepare_BadCommit(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	mustGit(t, src, "init", "--quiet")
	mustGit(t, src, "config", "user.email", "test@example.com")
	mustGit(t, src, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, src, "add", ".")
	mustGit(t, src, "commit", "--quiet", "-m", "first")

	dst := filepath.Join(t.TempDir(), "ws")
	if err := Prepare(context.Background(), src, "0000000000000000000000000000000000000000", dst); err == nil {
		t.Fatal("expected error for unknown base commit")
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
