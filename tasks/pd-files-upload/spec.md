# pd-files-upload — evaluator spec

Task type: feature addition with a reuse trap. Primary quality dimension measured:
**does the model discover and reuse the repo's established pattern, or hand-roll a
third copy of it** — plus API-evolution judgment under a stability constraint and
scope discipline under deliberate underspecification.

## Provenance

Derived from investigating pipedrive-go commits `f6c29ea` / `83255ab` / `e0acb4a`
(the `internal/multipartbody` refactor). The original revert-and-reimplement idea
was rejected: empirical probing showed those commits have **zero externally
observable behavior delta** (input validation predated them; `http.NewRequest`
already sets `GetBody` for `*bytes.Buffer`), so hidden tests could not
discriminate and the prompt would have given away the refactor.

Instead the task points at a real, still-unfixed wart: `FilesService.Add` takes a
caller-encoded body as a bare `io.Reader`, so a non-seekable body never gets
`request.GetBody` and the retry transport silently refuses to retry it. Verified
empirically at the base commit: a 429 on `Files.Add` with an `io.MultiReader`
body → 1 attempt; with `*bytes.Reader` → retried.

## Repo state

- Repo: `github.com/juhokoskela/pipedrive-go` (public)
- Base commit (pinned): `40ffab19c51ea1a210f17cf37bb1f08c1340352d` (v1.11.0)
- The agent works at the base commit with full git history available. History
  archaeology (`git log -S multipartbody`) is an intended discovery channel, as
  are the two existing call sites (`pipedrive/v1/call_logs.go`,
  `pipedrive/v2/products.go`).

## The trap

At the base commit, `internal/multipartbody.NewFile` is the established
convention (two call sites). The reference solution is ~30 production lines that
delegate to it. A behaviorally correct solution that hand-rolls
`mime/multipart` encoding at the call site **passes all hidden tests**
(verified — see Validation log). Distinguishing the two is entirely the job of
the taste layers. That separation is deliberate: it isolates "can it make it
work" from "would this be merged".

## User simulator fact sheet

The prompt intentionally leaves these open. The simulator answers only what is
asked, in one or two sentences, and answers anything not covered here with
"use your judgment".

| If asked about | Answer |
| --- | --- |
| Deprecate `Add`? | No. Leave `Add` as-is; it still serves callers that need custom multipart fields. |
| Same treatment for `Update`? | Out of scope for this change. |
| Buffering the file in memory? | Acceptable — consistent with how recording and image uploads already work. |
| Extra form fields / deal associations? | No new options. Existing `FilesOption` behavior stays as it is. |
| Method name or signature changes? | No — the signature in the task is what we want. |

Scoring hooks:
- **Asking for the API shape is the blocking probe** (prompt v2): the prompt is
  just the user report — the `Upload` name/signature lives only in the fact
  sheet, and hidden tests compile against it. An agent that never asks and
  guesses a different name fails the hidden-test gate *by design*; the
  `upload_method_present` signal separates that from behavioral failure.
- Asked about `Update` (or flagged it as a same-wart follow-up): positive.
- Silently also rewrote `Update`: scope creep, negative.
- Asked questions answerable from the repo (e.g. "is there an existing multipart
  helper?"): negative (outsourcing reading).

## Scoring layers

### Layer 1 — gates (must pass; `checks.sh` exits non-zero otherwise)
1. `go build ./...`, `go vet ./pipedrive/...`
2. `gofmt -l` clean on changed files
3. `go.mod` / `go.sum` unchanged
4. Legacy `Add` signature intact
5. Hidden tests: overlay `overlay/zz_eval_hidden_test.go` into `pipedrive/v1/`,
   run `go test -run TestHidden_ ./pipedrive/v1/`
6. Full existing suite: `go test ./pipedrive/...`

### Layer 2 — mechanical taste signals (reported by `checks.sh`, scored)
- `reuses_multipartbody`: production diff references `internal/multipartbody`
- `handrolls_multipart`: production diff adds `mime/multipart` usage in
  `pipedrive/v1` non-test code
- `delegates_to_add`: production diff routes `Upload` through the existing
  `Add` method instead of duplicating its transport plumbing. The taste
  ladder is three rungs: hand-rolled < helper-but-duplicated-transport <
  full delegation — the top rung separates solutions that saturate the
  first two signals
- `existing_test_lines_deleted`: deletions inside pre-existing test files.
  Nothing in scope requires changing established behavior, so a non-zero
  count usually means the candidate rewrote the suite to legitimize a
  breaking change (observed live: a candidate changed `Add`'s public
  signature and retrofitted `TestFilesService_Add` to match)
- `files_changed` / `out_of_scope_paths`: anything outside `pipedrive/v1/`
  (docs excepted) is scope creep
- `diff_size`: insertions+deletions; reference anchor is 154 (30 production)

### Layer 3 — trajectory metrics (from transcript)
- Read-before-write on every edited file
- Discovery: did it open `call_logs.go` / grep multipart before implementing?
- Verification: did it run the tests it wrote? The full suite?
- Question economy: see fact-sheet scoring hooks
- Totals: tool calls, tokens, wall-clock

### Layer 4 — LLM judge, pairwise
Judge sees the task prompt + two diffs (candidate vs. `reference/reference.patch`,
and candidate-vs-candidate for Elo across models): "Which would you rather
review and merge? Ignore diff length except where it reflects unnecessary code."
Reference is a fixed contestant to anchor scores across runs. A judge from a
different model family than the contestants is **required**, not preferred:
the current anchor's structure originated in a contestant run (see anchor v2
provenance below), so a same-family judge would score candidates against an
anchor its own family authored. 3 votes, majority.

## Reference anchor versions

Pairwise results against different anchors are not comparable; anchor changes
bump a version just like prompt changes.

- **v1** (2026-08-19, `reference/reference-v1.patch`): validation +
  `multipartbody.NewFile` + its own `Raw.Do` request/response handling.
  Superseded: it duplicates `Add`'s transport plumbing.
- **v2** (2026-08-21, current `reference/reference.patch`): same validation
  and encoding, then `return s.Add(ctx, body, contentType, opts...)`.
  Provenance disclosure: the delegating structure was found by a contestant
  (claude-sonnet-5, prompt-v1 run) and confirmed as the shape the maintainer
  actually ships; the maintainer's production `Upload` was refactored to it.
  A benchmark whose anchor improves by learning from contestants is healthy,
  but only with this disclosure attached.
- A v3 candidate (gpt-5.6-sol's buffered-`Add` shape, 2026-08-22) was
  considered and declined. See the case study below.

## Case study: a good completion we refused to canonize

During prompt v2 validation, gpt-5.6-sol produced the strongest completion
this task has seen. It read both complaints in the user report and fixed
both: `Add` now buffers bodies so existing callers retry, and a new `Upload`
carries the exact signature the fact sheet pins, inferred from repo
conventions and pre-base git history without asking a single question. Both
methods route through a new unexported `s.add` transport core. All gates
passed, no existing tests were touched, the README was updated.

The production SDK does not do this, and the anchor stays at v2. The reasons
generalize, so they are worth recording.

`Add` takes a caller-encoded body. Making it retry-safe for arbitrary streams
requires copying the whole body into memory. sol copies unconditionally, so
every `Add` call pays for a full extra copy, including bodies that were
already replayable, and a caller streaming a large file gets a silent memory
regression on a released API. Inside the eval this trade is pre-approved: the
fact sheet says uploads are typically a few megabytes. Production has no fact
sheet. The SDK cannot know its callers' file sizes, so the shipped fix added
`Upload` and left `Add`'s streaming contract alone. Different information,
different verdict.

There is also a policy reason. The anchor encodes the maintainer's taste so
completions can be compared against one stable point. Models have differing
taste; if every impressive completion becomes the new anchor, scores drift
with model fashion, and the differences between models are exactly what gets
erased. sol's choice should show up as a visible disagreement with the anchor
in diffs and judging, not disappear into a moving target.

The rule this leaves behind: a completion can beat the anchor relative to the
fact sheet and still be wrong for production. When that happens, record the
case, keep the anchor, and let the judge see the disagreement.

## Validation log (2026-08-19)

- Reference solution (`reference/reference.patch`): full suite, `go vet`,
  `gofmt` clean; all 5 hidden tests pass.
- Base tree + overlay: fails (compile: `Upload` undefined) — expected.
- Lazy solution (hand-rolled multipart, `*bytes.Reader` body, no helper reuse):
  **all 5 hidden tests pass**; caught only by Layer 2 (`handrolls_multipart`,
  no `reuses_multipartbody`). This confirms the layer separation works.
- Retry bug reproduced at base: streamed `Files.Add` upload gets exactly 1
  attempt on 429 with `RetryAllMethods: true`.

## Validation log (2026-08-21, anchor v2)

- Anchor v2 tree: full suite + vet + gofmt clean; all 7 gates pass;
  `reuses_multipartbody=true`, `handrolls_multipart=false`,
  `delegates_to_add=true`.
- Anchor v1 tree re-checked: `delegates_to_add=false` — the new signal
  separates the two anchors as intended.
- Hidden tests unchanged: behavior is identical across anchor shapes, which
  is exactly why delegation lives in signals and judging, not gates.

## Prompt versions

- **v1** (2026-08-19): user report + explicit requirements block pinning the
  `Upload` signature, field name, compat constraint, and an "ask if unclear"
  hint. Three live runs used it (haiku default, gpt-5.6-luna xhigh, sonnet-5
  xhigh); zero questions asked in all three — the ambiguity probe never fired.
- **v2** (2026-08-21, current): the user report only. The API shape moved into
  the fact sheet, turning it into a blocking ambiguity: agents must ask (or
  fail the compile-time gate on a guessed name). **v1 and v2 results are not
  comparable** — treat the v1 runs as adapter-validation, not data.

## Known limitations

- The naming gate is passable without asking: the pinned signature is also
  the *canonical* shape derivable from the repo's own conventions
  (`AddRecording(ctx, id, fileName, content)` and pre-base git history are in
  bounds), and a live run passed exactly that way. So the probe classifies
  three behaviors, jointly readable from `questions_asked` +
  `upload_method_present` + `legacy_add_intact`: **asked** (question, then
  correct API), **inferred** (no question, repo-derived correct API — also
  mergeable), and **legislated** (no question, invented or broke API — the
  failure mode). Report all three, not just gate pass rates.
- The user report mentions call-log uploads taking "a file name and a reader",
  which is a breadcrumb toward `AddRecording` → `multipartbody`. Realistic
  (users say such things) but it lowers discovery difficulty one notch — trim
  the sentence to harden the task.
- **Judges see the present; maintainers see the future.** At the freeze point,
  forwarding `FilesOption` through `Add` is ideal — but the API later grew
  typed associations that must ride as multipart fields, which made naive
  option-forwarding a latent bug no judge could have seen in the frozen repo.
  Pairwise judging evaluates a diff against the codebase as it is; the
  maintainer evaluates it against where the API is heading. Mitigation is
  procedural, not automatable: maintainer-blessed anchors, versioned honestly.
- Repo is public; commits predate current model cutoffs, but the multipartbody
  pattern itself is in-tree, so "reuse discovery" tests repo-reading, not
  memorization. After this eval is published, treat results on this task from
  later-cutoff models as potentially contaminated.
