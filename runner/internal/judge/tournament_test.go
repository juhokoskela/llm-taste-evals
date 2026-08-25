package judge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type trackingVoter struct {
	calls  atomic.Int32
	active atomic.Int32
	max    atomic.Int32
	delay  time.Duration
	fail   bool
}

func (v *trackingVoter) Vote(ctx context.Context, job Job) VoteResult {
	v.calls.Add(1)
	active := v.active.Add(1)
	defer v.active.Add(-1)
	for {
		seen := v.max.Load()
		if active <= seen || v.max.CompareAndSwap(seen, active) {
			break
		}
	}
	select {
	case <-time.After(v.delay):
	case <-ctx.Done():
		return VoteResult{Job: job, Err: context.Cause(ctx)}
	}
	attempt := Attempt{
		Number: 1, StartedAt: time.Now().UTC(), Duration: v.delay,
		Raw: []byte(`{"type":"turn.completed"}` + "\n"), Response: []byte(`{"winner":"A"}`),
	}
	if v.fail {
		attempt.Err = errors.New("rate limited")
		return VoteResult{Job: job, Attempts: []Attempt{attempt}, Err: attempt.Err}
	}
	return VoteResult{
		Job: job, Attempts: []Attempt{attempt},
		Decision: Decision{Winner: "A", Confidence: "high", Summary: "A is better", Reasons: []string{"smaller scope"}},
	}
}

func TestRunTournamentBoundsConcurrencyAndResumes(t *testing.T) {
	t.Parallel()
	outDir := t.TempDir()
	jobs := testJobs(12)
	manifest := testManifest()
	voter := &trackingVoter{delay: 10 * time.Millisecond}
	votes, err := RunTournament(context.Background(), TournamentConfig{
		OutDir: outDir, Manifest: manifest, Jobs: jobs, Voter: voter,
		Workers: 4, FailureLimit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(votes) != len(jobs) || voter.calls.Load() != int32(len(jobs)) {
		t.Fatalf("votes=%d calls=%d", len(votes), voter.calls.Load())
	}
	if voter.max.Load() < 2 || voter.max.Load() > 4 {
		t.Fatalf("maximum concurrency = %d, want 2..4", voter.max.Load())
	}
	if _, err := os.Stat(filepath.Join(outDir, "judge.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock survived completed run: %v", err)
	}
	raw, err := filepath.Glob(filepath.Join(outDir, "raw", "*.jsonl"))
	if err != nil || len(raw) != len(jobs) {
		t.Fatalf("raw artifacts=%d err=%v", len(raw), err)
	}

	second := &trackingVoter{delay: time.Millisecond}
	resumed, err := RunTournament(context.Background(), TournamentConfig{
		OutDir: outDir, Manifest: manifest, Jobs: jobs, Voter: second,
		Workers: 4, FailureLimit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed) != len(jobs) || second.calls.Load() != 0 {
		t.Fatalf("resume votes=%d new calls=%d", len(resumed), second.calls.Load())
	}
}

func TestRunTournamentStopsAfterFailureLimit(t *testing.T) {
	t.Parallel()
	voter := &trackingVoter{delay: time.Millisecond, fail: true}
	_, err := RunTournament(context.Background(), TournamentConfig{
		OutDir: t.TempDir(), Manifest: testManifest(), Jobs: testJobs(100), Voter: voter,
		Workers: 3, FailureLimit: 2,
	})
	if err == nil {
		t.Fatal("expected circuit-breaker error")
	}
	if calls := voter.calls.Load(); calls >= 100 {
		t.Fatalf("circuit breaker dispatched all %d jobs", calls)
	}
}

func TestRunTournamentRecoversStaleLock(t *testing.T) {
	t.Parallel()
	outDir := t.TempDir()
	writeTestFile(t, filepath.Join(outDir, "judge.lock"), "pid=999999999\n")
	voter := &trackingVoter{delay: time.Millisecond}
	_, err := RunTournament(context.Background(), TournamentConfig{
		OutDir: outDir, Manifest: testManifest(), Jobs: testJobs(1), Voter: voter,
		Workers: 1, FailureLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testJobs(count int) []Job {
	jobs := make([]Job, count)
	for i := range jobs {
		jobs[i] = Job{Key: shortHash(string(rune(i + 1))), PairKey: "pair", Vote: i + 1, CandidateA: "a", CandidateB: "b"}
	}
	return jobs
}

func testManifest() Manifest {
	return Manifest{
		FormatVersion: formatVersion, Task: "task", TaskVersion: 1, VotesPerPair: 3,
		Judge: JudgeConfig{Provider: "test", Model: "test", Effort: "test"},
	}
}
