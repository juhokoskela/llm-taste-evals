// Command judge runs a blinded, resumable pairwise merge-quality tournament
// over gate-passing eval attempts.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/judge"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "judge:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("judge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var (
		taskDir      = flags.String("task", "", "task directory (contains task.json and judge.md)")
		runsDir      = flags.String("runs", "", "directory containing scored task runs")
		outDir       = flags.String("out", "", "durable tournament output directory")
		model        = flags.String("model", "gpt-5.6-sol", "Codex judge model")
		effort       = flags.String("effort", "xhigh", "Codex judge reasoning effort")
		workers      = flags.Int("workers", 3, "maximum concurrent Codex votes")
		votes        = flags.Int("votes", 3, "independent votes per pair (must be odd)")
		attempts     = flags.Int("attempts", 3, "maximum Codex attempts per vote")
		timeout      = flags.Duration("timeout", 20*time.Minute, "timeout for one Codex attempt")
		failureLimit = flags.Int("failure-limit", 3, "consecutive failed votes before stopping")
		limit        = flags.Int("limit", 0, "maximum pending votes this invocation (0 means all)")
		codexCommand = flags.String("codex", "codex", "Codex CLI path")
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *taskDir == "" || *runsDir == "" || *outDir == "" {
		flags.Usage()
		return fmt.Errorf("-task, -runs, and -out are required")
	}
	if *workers < 1 || *attempts < 1 || *failureLimit < 1 || *limit < 0 || *timeout <= 0 {
		return fmt.Errorf("workers, attempts, failure-limit, and timeout must be positive; limit cannot be negative")
	}
	if *votes < 1 || *votes%2 == 0 {
		return fmt.Errorf("votes must be a positive odd number")
	}

	inputs, err := judge.LoadInputs(*taskDir, *runsDir)
	if err != nil {
		return err
	}
	judgeConfig := judge.JudgeConfig{Provider: "codex-subscription", Model: *model, Effort: *effort}
	manifest := inputs.Manifest(judgeConfig, *votes)
	jobs := judge.Schedule(inputs, *votes)
	fmt.Fprintf(stderr, "judge: %d substantive runs, %d gate-passing runs, %d exclusions\n", len(inputs.Runs), len(inputs.Candidates)-1, len(inputs.Excluded))
	fmt.Fprintf(stderr, "judge: %d candidates including reference, %d pairs, %d votes, %d workers\n", len(inputs.Candidates), len(inputs.Candidates)*(len(inputs.Candidates)-1)/2, len(jobs), *workers)

	voter, err := judge.NewCodexVoter(judge.CodexOptions{
		Command: *codexCommand, Model: *model, Effort: *effort,
		Timeout: *timeout, MaxAttempts: *attempts,
	})
	if err != nil {
		return err
	}
	defer voter.Close()

	lastProgress := judge.Progress{Completed: -1, Failed: -1}
	progress := func(p judge.Progress) {
		if p.Completed == 0 || p.Completed == p.Total || p.Completed%10 == 0 || p.Failed != lastProgress.Failed {
			fmt.Fprintf(stderr, "judge: %d/%d votes complete, %d failed this invocation\n", p.Completed, p.Total, p.Failed)
		}
		lastProgress = p
	}
	voteRecords, tournamentErr := judge.RunTournament(ctx, judge.TournamentConfig{
		OutDir: *outDir, Manifest: manifest, Jobs: jobs, Voter: voter,
		Workers: *workers, Limit: *limit, FailureLimit: *failureLimit, Progress: progress,
	})
	report, reportErr := judge.WriteReport(*outDir, manifest, voteRecords)
	if reportErr == nil {
		fmt.Fprintf(stdout, "%s\n", *outDir+"/report.md")
		fmt.Fprintf(stderr, "judge: report is %s (%d/%d votes)\n", map[bool]string{true: "complete", false: "partial"}[report.Complete], report.VotesCompleted, report.VotesExpected)
	}
	return errors.Join(tournamentErr, reportErr)
}
