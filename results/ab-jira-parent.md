# ab-jira-parent results

20 configurations, 3 runs each. Claude models ran through claudecode, GPT models
through codex, the rest through opencode.

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

This task runs on a private repository, so what follows is scores, cost, usage,
and analysis. The diffs, transcripts, and the repo's own identifiers stay
unpublished. Where a run's behavior turns on the repo's existing string-or-number
id parser, it is described as the shared parser rather than named.

| Rank | Model and effort | Score | Runs averaged | Mean cost | Mean total tokens | Mean input | Mean output | Mean cache hit |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | Claude Fable 5 · xhigh | 4.57 | 3 | $4.187 | 1,817,749 | 1,799,403 | 18,345 | 95.7% |
| 2 | Claude Opus 4.6 · xhigh | 4.43 | 3 | $2.653 | 1,467,457 | 1,455,679 | 11,778 | 93.4% |
| 3 | Claude Fable 5.1 · xhigh | 4.20 | 3 | $6.649 | 1,746,485 | 1,706,514 | 39,972 | 85.8% |
| 4 | Qwen3.8 Max · xhigh | 4.17 | 3 | $1.115 | 2,959,955 | 2,950,914 | 9,041 | 94.9% |
| 5 | Claude Opus 4.8 · xhigh | 3.67 | 3 | $3.661 | 2,520,623 | 2,502,386 | 18,237 | 94.1% |
| 6 | DeepSeek V4 Flash · xhigh | 3.67 | 3 | $0.037 | 968,689 | 963,362 | 5,328 | 92.4% |
| 7 | GLM-5.3 · default | 3.64 | 3 | $0.672 | 1,994,456 | 1,987,541 | 6,915 | 96.0% |
| 8 | GLM-5.3 Flash · default | 3.58 | 3 | $0.064 normal / $0.032 promo | 1,865,321 | 1,859,312 | 6,009 | 97.5% |
| 9 | GPT-5.6 Sol · max | 3.43 | 3 | $2.994 | 4,230,835 | 4,201,013 | 29,822 | 96.2% |
| 10 | GPT-5.6 Sol · xhigh | 3.37 | 3 | $1.971 | 2,415,505 | 2,395,068 | 20,437 | 94.3% |
| 11 | GPT-5.6 Luna · max | 3.30 | 3 | $0.207 | 5,807,832 | 5,775,607 | 32,225 | 95.8% |
| 12 | Claude Opus 5 · xhigh | 3.27 | 3 | $6.452 | 7,304,805 | 7,262,352 | 42,453 | 97.0% |
| 13 | GPT-5.5 · xhigh | 3.27 | 3 | $3.780 | 4,065,479 | 4,041,264 | 24,215 | 94.3% |
| 14 | DeepSeek V4 Pro · xhigh | 3.22 | 3 | $0.219 | 1,366,408 | 1,361,096 | 5,312 | 92.5% |
| 15 | GPT-5.6 Terra · xhigh | 3.20 | 3 | $1.221 | 3,025,831 | 3,005,740 | 20,091 | 94.5% |
| 16 | GPT-5.6 Luna · xhigh | 3.17 | 3 | $0.181 | 5,043,474 | 5,020,680 | 22,795 | 95.3% |
| 17 | Kimi K3 · xhigh | 3.12 | 3 | $0.950 | 2,047,961 | 2,039,805 | 8,156 | 97.3% |
| 18 | Claude Sonnet 5 · xhigh | 3.10 | 3 | $1.678 | 2,612,398 | 2,598,882 | 13,517 | 97.2% |
| 19 | GPT-5.6 Terra · max | 3.00 | 3 | $2.677 | 7,731,155 | 7,688,545 | 42,610 | 96.4% |
| 20 | Qwen3.8-27B custom · xhigh | 2.79 | 3 | n/a | 4,110,577 | 4,098,809 | 11,769 | 97.8% |

Cost for Qwen3.8-27B doesn't exist: it ran against a private Fireworks deployment. Public pricing seems to hover around $0.40 per MTok in, $2.50-3.00 per MTok out, with cache read discounts varying wildly – anywhere from 20x to ~3x.

Fable 5.1 uses the Fable 5 rate card except for cache reads, which cost
$0.25/MTok instead of $1.00/MTok. The recorded totals already use the lower
rate, so its $6.649 mean does not change. Repricing the three model calls gives
$2.842159, $5.079913, and $12.022295. Each aggregate run total also includes a
$0.001194 auxiliary Haiku call, producing the leaderboard costs of
$2.843353, $5.081107, and $12.023489.

GLM-5.3 Flash cost is calculated from the supplied rates rather than a provider
total. Uncached input is total input less cache reads. The normal run cost is
`uncached input × $0.15/MTok + cache reads × $0.03/MTok + output × $0.50/MTok`.
The temporary price applies the stated 50% discount to all three rates. Its
three normal-price runs cost $0.083948, $0.058558, and $0.050758; discounted,
they cost $0.041974, $0.029279, and $0.025379.

Fable 5.1 also produced three earlier run-number directories that ended in a
harness error after about two seconds. They contain zero tokens, no edits, and
no usable model output, so they are failed launch attempts rather than scored
runs and are excluded from the averages.

Three runs out of 60 passed the hidden tests, all after the simulator corrected
the model. The task hides one decision, what the parent parameter is called and
what JSON it accepts, and almost nothing in the field went looking for it. The
common failure is confident invention: a plausible parameter name, tests written
against that invention, and documentation that teaches it to callers.

## 1. Claude Fable 5 · xhigh

- Run 1: 42 calls. The strongest single result here. It implemented the wrong
  parent_key contract first, then rewrote it completely on simulator feedback.
  The final patch took the required inputs, rejected invalid values, and chose
  Jira's ID or Key member correctly. All six gates green. Go taste was the weak
  part: it duplicated the shared parser, unmarshaled the request twice, used a
  surprising `json:"-"` field, and skipped the reference's system-prompt and
  example updates. 11 of those 42 calls went on the abandoned first attempt and
  repeated validation.
- Run 2: 35 calls. Never recovered from the parent_key guess. The service told
  numeric strings from keys correctly, but the handler exposed the wrong field
  and could not take a bare integer. Its tests called the service directly,
  which tidily avoids the broken boundary. Hidden tests failed, the rest passed.
  Two test pipelines here could also mask a failing go test exit status.
- Run 3: 24 calls. Narrower still. It exposed parent_key, always used the key
  member, and tested only the contract it had invented. Five gates green, hidden
  tests failed. The efficiency came from committing early to an unsupported API
  and never looking for the shared parser.

Fable produced the best result and the one genuinely shippable run. $4.187 is
hard to justify when two of three runs still miss the central contract, and Opus
4.6 got nearly the same score for about 37% less.

## 2. Claude Opus 4.6 · xhigh

- Run 1: 59 calls, 224 added lines. Correct and all green: null, omission, keys,
  and bare positive integers accepted, ID or Key chosen correctly. Like Fable's
  passing run it built parent_key first and needed the correction. It also
  hand-wrote its own parseOptionalParent rather than reusing the shared parser,
  and skipped every documentation change the reference makes.
- Run 2: used the correct parent name and routed numeric strings to Jira IDs.
  The handler field stayed an ordinary string, so a bare JSON integer will not
  unmarshal. Hidden tests failed, the rest passed. The simulator raised numeric
  IDs and invalid inputs explicitly, and the model read a numeric string as
  equivalent to the required JSON number.
- Run 3: 50 calls. Back to parent_key. Competent ID-or-Key routing in the
  service, and the public request contract, null semantics, numeric decoding,
  validation, parser reuse, and documentation all wrong or absent. It declared
  completion after the simulator asked it to check types and validation.

The best price-to-quality balance among models that produced a passing run. One
success in three is still poor reliability, but $2.653 is far easier to defend
than Fable or either newer Opus.

## 3. Claude Fable 5.1 · xhigh

| Run | Idiomatic Go | Correctness | Tests, docs, scope | Execution | Efficiency | Weighted | Calls | Wasted |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 4.5 | 2.5 | 4.5 | 3.0 | 6.0 | 3.75 | 26 | 5 |
| 2 | 4.0 | 2.5 | 4.5 | 2.0 | 3.0 | 3.13 | 48 | 11 |
| 3 | 5.5 | 8.0 | 7.0 | 3.0 | 3.0 | 5.73 | 61 | 13 |

- Run 1: a polished implementation of the wrong API. It exposed
  `parent_key`, so correctly named `parent` input is silently ignored and a
  bare numeric parent cannot decode. The service correctly maps numeric strings
  to Jira's ID member and other strings to Key. Its handler regex, tests, two
  OpenAPI files, system prompt, README, E2E guide, and optional live E2E path all
  reinforce the invented field. The run's own repository search surfaced the
  shared parser, but it did not open or reuse it and never asked about the
  ambiguous contract. Hidden tests failed; the other five gates passed. Five of
  26 calls went to unnecessary E2E work or duplicate validation.
- Run 2: repeated the same `parent_key` design at greater cost. Even under that
  name, the string field rejects a bare JSON number and the regex accepts quoted
  zero. The service's ID-or-Key choice is correct, omission works, and no
  existing behavior broke. The model printed the complete shared parser and
  still added a Jira-specific regex, error, and helper. It changed 11 files,
  including an optional E2E flow and CI secret, and documented the wrong API in
  every caller-facing location. Three test insertion attempts failed because it
  chose an unwritable temporary directory, and the run ended with unrelated
  agent-memory work. Eleven of 48 calls were wasted. Hidden failed; all public
  gates passed.
- Run 3: the only shippable result. It first completed and committed the same
  `parent_key` implementation. Simulator feedback then supplied the required
  name and input matrix. The corrected handler accepts trimmed keys, bare
  positive integers, numeric strings, null, and omission; rejects the required
  invalid values; and the service maps ID and Key correctly. All six gates pass.
  It still hand-writes a 35-line parser after inspecting the existing helper and
  adds regex-based Jira-key validation that the reference deliberately leaves to
  Jira. The 11-file, 317-line patch includes unnecessary E2E and CI work. Turns
  3 and 4 drift into commit-history advice and two history rewrites, the first
  faulty, plus unrelated agent-memory writes. At least 13 of 61 calls were
  wasted.

Fable 5.1 ranks third because its corrected run is genuinely good, but the model
is poor value here. It averages $6.649, about 59% more than Fable 5, while
scoring 0.37 lower and matching its one-in-three hidden-pass rate. Opus 4.6 is
0.23 higher and costs 60% less. The extra Fable 5.1 spend mostly bought a long
recovery and repository ceremony.

## 4. Qwen3.8 Max · xhigh

- Run 1: correct parent name, trimmed, numeric strings to IDs and other strings
  to keys. Good handler and service tests, extensive docs, an optional
  end-to-end test. The string handler field still rejects bare JSON integers and
  no invalid-value handling appeared at all. Hidden tests failed. Thoughtful and
  oversized at nine files and 216 added lines.
- Run 2: chose parent_key while keeping correct service-side ID-or-Key routing,
  then wrapped another large end-to-end flow and extensive docs around the wrong
  API. The only run reviewed here that failed both the hidden tests and the
  ordinary full suite. Its trajectory includes an accidental duplicated edit,
  repair work, and commit setup after the implementation was already wrong.
- Run 3: 73 calls, three turns. Correct parent name again, with service routing
  pulled into a small issueParent helper. Tests covered omission, key mapping,
  and numeric-string mapping, with broad docs and integration coverage. It still
  decoded into a string, so the decisive bare-number and invalid-input cases
  failed. The extra turn went on committing rather than on the request shape.

One of the better values. Two runs sit close enough that a careful review could
repair them cheaply. Zero hidden-test passes rules it out as an unsupervised
implementer, but at $1.115 it drafts as well as the Claude and Codex models.

## 5. Claude Opus 4.8 · xhigh

- Run 1: 50 calls. Correct parent field, numeric strings sent through Jira's ID
  member, useful OpenAPI notes and a request example. The handler decoded
  straight into a string, so bare integer JSON failed. It missed the shared
  parser and the system-prompt update. Hidden tests failed.
- Run 2: parent_key plus a tiny jiraParentRef helper. Reasonable service logic,
  no Parent field, hidden suite red. Its one question was about post-
  implementation workflow, and its second turn committed the bad solution rather
  than revisiting the contract.
- Run 3: ParentKey again, with a more elaborate and over-commented service helper
  and examples for the wrong field. Questions were about committing and pushing.
  It set up a local Git identity and probed for remotes across three turns
  without asking anything about the ambiguous input.

38% more expensive than Opus 4.6, substantially lower score, zero hidden-test
passes. There is no version of this task where I would pick it over 4.6.

## 6. DeepSeek V4 Flash · xhigh

- Run 1: a compact three-file patch on parent_key and the key member. Tests for
  key mapping and omission, no handler boundary tests, no documentation. Hidden
  tests failed. Simple Go, wrong API.
- Run 2: invented parent_issue_key, key-only again. Handler and service tests
  plus one OpenAPI update, none of them touching the required bare number, null,
  or invalid inputs. 41 calls spent exploring SDK types and adjacent code before
  guessing another unsupported name.
- Run 3: much better. It used parent, chose ID or Key correctly, and tested both
  service mappings in 34 calls. The handler was still string-only, so bare
  integers and the invalid-value matrix failed, and it missed the shared parser
  and most docs. Its closest attempt.

3.7 cents a run. It tied Opus 4.8's score at roughly one hundredth of the cost,
which makes it excellent for a first patch and still leaves all three outputs
needing a contract-focused review before anything ships.

## 7. GLM-5.3 · default

| Run | Idiomatic Go | Correctness | Tests, docs, scope | Execution | Efficiency | Weighted | Calls | Wasted |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 4.0 | 5.0 | 6.0 | 3.0 | 3.5 | 4.35 | 56 | 18 |
| 2 | 5.0 | 2.0 | 3.0 | 2.0 | 3.0 | 3.00 | 58 | 10 |
| 3 | 4.5 | 2.5 | 4.0 | 3.5 | 4.0 | 3.58 | 49 | 6 |

- Run 1: the best of the three, but only after spending its first turn building,
  testing, documenting, and committing `parent_issue_key`. Simulator feedback
  moved it to the correct `parent` JSON field. Its final handler accepts keys,
  bare positive integers, numeric strings, null, and omission, rejects the
  required invalid matrix, and sends either Jira's ID or Key member. The design
  is still incompatible with the expected service contract: it exposes raw JSON
  plus separate parent ID and key fields instead of one normalized string. The
  41-line parser duplicates the shared helper and permits states the reference
  makes impossible. Hidden tests failed; the other five gates passed. The docs
  covered two OpenAPI descriptions but omitted request examples and the system
  prompt. At least 18 of 56 calls went into the discarded first implementation
  or avoidable duplicate work.
- Run 2: a polished implementation of the wrong API. It kept
  `parent_issue_key`, accepted only a string key, and always populated Jira's
  Key member. A valid `parent` is silently ignored; a bare integer cannot reach
  the service; invalid values under the correct field are also ignored rather
  than rejected. It then expanded into an end-to-end lifecycle test, environment
  documentation, and a CI workflow secret, all mechanically outside scope. It
  never searched for the shared parser. Ten of 58 calls were conservatively
  wasted, including the out-of-scope work and four unsuccessful YAML-validation
  attempts. Hidden tests failed; every public gate passed.
- Run 3: a smaller, more coherent wrong-contract patch. It chose
  `parent_issue_key`, but its service at least distinguishes numeric strings as
  Jira IDs and other strings as keys. Handler tests cover trimming and omission;
  service tests cover key, numeric string, and omission. They do not cross the
  decisive JSON boundary, and correct `parent` input is ignored. It consciously
  chose the longer invented name after considering `parent`, never asked a
  contract question, and never found the shared parser. Six of 49 calls were
  wasted. Hidden failed; build, public tests, formatting, vet, and dependency
  checks passed.

At $0.672 per run, GLM-5.3 lands just below the two 3.67 models and above every
Codex arm. That is respectable drafting quality for the price, but zero passing
runs and three different incompatible request shapes make it unsuitable without
a contract-aware reviewer. Flash is about 10 times cheaper at list price and
loses only 0.06 score.

## 8. GLM-5.3 Flash · default

| Run | Idiomatic Go | Correctness | Tests, docs, scope | Execution | Efficiency | Weighted | Calls | Wasted |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 3.5 | 5.0 | 6.0 | 2.5 | 2.5 | 4.03 | 67 | 20 |
| 2 | 5.0 | 1.5 | 3.5 | 3.0 | 4.0 | 3.23 | 51 | 7 |
| 3 | 4.5 | 2.5 | 4.5 | 3.0 | 3.5 | 3.50 | 51 | 6 |

- Run 1: initially implemented, validated, documented, and committed
  `parent_issue_key`; simulator feedback then supplied the real contract. The
  rewrite gets nearly all wire behavior right, including positive numbers,
  numeric strings, keys, null, omission, and invalid-value rejection. It still
  changes the service DTO from a string to an exported `*JiraParentRef` with a
  41-line custom unmarshaler. Hidden handler comparisons and service literals
  therefore cannot compile, and callers can construct empty or both-set refs.
  This is much more machinery than extending the shared parser. Tests are broad,
  but 173 test lines encode the incompatible type; docs omit examples and the
  system prompt. Twenty of 67 calls belong to the abandoned implementation or
  avoidable failed probes. Hidden failed; the other five gates passed.
- Run 2: clean TDD around `parent_key`, trimming, key mapping, and omission.
  Unfortunately, every correctly named `parent` value is ignored and the
  service cannot send a Jira parent ID. The tests omit bare integers, null, zero,
  negatives, booleans, objects, and ID routing; both OpenAPI files document the
  invented key-only field. It saw the `parent_key` versus `parent` choice, chose
  by naming analogy, asked nothing, and never searched for the existing parser.
  Two validation pipelines also masked the test command's exit status, although
  the independent harness confirms the public suite passed. Seven of 51 calls
  were wasted. Hidden failed; all other gates passed.
- Run 3: also used `parent_key`, but correctly routes a numeric string to Jira's
  ID member and another string to Key. Its 131-line service-test addition repeats
  setup for three simple cases, while the handler still cannot accept the real
  field or a bare integer and admits quoted zero or negative IDs. It updated both
  OpenAPI documents and README with the wrong API, leaving examples and the
  system prompt untouched. After a commit failed, it persisted the user's name
  and email in repository-local Git configuration despite recognizing that it
  should not. Six of 51 calls were wasted. Hidden failed; every public gate
  passed.

Flash is the stronger economic choice within the GLM pair. Its mean is $0.064421
at the normal rates and $0.032211 during the half-price promotion, versus
$0.671649 for GLM-5.3, for only 0.06 less score. DeepSeek V4 Flash is still the
better measured option at normal price: it scores 0.09 higher for $0.037. The
promotion makes GLM Flash slightly cheaper, but neither model produced a
shippable run.

## 9. GPT-5.6 Sol · max

- Run 1: 51 calls. parent_key, key-only mapping, OpenAPI examples, service
  tests. No numeric IDs, no null, no invalid input, no system prompt. The
  trajectory took in web research, SDK exploration, Git work, and troubleshooting
  an unavailable linter, and never asked about the contract.
- Run 2: 15 added lines and three deletions, the smallest patch in the task.
  Same parent_key and key-only choice. It picked the parameter name on naming
  aesthetics after research instead of resolving the ambiguity. Hidden tests
  failed.
- Run 3: better omission assertions, examples, and a README note, all wrapped
  around the same wrong string-key contract. The extra documentation makes the
  mistake easier for callers to find. Time again went on linter and commit setup
  rather than the JSON boundary.

Too expensive for this. Sol xhigh scored within 0.06 for about a third less, and
much cheaper models produced equally repairable patches.

## 10. GPT-5.6 Sol · xhigh

- Run 1: parent_issue_key, always sending a Jira key. It followed TDD and stayed
  fairly focused, and missed every heterogeneous-input requirement. Its second
  turn chased an unavailable linter after committing the wrong API.
- Run 2: switched to parent_key and added table-driven service cases for
  sub-tasks and stories. Both cases only assert key mapping, so the table tests
  Jira labels rather than the request boundary. Two web searches and extra PR
  workflow did not lead it to the shared parser.
- Run 3: back to parent_issue_key, with a compact omission check and terse
  OpenAPI notes. The leanest Sol trajectory at 26 calls. It deleted five existing
  test lines and called the change a small backward-compatible extension before
  establishing what the public contract was.

The better of the two Sol settings: 0.06 score for about 34% less money. The
absolute result is weak either way, because all three runs failed the same
judgment test.

## 11. GPT-5.6 Luna · max

- Run 1: parent_key, key-only mapping, both OpenAPI files, and the system
  prompt. It ran 11 test commands and read the neighbouring handler that already
  uses the shared parser, and still missed it. Time also went on a flaky startup
  test, cache permissions, and commit identity.
- Run 2: same API, with table-driven service tests for sub-task and epic keys.
  Thorough-looking tests that stay entirely inside the string-key case. No
  system-prompt update, hidden tests failed.
- Run 3: parent_key and key-only again, with cleaner separate handler and service
  tests plus system-prompt docs. The fastest max run. Its search for handler
  conventions never reached the parser.

Cheap enough to be useful for routine scaffolding, and 0.13 above Luna xhigh for
2.6 cents more, which makes max the better Luna setting. Neither showed usable
autonomous contract judgment.

## 12. Claude Opus 5 · xhigh

- Run 1: 63 calls, 15 test runs, the wrong parent_key design spread across 12
  files. Alias rejection, reference helpers, end-to-end tests, CI workflow
  changes, README text, OpenAPI docs, system-prompt guidance. The missing Parent
  field still failed the hidden suite, and the documentation actively teaches the
  wrong API.
- Run 2: 119 calls, 19 test runs, 11 files. Regex validation, ID-or-Key mapping,
  workflow changes, integration coverage, temporary HTTP tests, branch work, and
  an unrelated investigation into global error formatting. The regex is stricter
  than asked and rejects values Jira should be deciding on. This single run cost
  $10.08 and still failed the contract.
- Run 3: 55 calls, 46 shell commands, 238 additions. ParentKey again, regex
  validation again, heavy tests, and no acceptance of the required raw integer or
  null. It asked nothing.

The clearest bad value in either ranking. Most expensive model in the task, no
passing run, and every extra dollar went into expanding scope around an
assumption it never checked.

## 13. GPT-5.5 · xhigh

- Run 1: invented separate parent_key and parent_id parameters, rejected using
  both together, and populated the matching Jira member correctly. That shows
  real understanding of Jira's wire format and misses the single union input the
  task wants. It updated both OpenAPI files and the system prompt, then failed
  hidden tests.
- Run 2: 74 calls, 65 shell commands, 147 additions. parent_issue_key plus a
  validated string parent_issue_id. It opened the exact file region holding the
  shared parser and kept its own duplicate split-field parser anyway.
- Run 3: 81 calls, 254 additions. parent_key, parent_id, epic_key, and arbitrary
  custom fields. Handler and service helpers duplicated normalization while the
  actual parent input stayed unsupported.

More expensive than Opus 4.6 and much lower scoring. It reasons well about Jira
and reaches for new fields whenever it is unsure, which is the wrong instinct on
a task about restraint.

## 14. DeepSeek V4 Pro · xhigh

- Run 1: parent_key, key-only mapping, handler and service tests, omission
  coverage, one OpenAPI update. Clean, compact, wrong at every decisive
  boundary.
- Run 2: parent_key, epic_link_key, and epic_link_field, building an Epic Link
  custom field. Well beyond scope, and built on a distinction the required API
  does not have. 141 added lines and five deletions with no bare numbers, no
  null, no ID mapping.
- Run 3: 44 calls. Back to the simpler parent_key design with trimming, omission
  tests, and less OpenAPI documentation. It never searched for the shared parser.

Flash dominates it here: almost six times cheaper, higher scoring, and Pro's
most ambitious run made the API worse through speculative configuration.

## 15. GPT-5.6 Terra · xhigh

- Run 1: parent_issue_key, key-only mapping, and useful examples in both OpenAPI
  files. No real field name, no numeric support, no null handling, no parser
  reuse, no system-prompt guidance.
- Run 2: 31 calls. parent_issue_key again with a focused service test. It
  inspected the neighbouring handler's id conventions and stopped short of the
  parser. An efficient delivery of the wrong contract.
- Run 3: invented mutually exclusive parent_issue_key and epic_key fields, plus
  validation for that fictional distinction. Jira and the reference both need one
  parent field, so the extra API is complexity with no correctness to show for
  it.

Much better value than Terra max, though Luna max costs about a sixth as much
and scores higher. Run 3 shows the same habit as GPT-5.5: turning uncertainty
into API design.

## 16. GPT-5.6 Luna · xhigh

- Run 1: 30 calls, lean. parent_key, redundant service-side trimming, key-only
  mapping. It edited an existing happy-path test to include a parent, which
  weakened the omitted-parent coverage.
- Run 2: parent_key again, with a useful ordinary-issue omission assertion and
  the field documented in both OpenAPI files. It read the neighbouring handler's
  conventions and again did not find the parser.
- Run 3: parent_issue_key, still populating only the key member. Separate tests
  make the code look orderly, and none of them crosses the real JSON boundary.

The cheapest Codex configuration, and its 2.6-cent saving over Luna max costs
0.13 score. Max is the more sensible Luna choice, though neither came close to
shippable.

## 17. Kimi K3 · xhigh

- Run 1: parent_key, key-only mapping, TDD, both OpenAPI files, a focused
  six-file patch. Having finished the wrong implementation, it spent further
  turns creating a branch, choosing a local Git identity, and probing for push
  and PR support.
- Run 2: the same design with better omission tests and clean handler coverage.
  Its only question was again whether to commit, not what input the action
  should expose.
- Run 3: more documentation, including the system prompt and README, around the
  same wrong parameter, then another branch and local identity after asking about
  workflow.

Roughly 26 times DeepSeek Flash's price for a lower score. The disciplined TDD
counts in its favor. Treating repository ceremony as more urgent than the
product contract does not.

## 18. Claude Sonnet 5 · xhigh

- Run 1: 34 calls. parent_key, trimming, key-only mapping, handler and service
  tests, both OpenAPI notes. A small, direct patch to the wrong result.
- Run 2: almost the same seven-line production change. Numeric strings still
  went to Jira's key member, and it never looked at adjacent parsing
  conventions. Hidden tests failed.
- Run 3: 77 calls, 52 shell commands. Trimming and omission tests, only the
  reduced OpenAPI file updated, and no improvement to the underlying design.

Cheaper than the Opus models and still about 45 times DeepSeek Flash's price for
a lower score. It may draft ordinary changes well enough. This task found no
evidence of autonomous contract reasoning.

## 19. GPT-5.6 Terra · max

- Run 1: parent_key, key-only mapping, separate tests, two OpenAPI notes.
  Missing the correct field, ID mapping, validation, examples, system prompt, and
  parser reuse.
- Run 2: 74 calls, 10 test runs, the same design. It found and printed the shared
  parser on screen and ignored it. Its only question was about Git author
  identity.
- Run 3: parent_key again, with an existing service test edited to require a
  parent and much of the documentation left out. Extra calls went on linter
  installation, race tests, and repeated status checks.

Plainly dominated: about 13 times Luna max's price and more than twice Terra
xhigh's, for the lowest score among the priced models.

## 20. Qwen3.8-27B custom · xhigh

- Run 1: 84 calls, 19 edits. parent_key plus an invented epic_name mode that
  brought a new field-client interface, mutex-protected per-tenant caching, field
  discovery, and custom-field creation. Setting Epic Name does not create the
  parent relationship the task needs. A much larger and riskier API for nothing.
- Run 2: more restrained in production code, parent_key and key-only mapping,
  then a 76-line optional end-to-end test and a new environment-variable
  workflow. Integration scaffolding cannot make up for a broken JSON contract.
- Run 3: 90 calls, 12 test runs, 378 lines. parent_key and epic_key, where the
  epic path creates the issue, fetches the epic summary, and writes that summary
  into a hardcoded customfield_10014, with elaborate partial-failure handling.
  Non-portable and outside scope.
