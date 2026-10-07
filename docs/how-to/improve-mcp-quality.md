# Improve how well assistants drive the MCP surface

Raise the share of everyday jobs a real assistant gets right through Margince's
MCP surface, and widen which tools those jobs prove. The loop is: sweep, read
why each failure failed, fix the layer that caused it, prove the fix for free,
re-run the paid lane, publish.

The primary targets are the two assistants users actually connect:

| Target | `E2E_LLM_CANDIDATE` | `E2E_LLM_VIA` | Verdict folder |
|---|---|---|---|
| Claude (its CLI's default model) | `claude` | `cli` | `claude-sonnet-5-5@claude-cli` |
| GPT through Codex | `gpt` | `cli` | `gpt-5.6-sol@codex-cli` |

Mistral through OpenRouter, and the neutral bridge (`E2E_LLM_VIA=api|openrouter`)
that compares vendors on equal terms, are nice to have: run them once a fix holds
on both targets.

How to run the lane, its exits and its harness stops are in
[test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md); this
page assumes it. The results publish to
[reference/mcp-tool-coverage.md](../reference/mcp-tool-coverage.md).

```
  sweep both targets ──► triage each red case ──► fix the layer that caused it
          ▲                (sections 2–3)            (section 4)
          │                                               │
   publish verdicts ◄── paid re-run: reds + controls ◄── free checks
     (section 7)          (section 6)                  (section 5)
```

## 1. Sweep both targets, and keep the transcripts apart

Run the full sweep for each target, one after the other. Never run them in parallel with
each other or with other heavy work on the same Claude subscription, because the
judge draws on it too. Use the scenario-at-a-time loop in
[test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md#sweeping-through-a-subscription-limit)
so a subscription limit pauses the sweep instead of ending it.

`e2e/llm/records/` holds one transcript per run of each scenario (`run1`,
`run2`, …) from the last sweep of it, whichever candidate made it. Copy every
run's transcript aside after each scenario, per candidate, or the
second sweep erases the evidence for the first:

```bash
mkdir -p .tmp/sweep/gpt && cp e2e/llm/records/"$name".run*.jsonl .tmp/sweep/gpt/
```

Keep each scenario's lane log too: its failure lines name what did not hold.

## 2. Triage: which layer failed

Every red case has one of the causes below, and each needs a different fix.
Decide which before changing anything.

| Cause | What you see | Fixed in |
|---|---|---|
| **Harness** | `HARNESS:` stop; every run "called nothing"; degraded search | the lane; see the harness table in the run guide |
| **Grader** | the final answer is right, but a `must_mention` regex or a judged criterion said no | `e2e/llm/scenarios/*.yaml`, `criteria.yaml` |
| **Product: copy** | the model took a plausible wrong path, and a tool description or the server instructions pointed it there | `backend/internal/modules/agents/toolcopy_*.go`, `shared/ports/mcp/mcp.go` |
| **Product: result shape** | the answer needed a fact no tool result carried, e.g. a uuid where a name was needed | the tool's result type, or `backend/api/crm.yaml` |
| **Model** | the guidance was clear and the facts were there, and it still went wrong | nothing, unless a copy change can make the right path more obvious to every model |

A case that fails on one target and passes on the other is still usually a
product or grader cause: the passing model got past an obstacle the failing one
did not.

## 3. Read the evidence

Work through these in order for each failing run.

1. **The lane log's failure lines.** These say which tool was never called, which
   pattern never matched, or which judged criterion was refused, with the judge's
   one-sentence reason.
2. **The tool calls in order.** Print them with the snippet in section 6 of the run guide.
   `python3 e2e/llm/why.py <scenario>` sets the tools the scenario requires beside
   what each run was offered and what it called.
3. **The first call.** On the failing runs, the first call usually decides the
   route. Compare it with a passing run of the same case, from the other target
   or another model. If the runs that open on tool A all pass and the runs that
   open on tool B all fail, read B's description for what sent the model there.
4. **Whether the fact existed.** Search every `tool_result` in the transcript for
   the fact the answer needed: an owner's name, a column mapping, a run id. If no
   result ever carried it, the cause is the result shape, not the model.
5. **Refusals.** Quote any error text a tool returned. A refusal that teaches the
   model the right argument costs one extra turn. A refusal that sends the model
   to another tool is copy.
6. **The final answer, read against the guard.** `check.py` matches
   `must_mention` against every text block the assistant wrote, not just the
   last. A run can pass on a sentence it wrote halfway through, so read the final
   answer before trusting a pass. For a regex failure, write down the sentence
   the model actually wrote, and decide whether a human grader would accept it.

## 4. Fix the layer

**Copy fixes have to help every model.** Write the rule the right answer
follows, never the scenario's own facts. If a test fixture says "October", the
instruction's example must not. Keep a rule conditional when it applies only
sometimes, for example "when the figures go into a written document". That way
the neighbouring cases that rightly take the other path are not pulled across.

- Each tool's copy has a token ceiling: `TestNoSingleToolTakesMoreOfTheWindowThanItsShare`.
  Tighten existing wording before you add new wording.
- A rule that belongs to every tool goes in the server instructions
  (`shared/ports/mcp/mcp.go`). A rule that belongs to one tool goes in that
  tool's `Purpose`, `Instead`, `Limits` or `Retain`.
- Copy edits regenerate derived pages. In `backend/`, run
  `go test ./internal/compose/ -count=1` with `-update-mcp-info`,
  `-update-ai-prompts`, `-update-agent-tool-budget` and
  `-update-mcp-tool-coverage`, then commit what changed.
- Copy edits also put AI certification records that read that copy into "re-check
  pending" ([certify-an-ai-model.md](certify-an-ai-model.md)). Say so in the PR.

**Result-shape fixes** reuse the engine that already computes the fact. When
one tool already returns the fact that another tool lacks, share that
implementation instead of writing a second one. The owner's name on
`search_records`, `list_records` and `read_record` rows comes from the same
seat namer `query_workspace` uses. A field added to `crm.yaml` is additive, and
`make contract-breaking-check` holds it.

**Grader fixes never let a wrong answer through.** Widen a pattern only for a
sentence a real run actually wrote and that a human would accept. In the same
change, add a should-pass test for that sentence and a should-fail test for the
defect the guard exists to catch, in `e2e/llm/tests/test_scenario_guards.py`.
A judged criterion that misreads answers is fixed with labelled fixtures under
`e2e/llm/testdata/<case>/`; the judge's accuracy is scored against them.

**Model causes** are not fixed by lowering the bar. Leave the case red. Record
what went wrong, and try a copy change only if it states a rule that is true
for every model.

## 5. Prove it for free

```bash
make test-e2e-llm-check                       # checker, judge replays, bridge
(cd e2e/llm && python3 -m unittest discover tests)
make check-go                                 # copy, budgets, derived pages
```

Run a copy fix's own Go tests too. A grader fix should be seen failing before the
YAML change, so you know the new test actually tests the guard.

## 6. Re-run what changed, plus controls

Re-run only the scenarios a fix should move, on both targets, plus controls:

- every case that was red;
- every case that requires or permits a tool whose copy you changed. A
  conditional rule about documents still has to leave plain number questions
  on `run_report`.
- one case that already passed on both targets, to catch a broken harness.

```bash
for c in claude gpt; do
  for s in case20_put_it_in_the_board_pack case7_ask_for_a_number; do
    MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=$c E2E_LLM_VIA=cli \
      E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
      SCENARIO=$s make e2e-llm
  done
done
```

The lane is not deterministic: three runs, with a pass at two. A move from 1/3
to 2/3 is noise until a second sweep agrees. A move from 0/3 to 3/3 is a fix.

## 7. Widen what the scenarios prove

Section 4 of the coverage page lists the tools no case requires. Not every tool
there needs one:

- **Tools that act, especially irreversibly** (sending, moving a deal, booking,
  bulk edits, deciding approvals) need a case. An untried one is a write no
  measurement has ever asked a model to choose correctly.
- **Plumbing** (`describe_*`, `whoami`, `read_record`, `list_records`) stays in
  `may_call`. Requiring a step on the way grades the route, not the outcome.
- **A tool no realistic errand needs** is a candidate to merge or retire, since
  every tool costs prompt tokens on every call.

Write a case as an errand a user would actually say, never a tool drill.

- `must_call` lists the tools the job cannot be done without. Use `a|b` where either of
  two engines gives a correct answer, and `must_call_with` where the argument
  is what the case tests, such as `decide_approval` with `approve`.
- `writes: true` if the case changes anything, and `must_end_with` for the
  records it must leave changed. The answer can claim a write that never
  landed; the end state cannot.
- Seed through the real writers, never raw SQL, so the audit trail is real.
- Give each criterion a judged statement in `criteria.yaml`, with labelled
  correct and defective answers under `e2e/llm/testdata/<case>/`.
- Copy the long comments in the existing scenarios. Each guard explains what it
  catches and why.

Run a new case once on each target before you trust it. A case that passes
without depending on the thing it names proves nothing. Break the product
behaviour it covers, and watch the case go red.

## 8. Publish

Commit the verdicts under
`backend/internal/compose/aicert/records/mcp_e2e/<folder>/` and regenerate the
coverage page. The steps are in [the run guide](test-the-mcp-surface-end-to-end.md#8-publish-the-result).
In the PR, name each case that moved, its before and after, and the layer each
fix was in.
