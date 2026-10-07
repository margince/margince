# Add an AI task or invocation site

A checklist for putting a new AI call into the product. The AI surface is
contract-first like the HTTP API: you declare it, regenerate, then implement.
The build refuses a site the contract never declared, a shipped task whose
site nobody wrote, and a site no certification case can measure.

Why it works this way:
[explanation/ai-runtime.md](../explanation/ai-runtime.md). Step 6, writing the
certification case, has its own guide:
[write-a-certification-case.md](write-a-certification-case.md). To certify a
model **binding** that already exists (a swap, a cheaper candidate), you want
[certify-an-ai-model.md](certify-an-ai-model.md) instead. For the branch
and PR mechanics every change goes through, see
[CONTRIBUTING.md](../../CONTRIBUTING.md).

> Everything here is free except one step. Steps 1–7 need no model, no key
> and no network. Only step 8 (`make e2e-ai`) calls a real provider, and it bills
> **your own API key**; Margince runs no inference of its own. You do not have to
> run it: a site that has never been certified is reported as `absent`, and you
> may open a PR in that state.

## Quick start: a second prompt on a task that already exists

The common case, and the cheapest way to see the whole loop. `enrich` already has
a ladder, a budget posture and a **lane** (the field on `compose.ModelPath` that
hands a task to the process running it), so all you add is one more place that
calls it.

```bash
# 1. name the new site in the contract, under the task's sites:
#      sites: [signature, letterhead]        # backend/api/ai-tasks.yaml
make gen                                   # compiles it into tasks_gen.go

# 2. write the call site in internal/compose/, then register it +
#    bind its certification case (one line, in NewTaskCensus):
#      oneShot(ai.TaskEnrich, "letterhead", letterheadCases{})
#                                            internal/compose/aitaskregistry.go

# 3. write internal/compose/certcase_letterhead.go
#    and  internal/compose/aicert/corpus/enrich/letterhead_01.yaml

make check                                 # free: every gate names what is missing
make e2e-ai-report                         # free: your site now shows as `absent`

# 4. carry the new site into the committed certification page
cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
```

Run `make check` early and often; it is the feedback loop. The gates name the
file you have not written yet, in the order you need them, so a red test tells
you the next step. You do not need a working AI setup for any of it.

When you are ready to spend, `make e2e-ai TASK=enrich` certifies for real
(step 8) and `make e2e-ai-report` then shows a band instead of `absent`.

## Task, or site?

A **task** is the routing, budget and cost unit: it owns a fallback ladder, an
execution mode and a budget posture, and may carry several prompts. A **site** is
one named place in the build that calls the model. A task can carry several sites
(`cold_start`, `voice_build` and `summarize` each do). `make e2e-ai-report` prints
the current number of tasks and sites, with no model spend.

| You are adding | Do |
|---|---|
| Another prompt for the same workload: a second pass, an evaluation call, a fan-out lane | a **site**: one name in the task's `sites[]`, then steps 3–9 |
| A workload that deserves its own ladder, budget posture or cost line | a **task**: all nine steps |

Do not reuse an existing task's name for a different workload. Routing, budget
limits, tracing and the certification record are all tracked per task name. Two
workloads sharing one name have their spend merged in reporting, and certifying
that name then proves neither of them works.

Every site declares a **kind**, which states how the model is invoked. The kind
caps how much of the site one certification run can cover:

| Kind | The site… | A run can certify at most |
|---|---|---|
| `one_shot` | builds one request and reads one reply | `full_invocation` |
| `multi_turn` | answers inside a conversation the caller supplies | `single_turn` |
| `agent_loop` | reasons over a cumulative, tool-fed window | `single_turn` |

## Steps

1. **Get the declaration into the contract.** `backend/api/ai-tasks.yaml` is a
   **mirror**: the normative AI task contract is maintained in a specification
   repository the maintainers own, and this file must match it verbatim. So the
   declaration is agreed there first, and lands here as a copy.

   What that means for you depends on where you are:

   - Maintainer, or working with spec access: land the declaration in the
     specification repo, then copy it down into `ai-tasks.yaml` unchanged.
   - Outside contributor: **open an issue** proposing the task or site, with
     the YAML block below filled in and a sentence on what calls it. A maintainer
     lands it upstream; your PR then carries the mirrored copy and everything from
     step 2 on. Do not skip ahead and hand-edit `ai-tasks.yaml` on its own: a
     mirror with no upstream declaration behind it cannot be merged, however green
     the build is. No gate catches this order, because every check compares the
     code against this repository's copy of the contract.

   The declaration itself, the shape to put in that issue and to mirror here:

   ```yaml
   tasks:
     meeting_notes:                       # illustrative — not a shipped task
       ladder: [cheap_cloud, premium]     # ordered capability tiers, not models
       execution_mode: interactive        # interactive | background
       on_budget_exhausted: degrade       # pairs with execution_mode, always:
       status: shipped                    #   interactive↔degrade, background↔queue
       # a bare name is a site of kind one_shot; write
       # {name: chat, kind: multi_turn} for any other kind
       sites: [summarise]
       company_context: none              # or {scopes: [...], token_budget: N}
       # no_payload: true                 # content that must never be captured
       # cost_unit: per_message           # only if the estimator prices it
       doc: "one line on what this task is for"
   ```

   - `status`: `shipped` obliges every name in `sites[]` to exist, be
     registered, own a certification case and own a corpus scenario. `planned`
     forbids all four. Declare `planned` while the site is unwritten. Change it to
     `shipped` in the commit that adds the site.
   - `company_context`: `none`, or scopes + `token_budget` (+ `conditional`
     for "only when the caller asks"). It is required: an absent policy is a
     build error, never a runtime default.
   - `no_payload: true`: content from this task must never reach
     `ai_call_payload`, whatever the deployment's capture posture says. It is a
     parsed field so that the build enforces this data-protection control.
   - `cost_unit`: only for a task the pre-flight estimator prices, and only
     a name `internal/compose/costestimate` implements (today `per_message`,
     `per_contact`). Naming a rule that does not exist, or implementing one
     nothing names, fails the build. Omit it for an unpriced task.

2. **Regenerate** with `make gen`. `tools/gen-aitasks` compiles the contract into
   `internal/modules/ai/tasks_gen.go` (your `ai.TaskX` constant, `ai.SitesFor`,
   `ai.Status`, `ai.CompanyContextFor`) and rewrites
   `config/margince.schema.json`. Never hand-edit either; commit both with
   the contract in one commit, or the drift gate fails.

3. **Wire the task's lane** *(new task only)*.
   A **lane** is how a task reaches a running process. Declaring and binding it
   happen in `internal/compose/brain.go`:

   1. Declare it: a field on `compose.ModelPath`, named for the task it
      serves.
   2. Bind it in `modelPathForRouter`, or the field stays nil and the task
      never reaches the Router: `MeetingNotes: brain(ai.TaskMeetingNotes),`.
   3. Hand it to a role: an `Option` in the compose package (the
      `WithColdStart` / `WithOfferDraft` pattern), passed by the process that
      runs the workload: `cmd/api` for an `interactive` task, `cmd/worker` for a
      `background` one.

   Two gates read this. `TestEveryModelLaneIsWiredToTheTaskItIsNamedFor` names
   the missing bind for you; `TestEveryCensusedSiteRidesALaneAProcessRoleWires`
   fails when no `cmd/` role passes the lane anywhere, so a site cannot be
   registered, scored and recorded while no binary reaches it.

4. **Write the call site** in `internal/compose/`. The import DAG lets only
   `compose` and `cmd` depend on the `ai` module, so a request builder inside `internal/modules/<name>/` fails
   `arch-lint` before any test runs. Every call goes through the Router via the
   lane, and `TestNoModelClientOutsideTheGate` fails a model client built
   anywhere else. Keep the builder and the validator reachable: the
   certification case must call the same two functions, not a copy of them.

5. **Register the site and bind its case** with one line in the **census**
   (`compose.NewTaskCensus`, `internal/compose/aitaskregistry.go`), the list of
   every invocation site this build ships, checked against the contract on every
   boot and in every test run.

   ```go
   oneShot(ai.TaskMeetingNotes, "summarise", meetingNotesCases{})
   ```

   `oneShot` / `multiTurn` / `agentLoop` are the helpers; use the one
   matching the kind the contract declares. Write the line by hand; the census
   is not derived from the contract.

6. **Write the certification case and its scenario**:
   `internal/compose/certcase_<site>.go` plus at least one fixture under
   `internal/compose/aicert/corpus/<task>/`. This is the substantial half:
   [write-a-certification-case.md](write-a-certification-case.md).

7. **Verify** with `make check`. What each omission looks like:

   | Missing | Fails as |
   |---|---|
   | contract edited but not regenerated | `make drift`: a generated file differs |
   | a shipped task's site not registered | `task X is shipped but its site "y" is not registered` |
   | a site the contract never declared | `…the contract declares no such site (add it to sites[]…)` |
   | a site registered on a `planned` task | `task X is planned but site "y" is registered` |
   | a registered site with no case | `TestTaskCensusBindsACaseToEverySite` |
   | a case claiming more than its kind allows | `…claims more than its kind's "…" — a case may only narrow` |
   | a shipped site with no scenario | `shipped sites with no corpus scenario: […] — each is a prompt that ships uncertified` |
   | a `planned` task carrying a corpus scenario | `planned tasks carry corpus scenarios: […] — a task nobody built cannot be certified` |
   | a `planned` task carrying a certification record | `planned tasks carry certification records: […] — the record claims a band for a prompt that does not ship` |
   | a fixture that is not the shape its site takes | `TestEveryCorpusScenarioPreparesAgainstItsSite` |
   | a closed answer enum with a kind no accepted scenario names | `TestEveryClosedAnswerKindCarriesAScenario`. Each kind of the enum needs its own accepted scenario; one per site is not enough |
   | a task with no lane, or a lane no role wires | `TestEveryCensusedSiteRidesALaneAProcessRoleWires` |
   | a new `.go` file with no SPDX header | `TestEveryHandWrittenGoFileCarriesTheLicenseHeader` |

8. **Certify it** once the gates are green. This is the one step that costs money.
   It needs a provider key in your environment (`GEMINI_API_KEY`,
   `ANTHROPIC_API_KEY`, …) and the two models the run binds: `MODEL=` for the
   candidate and `JUDGE=` for the second model that grades it. Both are set up in
   [certify-an-ai-model.md § Prerequisites](certify-an-ai-model.md#prerequisites).
   Read that first, or the run fails on a missing key.

   ```
   # real calls on your key; MODEL is required. JUDGE defaults to claude_cli:claude-sonnet-4-6
   make e2e-ai TASK=<your task> MODEL=gemini:gemini-3.1-flash-lite
   make e2e-ai-report                # free: band, scope, binding, counts, scenario coverage
   cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
   ```

   That last line rewrites
   [reference/ai-certification.md](../reference/ai-certification.md) from the
   record the run just wrote. It is free, and `make check` fails until the
   committed page matches.

   `TASK=` takes the name you declared in step 1. A name with no scenarios behind
   it (a typo, or a task with no corpus) stops the run with
   `task "…" has no scenarios under …` before any provider request is made, so
   getting it wrong costs you nothing.

   Commit the record under `internal/compose/aicert/records/<task>/`.

9. **Ship it**: contract, generated files, site, census line, case, scenario and
   record in the PR ([CONTRIBUTING.md](../../CONTRIBUTING.md) has the branch
   and gate rules). You may skip step 8: the report then reads `absent` for
   your site, and the paid lane never gates a merge.

   Two gates about the activity rail stop you here if the task is new (a new site
   on an existing task trips neither):

   - `TestEveryKindSomethingProducesIsOneTheContractCanExpress`: a task the
     router announces under a name `AiActivityKind` does not carry is a kind the
     wire cannot express, and the rail renders nothing for that AI work. Its
     failure prints an `align:` line naming the file and what to add.
   - The frontend census: `ACTIVITY_LINE` in
     `frontend/src/app/ai-activity-lines.ts` is typed `Record`, not
     `Partial<Record>`, so a new kind is a compile error. Either write its copy
     in en/de/vi for all six states, or state in code why it is not shown.

   Default to not shown. Show the kind only if a rep waits on the work. If no
   human can see it, say so in code. A system principal with no `on_behalf_of`
   is workspace-scoped with a NULL `actor_user_id`, and the feed filters on the
   reader's own id. See
   [explanation/ai-activity-rail.md](../explanation/ai-activity-rail.md).

   By default the router reports your task. If the task owns a durable row with
   its own lifecycle, consider registering it as a **carrier** in
   `ai.railOwners` instead. Only a carrier can say `queued`/`running` and declare
   the lease that makes `stalled` derivable.

## Adding a decision site

A site may also carry a **decision form**: a typed question a decision model
answers before the task's ladder, falling back to the LLM prompt whenever it is
unsure ([how the lane works](../explanation/ai-runtime.md#the-decision-lane)).
`site_triage` is the worked example: `sitetriage_decision.go` beside
`sitetriage.go`, and `certcase_sitetriage.go`.

1. **Declare it.** Add `decision: true` to the task in `ai-tasks.yaml` and run
   `make gen`. Only a `shipped`, `background` task may declare one: the attempt
   is an extra network call that no interactive deadline budgets for.
2. **Build the request** from the same inputs the LLM request builder reads,
   with `choiceQuestion` under the `decisionQuestionKey`, and call `ai.Decide`
   where the site called `ai.Ask`, passing the site name and a gate.
3. **Give the case the form.** The certification case implements
   `aitasks.DecisionCase`; `TestEveryDecisionTaskHasAnAdapterAndEveryAdapterIsDeclared`
   fails a declaration without one, and one without a declaration.
4. **Certify it** ([certify-a-decision-site.md](certify-a-decision-site.md)).
   Until its row is in `decisioncert_gen.go`, the lane skips the site.
5. **Regenerate the prompts page**, which now shows the site's decision question:
   `cd backend && go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts`.

Write the question to the decision-model guideline, not by pasting the prompt:

- [ ] One short question an expert answers in a second, written in
      full in `instructions`, naming state fields by backtick path
      (`` `page.text` ``). Question ids are never sent to the model.
- [ ] The state is JSON with meaningful keys and coded values spelled out in
      words. It carries the same inputs as the LLM request; a test beside
      the adapter holds that, as `TestTriageDecisionStateCarriesTheLLMInputs` does.
- [ ] The criteria keys equal the LLM's answer enum (a test holds that too), and
      each criterion says what makes *that* label right, including the trap it
      must not fall into. Add an `other` label if the list is not exhaustive.
- [ ] No fence, no output format, no language rules, no "state your
      confidence": the wire keeps data apart from instructions and computes
      confidence itself.
- [ ] The gate reads the answer into the site's existing answer type at the
      site's own floor constant. There is no second floor; below it, the
      ladder answers.
- [ ] Do not split a multi-factor judgment into several questions whose
      confidences you multiply: combined confidence was measured unsafe here.
- [ ] A `local_only` task's text never reaches a cloud decision model. That is
      the router's rule, so the site needs no egress field of its own.

## Notes

- A record covers one (provider, model, env) binding. Editing the prompt, the
  request builder, the grader or the scoring rule re-stamps the version and marks
  the record **stale**. Re-certify instead of hand-editing a record.
- **Renaming** a task or site starts upstream like any other contract change.
  It then lands here as the mirrored declaration, the census line, the case's
  `Site()`, the corpus `site:` field, the record directory, and any exemption
  entry keyed by the old name.
- **Retiring a site** means deleting its scenarios and its record too. A record
  left behind asserts a band for a prompt that no longer ships.

## Verify the new site is probeable

`make ai-probe ARGS=list` reads the census, so a newly registered site appears
there with no change to the probe. If it does not, the registration did not land.
`make ai-probe ARGS='scaffold <task>/<variant>'` then confirms the corpus scenario
round-trips into something runnable. See [debug an AI task](debug-an-ai-task.md).
