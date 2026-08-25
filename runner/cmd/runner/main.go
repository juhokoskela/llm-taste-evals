// Command runner executes eval attempts: it drives an agent CLI against a
// task's pinned repo state, simulates the user at turn boundaries, and scores
// the outcome with the task's checks.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/run"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/simulator"
	"github.com/juhokoskela/llm-taste-evals/runner/internal/task"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "runner:", err)
		os.Exit(1)
	}
}

func realMain() error {
	var (
		taskDir     = flag.String("task", "", "task directory (contains task.json)")
		harnessName = flag.String("harness", "", "agent harness: claudecode or codex")
		model       = flag.String("model", "", "model id to pass to the harness")
		effort      = flag.String("effort", "", "reasoning effort, in the harness's own vocabulary (claudecode: low..max, codex: minimal..xhigh)")
		runs        = flag.Int("runs", 1, "number of attempts")
		outDir      = flag.String("out", "runs", "output directory")
		repo        = flag.String("repo", "", "override the task's repo source (e.g. a local clone)")
		simModel    = flag.String("sim-model", "", "simulator model (default: cheap model of whichever provider key is set)")
	)
	flag.Parse()

	if *taskDir == "" || *harnessName == "" || *model == "" {
		flag.Usage()
		return fmt.Errorf("-task, -harness, and -model are required")
	}

	t, err := task.Load(*taskDir)
	if err != nil {
		return err
	}
	h, err := harness.New(*harnessName, *effort)
	if err != nil {
		return err
	}

	sim, err := buildSimulator(*simModel, t)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for i := 1; i <= *runs; i++ {
		id := *model
		if *effort != "" {
			id += "-" + *effort
		}
		runID := fmt.Sprintf("%s-run%02d-%d", sanitize(id), i, time.Now().UnixNano())
		fmt.Printf("=== %s / %s / %s (%d/%d)\n", t.Name, *harnessName, *model, i, *runs)

		res, err := run.Do(ctx, run.Config{
			Task:         t,
			Harness:      h,
			Model:        *model,
			Effort:       *effort,
			Sim:          sim,
			OutDir:       *outDir,
			RunID:        runID,
			RepoOverride: *repo,
		})
		if err != nil {
			return fmt.Errorf("run %s: %w", runID, err)
		}
		printSummary(res)
	}
	return nil
}

// buildSimulator picks the simulator provider from the model name, or from
// whichever API key is available when no model is given. The simulator is a
// fact-sheet classifier, so contestant and simulator may share a provider —
// one vendor key is enough for a run.
func buildSimulator(model string, t *task.Task) (simulator.Simulator, error) {
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")
	fireworksKey := os.Getenv("FIREWORKS_API_KEY")
	oauthToken := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")

	if model == "" {
		switch {
		case anthropicKey != "" || oauthToken != "":
			model = "claude-haiku-4-5-20251001"
		case openaiKey != "":
			model = "gpt-5-mini"
		case fireworksKey != "":
			model = "accounts/fireworks/models/gpt-oss-120b"
		default:
			return nil, fmt.Errorf("the user simulator needs ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN, OPENAI_API_KEY, or FIREWORKS_API_KEY")
		}
	}

	switch {
	case strings.HasPrefix(model, "claude"):
		if anthropicKey != "" {
			return &simulator.Anthropic{
				APIKey: anthropicKey, Model: model,
				TaskPrompt: t.Prompt(), FactSheet: t.FactSheetText(),
			}, nil
		}
		if oauthToken != "" {
			// Subscription auth: run the simulator through the claude CLI.
			return &simulator.ClaudeCLI{
				Model:      model,
				TaskPrompt: t.Prompt(), FactSheet: t.FactSheetText(),
			}, nil
		}
		return nil, fmt.Errorf("simulator model %q needs ANTHROPIC_API_KEY or CLAUDE_CODE_OAUTH_TOKEN", model)
	case strings.HasPrefix(model, "accounts/fireworks/"):
		if fireworksKey == "" {
			return nil, fmt.Errorf("simulator model %q needs FIREWORKS_API_KEY", model)
		}
		// Fireworks serves the OpenAI chat-completions dialect.
		return &simulator.OpenAI{
			APIKey: fireworksKey, Model: model,
			BaseURL:    "https://api.fireworks.ai/inference",
			TaskPrompt: t.Prompt(), FactSheet: t.FactSheetText(),
		}, nil
	default:
		if openaiKey == "" {
			return nil, fmt.Errorf("simulator model %q needs OPENAI_API_KEY", model)
		}
		return &simulator.OpenAI{
			APIKey: openaiKey, Model: model,
			ReasoningEffort: "minimal",
			TaskPrompt:      t.Prompt(), FactSheet: t.FactSheetText(),
		}, nil
	}
}

func printSummary(r *run.Result) {
	fmt.Printf("  end: %s after %d turns, %.0fs, %d in (%d cached) / %d out tokens\n",
		r.EndReason, r.Turns, r.DurationSec,
		r.Usage.InputTokens, r.Usage.CacheReadTokens, r.Usage.OutputTokens)
	fmt.Printf("  gates: %d failed of %d\n", r.GatesFailed, len(r.Gates))
	for name, pass := range r.Gates {
		if !pass {
			fmt.Printf("    FAIL %s\n", name)
		}
	}
	for name, value := range r.Signals {
		fmt.Printf("  signal %s = %s\n", name, value)
	}
	fmt.Printf("  metrics: %d reads, %d edits, %d shell, %d test runs, %d questions",
		r.Metrics.Reads, r.Metrics.Edits, r.Metrics.ShellCommands, r.Metrics.TestRuns, r.Metrics.QuestionsAsked)
	if len(r.Metrics.EditedWithoutRead) > 0 {
		fmt.Printf(", edited-without-read: %s", strings.Join(r.Metrics.EditedWithoutRead, ", "))
	}
	fmt.Println()
	fmt.Printf("  result: %s\n", r.Workspace)
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		default:
			return '-'
		}
	}, s)
}
