<!-- prose:plain -->
# Write a certification case (and its scenarios)

This is step 6 of [add-an-ai-task.md](add-an-ai-task.md), on its own page because this step needs the most work.
A **certification case** links one call site to the production code that serves it. So a certification run
measures the request that the product sends, judged by the validator that the product applies.

**Call production, never build it again.** A case that builds the request again, or writes the validator again,
tests a copy. When someone later breaks the real builder or validator, that copy keeps passing, because it
never touched the code that failed.

Two files for each site:

```text
internal/compose/certcase_<site>.go              the case
internal/compose/aicert/corpus/<task>/*.yaml     one or more scenarios
```

Both `.go` files you add here (the case and its test) need the two-line BUSL-1.1 SPDX header. Every Go file
that a human writes in this repository carries it (see *License headers* in the rulebook). `make check` fails
a file that skips it.

## What a reply can be: the four outcomes

All of the page below uses these four words. `Evaluate` returns one of them, and they stay separate because
they fail for different reasons, and need different fixes:

| Outcome | Means |
|---|---|
| `accepted` | the production validator accepted the reply **and** it is the answer the fixture expects |
| `wrong_answer` | a reply in the right form that the validator accepted, which says something else: a measure of the model, not a bug |
| `invalid` | the production validator **refused** the reply: the deterministic sign that the model made something no one can use |
| `abstained` | the reply passed the validator and carries nothing, **and** the site counts that as finished work |

`invalid` and `abstained` are the pair to get right. A validator that refused all that a reply claimed, and a
reply that claimed nothing, both leave no rows. Yet they are not the same event at all. The first is a model
that makes things up past a gate. The second is a model that refuses to make things up.

Where an empty result *is* the failure, report `invalid` instead. The cold start is such a case. When it
reads fields from a source, an empty result turns into a message for the human: the source cannot be read.

## The loop: scenario first, then the case, then spend

Certification is a lane that costs money and needs the network, so the whole loop is built to run offline
first. Work in this order, and you spend nothing until you already know that the thing you measure works.

1. **Write the scenario.** The fixture is the input that production gets, and `expect` is what a right answer
   looks like.

   When you write it first, you must answer the question the case has to answer. *What makes a right reply
   here different from one that only looks right?*
2. **Write the case**: the functions below. `make check` is red until the census line and the case
   agree; that is the state you expect.
3. **Drive it offline with fixed replies.** Write `certcase_<site>_test.go` next to the case.

   Give it a stub completer that answers with a fixed string, and run `Prepare → Run → Evaluate` against it.
   Check the outcome word for a right reply, and for each way the validator can refuse one. Also check it
   for the answer that is in the right form but wrong. This is the test that pins the case. It needs no
   network, and it is where you find the bugs of a case at no cost:

   ```go
   type letterheadStub struct{ answer string }

   func (s *letterheadStub) Complete(context.Context, model.Request) (model.Response, error) {
       return model.Response{Text: s.answer}, nil
   }

   prepared, err := letterheadCases{}.Prepare(fixture, expected)
   // …
   trace, err := prepared.Run(context.Background(), &letterheadStub{answer: wellFormedButWrong})
   if got := prepared.Evaluate(trace); got.Result != aitasks.OutcomeWrongAnswer {
       t.Errorf("a well-formed wrong answer reports %q, want wrong_answer", got.Result)
   }
   ```

4. **Run `make check`.** The corpus gates run your scenario against your case without a model:
   - `TestEveryCorpusScenarioPreparesAgainstItsSite` catches a fixture of the wrong shape, or an expectation
     that the validator could never meet.
   - `TestEachAbstentionScenarioCatchesTheFabricationItTargets` runs every abstention scenario twice. It runs
     it once with the answer it calls right, and once with the made-up answer it is there to refuse. A
     scenario that passes for any reply the model makes does more harm than no scenario.
   - `TestEveryClosedAnswerKindCarriesAScenario` names the kinds of the answer enum of your site that still
     have no accepted scenario (see below).
5. **Generate the certification page again**, in the same commit as the scenario:

   ```
   cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
   ```

   [reference/ai-certification.md](../reference/ai-certification.md) lists the scenarios of every site and
   links each case. So a new scenario turns `make check` red until the committed page has it. The command is
   free: no model, no network.
6. **Then spend.** Run `make e2e-ai TASK=<task>`, and read the band with `make e2e-ai-report`.

   The page carries the records of each site, and its scenarios too. So run the command from step 5 again
   once the run writes one.

## The interface

`aitasks.CaseFactory` (`internal/compose/aitasks/case.go`), with one copy of the code for each site:

| Method | What it must do |
|---|---|
| `Site()` | give the same task / variant / kind that the census line claims; the gate reports it when they do not agree, and says so |
| `Prepare(fixture, expected)` | parse the fixture into the shape that **production** gets, refuse an expectation that the validator of this site could never meet, and return a `PreparedCase` closed over both |
| `Run(ctx, completer)` | make the production call, and return every request it made in the `Trace` |
| `Evaluate(trace)` | apply the production validator, then compare with the expectation, and report an `Outcome` |
| `CertifiedScope()` *(not required)* | narrow the claim when a run covers only part of the site |

Two rules for every case:

- **Refuse in `Prepare` what no reply can reach.** Examples are a label outside the closed set, a count that
  cannot match, or a fixture longer than the read keeps. To name it here costs a parse. To find it after a
  run that costs money wastes that money, and leaves a band that measured nothing.
- **`Prepare` takes the fixture and the expectation separately.** The fixture is what production gets; the
  expectation is what the corpus claims about the reply. If you put them into one value, any gate that changes a
  fixture can change a claim, and no one sees it.

## By kind

The kind that the contract declares decides what `Run` can do.

### `one_shot`: build a request, read a reply

This is most cases. `Run` builds the production request and makes one call:

```go
func (c *classifyCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
    req := classifyRequest(c.batch)                    // the production builder
    trace := aitasks.Trace{Requests: []model.Request{req}}
    resp, err := completer.Complete(ctx, req)
    if err != nil {
        return trace, fmt.Errorf("capture_classify/classify: %w", err)
    }
    trace.Output = resp.Text
    return trace, nil
}
```

Then `Evaluate` runs the checks of the engine, in the order of the engine: parse, production validator, and
only then the compare against the expectation. The order counts: a refused reply has no answer to compare.

**Send it as it is.** Production may send the same request again as a retry for shape. It may also ask again
on the next rung for an item below the floor. A case that does either would certify the answer a model gives
*after it is asked to try again*, and not the answer it gives. Every case refuses that one, in the same way.
Reference: `certcase_captureclassify.go`.

### `multi_turn`: one turn in a conversation from the fixture

The fixture carries the conversation that the caller would have built (`history`, the new `message`, and any
context the turn is built from). `Run` builds the request for **that one turn**:

```go
req, err := onboardingCompanyAnswerRequest(c.message, c.history, c.conversation, c.locale, c.selection)
```

The rest of the conversation is *part of the fixture, and does not run*, which is what `single_turn` scope
says. In `Prepare`, build the gate of the validator from the message, history and context of the request.
That is why `Prepare` exists. A validator that cannot see the data from the fixture is a different check from
the real one. It passes replies that production would refuse, while it claims to stand for it.

Reference: `certcase_companymessage.go`, whose scenario turns on `next_required_field`. The same reply, sent as
it is in a conversation that asked nothing, would be a change that no one asked for.

### `agent_loop`: a window that tools fill as it runs

`agent_loop` is an engine. Each of its sites is one scheduled agent, and each site gets its own case
(`agentLoopCases{agent: name}`, registered for each site from `ai.AgentsFor`). There is no one request to
build, and to make one up would make the case say the wrong thing about what it runs. So `Run` drives the
**real loop** with a stand-in model that records. `Evaluate` replays the reply through the same loop to see
which step it makes.

The window is the agent's own. `Prepare` finds the agent through `ScheduledAgentSpecByName`, the same function
that the runner service uses. So the goal, the tool allowlist and the language rule are the ones production
uses, and the runner itself limits the registry to the allowlist. A fixture carries only what changes between
runs of one agent:

```yaml
fixture:
  trigger_ref: morning_brief:2026-09-24:d48b383f3e8acec5d620c82b8c9b4202  # as the scheduler mints it
  grounding:                         # what retrieval seeded; every seed enters at T2
    - source_id: deal:0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e07
      content: Heat recovery — renewal due Friday.
```

The case refuses by name a `goal`, a `tools` surface or a `trust_tier` for each seed: each would certify a
window that no run gets. The expectation is the step that the turn should take, and `Prepare` refuses one that
names a tool this agent cannot call. Reference: `certcase_agentloop.go`.

Every shipped `agent_loop` case has no judge: its check reads the one step whole. A case that does carry a
rubric must say that it grades the `FIRST step only` of the turn (the words `corpusagentloop_test.go`
requires). If not, the judge marks a right first call down for steps it could not see.

## The scenario file

`internal/compose/aicert/corpus/<task>/<name>.yaml`:

```yaml
name: meeting_request_from_reply
task: capture_classify
site: classify                     # which registered site is under certification
source: hand_authored              # must be this — `extracted:` is refused
sanitized_by: hand_authored/<who>  # who reviewed it for sensitive content
fixture:                           # the DATA production is given — never a prompt
  - subject: 'Re: pricing walkthrough'
    body: |
      Could we grab 30 minutes Thursday afternoon?
expect:
  outcome: accepted                # accepted | wrong_answer | invalid | abstained
  answer: [meeting]                # in the SITE's vocabulary — read its Prepare
  rubric: >
    What the grader is told to score, and why it matters to the product.
  bands: {certified_min: 70, degraded_min: 50, floor: 40}   # required unless judge: none
  caps: {max_tokens: 400, p95_latency_ms: 6000}             # optional ceilings
```

For each field, and what each one is checked against, see
[explanation/ai-runtime.md](../explanation/ai-runtime.md#every-field-in-a-scenario-file).
These rules decide whether a scenario is worth having:

- **A scenario holds the input, never the prompt.** The case of the site builds the request. A scenario that
  carries a prompt certifies a copy, and could not make the same request in any case. The product mints a new
  secret marker for each call, to fence data it does not trust. So no fixed text can stand in for a real prompt.
- **`expect.caps` is a gate.** To go over one fails the run, like a wrong reply. `max_tokens` is the budget of
  the model's **answer** alone. It is not the budget of the input of your fixture, which the model cannot make
  smaller. A scenario with a long input and a small cap tests drafting within budget, and not prompt length.
- **`expect.answer` has no shared shape.** It can be one token, a list, a map, or a `{min,max}` band. Each
  site owns its vocabulary, because what makes a right answer different from a wrong one is not the same for
  each site. Read the `Prepare` of that site before you write one.
- **`expect.outcome` does not have to be `accepted`.** A run passes when the validator of the site reports the
  outcome that the scenario named. That lets a scenario exist whose right answer is *silence*.
- **`judge: none`** is for a check that sees all that a judge would. Such a case carries no `rubric` and no
  `bands`, and says in `judge_none_reason` why the check alone is enough.

  `corpusjudgeless_test.go` requires a proof for it. The answer it calls right must reach its outcome. The
  wrong answer that its rubric catches must reach the failure named in the case, not a reply in the wrong form.
  If the reader sees something that the check never reads, such as a free text summary, the judge stays.
- **A rubric asks only for declared fields**: what the reply of the site can carry. A rubric that scores a
  field the schema does not declare measures nothing. The model cannot make that field, even when the rest of
  its answer is right. So that part of the rubric can only mark a right reply down. Read the request builder
  and the answer schema first.
- **Every fixture is made up.** No real company, deal or contact data goes under this tree.

Point a scenario at one thing that can go wrong, and make it a real test. Examples are an instruction put into
evidence, a page with no facts in it, or two sources that do not agree on a price. Another is a cap that
production holds and the prompt states. A fixture that the model handles with no effort reports a band that no
one learns from.

## Scope: what a run may claim

A record names how much of the site the run covered, from all of it down to one call:

| Scope | Means |
|---|---|
| `full_invocation` | the run drives the whole production call; to certify it certifies the site |
| `single_turn` | the window is seeded and one reply graded; the rest of the conversation or tool loop is part of the fixture, and does not run |
| `single_call` | the run makes one of the calls that the site makes for each use; the answer that production serves is built from calls the run never made, and no one measures the step that puts them together |

The scope comes from the kind by default (`one_shot` → `full_invocation`, else `single_turn`). A case may only
**narrow** it, and `TestOnlyTheCasesThatMeasureLessNarrowWhatTheyCertify` refuses a case that claims more.
Declare `single_call` when the site asks again for an item below the floor. Also declare it when the site asks
again after an answer it cannot read, or breaks the work into pages and merges the replies:

```go
func (captureClassifyCases) CertifiedScope() string { return aitasks.ScopeSingleCall }
```

A narrower scope also needs its entry, with the reason, in the `narrowedSites` map of that test. So the list
cannot get longer without a reason. The retry for shape in the model runtime is not a reason. Every case refuses
it in the same way. So a note that is true of every site would tell a reader nothing about any of them.

## When the band is not what you expect

Read the **payload trace** before you touch the prompt. Every candidate and judge call goes to
`.tmp/aicert/*.jsonl` (on by default, gitignored, after secrets are removed). There is one record for each
call, with `role`, `scenario`, `run`, `call`, and the request and answer as the product would have captured
them. In most cases you find a reply that the validator of the site refuses, and not a model that answers
wrong. An example is an evidence quote in other words, where the gate needs the exact words.

The run settings (`MODEL=`, `JUDGE=`, `RUNS=`, `TRACE=`), the verdict math, and how to read a record are all in
[certify-an-ai-model.md](certify-an-ai-model.md).

## The gates that only check what you wrote here

The full list of gates that catch what a case or scenario can get wrong is step 7 of
[add-an-ai-task.md](add-an-ai-task.md#steps). Know these by name while you write, because they read the
*content* of a scenario, and not only that it is there:

| Gate | Refuses |
|---|---|
| `TestEveryCorpusScenarioPreparesAgainstItsSite` | a fixture that is not the shape the site takes, or an expectation that its validator could never meet; the gate finds it without a model, before a run that costs money |
| `TestEachAbstentionScenarioCatchesTheFabricationItTargets` | an abstention scenario that grades the right answer, and the made-up answer it is there to catch, the same way. Such a scenario would pass for any reply the model makes |
| `TestEveryClosedAnswerKindCarriesAScenario` | a closed answer vocabulary with a kind that no `accepted` scenario names. One scenario can meet "this site has a corpus", while most of the enum gets no score |

`TestEveryClosedAnswerKindCarriesAScenario` is the one you will not expect, if your site answers from an
enum. It reads the vocabulary from the answer schema that the request of the site carries. It groups by
**enum**, and not by site, because the onboarding conversation sites share one schema. Each of them narrows it
in text that the gate cannot read. A kind is covered when *some* site that shares that enum scores it, on any
of them whose own prompt allows it.

Only an `accepted` scenario counts. An answer that refuses, or an abstention, names a kind without asking a
model to make it. So to count one would leave that branch of the prompt with no grade, while the gate is
green. Write the missing kinds as accepted scenarios, never as an abstention that only names them.

## Try it with a probe before you commit it

Get a scenario right before it goes into the corpus. `make ai-probe` runs one against its site through the
same `Prepare`/`Run`/`Evaluate` path, from a file of your own that never leaves the gitignored `.tmp/aitask/`.
That includes `--ai-fake`, which costs nothing, and still runs the fixture shape and the production
validator. See [debug-an-ai-task.md](debug-an-ai-task.md).
