package judge

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/juhokoskela/llm-taste-evals/runner/internal/harness"
)

type PairOutcome struct {
	PairKey  string `json:"pair_key"`
	LeftID   string `json:"left_id"`
	RightID  string `json:"right_id"`
	WinnerID string `json:"winner_id,omitempty"`
	Tied     bool   `json:"tied"`
	Votes    int    `json:"votes"`
}

func (o PairOutcome) valueFor(id string) float64 {
	if o.Tied {
		return 0.5
	}
	if o.WinnerID == id {
		return 1
	}
	return 0
}

type CandidateScore struct {
	Rank               int     `json:"rank"`
	ID                 string  `json:"id"`
	Kind               string  `json:"kind"`
	Harness            string  `json:"harness,omitempty"`
	Model              string  `json:"model,omitempty"`
	Effort             string  `json:"effort,omitempty"`
	BradleyTerryElo    float64 `json:"bradley_terry_elo"`
	WinVsReference     float64 `json:"win_probability_vs_reference"`
	DirectVsReference  float64 `json:"direct_result_vs_reference"`
	DirectPairComplete bool    `json:"direct_pair_complete"`
}

type ModelScore struct {
	Rank                    int     `json:"rank"`
	Harness                 string  `json:"harness"`
	Model                   string  `json:"model"`
	Effort                  string  `json:"effort,omitempty"`
	Runs                    int     `json:"runs"`
	GatePasses              int     `json:"gate_passes"`
	AverageMergeProbability float64 `json:"average_merge_probability"`
	PassedRunProbability    float64 `json:"passed_run_probability,omitempty"`
}

type HeadToHead struct {
	Row         string  `json:"row"`
	Column      string  `json:"column"`
	Probability float64 `json:"probability"`
	Samples     int     `json:"samples"`
}

type Report struct {
	Complete          bool             `json:"complete"`
	VotesCompleted    int              `json:"votes_completed"`
	VotesExpected     int              `json:"votes_expected"`
	PairsCompleted    int              `json:"pairs_completed"`
	PairsExpected     int              `json:"pairs_expected"`
	CandidateScores   []CandidateScore `json:"candidate_scores"`
	ModelScores       []ModelScore     `json:"model_scores"`
	HeadToHead        []HeadToHead     `json:"head_to_head"`
	JudgeUsage        harness.Usage    `json:"judge_usage"`
	PresentationAWins int              `json:"presentation_a_wins"`
	PresentationBWins int              `json:"presentation_b_wins"`
	TiedVotes         int              `json:"tied_votes"`
}

func WriteReport(outDir string, manifest Manifest, votes []VoteRecord) (*Report, error) {
	report, err := BuildReport(manifest, votes)
	if err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode report: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(outDir, "report.json"), append(raw, '\n'), 0o644); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(filepath.Join(outDir, "report.md"), []byte(renderMarkdown(manifest, report)), 0o644); err != nil {
		return nil, err
	}
	if err := writeHeadToHead(filepath.Join(outDir, "head-to-head.csv"), report); err != nil {
		return nil, err
	}
	return report, nil
}

func BuildReport(manifest Manifest, votes []VoteRecord) (*Report, error) {
	outcomes, err := majorityOutcomes(votes, manifest.VotesPerPair)
	if err != nil {
		return nil, err
	}
	expectedPairs := len(manifest.Candidates) * (len(manifest.Candidates) - 1) / 2
	report := &Report{
		VotesCompleted: len(votes), VotesExpected: expectedPairs * manifest.VotesPerPair,
		PairsCompleted: len(outcomes), PairsExpected: expectedPairs,
	}
	report.Complete = report.VotesCompleted == report.VotesExpected && report.PairsCompleted == report.PairsExpected
	for _, vote := range votes {
		report.JudgeUsage.InputTokens += vote.Usage.InputTokens
		report.JudgeUsage.CacheReadTokens += vote.Usage.CacheReadTokens
		report.JudgeUsage.CacheCreationTokens += vote.Usage.CacheCreationTokens
		report.JudgeUsage.OutputTokens += vote.Usage.OutputTokens
		switch vote.Winner {
		case "A":
			report.PresentationAWins++
		case "B":
			report.PresentationBWins++
		case "tie":
			report.TiedVotes++
		}
	}

	ratings := fitBradleyTerry(manifest.Candidates, outcomes)
	report.CandidateScores = scoreCandidates(manifest.Candidates, outcomes, ratings)
	report.ModelScores = scoreModels(manifest.Runs, ratings)
	report.HeadToHead = scoreHeadToHead(manifest.Runs, outcomes)
	return report, nil
}

func majorityOutcomes(votes []VoteRecord, votesPerPair int) ([]PairOutcome, error) {
	groups := make(map[string][]VoteRecord)
	for _, vote := range votes {
		groups[vote.PairKey] = append(groups[vote.PairKey], vote)
	}
	var outcomes []PairOutcome
	for pairKey, group := range groups {
		if len(group) > votesPerPair {
			return nil, fmt.Errorf("pair %s has %d votes, want at most %d", pairKey, len(group), votesPerPair)
		}
		if len(group) < votesPerPair {
			continue
		}
		ids := []string{group[0].CandidateA, group[0].CandidateB}
		sort.Strings(ids)
		counts := map[string]int{"tie": 0, ids[0]: 0, ids[1]: 0}
		seenVotes := make(map[int]bool, len(group))
		for _, vote := range group {
			if vote.Vote < 1 || vote.Vote > votesPerPair || seenVotes[vote.Vote] {
				return nil, fmt.Errorf("pair %s has invalid or duplicate vote number %d", pairKey, vote.Vote)
			}
			seenVotes[vote.Vote] = true
			pair := []string{vote.CandidateA, vote.CandidateB}
			sort.Strings(pair)
			if pair[0] != ids[0] || pair[1] != ids[1] {
				return nil, fmt.Errorf("pair %s contains inconsistent candidates", pairKey)
			}
			if vote.Winner == "tie" {
				counts["tie"]++
			} else if vote.WinnerID == ids[0] || vote.WinnerID == ids[1] {
				counts[vote.WinnerID]++
			} else {
				return nil, fmt.Errorf("pair %s has invalid winner %q", pairKey, vote.WinnerID)
			}
		}
		outcome := PairOutcome{PairKey: pairKey, LeftID: ids[0], RightID: ids[1], Votes: len(group)}
		if counts[ids[0]] > counts[ids[1]] && counts[ids[0]] > counts["tie"] {
			outcome.WinnerID = ids[0]
		} else if counts[ids[1]] > counts[ids[0]] && counts[ids[1]] > counts["tie"] {
			outcome.WinnerID = ids[1]
		} else {
			outcome.Tied = true
		}
		outcomes = append(outcomes, outcome)
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].PairKey < outcomes[j].PairKey })
	return outcomes, nil
}

func fitBradleyTerry(candidates []Candidate, outcomes []PairOutcome) map[string]float64 {
	index := make(map[string]int, len(candidates))
	for i, candidate := range candidates {
		index[candidate.ID] = i
	}
	scores := make([]float64, len(candidates))
	degrees := make([]int, len(candidates))
	for _, outcome := range outcomes {
		degrees[index[outcome.LeftID]]++
		degrees[index[outcome.RightID]]++
	}
	maxDegree := 1
	for _, degree := range degrees {
		if degree > maxDegree {
			maxDegree = degree
		}
	}
	const priorPrecision = 1.0 / 16.0
	step := 0.1 / float64(maxDegree)
	for range 100000 {
		gradient := make([]float64, len(scores))
		for _, outcome := range outcomes {
			left, right := index[outcome.LeftID], index[outcome.RightID]
			prediction := sigmoid(scores[left] - scores[right])
			residual := outcome.valueFor(outcome.LeftID) - prediction
			gradient[left] += residual
			gradient[right] -= residual
		}
		maxChange := 0.0
		for i := range candidates {
			gradient[i] -= priorPrecision * scores[i]
			change := step * gradient[i]
			scores[i] += change
			maxChange = max(maxChange, math.Abs(change))
		}
		if maxChange < 1e-10 {
			break
		}
	}
	referenceScore := 0.0
	if reference, ok := index["reference"]; ok {
		referenceScore = scores[reference]
	}
	ratings := make(map[string]float64, len(candidates))
	for i, candidate := range candidates {
		ratings[candidate.ID] = scores[i] - referenceScore
	}
	return ratings
}

func scoreCandidates(candidates []Candidate, outcomes []PairOutcome, ratings map[string]float64) []CandidateScore {
	direct := make(map[string]float64)
	for _, outcome := range outcomes {
		if outcome.LeftID == "reference" {
			direct[outcome.RightID] = outcome.valueFor(outcome.RightID)
		} else if outcome.RightID == "reference" {
			direct[outcome.LeftID] = outcome.valueFor(outcome.LeftID)
		}
	}
	scores := make([]CandidateScore, 0, len(candidates))
	for _, candidate := range candidates {
		logStrength := ratings[candidate.ID]
		score := CandidateScore{
			ID: candidate.ID, Kind: candidate.Kind, Harness: candidate.Harness,
			Model: candidate.Model, Effort: candidate.Effort,
			BradleyTerryElo: 1500 + logStrength*400/math.Log(10),
			WinVsReference:  sigmoid(logStrength),
		}
		if value, ok := direct[candidate.ID]; ok {
			score.DirectVsReference = value
			score.DirectPairComplete = true
		}
		scores = append(scores, score)
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].BradleyTerryElo != scores[j].BradleyTerryElo {
			return scores[i].BradleyTerryElo > scores[j].BradleyTerryElo
		}
		return scores[i].ID < scores[j].ID
	})
	for i := range scores {
		scores[i].Rank = i + 1
	}
	return scores
}

func scoreModels(runs []Run, ratings map[string]float64) []ModelScore {
	groups := make(map[string]*ModelScore)
	passedSums := make(map[string]float64)
	for _, run := range runs {
		key := run.groupKey()
		group := groups[key]
		if group == nil {
			group = &ModelScore{Harness: run.Harness, Model: run.Model, Effort: run.Effort}
			groups[key] = group
		}
		group.Runs++
		if run.GatesFailed == 0 {
			group.GatePasses++
			passedSums[key] += sigmoid(ratings[run.RunID])
		}
	}
	scores := make([]ModelScore, 0, len(groups))
	for key, group := range groups {
		group.AverageMergeProbability = passedSums[key] / float64(group.Runs)
		if group.GatePasses > 0 {
			group.PassedRunProbability = passedSums[key] / float64(group.GatePasses)
		}
		scores = append(scores, *group)
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].AverageMergeProbability != scores[j].AverageMergeProbability {
			return scores[i].AverageMergeProbability > scores[j].AverageMergeProbability
		}
		return modelLabel(scores[i]) < modelLabel(scores[j])
	})
	for i := range scores {
		scores[i].Rank = i + 1
	}
	return scores
}

func scoreHeadToHead(runs []Run, outcomes []PairOutcome) []HeadToHead {
	pairs := make(map[string]PairOutcome, len(outcomes))
	for _, outcome := range outcomes {
		pairs[runPairKey(outcome.LeftID, outcome.RightID)] = outcome
	}
	groups := make(map[string][]Run)
	for _, run := range runs {
		groups[run.groupKey()] = append(groups[run.groupKey()], run)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var results []HeadToHead
	for _, row := range keys {
		for _, column := range keys {
			if row == column {
				continue
			}
			var sum float64
			var samples int
			for _, a := range groups[row] {
				for _, b := range groups[column] {
					value, ok := runMatchValue(a, b, pairs)
					if ok {
						sum += value
						samples++
					}
				}
			}
			if samples > 0 {
				results = append(results, HeadToHead{
					Row: groupLabel(groups[row][0]), Column: groupLabel(groups[column][0]),
					Probability: sum / float64(samples), Samples: samples,
				})
			}
		}
	}
	return results
}

func runMatchValue(a, b Run, pairs map[string]PairOutcome) (float64, bool) {
	switch {
	case a.GatesFailed > 0 && b.GatesFailed > 0:
		return 0.5, true
	case a.GatesFailed > 0:
		return 0, true
	case b.GatesFailed > 0:
		return 1, true
	default:
		outcome, ok := pairs[runPairKey(a.RunID, b.RunID)]
		if !ok {
			return 0, false
		}
		return outcome.valueFor(a.RunID), true
	}
}

func runPairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
}

func sigmoid(value float64) float64 {
	if value >= 0 {
		exp := math.Exp(-value)
		return 1 / (1 + exp)
	}
	exp := math.Exp(value)
	return exp / (1 + exp)
}

func renderMarkdown(manifest Manifest, report *Report) string {
	var b strings.Builder
	b.WriteString("# Pairwise merge-quality report\n\n")
	fmt.Fprintf(&b, "- Task: `%s` v%d\n", manifest.Task, manifest.TaskVersion)
	fmt.Fprintf(&b, "- Judge: `%s` / `%s` / `%s`\n", manifest.Judge.Provider, manifest.Judge.Model, manifest.Judge.Effort)
	fmt.Fprintf(&b, "- Completion: %d/%d votes, %d/%d pairs (%s)\n", report.VotesCompleted, report.VotesExpected, report.PairsCompleted, report.PairsExpected, completeLabel(report.Complete))
	fmt.Fprintf(&b, "- Judge tokens: %d input (%d cache reads, %d cache writes), %d output\n", report.JudgeUsage.InputTokens, report.JudgeUsage.CacheReadTokens, report.JudgeUsage.CacheCreationTokens, report.JudgeUsage.OutputTokens)
	fmt.Fprintf(&b, "- Presented-side results: A %d, B %d, ties %d\n\n", report.PresentationAWins, report.PresentationBWins, report.TiedVotes)
	b.WriteString("Scores use majority outcomes from the blinded votes. Bradley-Terry strengths are fit independent of execution order, with the reference fixed at 1500 Elo and a weak Gaussian prior to keep separated data finite. A model's final score is the mean per-run probability of beating the reference; mandatory-gate failures contribute zero, and empty/harness-error runs are excluded.\n\n")
	b.WriteString("This judge is in the same broad model family as some contestants. Blinding removes model identity from each prompt, but it cannot eliminate family/style preference; treat the result as a Sol-judged view rather than a neutral cross-family ground truth.\n\n")
	b.WriteString("## Model ranking\n\n")
	b.WriteString("| Rank | Harness / model / effort | Runs | Gates | Avg merge probability | Passed-run probability |\n")
	b.WriteString("| ---: | --- | ---: | ---: | ---: | ---: |\n")
	for _, score := range report.ModelScores {
		fmt.Fprintf(&b, "| %d | %s | %d | %d/%d | %.1f%% | %.1f%% |\n", score.Rank, markdownEscape(modelLabel(score)), score.Runs, score.GatePasses, score.Runs, 100*score.AverageMergeProbability, 100*score.PassedRunProbability)
	}
	b.WriteString("\n## Gate-passing run ranking\n\n")
	b.WriteString("| Rank | Candidate | BT Elo | Win vs reference | Direct vs reference |\n")
	b.WriteString("| ---: | --- | ---: | ---: | ---: |\n")
	for _, score := range report.CandidateScores {
		direct := "—"
		if score.DirectPairComplete {
			direct = fmt.Sprintf("%.0f%%", 100*score.DirectVsReference)
		}
		fmt.Fprintf(&b, "| %d | `%s` | %.0f | %.1f%% | %s |\n", score.Rank, score.ID, score.BradleyTerryElo, 100*score.WinVsReference, direct)
	}
	b.WriteString("\nThe complete pairwise model matrix is in `head-to-head.csv`; raw structured votes and untouched Codex JSONL transcripts are retained beside this report.\n")
	return b.String()
}

func writeHeadToHead(path string, report *Report) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-head-to-head-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	writer := csv.NewWriter(tmp)
	if err := writer.Write([]string{"row", "column", "row_win_probability", "samples"}); err != nil {
		tmp.Close()
		return err
	}
	for _, value := range report.HeadToHead {
		if err := writer.Write([]string{value.Row, value.Column, strconv.FormatFloat(value.Probability, 'f', 6, 64), strconv.Itoa(value.Samples)}); err != nil {
			tmp.Close()
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
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
	return os.Rename(tmpName, path)
}

func modelLabel(score ModelScore) string {
	return strings.Join(nonEmpty(score.Harness, score.Model, score.Effort), " / ")
}

func groupLabel(run Run) string {
	return strings.Join(nonEmpty(run.Harness, run.Model, run.Effort), " / ")
}

func nonEmpty(values ...string) []string {
	result := values[:0]
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func completeLabel(complete bool) string {
	if complete {
		return "complete"
	}
	return "partial"
}

func markdownEscape(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
