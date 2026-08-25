// Package judge runs blinded pairwise merge-quality tournaments over scored
// eval attempts.
package judge

import (
	"context"
	"time"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
)

const formatVersion = 1

type Run struct {
	Task        string            `json:"task"`
	TaskVersion int               `json:"task_version"`
	Harness     string            `json:"harness"`
	Model       string            `json:"model"`
	Effort      string            `json:"effort,omitempty"`
	RunID       string            `json:"run_id"`
	EndReason   string            `json:"end_reason"`
	GatesFailed int               `json:"gates_failed"`
	Usage       harness.Usage     `json:"usage"`
	ResultPath  string            `json:"result_path"`
	PatchPath   string            `json:"patch_path"`
	PatchSHA256 string            `json:"patch_sha256"`
	Signals     map[string]string `json:"signals,omitempty"`
}

func (r Run) groupKey() string {
	return r.Harness + "\x00" + r.Model + "\x00" + r.Effort
}

type Candidate struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	RunID       string `json:"run_id,omitempty"`
	Harness     string `json:"harness,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	PatchPath   string `json:"patch_path"`
	PatchSHA256 string `json:"patch_sha256"`

	patch string
}

type Exclusion struct {
	ResultPath string `json:"result_path"`
	RunID      string `json:"run_id,omitempty"`
	Reason     string `json:"reason"`
}

type JudgeConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
}

type Manifest struct {
	FormatVersion  int         `json:"format_version"`
	Task           string      `json:"task"`
	TaskVersion    int         `json:"task_version"`
	TaskPromptHash string      `json:"task_prompt_sha256"`
	ContractHash   string      `json:"contract_sha256"`
	AnchorHash     string      `json:"anchor_sha256"`
	VotesPerPair   int         `json:"votes_per_pair"`
	Judge          JudgeConfig `json:"judge"`
	Runs           []Run       `json:"runs"`
	Candidates     []Candidate `json:"candidates"`
	Excluded       []Exclusion `json:"excluded"`
}

type Job struct {
	Key        string `json:"key"`
	PairKey    string `json:"pair_key"`
	Vote       int    `json:"vote"`
	CandidateA string `json:"candidate_a"`
	CandidateB string `json:"candidate_b"`

	prompt string
}

type Decision struct {
	Winner     string   `json:"winner"`
	Confidence string   `json:"confidence"`
	Summary    string   `json:"summary"`
	Reasons    []string `json:"reasons"`
	RisksA     []string `json:"risks_a"`
	RisksB     []string `json:"risks_b"`
}

type Attempt struct {
	Number    int
	StartedAt time.Time
	Duration  time.Duration
	Raw       []byte
	Response  []byte
	Usage     harness.Usage
	Err       error
}

type VoteResult struct {
	Job      Job
	Decision Decision
	Attempts []Attempt
	Err      error
}

type VoteRecord struct {
	Key         string        `json:"key"`
	PairKey     string        `json:"pair_key"`
	Vote        int           `json:"vote"`
	CandidateA  string        `json:"candidate_a"`
	CandidateB  string        `json:"candidate_b"`
	Winner      string        `json:"winner"`
	WinnerID    string        `json:"winner_id,omitempty"`
	Confidence  string        `json:"confidence"`
	Summary     string        `json:"summary"`
	Reasons     []string      `json:"reasons"`
	RisksA      []string      `json:"risks_a"`
	RisksB      []string      `json:"risks_b"`
	Attempts    int           `json:"attempts"`
	Duration    time.Duration `json:"duration"`
	Usage       harness.Usage `json:"usage"`
	Artifacts   []Artifact    `json:"artifacts"`
	CompletedAt time.Time     `json:"completed_at"`
}

type Artifact struct {
	Attempt      int           `json:"attempt"`
	StartedAt    time.Time     `json:"started_at"`
	Duration     time.Duration `json:"duration"`
	RawPath      string        `json:"raw_path"`
	ResponsePath string        `json:"response_path,omitempty"`
	Usage        harness.Usage `json:"usage"`
	Error        string        `json:"error,omitempty"`
}

type FailureRecord struct {
	Key         string     `json:"key"`
	PairKey     string     `json:"pair_key"`
	Vote        int        `json:"vote"`
	CandidateA  string     `json:"candidate_a"`
	CandidateB  string     `json:"candidate_b"`
	Error       string     `json:"error"`
	Artifacts   []Artifact `json:"artifacts"`
	CompletedAt time.Time  `json:"completed_at"`
}

type Voter interface {
	Vote(context.Context, Job) VoteResult
}

type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
}
