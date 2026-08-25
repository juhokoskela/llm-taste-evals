# Pairwise merge-quality report

- Task: `pd-files-upload` v2
- Judge: `codex-subscription` / `gpt-5.6-sol` / `xhigh`
- Completion: 828/828 votes, 276/276 pairs (complete)
- Judge tokens: 16990646 input (4292864 cache reads, 0 cache writes), 1191249 output
- Presented-side results: A 408, B 419, ties 1

Scores use majority outcomes from the blinded votes. Bradley-Terry strengths are fit independent of execution order, with the reference fixed at 1500 Elo and a weak Gaussian prior to keep separated data finite. A model's final score is the mean per-run probability of beating the reference; mandatory-gate failures contribute zero, and empty/harness-error runs are excluded.

This judge is in the same broad model family as some contestants. Blinding removes model identity from each prompt, but it cannot eliminate family/style preference; treat the result as a Sol-judged view rather than a neutral cross-family ground truth.

## Model ranking

| Rank | Harness / model / effort | Runs | Gates | Avg merge probability | Passed-run probability |
| ---: | --- | ---: | ---: | ---: | ---: |
| 1 | opencode / fireworks-ai/accounts/fireworks/models/deepseek-v4-pro-0813 / xhigh | 3 | 1/3 | 8.1% | 24.3% |
| 2 | claudecode / claude-opus-4-6 / xhigh | 3 | 3/3 | 4.4% | 4.4% |
| 3 | claudecode / claude-sonnet-5 / xhigh | 3 | 2/3 | 2.5% | 3.7% |
| 4 | claudecode / claude-opus-4-8 / xhigh | 2 | 2/2 | 1.5% | 1.5% |
| 5 | codex / gpt-5.6-sol / xhigh | 3 | 3/3 | 0.2% | 0.2% |
| 6 | opencode / fireworks-ai/accounts/fireworks/models/kimi-k3 / max | 3 | 2/3 | 0.0% | 0.0% |
| 7 | opencode / fireworks-ai/accounts/fireworks/models/qwen3p8-max / max | 3 | 2/3 | 0.0% | 0.0% |
| 8 | claudecode / claude-fable-5 / xhigh | 3 | 3/3 | 0.0% | 0.0% |
| 9 | claudecode / claude-opus-5 / xhigh | 3 | 3/3 | 0.0% | 0.0% |
| 10 | opencode / fireworks-ai/accounts/fireworks/models/qwen3p8-max / xhigh | 3 | 1/3 | 0.0% | 0.0% |
| 11 | opencode / fireworks-ai/accounts/fireworks/models/deepseek-v4-flash-0731 / xhigh | 3 | 1/3 | 0.0% | 0.0% |
| 12 | codex / gpt-5.5 / xhigh | 3 | 0/3 | 0.0% | 0.0% |
| 13 | codex / gpt-5.6-luna / xhigh | 3 | 0/3 | 0.0% | 0.0% |
| 14 | codex / gpt-5.6-terra / xhigh | 3 | 0/3 | 0.0% | 0.0% |
| 15 | opencode / fireworks-ai/accounts/fireworks/models/kimi-k3 / xhigh | 3 | 0/3 | 0.0% | 0.0% |
| 16 | opencode / fireworks-ai/qwen3.8-27b / xhigh | 3 | 0/3 | 0.0% | 0.0% |

## Gate-passing run ranking

| Rank | Candidate | BT Elo | Win vs reference | Direct vs reference |
| ---: | --- | ---: | ---: | ---: |
| 1 | `reference` | 1500 | 50.0% | — |
| 2 | `fireworks-ai-accounts-fireworks-models-deepseek-v4-pro-0813-xhigh-run01-1787481139705297794` | 1302 | 24.3% | 0% |
| 3 | `claude-opus-4-6-xhigh-run02-1787483215624126963` | 1149 | 11.7% | 0% |
| 4 | `claude-sonnet-5-xhigh-run03-1787503346828545128` | 1017 | 5.8% | 0% |
| 5 | `claude-opus-4-8-xhigh-run02-1787483545634051880` | 898 | 3.0% | 0% |
| 6 | `claude-sonnet-5-xhigh-run01-1787502347821666304` | 786 | 1.6% | 0% |
| 7 | `claude-opus-4-6-xhigh-run03-1787483699295762465` | 681 | 0.9% | 0% |
| 8 | `claude-opus-4-6-xhigh-run01-1787482425658791042` | 580 | 0.5% | 0% |
| 9 | `gpt-5.6-sol-xhigh-run01-1787503580372491514` | 482 | 0.3% | 0% |
| 10 | `gpt-5.6-sol-xhigh-run03-1787502875393137548` | 386 | 0.2% | 0% |
| 11 | `gpt-5.6-sol-xhigh-run01-1787502411320655083` | 292 | 0.1% | 0% |
| 12 | `claude-opus-4-8-xhigh-run01-1787482476279665885` | 198 | 0.1% | 0% |
| 13 | `fireworks-ai-accounts-fireworks-models-kimi-k3-max-run03-1787486425046706129` | 105 | 0.0% | 0% |
| 14 | `fireworks-ai-accounts-fireworks-models-qwen3p8-max-max-run03-1787486351497357720` | 11 | 0.0% | 0% |
| 15 | `fireworks-ai-accounts-fireworks-models-qwen3p8-max-max-run01-1787485516052691791` | -83 | 0.0% | 0% |
| 16 | `fireworks-ai-accounts-fireworks-models-kimi-k3-max-run01-1787485494190787462` | -179 | 0.0% | 0% |
| 17 | `claude-fable-5-xhigh-run03-1787500982266101630` | -277 | 0.0% | 0% |
| 18 | `claude-opus-5-xhigh-run01-1787499680647075458` | -378 | 0.0% | 0% |
| 19 | `claude-fable-5-xhigh-run02-1787500319621635421` | -483 | 0.0% | 0% |
| 20 | `claude-opus-5-xhigh-run02-1787500436344803961` | -595 | 0.0% | 0% |
| 21 | `fireworks-ai-accounts-fireworks-models-qwen3p8-max-xhigh-run01-1787481948864951877` | -714 | 0.0% | 0% |
| 22 | `claude-opus-5-xhigh-run03-1787501147580365971` | -846 | 0.0% | 0% |
| 23 | `claude-fable-5-xhigh-run01-1787499709359416055` | -999 | 0.0% | 0% |
| 24 | `fireworks-ai-accounts-fireworks-models-deepseek-v4-flash-0731-xhigh-run03-1787481776184962922` | -1197 | 0.0% | 0% |

The complete pairwise model matrix is in `head-to-head.csv`; raw structured votes and untouched Codex JSONL transcripts are retained beside this report.
