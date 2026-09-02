package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/scorerexec"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/simulator"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/task"
)

type mockHarness struct {
	turns []*harness.Turn
	next  int
	// messages records what each turn was started/resumed with.
	messages []string
	workdirs []string
}

func (m *mockHarness) Name() string { return "mock" }

func (m *mockHarness) Start(_ context.Context, workdir, _, prompt string) (*harness.Turn, error) {
	m.workdirs = append(m.workdirs, workdir)
	return m.take(prompt)
}

func (m *mockHarness) Resume(_ context.Context, workdir, _, _, message string) (*harness.Turn, error) {
	m.workdirs = append(m.workdirs, workdir)
	return m.take(message)
}

func (m *mockHarness) take(msg string) (*harness.Turn, error) {
	m.messages = append(m.messages, msg)
	if m.next >= len(m.turns) {
		return nil, fmt.Errorf("mock harness exhausted")
	}
	t := m.turns[m.next]
	m.next++
	return t, nil
}

func writeTestTask(t *testing.T) *task.Task {
	t.Helper()
	dir := t.TempDir()

	checks := `#!/usr/bin/env bash
echo "GATE build PASS"
echo "GATE hidden_tests FAIL"
echo "SIGNAL reuses_multipartbody false"
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "checks.sh"), []byte(checks), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("do the thing"), 0o644); err != nil {
		t.Fatal(err)
	}
	taskJSON := `{"name":"t1","repo":"unused","base_commit":"deadbeef","max_turns":5}`
	if err := os.WriteFile(filepath.Join(dir, "task.json"), []byte(taskJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	tk, err := task.Load(dir)
	if err != nil {
		t.Fatalf("task.Load: %v", err)
	}
	return tk
}

func TestFilterPreexisting(t *testing.T) {
	t.Parallel()

	ws := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet"}, {"config", "user.email", "t@e.st"}, {"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(ws, "old.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "--quiet", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	rev := exec.Command("git", "rev-parse", "HEAD")
	rev.Dir = ws
	out, err := rev.Output()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(out))

	got := filterPreexisting(context.Background(), ws, base,
		[]string{filepath.Join(ws, "old.go"), filepath.Join(ws, "brand_new.go")})
	if len(got) != 1 || !strings.HasSuffix(got[0], "old.go") {
		t.Errorf("filterPreexisting = %v, want only old.go", got)
	}
}

func TestWriteDiffIncludesUntrackedFiles(t *testing.T) {
	t.Parallel()

	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	ws := t.TempDir()
	runGit(ws, "init", "--quiet")
	runGit(ws, "config", "user.email", "t@e.st")
	runGit(ws, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(ws, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(ws, "add", ".")
	runGit(ws, "commit", "--quiet", "-m", "base")
	base := runGit(ws, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(ws, "tracked.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patch := filepath.Join(t.TempDir(), "diff.patch")
	writeDiff(context.Background(), ws, base, patch, nil)

	cloneDir := t.TempDir()
	rebuilt := filepath.Join(cloneDir, "rebuilt")
	runGit(cloneDir, "clone", "--quiet", ws, rebuilt)
	runGit(rebuilt, "apply", patch)
	for name, want := range map[string]string{
		"tracked.txt":   "after\n",
		"untracked.txt": "new\n",
	} {
		got, err := os.ReadFile(filepath.Join(rebuilt, name))
		if err != nil {
			t.Errorf("read reconstructed %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("reconstructed %s = %q, want %q", name, got, want)
		}
	}
}

func TestRunChecksTrustsOnlyCandidateWorkspace(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "safe.directory")
	t.Setenv("GIT_CONFIG_VALUE_0", "/unrelated")

	dir := t.TempDir()
	script := filepath.Join(dir, "checks.sh")
	contents := `#!/usr/bin/env bash
if [ "$(git config --get-all safe.directory)" = "$1" ]; then
	echo "GATE safe_directory PASS"
	exit 0
fi
echo "GATE safe_directory FAIL"
exit 1
`
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}

	gates, _, failed, err := runChecks(context.Background(), script, dir, nil)
	if err != nil {
		t.Fatalf("runChecks error: %v", err)
	}
	if !gates["safe_directory"] || failed != 0 {
		t.Fatalf("gates = %v, failed = %d", gates, failed)
	}
}

func TestPrepareScoringCopyIncludesOnlyRequiredEvaluatorFiles(t *testing.T) {
	tk := writeTestTask(t)
	for name, contents := range map[string]string{
		"spec.md":                   "private evaluator notes",
		"reference/reference.patch": "private reference",
		"overlay/hidden_test.go":    "package hidden",
	} {
		path := filepath.Join(tk.Dir(), name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "candidate.go"), []byte("package candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVAL_SCORER_ROOT", filepath.Join(t.TempDir(), "scorer"))
	t.Setenv("EVAL_SCORER_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("EVAL_SCORER_GID", strconv.Itoa(os.Getgid()))
	paths, err := scorerexec.Prepare()
	if err != nil {
		t.Fatalf("scorerexec.Prepare error: %v", err)
	}
	t.Cleanup(func() {
		if err := paths.Cleanup(); err != nil {
			t.Errorf("scorer cleanup: %v", err)
		}
	})

	checksPath, err := prepareScoringCopy(tk, source, paths)
	if err != nil {
		t.Fatalf("prepareScoringCopy error: %v", err)
	}
	for _, path := range []string{
		filepath.Join(paths.Workspace, "candidate.go"),
		checksPath,
		filepath.Join(paths.Task, "overlay", "hidden_test.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("required scorer file %s: %v", path, err)
		}
	}
	for _, name := range []string{"task.json", "task.md", "spec.md", "reference"} {
		if _, err := os.Stat(filepath.Join(paths.Task, name)); !os.IsNotExist(err) {
			t.Errorf("private task file %s copied to scorer: %v", name, err)
		}
	}
}

func TestScoreWorkspaceDoesNotMutateArchivedCandidate(t *testing.T) {
	tk := writeTestTask(t)
	checks := `#!/usr/bin/env bash
touch "$1/scorer-only"
echo "GATE isolated PASS"
`
	if err := os.WriteFile(tk.ChecksPath(), []byte(checks), 0o755); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	t.Setenv("EVAL_SCORER_ROOT", filepath.Join(t.TempDir(), "scorer"))
	t.Setenv("EVAL_SCORER_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("EVAL_SCORER_GID", strconv.Itoa(os.Getgid()))

	gates, _, failed, err := scoreWorkspace(context.Background(), tk, source, true, filepath.Join(t.TempDir(), "diff.patch"))
	if err != nil {
		t.Fatalf("scoreWorkspace error: %v", err)
	}
	if !gates["isolated"] || failed != 0 {
		t.Fatalf("gates = %v, failed = %d", gates, failed)
	}
	if _, err := os.Stat(filepath.Join(source, "scorer-only")); !os.IsNotExist(err) {
		t.Fatalf("scoring changed archived candidate: %v", err)
	}
}

func TestScoreWorkspaceRequiresIsolationForAgentRuns(t *testing.T) {
	tk := writeTestTask(t)
	_, _, _, err := scoreWorkspace(context.Background(), tk, t.TempDir(), true, filepath.Join(t.TempDir(), "diff.patch"))
	if err == nil || !strings.Contains(err.Error(), "require EVAL_SCORER_ROOT") {
		t.Fatalf("scoreWorkspace error = %v", err)
	}
}

func stubWorkspace(t *testing.T) {
	t.Helper()
	orig := prepareWorkspace
	prepareWorkspace = func(_ context.Context, _, _, dst string) error {
		return os.MkdirAll(dst, 0o755)
	}
	t.Cleanup(func() { prepareWorkspace = orig })
}

func TestDo_QuestionThenEnd(t *testing.T) {
	stubWorkspace(t)

	h := &mockHarness{turns: []*harness.Turn{
		{
			SessionID: "s1",
			Final:     "Should Update get the same treatment?",
			Events: []events.Event{
				{Type: events.TypeRead, Path: "a.go"},
				{Type: events.TypeAssistantMessage, Text: "question"},
			},
			Usage: harness.Usage{InputTokens: 10, OutputTokens: 5},
		},
		{
			SessionID: "s1",
			Final:     "Done.",
			Events: []events.Event{
				{Type: events.TypeEdit, Path: "a.go"},
				{Type: events.TypeTestRun, Command: "go test ./..."},
			},
			Usage: harness.Usage{InputTokens: 20, OutputTokens: 7},
		},
	}}
	sim := &simulator.Static{Decisions: []simulator.Decision{
		{Action: simulator.ActionAnswer, Reply: "Out of scope."},
		{Action: simulator.ActionEnd},
	}}

	out := t.TempDir()
	res, err := Do(context.Background(), Config{
		Task:    writeTestTask(t),
		Harness: h,
		Model:   "m",
		Sim:     sim,
		OutDir:  out,
		RunID:   "run1",
	})
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}

	if res.EndReason != "simulator_end" {
		t.Errorf("EndReason = %q", res.EndReason)
	}
	if res.Turns != 2 {
		t.Errorf("Turns = %d, want 2", res.Turns)
	}
	if res.Usage.InputTokens != 30 || res.Usage.OutputTokens != 12 {
		t.Errorf("Usage = %+v", res.Usage)
	}
	if res.Metrics.QuestionsAsked != 1 {
		t.Errorf("QuestionsAsked = %d, want 1", res.Metrics.QuestionsAsked)
	}
	if len(h.messages) != 2 || h.messages[0] != "do the thing" || h.messages[1] != "Out of scope." {
		t.Errorf("harness messages = %q", h.messages)
	}

	if !res.Gates["build"] || res.Gates["hidden_tests"] {
		t.Errorf("Gates = %v", res.Gates)
	}
	if res.GatesFailed != 1 {
		t.Errorf("GatesFailed = %d, want 1", res.GatesFailed)
	}
	if res.Signals["reuses_multipartbody"] != "false" {
		t.Errorf("Signals = %v", res.Signals)
	}

	runDir := filepath.Join(out, "t1", "mock", "run1")
	raw, err := os.ReadFile(filepath.Join(runDir, "result.json"))
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	var onDisk Result
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("parse result.json: %v", err)
	}
	if onDisk.EndReason != "simulator_end" {
		t.Errorf("persisted EndReason = %q", onDisk.EndReason)
	}
	for _, name := range []string{"events.jsonl", "raw-turn-1.jsonl", "raw-turn-2.jsonl"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Errorf("missing artifact %s: %v", name, err)
		}
	}
}

func TestDo_MaxTurns(t *testing.T) {
	stubWorkspace(t)

	turns := make([]*harness.Turn, 5)
	for i := range turns {
		turns[i] = &harness.Turn{SessionID: "s", Final: "still going"}
	}
	sim := &simulator.Static{Decisions: []simulator.Decision{
		{Action: simulator.ActionNudge, Reply: "continue"},
		{Action: simulator.ActionNudge, Reply: "continue"},
		{Action: simulator.ActionNudge, Reply: "continue"},
		{Action: simulator.ActionNudge, Reply: "continue"},
		{Action: simulator.ActionNudge, Reply: "continue"},
	}}

	res, err := Do(context.Background(), Config{
		Task:    writeTestTask(t),
		Harness: &mockHarness{turns: turns},
		Model:   "m",
		Sim:     sim,
		OutDir:  t.TempDir(),
		RunID:   "run1",
	})
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if res.EndReason != "max_turns" || res.Turns != 5 {
		t.Errorf("EndReason = %q, Turns = %d; want max_turns after 5", res.EndReason, res.Turns)
	}
}

func TestDo_HarnessErrorStillScores(t *testing.T) {
	stubWorkspace(t)

	res, err := Do(context.Background(), Config{
		Task:    writeTestTask(t),
		Harness: &mockHarness{}, // exhausted immediately -> error on first turn
		Model:   "m",
		Sim:     &simulator.Static{},
		OutDir:  t.TempDir(),
		RunID:   "run1",
	})
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if res.EndReason != "harness_error" {
		t.Errorf("EndReason = %q, want harness_error", res.EndReason)
	}
	if len(res.Gates) == 0 {
		t.Error("checks should still run after a harness error")
	}
}

func TestDo_ArchivesIsolatedWorkspace(t *testing.T) {
	stubWorkspace(t)

	agentRoot := filepath.Join(t.TempDir(), "agent")
	t.Setenv("EVAL_AGENT_ROOT", agentRoot)
	t.Setenv("EVAL_AGENT_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("EVAL_AGENT_GID", strconv.Itoa(os.Getgid()))
	t.Setenv("EVAL_SCORER_ROOT", filepath.Join(t.TempDir(), "scorer"))
	t.Setenv("EVAL_SCORER_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("EVAL_SCORER_GID", strconv.Itoa(os.Getgid()))

	h := &mockHarness{turns: []*harness.Turn{{SessionID: "s", Final: "Done."}}}
	originalPrepare := prepareWorkspace
	prepareWorkspace = func(_ context.Context, _, _, dst string) error {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, "solution.go"), []byte("package solution\n"), 0o644)
	}
	t.Cleanup(func() { prepareWorkspace = originalPrepare })

	out := t.TempDir()
	res, err := Do(context.Background(), Config{
		Task:    writeTestTask(t),
		Harness: h,
		Model:   "m",
		Sim:     &simulator.Static{},
		OutDir:  out,
		RunID:   "isolated",
	})
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if len(h.workdirs) != 1 || h.workdirs[0] != filepath.Join(agentRoot, "workspace") {
		t.Fatalf("harness workdirs = %q", h.workdirs)
	}
	if _, err := os.Stat(filepath.Join(res.Workspace, "solution.go")); err != nil {
		t.Fatalf("archived solution: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentRoot, "workspace")); !os.IsNotExist(err) {
		t.Fatalf("active workspace survived cleanup: %v", err)
	}
}

func TestArchiveWorkspacePreservesFilesAndSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "archive")
	if err := os.MkdirAll(filepath.Join(src, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(src, "bin", "tool")
	if err := os.WriteFile(file, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("bin/tool", filepath.Join(src, "tool-link")); err != nil {
		t.Fatal(err)
	}

	if err := archiveWorkspace(src, dst); err != nil {
		t.Fatalf("archiveWorkspace error: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dst, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "payload" {
		t.Fatalf("archived contents = %q", raw)
	}
	info, err := os.Stat(filepath.Join(dst, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("archived mode = %o", info.Mode().Perm())
	}
	link, err := os.Readlink(filepath.Join(dst, "tool-link"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "bin/tool" {
		t.Fatalf("archived link = %q", link)
	}
}
