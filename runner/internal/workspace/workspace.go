// Package workspace prepares an isolated repository checkout for one run.
package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Prepare clones repo into dst, detaches at baseCommit, and strips everything
// that could reveal post-base history: the origin remote, all branches and
// tags (the mirror's refs may point at the task's solution), reflogs, and —
// because deleted refs still leave commits recoverable via fsck — every
// object unreachable from baseCommit. Network-level isolation is still the
// operator's job — run the runner inside a sandbox with API-only egress.
func Prepare(ctx context.Context, repo, baseCommit, dst string) error {
	// Local clones normally hardlink object files to the source repository.
	// The contestant owns dst in isolated runs, so force independent copies to
	// prevent ownership or content changes from reaching the private mirror.
	if err := git(ctx, "", "clone", "--quiet", "--no-hardlinks", repo, dst); err != nil {
		return err
	}
	if err := git(ctx, dst, "checkout", "--quiet", "--detach", baseCommit); err != nil {
		return err
	}
	if err := git(ctx, dst, "remote", "remove", "origin"); err != nil {
		return err
	}

	refs, err := gitOutput(ctx, dst, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags")
	if err != nil {
		return err
	}
	for _, ref := range strings.Fields(refs) {
		if err := git(ctx, dst, "update-ref", "-d", ref); err != nil {
			return err
		}
	}
	if err := git(ctx, dst, "reflog", "expire", "--expire=now", "--all"); err != nil {
		return err
	}
	return git(ctx, dst, "gc", "--prune=now", "--quiet")
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}
	return stdout.String(), nil
}

func git(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}
	return nil
}
