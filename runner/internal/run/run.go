// Package run executes one eval attempt: workspace prep, the agent/simulator
// turn loop, checks, and result assembly.
package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/agentexec"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/events"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/scorerexec"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/simulator"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/task"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/workspace"
)

type Config struct {
	Task    *task.Task
	Harness harness.Harness
	Model   string
	Effort  string
	Sim     simulator.Simulator
	OutDir  string
	RunID   string
	// RepoOverride replaces the task's repo source (e.g. a local clone
	// instead of the canonical URL). The pinned base commit still applies.
	RepoOverride string
}

type Result struct {
	Task        string            `json:"task"`
	TaskVersion int               `json:"task_version"`
	Harness     string            `json:"harness"`
	Model       string            `json:"model"`
	Effort      string            `json:"effort,omitempty"`
	RunID       string            `json:"run_id"`
	StartedAt   time.Time         `json:"started_at"`
	DurationSec float64           `json:"duration_sec"`
	Turns       int               `json:"turns"`
	EndReason   string            `json:"end_reason"`
	GatesFailed int               `json:"gates_failed"`
	Gates       map[string]bool   `json:"gates"`
	Signals     map[string]string `json:"signals"`
	Metrics     events.Metrics    `json:"metrics"`
	Usage       harness.Usage     `json:"usage"`
	Workspace   string            `json:"workspace"`
}

func Do(ctx context.Context, cfg Config) (*Result, error) {
	runDir := filepath.Join(cfg.OutDir, cfg.Task.Name, cfg.Harness.Name(), cfg.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	agentPaths, err := agentexec.Prepare()
	if err != nil {
		return nil, fmt.Errorf("prepare agent isolation: %w", err)
	}
	defer agentPaths.Cleanup()

	repo := cfg.Task.Repo
	if cfg.RepoOverride != "" {
		repo = cfg.RepoOverride
	}
	archivedWorkspace := filepath.Join(runDir, "workspace")
	ws := archivedWorkspace
	if agentPaths != nil {
		ws = agentPaths.Workspace
	}
	if err := prepareWorkspace(ctx, repo, cfg.Task.BaseCommit, ws); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}
	if agentPaths != nil {
		if err := agentexec.OwnTree(ws); err != nil {
			return nil, fmt.Errorf("prepare workspace ownership: %w", err)
		}
	}

	res := &Result{
		Task:        cfg.Task.Name,
		TaskVersion: cfg.Task.Version,
		Harness:     cfg.Harness.Name(),
		Model:       cfg.Model,
		Effort:      cfg.Effort,
		RunID:       cfg.RunID,
		StartedAt:   time.Now(),
		Workspace:   archivedWorkspace,
	}

	snapshotTask(cfg.Task, runDir)

	allEvents := loop(ctx, cfg, ws, runDir, res)
	if err := agentexec.KillAll(); err != nil {
		return nil, fmt.Errorf("stop contestant processes: %w", err)
	}

	res.Metrics = events.Compute(allEvents)
	res.Metrics.EditedWithoutRead = filterPreexisting(ctx, ws, cfg.Task.BaseCommit, res.Metrics.EditedWithoutRead)
	res.DurationSec = time.Since(res.StartedAt).Seconds()

	if err := writeJSONL(filepath.Join(runDir, "events.jsonl"), allEvents); err != nil {
		return nil, err
	}

	scoringSource := ws
	if agentPaths != nil {
		if err := archiveWorkspace(ws, archivedWorkspace); err != nil {
			return nil, fmt.Errorf("archive workspace: %w", err)
		}
		scoringSource = archivedWorkspace
	}

	gates, signals, failed, err := scoreWorkspace(
		ctx,
		cfg.Task,
		scoringSource,
		agentPaths != nil,
		filepath.Join(runDir, "diff.patch"),
	)
	if err != nil {
		return nil, fmt.Errorf("score workspace: %w", err)
	}
	res.Gates, res.Signals, res.GatesFailed = gates, signals, failed

	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), raw, 0o644); err != nil {
		return nil, fmt.Errorf("write result: %w", err)
	}
	return res, nil
}

// snapshotTask copies the task definition into the run directory so every
// run records the exact task state it executed against.
func snapshotTask(t *task.Task, runDir string) {
	for _, name := range []string{"task.json", t.PromptFile} {
		raw, err := os.ReadFile(filepath.Join(t.Dir(), name))
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(runDir, "task-snapshot-"+name), raw, 0o644)
	}
}

// loop drives the agent/simulator exchange until the simulator ends the run,
// the turn budget is exhausted, or the harness fails. Failures set EndReason
// rather than aborting so a partial trajectory still gets scored.
func loop(ctx context.Context, cfg Config, ws, runDir string, res *Result) []events.Event {
	var all []events.Event
	seq := 0
	sessionID := ""
	message := cfg.Task.Prompt()

	appendTurn := func(turnNum int, evs []events.Event) {
		for _, ev := range evs {
			seq++
			ev.Seq = seq
			ev.Turn = turnNum
			all = append(all, ev)
		}
	}

	for turnNum := 1; ; turnNum++ {
		res.Turns = turnNum

		turnCtx, cancel := context.WithTimeout(ctx, cfg.Task.TurnTimeoutDur())
		var t *harness.Turn
		var err error
		if sessionID == "" {
			t, err = cfg.Harness.Start(turnCtx, ws, cfg.Model, message)
		} else {
			t, err = cfg.Harness.Resume(turnCtx, ws, cfg.Model, sessionID, message)
		}
		cancel()

		if err != nil {
			res.EndReason = "harness_error"
			if errors.Is(turnCtx.Err(), context.DeadlineExceeded) {
				res.EndReason = "turn_timeout"
			}
			appendTurn(turnNum, []events.Event{{Type: events.TypeOther, Tool: "error", Text: err.Error()}})
			return all
		}

		_ = os.WriteFile(filepath.Join(runDir, fmt.Sprintf("raw-turn-%d.jsonl", turnNum)), t.Raw, 0o644)
		sessionID = t.SessionID
		res.Usage.InputTokens += t.Usage.InputTokens
		res.Usage.CacheReadTokens += t.Usage.CacheReadTokens
		res.Usage.CacheCreationTokens += t.Usage.CacheCreationTokens
		res.Usage.OutputTokens += t.Usage.OutputTokens
		res.Usage.CostUSD += t.Usage.CostUSD

		turnEvents := append(t.Events, events.Event{Type: events.TypeTurnEnd, Text: truncate(t.Final, 2000)})
		appendTurn(turnNum, turnEvents)

		dec, err := cfg.Sim.Decide(ctx, t.Final)
		if err != nil {
			res.EndReason = "simulator_error"
			appendTurn(turnNum, []events.Event{{Type: events.TypeOther, Tool: "error", Text: err.Error()}})
			return all
		}

		switch dec.Action {
		case simulator.ActionEnd:
			res.EndReason = "simulator_end"
			return all
		case simulator.ActionAnswer:
			appendTurn(turnNum, []events.Event{
				{Type: events.TypeQuestion, Text: truncate(t.Final, 2000)},
				{Type: events.TypeUserMessage, Text: dec.Reply},
			})
		case simulator.ActionNudge:
			appendTurn(turnNum, []events.Event{{Type: events.TypeUserMessage, Text: dec.Reply}})
		}

		if turnNum >= cfg.Task.MaxTurns {
			res.EndReason = "max_turns"
			return all
		}
		message = dec.Reply
	}
}

// prepareWorkspace is a seam for tests; production wiring uses workspace.Prepare.
var prepareWorkspace = workspace.Prepare

// writeDiff exports the candidate's full diff from the scorer copy. Candidate
// git configuration therefore cannot execute helpers as root.
func writeDiff(ctx context.Context, ws, baseCommit, dst string, scorer *scorerexec.Paths) {
	cmd := scoringCommand(ctx, scorer, ws, "git", "-C", ws, "diff", "--binary", baseCommit)
	patch, err := cmd.Output()
	if err != nil {
		return
	}

	cmd = scoringCommand(ctx, scorer, ws, "git", "-C", ws, "ls-files", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		cmd = scoringCommand(ctx, scorer, ws, "git", "-C", ws, "diff", "--no-index", "--binary", "--", "/dev/null", path)
		filePatch, err := cmd.Output()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return
		}
		patch = append(patch, filePatch...)
	}
	_ = os.WriteFile(dst, patch, 0o644)
}

// filterPreexisting drops paths that do not exist at the base commit:
// a newly created file cannot be read before it is written, so flagging it
// as edited-without-read is noise.
func filterPreexisting(ctx context.Context, ws, baseCommit string, paths []string) []string {
	var kept []string
	for _, p := range paths {
		rel := strings.TrimPrefix(strings.TrimPrefix(p, ws), "/")
		cmd := evaluatorCommand(ctx, ws, "git", "-C", ws, "cat-file", "-e", baseCommit+":"+rel)
		if cmd.Run() == nil {
			kept = append(kept, p)
		}
	}
	return kept
}

func scoreWorkspace(
	ctx context.Context,
	t *task.Task,
	source string,
	requireIsolation bool,
	diffPath string,
) (gates map[string]bool, signals map[string]string, failed int, err error) {
	paths, err := scorerexec.Prepare()
	if err != nil {
		return nil, nil, 0, err
	}
	if paths == nil {
		if requireIsolation {
			return nil, nil, 0, fmt.Errorf("isolated agent runs require EVAL_SCORER_ROOT")
		}
		writeDiff(ctx, source, t.BaseCommit, diffPath, nil)
		return runChecks(ctx, t.ChecksPath(), source, nil)
	}
	defer func() {
		if cleanupErr := paths.Cleanup(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	checksPath, err := prepareScoringCopy(t, source, paths)
	if err != nil {
		return nil, nil, 0, err
	}
	writeDiff(ctx, paths.Workspace, t.BaseCommit, diffPath, paths)
	return runChecks(ctx, checksPath, paths.Workspace, paths)
}

func prepareScoringCopy(t *task.Task, source string, paths *scorerexec.Paths) (string, error) {
	if err := archiveWorkspace(source, paths.Workspace); err != nil {
		return "", fmt.Errorf("copy scorer workspace: %w", err)
	}

	checksPath := filepath.Join(paths.Task, filepath.Base(t.ChecksScript))
	if err := copyFile(t.ChecksPath(), checksPath, 0o700); err != nil {
		return "", fmt.Errorf("copy checks script: %w", err)
	}
	overlay := filepath.Join(t.Dir(), "overlay")
	if info, err := os.Stat(overlay); err == nil && info.IsDir() {
		if err := archiveWorkspace(overlay, filepath.Join(paths.Task, "overlay")); err != nil {
			return "", fmt.Errorf("copy hidden tests: %w", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect hidden tests: %w", err)
	}
	if err := paths.OwnTree(); err != nil {
		return "", err
	}
	return checksPath, nil
}

func runChecks(ctx context.Context, script, ws string, scorer *scorerexec.Paths) (map[string]bool, map[string]string, int, error) {
	cmd := scoringCommand(ctx, scorer, ws, "bash", script, ws)
	out, err := cmd.Output()
	// A non-zero exit is the gate-failure count, not an execution error.
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, nil, 0, err
		}
	}

	gates := make(map[string]bool)
	signals := make(map[string]string)
	failed := 0

	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		switch fields[0] {
		case "GATE":
			pass := fields[2] == "PASS"
			gates[fields[1]] = pass
			if !pass {
				failed++
			}
		case "SIGNAL":
			signals[fields[1]] = strings.Join(fields[2:], " ")
		}
	}
	return gates, signals, failed, nil
}

func scoringCommand(ctx context.Context, scorer *scorerexec.Paths, workdir, name string, args ...string) *exec.Cmd {
	if scorer != nil {
		return scorer.Command(ctx, workdir, name, args...)
	}
	return evaluatorCommand(ctx, workdir, name, args...)
}

// evaluatorCommand lets the root scorer inspect the exact contestant-owned
// workspace without trusting any other repository in the container.
func evaluatorCommand(ctx context.Context, ws, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = evaluatorEnv(ws)
	return cmd
}

func evaluatorEnv(ws string) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if name == "GIT_CONFIG_COUNT" || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0="+ws,
	)
}

func writeJSONL(path string, evs []events.Event) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ev := range evs {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("write event: %w", err)
		}
	}
	return nil
}

// archiveWorkspace copies a completed isolated workspace into the private run
// directory. WalkDir does not follow contestant-created symlinks.
func archiveWorkspace(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported workspace file %s with mode %s", path, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
