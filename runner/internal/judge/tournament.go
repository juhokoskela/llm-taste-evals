package judge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
)

type TournamentConfig struct {
	OutDir       string
	Manifest     Manifest
	Jobs         []Job
	Voter        Voter
	Workers      int
	Limit        int
	FailureLimit int
	Progress     func(Progress)
}

func RunTournament(ctx context.Context, cfg TournamentConfig) ([]VoteRecord, error) {
	if cfg.OutDir == "" || cfg.Voter == nil {
		return nil, fmt.Errorf("output directory and voter are required")
	}
	if cfg.Workers < 1 {
		return nil, fmt.Errorf("workers must be positive")
	}
	if cfg.FailureLimit < 1 {
		return nil, fmt.Errorf("failure limit must be positive")
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	unlock, err := lockOutput(cfg.OutDir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := ensureManifest(cfg.OutDir, cfg.Manifest); err != nil {
		return nil, err
	}

	votes, completed, err := loadVotes(filepath.Join(cfg.OutDir, "votes.jsonl"))
	if err != nil {
		return nil, err
	}
	pending := make([]Job, 0, len(cfg.Jobs)-len(completed))
	for _, job := range cfg.Jobs {
		if !completed[job.Key] {
			pending = append(pending, job)
		}
	}
	if cfg.Limit > 0 && len(pending) > cfg.Limit {
		pending = pending[:cfg.Limit]
	}

	progress := Progress{Completed: len(votes), Total: len(cfg.Jobs), Pending: len(cfg.Jobs) - len(votes)}
	if err := writeProgress(cfg.OutDir, progress); err != nil {
		return nil, err
	}
	if cfg.Progress != nil {
		cfg.Progress(progress)
	}
	if len(pending) == 0 {
		return votes, nil
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobs := make(chan Job)
	results := make(chan VoteResult)
	var workers sync.WaitGroup
	for range cfg.Workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-runCtx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					results <- cfg.Voter.Vote(runCtx, job)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, job := range pending {
			select {
			case jobs <- job:
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	consecutiveFailures := 0
	for result := range results {
		artifacts, persistErr := persistAttempts(cfg.OutDir, result)
		if persistErr != nil {
			cancel(persistErr)
			continue
		}
		if result.Err != nil {
			if errors.Is(result.Err, context.Canceled) && context.Cause(runCtx) != nil {
				continue
			}
			failure := FailureRecord{
				Key: result.Job.Key, PairKey: result.Job.PairKey, Vote: result.Job.Vote,
				CandidateA: result.Job.CandidateA, CandidateB: result.Job.CandidateB,
				Error: result.Err.Error(), Artifacts: artifacts, CompletedAt: time.Now().UTC(),
			}
			if err := appendJSON(filepath.Join(cfg.OutDir, "failures.jsonl"), failure); err != nil {
				cancel(err)
				continue
			}
			progress.Failed++
			consecutiveFailures++
			if consecutiveFailures >= cfg.FailureLimit {
				cancel(fmt.Errorf("stopped after %d consecutive vote failures", consecutiveFailures))
			}
		} else {
			record := makeVoteRecord(result, artifacts)
			if err := appendJSON(filepath.Join(cfg.OutDir, "votes.jsonl"), record); err != nil {
				cancel(err)
				continue
			}
			votes = append(votes, record)
			progress.Completed++
			progress.Pending = progress.Total - progress.Completed
			consecutiveFailures = 0
		}
		if err := writeProgress(cfg.OutDir, progress); err != nil {
			cancel(err)
		}
		if cfg.Progress != nil {
			cfg.Progress(progress)
		}
	}
	if cause := context.Cause(runCtx); cause != nil {
		return votes, cause
	}
	if progress.Failed > 0 {
		return votes, fmt.Errorf("%d votes failed; rerun to retry them", progress.Failed)
	}
	return votes, nil
}

func makeVoteRecord(result VoteResult, artifacts []Artifact) VoteRecord {
	var winnerID string
	switch result.Decision.Winner {
	case "A":
		winnerID = result.Job.CandidateA
	case "B":
		winnerID = result.Job.CandidateB
	}
	var duration time.Duration
	var usage harness.Usage
	for _, artifact := range artifacts {
		duration += artifact.Duration
		usage.InputTokens += artifact.Usage.InputTokens
		usage.CacheReadTokens += artifact.Usage.CacheReadTokens
		usage.CacheCreationTokens += artifact.Usage.CacheCreationTokens
		usage.OutputTokens += artifact.Usage.OutputTokens
	}
	return VoteRecord{
		Key: result.Job.Key, PairKey: result.Job.PairKey, Vote: result.Job.Vote,
		CandidateA: result.Job.CandidateA, CandidateB: result.Job.CandidateB,
		Winner: result.Decision.Winner, WinnerID: winnerID,
		Confidence: result.Decision.Confidence, Summary: result.Decision.Summary,
		Reasons: result.Decision.Reasons, RisksA: result.Decision.RisksA, RisksB: result.Decision.RisksB,
		Attempts: len(result.Attempts), Duration: duration, Usage: usage, Artifacts: artifacts,
		CompletedAt: time.Now().UTC(),
	}
}

func persistAttempts(outDir string, result VoteResult) ([]Artifact, error) {
	rawDir := filepath.Join(outDir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return nil, fmt.Errorf("create raw output directory: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	artifacts := make([]Artifact, 0, len(result.Attempts))
	for _, attempt := range result.Attempts {
		base := fmt.Sprintf("%s-%s-attempt-%02d", result.Job.Key, stamp, attempt.Number)
		rawName := filepath.Join("raw", base+".jsonl")
		if err := writeFileAtomic(filepath.Join(outDir, rawName), attempt.Raw, 0o644); err != nil {
			return nil, err
		}
		artifact := Artifact{
			Attempt: attempt.Number, StartedAt: attempt.StartedAt, Duration: attempt.Duration,
			RawPath: filepath.ToSlash(rawName), Usage: attempt.Usage,
		}
		if len(attempt.Response) > 0 {
			responseName := filepath.Join("raw", base+".response.json")
			if err := writeFileAtomic(filepath.Join(outDir, responseName), attempt.Response, 0o644); err != nil {
				return nil, err
			}
			artifact.ResponsePath = filepath.ToSlash(responseName)
		}
		if attempt.Err != nil {
			artifact.Error = attempt.Err.Error()
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func ensureManifest(outDir string, want Manifest) error {
	path := filepath.Join(outDir, "manifest.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		encoded, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			return fmt.Errorf("encode manifest: %w", err)
		}
		return writeFileAtomic(path, append(encoded, '\n'), 0o644)
	}
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var got Manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("judge inputs differ from %s; use a new output directory", path)
	}
	return nil
}

func loadVotes(path string) ([]VoteRecord, map[string]bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, make(map[string]bool), nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open votes: %w", err)
	}
	defer file.Close()
	var votes []VoteRecord
	completed := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		var vote VoteRecord
		if err := json.Unmarshal(scanner.Bytes(), &vote); err != nil {
			return nil, nil, fmt.Errorf("parse votes line %d: %w", len(votes)+1, err)
		}
		if vote.Key == "" || completed[vote.Key] {
			return nil, nil, fmt.Errorf("invalid or duplicate completed vote key %q", vote.Key)
		}
		completed[vote.Key] = true
		votes = append(votes, vote)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read votes: %w", err)
	}
	return votes, completed, nil
}

func appendJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(value); err != nil {
		file.Close()
		return fmt.Errorf("append %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func writeProgress(outDir string, progress Progress) error {
	raw, err := json.MarshalIndent(progress, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(outDir, "progress.json"), append(raw, '\n'), 0o644)
}

func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-judge-")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func lockOutput(outDir string) (func(), error) {
	path := filepath.Join(outDir, "judge.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) && staleLock(path) {
		if removeErr := os.Remove(path); removeErr != nil {
			return nil, fmt.Errorf("remove stale judge lock %s: %w", path, removeErr)
		}
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	}
	if err != nil {
		return nil, fmt.Errorf("lock judge output %s: %w", path, err)
	}
	_, writeErr := fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

func staleLock(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := strings.TrimSpace(string(raw))
	pid, err := strconv.Atoi(strings.TrimPrefix(text, "pid="))
	if err != nil || pid < 1 {
		return true
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	err = process.Signal(syscall.Signal(0))
	return err != nil && !errors.Is(err, syscall.EPERM)
}
