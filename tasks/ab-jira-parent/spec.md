# ab-jira-parent – evaluator spec (public, redacted)

**Evaluator inputs for this task are withheld.** The task runs against
`agent-bridge`, a private repository, so the pinned base commit, hidden test
overlay, reference patch, and `checks.sh` are not published, because they
contain that repo's source. What is published here is the agent-facing prompt, the
user-simulator fact sheet, and the scoring design, so the methodology is
reviewable even though the task is not runnable outside the private mirror.
Scores, cost, usage, and per-run analysis are published in
[results/ab-jira-parent.md](../../results/ab-jira-parent.md); the diffs,
transcripts, and the repo's own identifiers are not.

Task type: feature addition with a blocking ambiguity and a reuse trap.
Primary dimensions: **ask-vs-infer on input shape** (the string-or-number
parent encoding) and **helper discovery** (does the candidate find the
repo's existing string-or-number id parser, or hand-roll a second one).

## Repo state

- Repo: `agent-bridge` (private), mounted as a local mirror
- Base commit: pinned to the parent of the real feature commit
- Reference: the squash of the maintainer's three shipped commits – the
  feature, its OpenAPI documentation, and a follow-up that allows a null
  parent. Ground truth for null parent is the follow-up's semantics:
  explicit null means no parent (maintainer decision, 2026-08-24).

## The traps

1. **Reuse**: the repo already has one canonical helper for accepting a
   string-or-number JSON id, used elsewhere in the same package tree. The
   lazy implementation hand-rolls a `json.RawMessage` type switch in the Jira
   handler instead. A mechanical signal detects which one the diff did.
2. **Blocking ambiguity**: the prompt never names the parameter or says it can
   arrive as a bare number or null. The fact sheet pins the JSON contract but
   gives no Go type, field, helper, or file. A candidate that ships
   string-only parent handling fails a hidden test; one that rejects an
   explicit null fails another. History note: the maintainer's own first
   commit got null wrong and fixed it two commits later. The ambiguity is real,
   not manufactured.
3. **ID-vs-Key mapping**: Jira's create payload distinguishes parent `ID` from
   parent `Key`; numeric strings must go as ID, others as Key. Hidden service
   tests assert the wire payload on both sides.

## User simulator fact sheet

The prompt intentionally leaves these open. The simulator answers only what is
asked, in one or two sentences, and answers anything not covered here with
"use your judgment".

| If asked about | Answer |
| --- | --- |
| Parameter name and accepted values | An optional `parent` on `jira.createIssue`. It accepts an issue key string like `"AI-16"` or a positive integer id like `10001`; an explicit null means no parent. |
| Key vs numeric id on the wire | Numeric values go as the parent id, non-numeric strings as the parent key. |
| Invalid values (0, negative, bool, object) | Reject with the standard invalid-params error. |
| Same treatment for `jira.updateIssue`? | Out of scope for this change. |
| Validate issue-type/parent combinations? | No. Let Jira reject invalid combinations; pass the parent through. |

## Hidden tests

Handler level (JSON in, params out): trimmed key, numeric coerced to string,
explicit null allowed, omitted allowed, invalid values rejected. Service level
(params in, wire payload out): key mapping, id mapping, parent omitted when
unset. All reuse package stubs that exist at the base commit. The overlay
fails to compile at base, because the parent field does not exist yet, which is
the intended blocking gate.

## Scoring layers

Same four layers as [pd-files-upload](../pd-files-upload/spec.md): deterministic
gates, mechanical taste signals, trajectory metrics from the transcript, and
pairwise LLM judging against the reference anchor. The task-specific signal
records whether the diff reuses the shared parser.

## Validation log (2026-08-24)

- Reference tree: all 6 gates pass, exit 0, parser reuse detected.
- Base tree: only the hidden-test gate fails (compile: parent field missing),
  exit 1.
- Full suite runs offline (end-to-end tests are build-tagged out).
