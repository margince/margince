<!-- prose:plain -->
# Improve how assistants use the MCP surface

Make a real assistant get more daily jobs right through the
MCP surface of Margince, and add to the set of tools these jobs prove. The loop goes like this.
Sweep, and read why each failure failed. Fix the layer that caused it, and prove the fix for free.
Then run the lane that costs money again, and publish.

The three targets are the assistants that users connect in real work:

| Target | `E2E_LLM_CANDIDATE` | `E2E_LLM_VIA` | Verdict folder |
|---|---|---|---|
| Claude (the default model of its CLI) | `claude` | `cli` | `claude-sonnet-5-5@claude-cli` |
| GPT through Codex | `gpt` | `cli` | `gpt-5.6-sol@codex-cli` |
| Gemini through Antigravity | `gemini` | `cli` | `gemini-3.8-flash-medium@agy-cli` |

Mistral through OpenRouter, and the bridge (`E2E_LLM_VIA=api|openrouter`)
that compares providers on equal terms, are good to have. Run them once a fix holds
on every target.

[test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md) covers how to run the lane,
how it ends, and the stops of its harness. This page takes that as read. The results go to
[reference/mcp-tool-coverage.md](../reference/mcp-tool-coverage.md).

```
  sweep every target ──► triage each red case ──► fix the layer that caused it
          ▲                (sections 2–3)            (section 4)
          │                                               │
   publish verdicts ◄── paid re-run: reds + controls ◄── free checks
     (section 7)          (section 6)                  (section 5)
```

## 1. Sweep every target, and keep the transcripts separate

Run the full sweep for each target, one after the other, as
[the run guide plans it](test-the-mcp-surface-end-to-end.md#4-plan-a-full-sweep-and-know-its-cost).
Never run them with other heavy work on the same Claude subscription, because the
judge uses it too. Use the loop that runs one scenario at a time in
[test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md#sweeping-through-a-subscription-limit).
Then a subscription limit pauses the sweep and does not end it.

`e2e/llm/records/` holds one transcript for each run of each scenario (`run1`,
`run2`, …) from the last sweep of it, from any candidate. After each scenario, copy the
transcript of every run to a folder for that candidate. If you do not, the
second sweep erases the evidence for the first:

```bash
mkdir -p .tmp/sweep/gpt && cp e2e/llm/records/"$name".run*.jsonl .tmp/sweep/gpt/
```

Keep the lane log of each scenario too: its failure lines name what failed to hold.

## 2. Triage: which layer failed

Every red case has one of the causes below, and each needs a different fix.
Decide which one from the transcript (section 3) before you change a thing.

| Cause | What you see | Fixed in |
|---|---|---|
| **Harness** | `HARNESS:` stop; every run "called nothing"; degraded search | the lane; see the harness table in the run guide |
| **Grader** | the final answer is right, but a check said no. A `not` guard reaches back over an earlier part of the sentence. A check knows the key of a field, and the model wrote its label. A pattern has `\\b` for `\b`: `check.py` drops the quotes and reads the rest as written | `e2e/llm/scenarios/*.yaml`, `criteria.yaml` |
| **Case** | the prompt asks the model to prefer one tool when another gives a right answer too | the prompt of the case; keep its checks |
| **Product: copy** | the model followed a wrong path that looked right, and a tool description or the server instructions pointed it there. Or the copy is false: it says a link moves a record when it adds one, or that a tool only offers when it applies and stores nothing. Or the key sentence sits on a tool the passport hides | `backend/internal/modules/agents/toolcopy_*.go`, `shared/ports/mcp/mcp.go` |
| **Product: result shape** | the answer needed a fact that no tool result carried: a uuid where it needed a name, or a list of meetings with no sign that no calendar is connected | the result type of the tool, or `backend/api/crm.yaml` |
| **Chance** | a long case with many steps fails once, and the transcript shows no cause | nothing yet: run it once more before you act |
| **Model** | the help was clear and the facts were there, and it still failed | nothing, unless a copy change can make the right path more clear to every model |

A case that fails on one target and passes on the other still has a
product or grader cause in most cases. The model that passes has a way past a problem that the model that fails
could not.

## 3. Read the evidence

Work through these in order for each run that fails.

1. **Read the failure lines of the lane log.** They say which tool was never called, or which
   pattern never matched. They also name a judged criterion that was refused, with the reason of the judge in one sentence.
2. **Read the tool calls in order.** Print them with the code in section 6 of the run guide.

   `python3 e2e/llm/why.py <scenario>` puts the tools that the scenario requires next to
   what each run could call, and what it called.
3. **Read the first call.** On the runs that fail, the first call decides the
   route in most cases.

   Compare it with a run of the same case that passes, from the other target
   or another model. Say the runs that open on tool `A` all pass, and the runs that
   open on tool `B` all fail. Then read the description of `B` for what sent the model there.
4. **Check whether the fact was there.** Search every `tool_result` in the transcript for
   the fact that the answer needed.

   That is a fact such as the name of an owner, a column map, or a run id. If no
   result ever carried it, the cause is the result shape, not the model.
5. **Read the errors.** Quote any error text that a tool returned.

   An error that shows the model the right argument costs one more turn. An error that sends the model
   to another tool is a copy problem.
6. **Read the final answer against the guard.** `check.py` matches `must_mention` against every text block.

   It reads every block the assistant wrote, not only the
   last one. A run can pass on a sentence it wrote part of the way through, so read the final
   answer before you trust a pass. For a regex failure, write down the sentence
   the model wrote, and decide whether a human grader would accept it.

## 4. Fix the layer

**Copy fixes have to help every model.** Write the rule that the right answer
follows, never the facts of the scenario itself. If a test fixture says `October`, the
example in the instructions must not. Make a rule apply only when needed, if it applies only
some of the time, for example "when the numbers go into a document". Then
cases close by, which take the other path for good reason, do not move to the new one.

- The copy of each tool has a token limit: `TestNoSingleToolTakesMoreOfTheWindowThanItsShare`.
  Make the current words shorter before you add new words.
- A rule for every tool goes in the server instructions
  (`shared/ports/mcp/mcp.go`). A rule for one tool goes in the
  `Purpose`, `Instead`, `Limits` or `Retain` of that tool.
- Copy edits generate the pages made from it again. In `backend/`, run
  `go test ./internal/compose/ -count=1` with `-update-mcp-info`,
  `-update-ai-prompts`, `-update-agent-tool-budget` and
  `-update-mcp-tool-coverage`, then commit what changed.
- Copy edits also put the AI certification records that read that copy into "re-check
  pending" ([certify-an-ai-model.md](certify-an-ai-model.md)). Say so in the PR.

**Result shape fixes** use the engine that already works out the fact. When
one tool already returns the fact that another tool does not have, share that
code, and do not write a second copy. The name of the owner on
`search_records`, `list_records` and `read_record` rows comes from the same
seat namer that `query_workspace` uses. A field added to `crm.yaml` is additive, and
`make contract-breaking-check` holds it.

**Grader fixes never let a wrong answer through.** Make a pattern match more only for a
sentence that a real run wrote, and that a human would accept. Prove the change both ways with
`e2e/llm/probe.py`: `--expect correct` on that sentence, `--expect defective` on the bug the guard is there to catch.
In the same change, add both as tests to `e2e/llm/tests/test_scenario_guards.py`.
To fix a judged criterion that reads answers wrong, use labelled fixtures under
`e2e/llm/testdata/<case>/`. The judge gets a score against them.

Never fix **model causes** by letting more answers pass. Leave the case red. Record
what failed, and try a copy change only if it states a rule that is true
for every model.

## 5. Prove it for free

```bash
make test-e2e-llm-check                       # checker, judge replays, bridge
(cd e2e/llm && python3 -m unittest discover tests)
make check-go                                 # copy, budgets, derived pages
```

Run the Go tests of a copy fix too. A grader fix should fail before the
YAML change, so that you know the new test checks the guard.

## 6. Run again what changed, plus controls

Run again only the scenarios that a fix should change, on every target, plus controls.
Never run only the red case: a copy fix for one case has turned a case close by red.
Every case gets every tool description in `tools/list`, so pick the set by the tools whose words moved:

- every case that was red, or that you fixed;
- every case that requires or allows a tool whose copy you changed. A
  rule about documents that applies only when needed must still leave plain number questions
  on `run_report`.
- one case that already passed on every target, to catch a harness that does not work.

```bash
for c in claude gpt; do
  for s in case20_put_it_in_the_board_pack case49_who_can_introduce_us; do
    MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=$c E2E_LLM_VIA=cli \
      E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
      SCENARIO=$s make e2e-llm
  done
done
```

The lane does not give the same result each time: it makes three runs, and two passes are a pass. A change from 1/3
to 2/3 is noise until a second sweep agrees. A change from 0/3 to 3/3 is a fix.

## 7. Prove more tools with the scenarios

Section 4 of the coverage page lists the tools that no case requires. Not every tool
there needs one:

- **Tools that change data** need a case, most of all when you cannot take the change back. Examples are send,
  move a deal, set up a meeting, edits to many records at once, and decide approvals. A tool with no case is a write that no
  test has ever asked a model to choose right.
- **Support tools** (`describe_*`, `whoami`, `read_record`, `list_records`) stay in
  `may_call`. To require a step on the way grades the route, not the outcome.
- **A tool that no real task needs** is a candidate to merge or remove, because
  every tool costs prompt tokens on every call.

Write a case as a task a user would ask for in real work, never as a test of one tool.

- `must_call` lists the tools the job needs. Use `a|b` where either of
  two engines gives a right answer. Use `must_call_with` where the argument
  is what the case tests, such as `decide_approvals` with `decision: approve`.
  A router is matched by the name the model called, so pin the value that
  picks its tool the same way: `read_forecast.view=movement`.
- Set `writes: true` if the case changes any data, and `must_end_with` for the
  records it must leave changed. The answer can claim a write that never
  happened; the end state cannot.
- Seed through the real writers, never SQL by hand, so the audit record is real.
- Give each criterion its text in `criteria.yaml`, with labelled
  right and wrong answers under `e2e/llm/testdata/<case>/`.
- Copy the long comments in the scenarios that exist. Each guard explains what it
  catches and why.

Run a new case once on each target before you trust it. A case that passes
without the thing it names proves nothing. Break the product
code it covers, and watch the case go red.

## 8. Publish

Commit the verdicts under
`backend/internal/compose/aicert/records/mcp_e2e/<folder>/`, and generate the
coverage page again. The steps are in [the run guide](test-the-mcp-surface-end-to-end.md#8-publish-the-result).
In the PR, name each case that changed, its before and after, and the layer each
fix was in. Run `make test-roles` too, as [the run guide](test-the-mcp-surface-end-to-end.md#9-run-the-role-sweep-with-every-pass) says.
