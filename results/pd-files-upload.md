# pd-files-upload results

16 configurations, 3 runs each, all at the pinned base commit behind the egress
proxy. Claude models ran through claudecode, GPT models through codex, the rest
through opencode.

Score is a weighted composite out of 10:

| Weight | Dimension |
| ---: | --- |
| 30% | Implementation correctness and golden drift |
| 25% | Idiomatic Go |
| 20% | Execution and trajectory |
| 15% | Tests, documentation, and scope |
| 10% | Tool efficiency |

Cost and token use are recorded and discussed. They never move a model up or
down the table.

The pairwise judge in [judgments/](../judgments/) feeds only part of this, so its
ranking is not this one. A diff can win on merge preference and still lose points
for scope creep, a broken contract, or three simulator corrections on the way
there.

| Rank | Model and effort | Score | Valid runs | Cost/run | Total tokens/run | Input/run | Output/run | Cache hit |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | GPT-5.6 Sol · xhigh | 5.50 | 3 | $1.505 | 1.874M | 1.857M | 17.2k | 95.09% |
| 2 | Claude Opus 4.8 · xhigh | 5.50 | 3 | $7.178 | 5.699M | 5.637M | 62.4k | 95.05% |
| 3 | Claude Opus 4.6 · xhigh | 5.33 | 3 | $4.482 | 3.461M | 3.438M | 22.3k | 95.05% |
| 4 | Claude Opus 5 · xhigh | 5.17 | 3 | $5.440 | 5.885M | 5.834M | 51.9k | 97.79% |
| 5 | Claude Sonnet 5 · xhigh | 5.17 | 3 | $2.830 | 4.885M | 4.847M | 37.6k | 97.07% |
| 6 | Claude Fable 5 · xhigh | 5.00 | 3 | $6.628 | 3.163M | 3.125M | 37.8k | 97.29% |
| 7 | DeepSeek V4 Pro 0813 · xhigh | 4.50 | 3 | $0.2332 | 1.541M | 1.525M | 15.3k | 94.57% |
| 8 | Kimi K3 · max | 4.17 | 3 | $1.491 | 2.894M | 2.866M | 28.0k | 97.27% |
| 9 | GPT-5.5 · xhigh | 3.67 | 3 | $1.900 | 1.928M | 1.912M | 16.6k | 94.82% |
| 10 | Qwen3.8 Max · max | 3.50 | 3 | $1.901 | 5.261M | 5.219M | 42.3k | 96.25% |
| 11 | GPT-5.6 Terra · xhigh | 3.17 | 3 | $0.8891 | 1.912M | 1.893M | 19.3k | 93.59% |
| 12 | Qwen3.8 Max · xhigh | 2.83 | 3 | $1.662 | 4.636M | 4.596M | 39.6k | 96.58% |
| 13 | Kimi K3 · xhigh | 2.83 | 3 | $1.093 | 2.126M | 2.107M | 19.3k | 97.00% |
| 14 | GPT-5.6 Luna · xhigh | 2.67 | 3 | $0.1313 | 3.258M | 3.233M | 24.3k | 94.96% |
| 15 | DeepSeek V4 Flash 0731 · xhigh | 2.33 | 3 | $0.08111 | 2.224M | 2.203M | 20.7k | 94.48% |
| 16 | Qwen3.8-27B custom · xhigh | 1.83 | 3 | n/a | 5.324M | 5.280M | 44.4k | 98.08% |

Cost for the custom Qwen3.8-27B arm is unreported: it ran against a private
Fireworks deployment that no longer exists.

Nobody cleared 5.5 out of 10. The whole field either changed a stable public
API, hand-rolled plumbing that already existed, or shipped several hundred
lines where 30 would do. Read the ranking as a ranking of failure modes.

## 1. GPT-5.6 Sol · xhigh

- Run 1: 25 calls, 144 added lines. Inferred the exact Upload signature with no
  correction, reused multipartbody, kept the public Add signature, passed every
  gate. It also made Add read every body fully into memory and routed both
  methods through a new private add.
- Run 2: 30 calls, 173 added lines. Same design, with the best retry tests in
  the field. One-shot readers and uploads both covered. Needed a second turn to
  finish, never a contract correction.
- Run 3: 29 calls, 156 additions. Same again, plus os.File coverage, all checks
  green.

Sol read both halves of the user report, the retry failure and the missing
convenience method, and got there on the shortest trajectory anyone managed.
Then it buffered every Add body unconditionally, which rewrites Add's contract
in silence and forces arbitrary custom bodies into memory. The spec already
records that design as one I considered and rejected for production.

A fifth of Opus 4.8's price at the same score, with less variance between runs.
I would still strip the Add buffering before merging.

## 2. Claude Opus 4.8 · xhigh

- Run 1: 76 calls, 407 additions. Built AddFile, association options, and new
  multipart machinery first. The simulator told it to use Upload and add no
  options, and it deleted most of that work. The final patch had the correct API
  and an intact Add, but duplicated Add's request plumbing and changed Raw.Do to
  replay seekable bodies.
- Run 2: 73 calls, 218 additions. Nearly the same opening mistake, then a
  focused Upload with strong multipart and retry tests after another direct
  correction. Still duplicated the transport path instead of delegating.
- Run 3: 78 calls, 381 additions. Wandered off like run 1, got corrected, tore
  most of it out, landed on the right API with duplicated plumbing.

The final diffs were strong, run 2 especially. All three needed the simulator to
state the contract after the model had already built a larger, wrong API. The
seekable-body work in run 1 was thoughtful and also changed reader ownership and
transport semantics well outside the requested fix.

$7.18 a run buys top-tier code that someone else had to aim. Against Sol, and
against 4.6, that is hard to defend.

## 3. Claude Opus 4.6 · xhigh

- Run 1: 104 calls, 106 additions. Changed both Add and Update, took two
  corrections to put them back, then added the exact Upload. Reused the helper,
  duplicated Add's request and response handling. Four turns and 104 calls for a
  106-line patch.
- Run 2: 97 calls, 142 additions. Changed Add first again. The corrected
  Files.Upload matches the reference structure exactly and delegates to Add,
  then it bolted on Persons.UploadPicture, which nobody asked for.
- Run 3: 62 calls, 104 additions. The simulator stopped it changing Add for the
  third time. Compact result, duplicated transport.

Run 2 contains the closest thing to the production implementation in this whole
set. Its independent judgment is weaker than the final diffs suggest, because
all three runs opened by breaking a stable public API.

Credible when you expect to correct it interactively. Not when the first patch
has to be right, and not at $4.48 next to Sol.

## 4. Claude Opus 5 · xhigh

- Run 1: 71 calls, 1,315 additions across ten files. The exact Upload signature
  and validated association IDs, wrapped in a large request-body replay
  abstraction, raw-client changes, multipart extensions, docs, and heavy tests.
  No delegation to Add.
- Run 2: 63 calls, 669 additions. A working upload plus raw seekable-body
  replay. It used a separate UploadFileOption, added unvalidated associations,
  missed the project association, duplicated transport handling.
- Run 3: 61 calls, 703 additions. Like run 2, with its own option hierarchy and
  a pile of association fields. Its seekable-body code can leave the reader at
  the wrong offset when the SeekEnd probe fails.

Opus 5 understood the retry mechanism every time and always produced a usable
entry point. It also treated a 30-line task as an SDK redesign every time. Some
of the association ideas do resemble the later production API, but the real one
validates them and delegates through Add. These runs did neither cleanly.

$5.44 for this little scope control is not defensible when Sonnet 5 is half the
price and Sol is cheaper and steadier.

## 5. Claude Sonnet 5 · xhigh

- Run 1: 60 calls, 133 additions. Replaced the Add signature, got corrected,
  then landed the exact Upload with Add intact, the helper reused, and every
  gate green. Duplicated Add's transport block, and burned a surprising amount
  of the run fighting a temporary module cache.
- Run 2: 66 calls, 242 additions. Added AddFile instead of Upload, plus seven
  association options and multipart helper changes. Add survived, the required
  public API never appeared, hidden tests failed.
- Run 3: 49 calls, 164 additions. Reached the exact API unaided, passed all
  gates, touched two files. Duplicated transport instead of delegating.

Two good runs and one outright contract miss. That miss is why an almost-perfect
individual result does not lift Sonnet above steadier models.

$2.83 is a fair middle. Materially cheaper than Opus, though Sol is more
consistent at roughly half again less.

## 6. Claude Fable 5 · xhigh

- Run 1: 44 calls, 498 additions. Added Upload with its own UploadFileOption
  family and changed Raw.Do to replay seekers. That replay is broken for a real
  os.File, because the first request body can close the file before GetBody
  seeks it. The test wrapped a strings.Reader, so it never saw the lifecycle
  failure.
- Run 2: 52 calls, 487 additions. Fixed the close by shielding caller-owned
  seekers from transport closure. Working upload, still a non-required option
  API, associations without real validation, no delegation.
- Run 3: 55 calls, 602 additions. Used the requested FilesOption type and passed
  all gates, then added generic multipart form fields to FilesOption, where they
  are meaningless for most file operations. Its seek probing handles restore
  errors badly.

Fable found the technical cause every time and produced gate-passing upload
behavior every time. Discipline is the problem. All three runs widened low-level
transport behavior and spent several hundred lines on a 30-line change.

Nearly Opus 4.8 money, more than Opus 5, without the judgment of either. Worst
value in the top half.

## 7. DeepSeek V4 Pro 0813 · xhigh

- Run 1: 48 calls, 96 additions. Guessed the wrong name, took the signature from
  the simulator, then produced the reference shape: exact API, helper reuse, Add
  untouched, direct delegation.
- Run 2: 46 calls, 126 additions. Replaced the Add signature and ended on a
  substantive simulator error. Hidden API and legacy compatibility both failed.
- Run 3: 49 calls, 205 additions. Kept Add but introduced AddFile, several
  association options, and multipart helper extensions. Upload behavior solved,
  required API missing.

The best cheap model here and the most volatile. One run was essentially ideal
after a naming correction. The other two never exposed the required method.

23 cents a run is remarkable for exploratory work or any loop that can validate
and retry on its own. I would not ship a single unreviewed run, but four
attempts still cost less than one Claude run.

## 8. Kimi K3 · max

- Run 1: 58 calls, 447 additions. Found the exact Upload API unaided and passed
  every gate, then added broad raw-body replay, tests, and docs that dwarf the
  actual change.
- Run 2: 64 calls, 262 additions. Changed Add to the new ergonomic signature and
  added association fields. Stable API broken, hidden tests failed.
- Run 3: 98 calls, 449 additions. Changed Add, got corrected to a new Upload,
  passed all gates, kept several speculative association options and duplicated
  request handling.

Two runs ended with a working Upload, so the capability is real. API stability
is not something it holds onto, and it kept expanding the task into transport or
association work.

Priced within a cent of Sol. Given the quality gap and much heavier tool use,
there is no reason to pick it here.

## 9. GPT-5.5 · xhigh

- Run 1: 52 calls, 192 additions. Kept Add, reused multipartbody, delegated
  correctly, and named the method AddFile. Also modified Raw.Do to replay
  seekable bodies.
- Run 2: 57 calls, 187 additions. Almost identical. Sound AddFile
  implementation, retry tests, no Upload.
- Run 3: 41 calls, 274 additions. Attempted a pseudo-overload by changing Add to
  take two any parameters and dispatch on runtime types. Unidiomatic Go, weaker
  compile-time safety, broken signature, and still no Upload.

The first two runs were structurally close and failed the basic API
requirement. The third reached for overloading, which is the one thing Go
deliberately does not have.

More expensive than Sol and substantially worse. No price argument for it.

## 10. Qwen3.8 Max · max

- Run 1: 80 calls, 497 additions. Exact Upload, Add preserved, every gate green.
  Also changed raw request handling to replay seekers and added a large test and
  doc footprint. Ended on a simulator error after the substantive work, so the
  run still counts.
- Run 2: 112 calls, 590 additions and 37 deletions. Changed Add and Update,
  added a wide payload option system, touched ten files. Breaking established
  APIs while rewriting unrelated file-update behavior is the worst drift in the
  set.
- Run 3: 68 calls, 372 additions. Working exact Upload with Add intact, carrying
  another broad raw replay implementation and duplicated transport logic.

Two decent behavioral results and one destructive redesign. Even the good runs
were several times the size of the reference.

Sol's price, three times Sol's tokens, two points below Sol's score.

## 11. GPT-5.6 Terra · xhigh

- Run 1: 17 calls, 168 additions. Tried to hold source compatibility by making
  Add take any arguments and dispatch between file-name and raw-body forms, plus
  a new AddRaw. Call sites might still compile, but the exported signature
  changed and Upload never existed.
- Run 2: 29 calls, 123 additions. Same runtime-overload design, slightly
  smaller. Hidden and legacy checks failed again.
- Run 3: 23 calls, 75 additions. Cleanly replaced Add with the ergonomic
  signature and added AddRaw. Simpler, openly breaking, still no Upload.

Fast and focused, and confidently solving the wrong API problem three times in a
row. The any-overloads are the least Go-like answer available: a stable typed
method traded for runtime errors.

Under a dollar is attractive until you notice every run misses the required
public contract. DeepSeek Pro is a quarter the price and produced one
near-perfect result.

## 12. Qwen3.8 Max · xhigh

- Run 1: 82 calls, 789 additions. Added Upload and kept Add, but changed the
  signature to take a new FilesUploadOption type. Hidden tests passed by luck,
  because they pass no options. The patch also brought seven association
  options, validation logic, raw replay changes, and multipart extensions.
- Run 2: 67 calls, 247 additions. Replaced Add outright with the ergonomic
  signature. Hidden and compatibility gates both failed.
- Run 3: 89 calls, 849 additions. Added AddFile instead of Upload, built a
  complex seekable multipart implementation, changed retry transport behavior,
  touched v2 products, added extensive tests and docs. Ambitious and almost
  entirely detached from the requested change.

Scope control is the defining failure. Even the run that passed the automated
checks exposed the wrong public option type, which the hidden tests never
probe.

Close to Sol's price, above Kimi max's, with a 4.6M-token appetite. Poor value.

## 13. Kimi K3 · xhigh

- Run 1: 67 calls, 466 additions. Replaced Add, added seven association options,
  expanded the multipart helper. Hidden and compatibility checks failed.
- Run 2: 63 calls, 153 additions. Simpler replacement of Add with good multipart
  replay tests. Still broke the stable API, still no Upload.
- Run 3: 46 calls, 112 additions. Nearly identical to run 2, changing Add again
  and documenting the breaking migration rather than preserving compatibility.

Kimi xhigh diagnosed the body replay issue three times out of three. It also
picked the same forbidden public API change three times out of three, after
substantial repository reading.

A dollar a run is not cheap next to DeepSeek Pro, Luna, or Terra, and the
consistency runs in the wrong direction.

## 14. GPT-5.6 Luna · xhigh

- Run 1: 42 calls, 98 additions. Replaced Add with the ergonomic file-name and
  reader signature, reused the existing helper, wrote retry tests. Hidden and
  compatibility checks failed.
- Run 2: 45 calls, 97 additions. The same solution with a public comment and
  good verification, and the same breaking decision.
- Run 3: 38 calls, 82 additions. The smallest version of that patch. Clean,
  idiomatic in isolation, and wrong for a stable v1 SDK.

Luna found the existing multipart pattern reliably and implemented it
compactly. It never worked out that API evolution, not multipart encoding, was
the decisive problem.

13 cents a run makes bulk experimentation viable. It does not make these patches
mergeable, though the mistakes are at least simple to spot.

## 15. DeepSeek V4 Flash 0731 · xhigh

- Run 1: 61 calls, 309 additions. Kept Add, added AddFile, several association
  options, helper changes, and retry transport work. No Upload.
- Run 2: 56 calls, 208 additions. Replaced Add, added generic multipart fields,
  ended on a simulator error. Both core API gates failed.
- Run 3: 62 calls, 445 additions. Added Upload and passed the automated gates
  using a new AddFileOption instead of FilesOption, along with a large option
  hierarchy and raw replay implementation. The hidden suite never exercises the
  variadic option type.

Flash reached working behavior occasionally. Its public API judgment and scope
control were weak throughout. Run 3 is the cleanest illustration of why passing
tests are not enough: the exported method still was not the specified method.

The cheapest row in the table, which makes it useful for generating candidate
ideas and useless for producing a patch you can trust unread.

## 16. Qwen3.8-27B custom · xhigh

- Run 1: 104 calls, 802 additions across nine files. AddFile, seven
  associations, multipart helper extensions, and a new general WithBodyReplacer
  request option. Add survived, Upload never existed.
- Run 2: 71 calls, 490 additions and 13 deletions. Changed Add, changed shared
  multipart signatures and their existing call sites, failed both API gates,
  ended on a simulator error.
- Run 3: 86 calls, 626 additions. Added AddFile and made Raw.Do read every
  streaming body fully into memory. Global behavior change, unbounded buffering,
  no Upload.

It understood the retry mechanics and never found the API contract. Largest
token usage outside the Claude leaders, some of the broadest diffs, and the most
tool calls.
