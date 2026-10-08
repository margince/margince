<!-- prose:plain -->
# Add an AI task or invocation site

A checklist for adding a new AI call to the product. The AI side is contract first, like the HTTP API: you
declare the call, run the generator, then write the code. The build refuses a site the contract never
declared, and a shipped task whose site no one wrote. It also refuses a site that no certification case can
measure.

Why it works this way: [explanation/ai-runtime.md](../explanation/ai-runtime.md). Step 6, writing the
certification case, has its own guide: [write-a-certification-case.md](write-a-certification-case.md). To
certify a model **binding** that already exists (another model for the same work), use
[certify-an-ai-model.md](certify-an-ai-model.md) instead. For the branch and pull request steps every change
goes through, see [CONTRIBUTING.md](../../CONTRIBUTING.md).

> Everything here is free but one step. Steps 1–7 need no model, no key and no call to a provider. Only step 8
> (`make e2e-ai`) calls a real provider, and it costs money on **your own API key**; Margince runs no model of its
> own. You do not have to run it. A site that was never certified shows as `absent`, and you may open
> a pull request in that state.

## First try: a second prompt on a task that already exists

This case comes up most, and it is a good way to see the whole loop. `enrich` already has a ladder, a
budget rule and a **lane**. A lane is the field on `compose.ModelPath` that hands a task to the process that
runs it. So all you add is one more place that calls it.

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

Run `make check` early and many times; it is how you learn what to do next. The gates name the file you have not
written yet, in the order you need them, so a red test tells you the next step. You do not need a working AI
setup for any of it.

When you are ready to spend, `make e2e-ai TASK=enrich` certifies for real (step 8). `make e2e-ai-report` then
shows a band instead of `absent`.

## Task, or site?

A **task** is the unit for routes, budget and cost. It owns a ladder of models to try in turn, a run mode and a budget rule,
and it may carry several prompts. A **site** is one named place in the build that calls the model. A task can
carry several sites (`cold_start`, `voice_build` and `summarize` each do). `make e2e-ai-report` prints the
number of tasks and sites today, with no model spend.

| You are adding | Do |
|---|---|
| Another prompt for the same work: a second pass, a grading call, a fan-out lane | a **site**: one name in the task's `sites[]`, then steps 3–9 |
| Work that needs its own ladder, budget rule or cost line | a **task**: every step |

Do not use the name of a task that exists for different work. Routes, budget limits, call traces and the
certification record are all tracked per task name. Two kinds of work that share one name have their spend
merged in reports. Then a certification of that name proves that neither of them works.

Every site declares a **kind**, which states how the model is called. The kind limits how much of the site
one certification run can cover:

| Kind | The site… | A run can certify at most |
|---|---|---|
| `one_shot` | builds one request and reads one reply | `full_invocation` |
| `multi_turn` | answers inside a chat the caller gives it | `single_turn` |
| `agent_loop` | works over text that each tool call adds to | `single_turn` |

## Steps

1. **Get the declaration into the contract.**
   `backend/api/ai-tasks.yaml` is a **mirror**.
   The AI task contract that rules is kept in a specification repository the maintainers own.
   This file must match it word for word.
   So the declaration is agreed there first, and lands here as a copy.

   What that means for you depends on where you are:

   - Maintainer, or working with access to the specification: land the declaration in the specification
     repository. Then copy it into `ai-tasks.yaml` with no change.
   - Someone outside the team: **open an issue** that asks for the task or site. Fill in the YAML block
     below, and add a sentence on what calls it. A maintainer lands it in the specification. Your pull request
     then carries the copy and everything from step 2 on.

     Do not skip to a later step and edit `ai-tasks.yaml` on its own. A mirror with no declaration in the specification behind it cannot be merged, even with a green build.
     No gate finds this order, because every check compares the code against this repository's copy.

   The declaration itself, the shape to put in that issue and to copy here:

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

   - `status`: `shipped` means every name in `sites[]` must exist, be registered, own a certification case
     and own a corpus scenario. `planned` allows none of the four. Declare `planned` while the site is not written yet.
     Change it to `shipped` in the commit that adds the site.
   - `company_context`: `none`, or scopes and `token_budget` (and `conditional` for "only when the caller
     asks"). You must set it: a missing rule is a build error, never a default at run time.
   - `no_payload: true`: content from this task must never reach `ai_call_payload`, for any capture rule the
     install sets. It is a field the build reads, so that the build holds this privacy rule.
   - `cost_unit`: only for a task that `internal/compose/costestimate` prices, and only a name that
     it builds (today `per_message`, `per_contact`). A name with no rule behind it,
     or a rule that no name points at, fails the build. Leave it out for a task with no price.

2. **Run the generator** with `make gen`.
   `tools/gen-aitasks` compiles the contract into `internal/modules/ai/tasks_gen.go`.
   That file holds your `ai.TaskX` value, `ai.SitesFor`, `ai.Status` and `ai.CompanyContextFor`.
   The tool also writes `config/margince.schema.json` again.
   Never edit either file by hand.
   Commit both with the contract in one commit, or the drift gate fails.

3. **Wire the task's lane** *(new task only)*.
   A **lane** is how a task reaches a running process. You declare it and bind it in
   `internal/compose/brain.go`:

   1. Declare it: a field on `compose.ModelPath`, named for the task it serves.
   2. Bind it in `modelPathForRouter`, or the field stays `nil` and the task never reaches the Router:
      `MeetingNotes: brain(ai.TaskMeetingNotes),`.
   3. Hand it to a role: an `Option` in the `compose` package (like `WithColdStart` or `WithOfferDraft`).
      The process that runs the work passes it.
      That is `cmd/api` for an `interactive` task, and `cmd/worker` for a `background` one.

   Two gates read this. `TestEveryModelLaneIsWiredToTheTaskItIsNamedFor` names the missing bind for you.
   `TestEveryCensusedSiteRidesALaneAProcessRoleWires` fails when no `cmd/` role passes the lane at all. So a
   site cannot be registered, scored and recorded while no binary reaches it.

4. **Write the call site** in `internal/compose/`.
   The import rules allow only `compose` and `cmd` to depend on the `ai` module.
   So a request builder inside `internal/modules/<name>/` fails `arch-lint` before any test runs.
   Every call goes through the Router by way of the lane.
   `TestNoModelClientOutsideTheGate` fails a model client built in any other place.
   Keep the builder and the checker in reach: the certification case must call the same two functions.

5. **Register the site and bind its case** with one line in the **census**.
   The census is `compose.NewTaskCensus` in `internal/compose/aitaskregistry.go`.
   It lists every invocation site this build ships, and every start and test run checks it against the
   contract.

   ```go
   oneShot(ai.TaskMeetingNotes, "summarise", meetingNotesCases{})
   ```

   `oneShot`, `multiTurn` and `agentLoop` are the helpers; use the one that matches the kind the contract
   declares. Write the line by hand; the census is not built from the contract.

6. **Write the certification case and its corpus scenario**:
   `internal/compose/certcase_<site>.go`, plus one fixture or more under
   `internal/compose/aicert/corpus/<task>/`. This is most of the work:
   [write-a-certification-case.md](write-a-certification-case.md).

7. **Check** with `make check`. What each missing part looks like:

   | Missing | Fails as |
   |---|---|
   | contract edited but `make gen` not run | `make drift`: a generated file is not the same |
   | a shipped task's site not registered | `task X is shipped but its site "y" is not registered` |
   | a site the contract never declared | `…the contract declares no such site (add it to sites[]…)` |
   | a site registered on a `planned` task | `task X is planned but site "y" is registered` |
   | a registered site with no case | `TestTaskCensusBindsACaseToEverySite` |
   | a case that claims more than its kind allows | `…claims more than its kind's "…" — a case may only narrow` |
   | a shipped site with no corpus scenario | `shipped sites with no corpus scenario: […] — each is a prompt that ships uncertified` |
   | a `planned` task that carries a corpus scenario | `planned tasks carry corpus scenarios: […] — a task nobody built cannot be certified` |
   | a `planned` task that carries a certification record | `planned tasks carry certification records: […] — the record claims a band for a prompt that does not ship` |
   | a fixture that is not the shape its site takes | `TestEveryCorpusScenarioPreparesAgainstItsSite` |
   | a closed answer enum with a kind that no accepted corpus scenario names | `TestEveryClosedAnswerKindCarriesAScenario`. Each kind of the enum needs its own accepted scenario; one per site is not enough |
   | a task with no lane, or a lane no role wires | `TestEveryCensusedSiteRidesALaneAProcessRoleWires` |
   | a new `.go` file with no SPDX header | `TestEveryHandWrittenGoFileCarriesTheLicenseHeader` |

8. **Certify it** once the gates are green. This is the one step that costs money.
   It needs a provider key in your environment (`GEMINI_API_KEY`, `ANTHROPIC_API_KEY`, …).
   It also needs the two models the run binds.

   `MODEL=` is the model under test, and `JUDGE=` is the second model that grades it.
   The setup steps in [certify-an-ai-model.md](certify-an-ai-model.md#prerequisites) set both up.
   Read that first, or the run fails on a missing key.

   ```
   # real calls on your key; MODEL is required. JUDGE defaults to claude_cli:claude-sonnet-4-6
   make e2e-ai TASK=<your task> MODEL=gemini:gemini-3.1-flash-lite
   make e2e-ai-report                # free: band, scope, binding, counts, scenario coverage
   cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert
   ```

   That last line writes [reference/ai-certification.md](../reference/ai-certification.md) again, from the
   record the run wrote. It is free, and `make check` fails until the committed page matches.

   `TASK=` takes the name you declared in step 1. A name with no corpus scenarios behind it (a typo, or a task
   with no corpus) stops the run with `task "…" has no scenarios under …`. That happens before any provider
   request, so a wrong name costs you nothing.

   Commit the record under `internal/compose/aicert/records/<task>/`.

9. **Ship it** in one pull request.
   Put in the contract, generated files, site, census line, case, corpus scenario and record.
   The branch and gate rules are in [CONTRIBUTING.md](../../CONTRIBUTING.md).
   You may skip step 8.
   The report then shows `absent` for your site, and the lane that costs money never blocks a merge.

   Two gates about the activity rail stop you here if the task is new. A new site on a task that exists fails
   neither:

   - `TestEveryKindSomethingProducesIsOneTheContractCanExpress`: the router may name a task with a name that
     `AiActivityKind` does not carry. The wire cannot carry that kind, and the rail shows nothing for that
     AI work. The failure prints an `align:` line that names the file and what to add.
   - The frontend census: `ACTIVITY_LINE` in `frontend/src/app/ai-activity-lines.ts` is typed `Record`, not
     `Partial<Record>`, so a new kind fails the compile. Write its text in `en`, `de` and `vi` for all six states.
     Or state in code why it does not show.

   By default, do not show it. Show the kind only if a rep waits on the work. If no human can see it, say so
   in code. A system `principal` with no `on_behalf_of` covers the whole workspace with a NULL `actor_user_id`,
   and the feed filters on the id of the reader. See
   [explanation/ai-activity-rail.md](../explanation/ai-activity-rail.md).

   By default the router reports your task. Your task may own a lasting row with its own states. Then
   think about registering it as a **carrier** in `ai.railOwners` instead. Only a carrier can say `queued` or
   `running`, and declare the `lease` from which the rail works out `stalled`.

## Adding a decision site

A site may also carry a **decision form**. That is a typed question that a decision model answers before the
task's ladder. When the decision model is not sure, the site goes back to the LLM prompt
([how the lane works](../explanation/ai-runtime.md#the-decision-lane)). `site_triage` is the worked example:
`sitetriage_decision.go` beside `sitetriage.go`, and `certcase_sitetriage.go`.

1. **Declare it.** Add `decision: true` to the task in `ai-tasks.yaml` and run `make gen`.
   Only a `shipped`, `background` task may declare one.
   The try is one more network call, and no `interactive` time limit allows for it.
2. **Build the request** from the same inputs the LLM request builder reads.
   Put `choiceQuestion` under the `decisionQuestionKey`.
   Call `ai.Decide` where the site called `ai.Ask`, and pass the site name and a gate.
3. **Give the case the form.** The certification case builds `aitasks.DecisionCase`.
   `TestEveryDecisionTaskHasAnAdapterAndEveryAdapterIsDeclared` fails a declaration without one.
   It also fails one without a declaration.
4. **Certify it** ([certify-a-decision-site.md](certify-a-decision-site.md)).
   Until its row is in `decisioncert_gen.go`, the lane skips the site.
5. **Build the prompts page again**, which now shows the site's decision question:
   `cd backend && go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts`.

Write the question to the guide for decision models. Do not copy the prompt:

- [ ] One short question that someone who knows the field answers in a second. Write it in full in
      `instructions`, and name state fields by their path in code (`` `page.text` ``). Question ids never go to
      the model.
- [ ] The state is JSON with keys that say what they hold, and coded values written out in words. It carries the same inputs
      as the LLM request. A test beside the adapter holds that, as `TestTriageDecisionStateCarriesTheLLMInputs`
      does.
- [ ] The keys of the rules equal the answer enum of the LLM (a test holds that too). Each rule says what
      makes *that* label right, and the wrong label it must not pick. Add an `other` label if the list does not
      cover every case.
- [ ] No code block, no output shape, no language rules, no "state your confidence". The wire keeps the data
      and the question in separate parts, and works out the confidence itself.
- [ ] The gate reads the answer into the site's answer type that exists, at the site's own floor value.
      There is no second floor; below it, the ladder answers.
- [ ] Do not break one question into several small ones and join their confidence. We measured that here,
      and it is not safe.
- [ ] The text of a `local_only` task never reaches a decision model in the cloud. That is a rule of the router,
      so the site needs no field of its own for where data goes.

## Notes

- A record covers one (provider, model, `env`) binding. An edit to the prompt, the request builder, the grader
  or the score rule gives a new version and marks the record **stale**. Certify again; do not edit a record
  by hand.
- **Renaming** a task or site starts in the specification, like any other contract change. It then lands
  here in each place the old name was used. That is the copied declaration, the census line, and the case's
  `Site()`. It is also the corpus `site:` field, the record folder, and any entry keyed by the old name.
- **Removing a site** means you also delete its corpus scenarios and its record. A record left behind claims a
  band for a prompt that no longer ships.

## Check that `ai-probe` sees the new site

`make ai-probe ARGS=list` reads the census, so a newly registered site shows there with no change to
`ai-probe`. If it does not show, the site was not registered. `make ai-probe ARGS='scaffold <task>/<variant>'` then
confirms the corpus scenario turns into something you can run. See [debug-an-ai-task.md](debug-an-ai-task.md).
