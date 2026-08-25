package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/task"
)

type Inputs struct {
	Task       *task.Task
	Prompt     string
	Contract   string
	Runs       []Run
	Candidates []Candidate
	Excluded   []Exclusion
}

func LoadInputs(taskDir, runsDir string) (*Inputs, error) {
	t, err := task.Load(taskDir)
	if err != nil {
		return nil, err
	}
	contract, err := os.ReadFile(filepath.Join(taskDir, "judge.md"))
	if err != nil {
		return nil, fmt.Errorf("read judge contract: %w", err)
	}
	anchorPath := filepath.Join(taskDir, "reference", "reference.patch")
	anchor, err := os.ReadFile(anchorPath)
	if err != nil {
		return nil, fmt.Errorf("read reference patch: %w", err)
	}
	if len(anchor) == 0 {
		return nil, fmt.Errorf("reference patch is empty")
	}

	in := &Inputs{Task: t, Prompt: t.Prompt(), Contract: string(contract)}
	err = filepath.WalkDir(runsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "result.json" {
			return nil
		}
		return in.loadRun(runsDir, path)
	})
	if err != nil {
		return nil, fmt.Errorf("scan runs: %w", err)
	}
	if len(in.Runs) == 0 {
		return nil, fmt.Errorf("no substantive runs found in %s", runsDir)
	}

	in.Candidates = append(in.Candidates, Candidate{
		ID:          "reference",
		Kind:        "reference",
		PatchPath:   filepath.ToSlash(filepath.Join("reference", "reference.patch")),
		PatchSHA256: hashBytes(anchor),
		patch:       string(anchor),
	})
	sort.Slice(in.Runs, func(i, j int) bool { return in.Runs[i].RunID < in.Runs[j].RunID })
	sort.Slice(in.Candidates, func(i, j int) bool { return in.Candidates[i].ID < in.Candidates[j].ID })
	sort.Slice(in.Excluded, func(i, j int) bool { return in.Excluded[i].ResultPath < in.Excluded[j].ResultPath })
	return in, nil
}

func (in *Inputs) loadRun(runsDir, resultPath string) error {
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", resultPath, err)
	}
	var r Run
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("parse %s: %w", resultPath, err)
	}
	relResult, err := filepath.Rel(runsDir, resultPath)
	if err != nil {
		return err
	}
	r.ResultPath = filepath.ToSlash(relResult)
	patchPath := filepath.Join(filepath.Dir(resultPath), "diff.patch")
	patch, patchErr := os.ReadFile(patchPath)

	reason := ""
	switch {
	case r.EndReason == "harness_error":
		reason = "harness_error"
	case r.Usage.InputTokens == 0:
		reason = "empty_usage"
	case patchErr != nil:
		reason = "missing_patch"
	case len(patch) == 0:
		reason = "empty_patch"
	case r.Task != in.Task.Name || r.TaskVersion != in.Task.Version:
		return fmt.Errorf("%s belongs to task %s v%d, want %s v%d", resultPath, r.Task, r.TaskVersion, in.Task.Name, in.Task.Version)
	}
	if reason != "" {
		in.Excluded = append(in.Excluded, Exclusion{ResultPath: r.ResultPath, RunID: r.RunID, Reason: reason})
		return nil
	}

	relPatch, err := filepath.Rel(runsDir, patchPath)
	if err != nil {
		return err
	}
	r.PatchPath = filepath.ToSlash(relPatch)
	r.PatchSHA256 = hashBytes(patch)
	in.Runs = append(in.Runs, r)
	if r.GatesFailed == 0 {
		in.Candidates = append(in.Candidates, Candidate{
			ID:          r.RunID,
			Kind:        "run",
			RunID:       r.RunID,
			Harness:     r.Harness,
			Model:       r.Model,
			Effort:      r.Effort,
			PatchPath:   r.PatchPath,
			PatchSHA256: r.PatchSHA256,
			patch:       string(patch),
		})
	}
	return nil
}

func (in *Inputs) Manifest(judge JudgeConfig, votes int) Manifest {
	return Manifest{
		FormatVersion:  formatVersion,
		Task:           in.Task.Name,
		TaskVersion:    in.Task.Version,
		TaskPromptHash: hashBytes([]byte(in.Prompt)),
		ContractHash:   hashBytes([]byte(in.Contract)),
		AnchorHash:     candidateByID(in.Candidates, "reference").PatchSHA256,
		VotesPerPair:   votes,
		Judge:          judge,
		Runs:           append([]Run(nil), in.Runs...),
		Candidates:     manifestCandidates(in.Candidates),
		Excluded:       append([]Exclusion(nil), in.Excluded...),
	}
}

func manifestCandidates(candidates []Candidate) []Candidate {
	out := make([]Candidate, len(candidates))
	for i, candidate := range candidates {
		candidate.patch = ""
		out[i] = candidate
	}
	return out
}

func candidateByID(candidates []Candidate, id string) Candidate {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	panic("unknown candidate " + id)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
