<!-- prose:plain -->
# Add an MCP test case and run it on Claude and Codex

Add a case to the test that `make e2e-llm` runs, and see it pass on Claude and on
Codex. It costs money. How the test runs, what it costs and how to read a failure are in
[test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md).

A case is one YAML file under `e2e/llm/scenarios/`. Copy the closest case and
change it. `case65-clear-out-the-dead-deals.yaml` is a short one that holds a
refusal: the model must ask before it archives.

1. **Write the prompt** the way a user would say it. Name no tool. Use the
   records that `e2e/llm/seed-llm-fixtures.sh` already writes where you can. Every
   other case sees a new fixture, and it can change what they count.
2. **Hold what the model calls.** `must_call` lists the tools it needs; `a|b`
   in one entry means any of them. `must_call_with` pins one argument
   (`tool.argument=value`). `must_not_call_with` fails the run on a call whose
   arguments match `tool~regex`, so `archive_record~.` fails on any archive.
   `may_call` lists the other tools a good run may use, for the reader. The
   checker does not fail a call outside it.
3. **Hold what the model says.** `must_mention` and `must_not_mention` are
   regexes, for names, dates and counts. Put a fact that needs a judge in
   `judge:` as one sentence that ends with when to answer no.
4. **Hold the world it leaves.** `must_end_with` reads the records back after
   the run ([test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md#the-end-state)). Set `writes: true` on a case whose tools can write,
   even a case that passes only when nothing is written.
5. **Name the criteria.** `criteria: [1]` needs an entry under the case's
   `name` in `e2e/llm/criteria.yaml`, with a `name` and a one-sentence
   `statement`.
6. **Check it for free**, then the Go gates that read every case:

   ```bash
   make test-e2e-llm-check
   cd backend && go test ./internal/compose/ -run 'TestEveryWritingScenarioDeclaresThatItWrites|TestTheLaneCanReachEveryToolACaseRequires|TestTheMCPToolCoverageIsPublished'
   ```

7. **Run it on Claude, then on Codex.** Both use the CLI route. Each needs
   its own login: a Claude credential (`CLAUDE_CODE_OAUTH_TOKEN`) and
   `codex login`. `E2E_LLM_ALL_RUNS=1` makes all three runs, so you read every
   transcript and not only the ones before the verdict was settled.

   ```bash
   for candidate in claude gpt; do
     MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=$candidate E2E_LLM_VIA=cli E2E_LLM_ALL_RUNS=1 \
       E2E_LLM_STACK_PRESET=config/presets/openrouter_cloud_eu.yaml \
       SCENARIO=case65_clear_out_the_dead_deals make e2e-llm
   done
   ```

   The verdicts go to `claude-sonnet-5-5@claude-cli/` and
   `gpt-5.6-sol@codex-cli/` under `backend/internal/compose/aicert/records/mcp_e2e/`.

8. **Watch it fail once.** A case can pass without testing what it names. Undo
   the product change the case is for. Run it on one candidate and confirm it
   goes red for the reason you wrote. Then put the change back.
9. **Publish** the verdicts and the page as [test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md#8-publish-the-result) says.
