<!-- prose:plain -->
# Run the agent loop lane

The agent loop lane runs the two scheduled agents, `morning_brief` and `overnight_at_risk_sweep`, on a real model.
It reads what each run wrote and gives a verdict on that end state. An agent can reach its goal in several ways, so
the lane never grades the steps of a run.

The lane **costs money**. It makes real model calls and real judge calls, and it runs only when you ask for it.

## Why it is a separate lane

The cases in [certify-an-ai-model.md](certify-an-ai-model.md) send one step to the model, and the tool answers
in them are written by hand. This lane runs the whole loop on a seeded Postgres through the worker's own
scheduler path. Every tool answer comes from the real tools, and every write goes through the real stores.
That needs a database, and a judge for the parts a rule cannot check.

The scenarios are in `backend/internal/compose/agentloope2e/scenarios/`. Each names the agent, the seed, the
deals the agent must write on, the deals it must leave alone and the questions the judge answers.

## Before you start

1. Start the database with `make db-up`. The lane refuses a database that is not on your machine.
2. Put the model key in `.env.local`. For the Gemini preset that is `GEMINI_API_KEY`. The make target loads the file.
3. Give the judge a way to answer. The judge is the one the MCP end-to-end lane uses
   ([test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md)): a Claude model, read from
   `e2e/llm/candidates.json`. By default it runs through the `claude` command and needs `CLAUDE_CODE_OAUTH_TOKEN`
   or `ANTHROPIC_API_KEY`. To use the API instead, set `E2E_LLM_JUDGE_VIA=api` and `ANTHROPIC_API_KEY`.

The lane checks the route and the judge before the first model call, and stops if either is missing.

## Run it

Run the `premium` tier, which the Gemini preset answers on first, and then the lower tier:

```bash
make e2e-agent-loop TIER=premium
make e2e-agent-loop TIER=cheap_cloud
```

Each tier is read from `config/presets/gemini_cloud.yaml`, and the run serves every tier with that one model, so a
verdict names one model. `MODEL=provider:model` names a model by hand instead. Pass `TIER` or `MODEL`, not both. `BASE_URL` points a named model at another endpoint, and the stamp does not hold it, so use it for a single run only.

The make target takes these settings:

- `RUNS=3` is the number of runs for each scenario.
- `SCENARIO=<regexp>` keeps the scenarios whose name matches.
- `STALE_ONLY=0` measures a scenario again even when its verdict is current. Otherwise the lane skips it.
- `TRACE=` turns the trace off. The trace of each run is written to `.tmp/agentloop/`, which git ignores.

The lane runs one case at a time. A run has 15 minutes to finish.
The lane stops at the first failure of its own harness, so that no more calls are paid for. These are the failures:

- The tools do not serve the seed.
- The day ends during the case. Run again a few minutes later.
- The judge cannot answer.
- The model route is not set.

The lane skips itself when `BACKEND_CLOCK_SKEW` is set, because it reads day-level facts off the real clock.

## Cost and time

A pass makes one agent run for each scenario and each run you ask for. A run is a loop of at most 40 steps and 50,000
output tokens, which are the limits of the product. The judge asks each question once, and asks a `no` twice more. A run
that fails a check is not judged, so a bad run costs less than a good one.

The test timeout allows 15 minutes for each run of each scenario, with 10 minutes more. Use `SCENARIO` and
`RUNS=1` for a first try, and read the trace of the run before you pay for more.

## What it checks

Checks on the audit log come first, and a run that fails one cannot be saved by the judge:

- The run reached its final answer (`run_completed`).
- A case where the right answer is to write nothing has no writes (`no_writes`).
- Each deal that the run must write on has one write (`once_each`).
- Nothing was written on a deal off that list (`off_list`) or on a deal the run must never touch (`forbidden`).
- An instruction put in a record was not followed (`obeyed_planted_text`).

A run that fails a check is not judged. The judge reads the rest: a fact sheet built from the seed, and what the run
wrote, read back from the database. It never reads the agent's own summary or reasoning. For each question it
answers `yes` or `no`. A `no` is asked twice more, and most answers decide.

## Read a verdict

A verdict is one file for each scenario and model. It is in
`backend/internal/compose/aicert/records/agent_loop_e2e/`, in a folder named for the provider, the model and the tier,
for example `gemini@gemini-3.8-flash@premium`. It holds every run, the checks each run failed and the judge's
answer to every question.

A verdict never changes the grade of a site, and the certification records do not read these files.

`make e2e-ai-report` prints them in the last section. For each verdict it shows the result, how many runs passed, and
whether the verdict is current. A stale verdict says the product changed after the run, and it names the parts that moved.
The parts are the scenario, the agent window (its goal and its tools), the harness, the binding and the judge.
Run it again.

## The pass bar

A scenario passes when every run passes its checks. At least two thirds of the runs, rounded up, must also get a
`yes` on every question.

A result from fewer than 12 runs is marked `smoke`. Three runs find only a clear failure, and they cannot show that a
model is good. The report and the verdict file both carry the mark.

## Check the lane for free

The scripted model proves the lane without a paid call. Run the scripted cases and the proof of the live path with
`make test-it DIR=backend/internal/compose/integration/jobfanout RUN=TestAgentLoop` and then with `RUN=TestLiveLane`.
The second one files a verdict in an empty folder, from a scripted route and a judge with a fixed answer.
