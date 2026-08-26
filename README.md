# llm-taste-evals

An evaluation harness for SWE agents that measures code quality and process
quality, not "does it compile with green tests". Modern frontier models mostly
clear the correctness bar. What separates them is *how* they get to a solution
and whether the diff they leave behind is mergeable.

Summary piece available at [juhokoskela.fi](https://juhokoskela.fi/research/a-benchmark-score-is-not-a-model/).

## Methodology

Each task is a real change on a real repository, scored in layers:

1. **Gates** (deterministic, must pass): build, vet, fmt, hidden behavior tests,
   full existing suite, no dependency changes, no API breakage.
2. **Mechanical taste signals** (deterministic, scored): did the diff reuse the
   repo's established helpers or hand-roll a copy; scope creep; diff size.
3. **Trajectory metrics** (from the transcript): read-before-write, discovery
   path, self-verification, question economy against a user-simulator fact
   sheet, tool-call and token totals.
4. **LLM judge** (pairwise, Bradley-Terry): "which diff would you rather review
   and merge", anchored by the reference solution as a fixed contestant.

Every task is built so that a *behaviorally correct but tasteless* solution
passes every gate. Separating "can it make it work" from "would this be merged"
is the entire point. Each task ships a validation log showing the separation
hold: the reference passes everything, and a hand-written lazy solution passes
the gates while tripping the taste signals.

Run each model on each task with 3–5 seeds minimum; single-run trajectory
variance is too high to conclude anything.

## Tasks

| Task | Repo | Dimension | Results |
| --- | --- | --- | --- |
| [pd-files-upload](tasks/pd-files-upload/spec.md) | pipedrive-go (public) | Reuse discovery, API evolution under v1.x stability, scope discipline | [16 configurations](results/pd-files-upload.md) |
| [ab-jira-parent](tasks/ab-jira-parent/spec.md) | agent-bridge (private) | Ask-vs-infer on input shape, helper discovery | [17 configurations](results/ab-jira-parent.md) |

`ab-jira-parent` runs against a private repository. Its prompt, fact sheet,
scoring design, and results are published; its pinned base commit, hidden tests,
reference patch, and `checks.sh` are not, because they contain that repo's
source. It is therefore not runnable from this clone, and its published results
carry no diffs, transcripts, or repo identifiers. Planned: a root-cause-depth
task.

`pd-files-upload` ships its hidden tests and reference solutions, because its
repo is public anyway. That makes the task reproducible and also contaminable.
Treat results on it from models with a cutoff after this repo's publication as
suspect.

## Results

[results/](results/) holds one leaderboard per task: score, cost, token use, and
cache hit per configuration, then a per-run write-up of what each model actually
did. Both tasks ran 3 seeds per configuration.

Nothing here is close to solved. The best score on `pd-files-upload` was 5.50 of
10, and on `ab-jira-parent` two runs out of 51 passed the hidden tests, both
after a simulator correction.

Score is a weighted composite out of 10:

| Weight | Dimension |
| ---: | --- |
| 30% | Implementation correctness and golden drift |
| 25% | Idiomatic Go |
| 20% | Execution and trajectory |
| 15% | Tests, documentation, and scope |
| 10% | Tool efficiency |

Cost and token use are recorded and discussed per model. They never move anyone
up or down a table. The pairwise judge in [judgments/](judgments/) is a separate
stage with its own ranking, which is not this one.

## Runner

`runner/` is a Go CLI (stdlib only) that executes attempts: it clones the
task's repo at the pinned base commit (removing the origin remote so the agent
cannot fetch the solution from post-base history), drives a vendor agent CLI
headlessly, simulates the user at every turn boundary, normalizes the
transcript into the common event schema, and scores the workspace with the
task's `checks.sh`.

```
cd runner && go build ./cmd/runner
ANTHROPIC_API_KEY=... ./runner \
  -task ../tasks/pd-files-upload \
  -harness claudecode \        # or: codex, opencode
  -model claude-sonnet-5 \     # opencode uses provider/model, e.g. anthropic/claude-sonnet-5
  -effort high \
  -runs 3
```

`-effort` sets reasoning effort and is part of the run identity (recorded in
`result.json`, embedded in the run id). The runner passes it through verbatim in
each harness's own vocabulary. claudecode takes low/medium/high/xhigh/max via
`--effort`, codex takes minimal/low/medium/high/xhigh via
`model_reasoning_effort`, and opencode passes it as `--variant`, whose values are
provider-specific. Nothing translates between vendors, by design. Pick
per-harness values yourself and treat effort as one more axis of the comparison
rather than a constant.

`opencode` is the neutral arm. One wrapper runs across every vendor, so
model-vs-model comparisons through it isolate the model from the vendor CLI's own
prompting and tooling. It authenticates from the same env keys and works offline
behind the egress proxy, because its bundled model catalog covers the blocked
models.dev fetch.

Each attempt writes `runs/<task>/<harness>/<run-id>/` containing
`events.jsonl` (normalized trajectory), `raw-turn-N.jsonl` (untouched CLI
output), `result.json` (gates, signals, metrics, usage), and the `workspace/`
the agent worked in.

Stored attempts for `pd-files-upload` are published under [runs/](runs/), and the
judge output for them under [judgments/](judgments/). Two exclusions: archived
`workspace/` directories are gitignored (each is an ~11 MB clone of the pinned
base repo, reproducible from `diff.patch` plus the base commit), and runs for
private-repo tasks are gitignored wholesale, since a transcript of an agent
reading a private repo contains that repo's source. Publishing runs for a new
task is opt-in in [.gitignore](.gitignore), not the default.

One published arm, `fireworks-ai/qwen3.8-27b`, ran against a custom Fireworks
deployment rather than a public model path. That model id names the arm and will
not resolve as an endpoint. The deployment no longer exists, and its runs and
votes are kept as recorded.

### Isolation: run it in Compose

Valid runs need real isolation. Task solutions sit in public git history, so the
agent must not reach GitHub or the Go module proxy. `docker-compose.yml` provides
that:

- The runner container sits on an internal-only network. Its one route out is a
  squid egress proxy whose domain allowlist
  ([docker/allowlist.txt](docker/allowlist.txt)) admits model APIs only.
- The image build pre-warms the Go module cache and runs use `GOPROXY=off`. That
  closes the `go get <task-repo>@<fixed-version>` hole while `go test` keeps
  working offline.
- Fresh `HOME` inside the container – no global `CLAUDE.md`/`AGENTS.md`,
  settings, or MCP servers leak in. Auth comes from env vars, and one vendor key
  is enough per run: the user simulator rides whichever key is present
  (`ANTHROPIC_API_KEY` → Haiku, `OPENAI_API_KEY` → gpt-5-mini, override with
  `-sim-model`). The runner copies Codex auth from `OPENAI_API_KEY` into a
  fresh `CODEX_HOME` for each attempt.
- Pinned toolchain: Go 1.26.6, Node 26, exact agent CLI versions as build
  args (see [docker/Dockerfile](docker/Dockerfile)).
- Post-run checks use a frozen copy of the contestant workspace under a
  separate UID. The scorer receives only `checks.sh` and the hidden overlay,
  has no provider credentials, and can use loopback test fixtures but no other
  network destination.

```
ANTHROPIC_API_KEY=... docker compose run --rm runner \
  -task /eval-private/tasks/pd-files-upload \
  -harness claudecode -model claude-sonnet-5 -runs 3 \
  -repo /eval-private/mirrors/pipedrive-go -out /eval-private/runs
```

The runner is the only root process. Task definitions, the source mirror, and
stored attempts live below the root-only `/eval-private` directory. Before each
attempt the runner creates a fresh `/agent` workspace, home, and temporary
directory, then starts the contestant CLI as UID 1000 with no privilege regain.
After the turn loop, the runner kills leftover contestant processes and archives
the workspace. Checks run against a separate `/score` copy as UID 1001, so
candidate tests cannot read evaluator inputs or modify the archived attempt.

The committed compose file mounts only what this repo ships: `tasks/`, `runs/`,
and a mirror of the public task repo at `../pipedrive-go` (override with
`PD_MIRROR`). Private tasks and their mirrors are mounted from a gitignored
`docker-compose.override.yml`, which Compose loads automatically:

```yaml
services:
  runner:
    volumes:
      - ./tasks-private:/eval-private/tasks-private:ro
      - ${AB_MIRROR:-../agent-bridge}:/eval-private/mirrors/agent-bridge:ro
```

Bare-metal runs remain possible for development (`EVAL_CLAUDE_CONFIG_DIR` /
`EVAL_CODEX_HOME` give config isolation) but do not have the filesystem,
identity, or network boundary used for scoring private tasks. Do not publish
results from them.

## Pairwise merge-quality judge

`cmd/judge` is a separate post-scoring stage. It admits only substantive runs
that passed every mandatory gate, adds the task's reference patch as an anchor,
and obtains three blinded votes for every pair. Gate-failed runs receive zero
mergeability during model aggregation; empty runs and harness failures are
excluded.

The command invokes `codex exec` with the local ChatGPT subscription login. It
runs votes concurrently with a fixed worker limit, disables tools and user
configuration, and persists each vote before scheduling more work. Reusing the
same output directory resumes from completed job keys; a changed patch, task,
contract, anchor, model, effort, or vote count requires a new output directory.

```sh
cd runner
go run ./cmd/judge \
  -task ../tasks/pd-files-upload \
  -runs ../runs/pd-files-upload \
  -out ../judgments/pd-files-upload/codex-gpt-5.6-sol-xhigh-v1 \
  -model gpt-5.6-sol \
  -effort xhigh \
  -workers 3
```

The output contains a frozen `manifest.json`, append-only `votes.jsonl`, raw
Codex JSONL transcripts, resumable `progress.json`, `report.json`, a readable
`report.md`, and the model-by-model `head-to-head.csv` matrix. The primary
ranking is an order-independent Bradley-Terry fit anchored at 1500; the report
also retains direct majority results against the reference and presentation-
side counts for position-bias auditing.

Pin and record the CLI versions you run; `--json` output shapes drift between
releases. Unknown transcript events degrade to `other` and the raw output is
always archived, so a version bump skews trajectory metrics rather than
crashing. Check `raw-turn-*.jsonl` if the metrics look off.

## Task anatomy

```
tasks/<task-id>/
  task.md        # agent-facing prompt (written as a bug report / feature request)
  spec.md        # evaluator spec: pinned base commit, fact sheet, scoring, validation log
  overlay/       # hidden tests, overlaid onto the candidate tree after the run
  reference/     # reference solution as a patch; judge anchor
  checks.sh      # gates + mechanical taste signals
```

A task on a private repo keeps this layout in a gitignored `tasks-private/`, and
publishes only `task.md` and a redacted `spec.md` under `tasks/`. See
[tasks/ab-jira-parent](tasks/ab-jira-parent/spec.md).

## License

MIT. See [LICENSE](LICENSE).
