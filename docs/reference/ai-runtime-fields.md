<!-- prose:plain -->
# AI runtime fields

The fields of the AI task contract and of a certification scenario file. How the runtime uses them:
[ai-runtime.md](../explanation/ai-runtime.md).

## Every field in `ai-tasks.yaml`

At the top level:

| Field | Shape | Means |
|---|---|---|
| `tiers` | ordered list | the classes of model. **The order matters**: it sets the order of the `Tier` values and of the enum in the routing schema, the same byte for byte on every generator run. |
| `tasks` | map of name → task | the jobs. Names are `snake_case` in lower case, and map one to one onto the generated `ai.TaskX` Go value. |
| `embed` | `{tier, cost_unit}` | the embedding job. It is **not** a task, because its tier is not a chat tier, and it has no prompt, no text answer and no path that returns text. So it has no sites and no certification duty. |
| `degrade_to` | map tier → tier | where a tier falls when the budget guard moves it down. `local_small` maps to itself: the floor. |

Per task:

| Field | Values | Means |
|---|---|---|
| `ladder` | ordered tiers | the **order to fall back**. The Router starts at the first rung, and walks to the next on a provider error or a failed schema check. A ladder with one rung (`site_extract`: `[premium]`) has no rung to fall to. |
| `execution_mode` | `interactive` \| `background` | who is waiting: a human in the middle of work, or a worker job. |
| `on_budget_exhausted` | `degrade` \| `queue` | what a spent monthly budget does. **Closed pairing rule:** `interactive` always pairs with `degrade`, `background` with `queue`. `queue` returns a typed deferral to the task's own lasting carrier; the Router never makes a job that nobody owns. |
| `status` | `shipped` \| `planned` | whether the task exists in this build. `shipped` means every site must be registered, have a case, and be covered by a scenario. `planned` means it may have no site, no scenario and no record. This is what stops a task that was never built from looking certified. |
| `sites` | list | the named sites. A bare string is a site of kind `one_shot`; `{name: x, kind: y}` declares another kind. |
| `sites[].kind` | `one_shot` \| `multi_turn` \| `agent_loop` | how the model is called, and so how much of the site one certification run can cover. A closed set: a new kind is a change to code and tests, because each needs a certification plan that can run it. |
| `sites[].tools` | tool names, on an `agent_loop` site only | `agent_loop` is the engine, and each of its sites is one scheduled agent. This is the only set of tools that run is given. It must be there and not empty (the runner refuses a job with none). It is never the whole served catalog, which `TestEveryAgentSpecNamesRegisteredTools` in compose fails. |
| `no_payload` | `true` (or left out) | content from this task must **never** reach `ai_call_payload`, no matter what the capture setting of the deployment says. It is a parsed field, so a data control does not depend on text in a `doc:` string. |
| `company_context` | `none` \| `{scopes, token_budget, conditional}` | the bounded block of company profile that this task's prompts may hold. **Not optional**: a missing policy is a build error, never a runtime default. |
| `company_context.scopes` | any of `identity`, `positioning`, `sales`, `offer`, `market`, `proof`, `administrative` | which bounded views of the company profile may be put in. That order is also the wire and fingerprint order, so listing the same scopes in a new order cannot change the hash. |
| `company_context.token_budget` | positive `int` | the size the renderer bounds the block to. It must be there with any scope (at zero the scopes would reach no prompt). It is refused without a scope, since a budget on a policy that selects nothing reads as a deleted scope list. |
| `company_context.conditional` | `true` (or left out) | put it in only when the caller asks, not always. |
| `cost_unit` | rule name, or left out | which rule prices this task before it runs (`per_message`, `per_contact`; `per_entity` for embed). The math stays in code. Naming the rule here lets the build prove the mapping is **whole** in both directions. Left out means not priced. |
| `doc` | string | copied into the comment of the generated Go value. Text only: nothing may depend on it. |

## Every field in a scenario file

One YAML file per scenario under `internal/compose/aicert/corpus/<task>/<name>.yaml`, loaded by
`LoadCorpus`:

| Field | Needed | Means |
|---|---|---|
| `name` | yes | the scenario's own name: what a record row and a failure message call it. |
| `task` | yes | must name a task the contract holds. |
| `site` | yes | which registered site is under certification. The site, not the task, is the unit: one scenario can never stand for a task's other prompts. |
| `source` | yes | provenance. Must be `hand_authored`. An `extracted:` scenario is refused at once, because the review and redaction path for one is not wired. |
| `sanitized_by` | yes | who reviewed this scenario for private content. Not empty, and it names a reviewer, not a tool. |
| `fixture` | yes | **the data production is given**, never the prompt production sends. The site's own case turns it into the request. That is what makes the run measure the shipped builder, not a copy of it. |
| `expect.outcome` | yes | which of `accepted` / `wrong_answer` / `invalid` / `abstained` the site's validator must report. Nothing puts `accepted` above the rest, so a scenario whose right answer is *silence* can exist. |
| `expect.answer` | when the result states content | the answer itself, **in that site's own vocabulary**: a bare token, a list, a map, a `{min,max}` band. There is no common shape, because what tells a right answer from a wrong one differs per site. |
| `expect.rubric` | when quality is scored | what the grader is told to look at. It may only ask for what the site's reply envelope can hold. A rubric that scores a field the schema cannot hold measures nothing, and can only mark a correct reply down. |
| `expect.bands` | yes | `certified_min` / `degraded_min` / `floor`: the 0–100 bars this scenario's judge scores are measured against. That is the pooled margin above `certified_min` or `degraded_min`, and the block and floor on its own upper bound. Leaving the block out is refused, with no default, since a missing gate would pass everything. |
| `expect.caps` | optional | the run's limits on cost and time. Going past one fails like a failed check of the shape, never in silence. `max_tokens` budgets the model's **answer** alone. It does not count the fixed input the model cannot make smaller, or the thinking inside a reasoning model. So a scenario with a large input and a small output cap tests drafting within budget, not prompt size. `p95_latency_ms` judges **cloud candidates only**, since the speed of an engine on the same host is a fact about the hardware. Both are read off the run's **pooled** calls: a site that answers in three requests spent all three. |
