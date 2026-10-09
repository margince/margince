<!-- prose:plain -->
# The agent surface & the model runtime

How AI agents *act* inside Margince, and what *runs the model* behind them. The write path covers how
data changes; here we look at the other side, how a proposal becomes a checked action. The rules that
govern it (the autonomy tiers, passports, and the one admission gate) are in
[authorization.md](authorization.md). What follows is what the agent *does* and how the model runtime
is put together.

## Two surfaces, one gate

An agent reaches the tool surface in one of two ways, and both go through the same registry and the
same admission gate. No path skips the gate.

- **Surface A**: an *outside* agent over MCP (`/mcp`, Streamable HTTP on the api), acting under a passport.
- **Surface B**: *our own* runner, the reason, act, observe loop that runs on its own (for example the
  passes that run while the team sleeps).

Both call every action through `agents.Registry.Invoke`, which admits each call through `platform/auth`
(**scope ∧ seat ∧ tier**) before any handler runs. A 🟢 call runs and is audited; a 🟡 call stages an
approval that a human must confirm first. Both surfaces share the gate because both call the same
registry, so nothing else has to keep them in line.

Most verbs that change data run on their own (🟢). `enrich` stages for approval (🟡), because the model
names the URL the server fetches. The reason, and how an installation sets a verb back to 🟡, is in
[authorization.md](authorization.md#autonomy-tiers--how-agent-actions-are-governed--).

A passport is also a REST Bearer credential, and REST governs it as MCP does. On REST too, 🟢 changes
run on their own and 🟡 ones stage for approval. Both stay inside the current seat and RBAC of the human
who granted the passport. Every call checks the credential again, so a revoked passport stops on the next tool call,
not at the next login. What a passport holds: [authorization.md](authorization.md#what-a-passport-is).

**Files reach a record from Surface A only.** `attach_document`
carries the whole file in its input, and two places keep a call's input. An approval stores it in
`approval.proposed_change`, and a run stores each step in `agent_run.trace`. So the tool is 🟢 with
no staging path.

`TestNoAgentLoopAttachesAToolThatCarriesAFile` (`internal/compose/agentcatalog_test.go`) fails when a
scheduled agent's tool list names any tool whose input bound is over 1 MiB. An agent can attach a
file and list a record's files (`list_documents`); no tool can fetch a file or return its contents.
The size and type bounds are in [configuration.md](../reference/configuration.md#uploads).

## The reason-act-observe loop (Surface B)

The runner (`internal/modules/agents/runner/`) is where the model proposes and the tool surface decides.
Each turn of the loop:

1. **Limit checks first**: each run has three hard limits. They are clock time, a **step budget**
   (`MaxSteps`, default 40), and an **output token budget** (`MaxOutputTokens`, default 50 000).
   Reaching one ends the run with a lower status. So one run that no human watches cannot use the
   whole model budget of the workspace.
2. **Reason**: one model call (`brain.Complete`).
3. **Parse** the proposed step. The protocol needs one of `tool` or `final`, never both. Bad output
   tries again with a note on what was wrong. After 3 bad steps in a row, the run ends with a lower
   status. It returns the part it reached (`partial: true`, `steps_completed`).
4. **End**: a `final` step completes the run.
5. **Act**: `registry.Invoke(tool, args)` (the *only* path the runner has to an action).
6. **Observe** the tool's result:
   - a **🟡 refusal** *pauses* the run on the staged approval (`awaiting_approval`); it never blocks;
   - a **scope or budget refusal** goes back to the model as something it *observed*, so the model plans
     again inside its rights;
   - a **success** is observed and the loop goes on.

**Resume:** when a human approves, the run sends the *same* staged call again, with the approval id.
When the human rejects it, the run observes "plan again without it." **Grounding** content put into
the run is marked as data to read, never as orders to follow (a guard against prompt injection). The
runner reaches records only through the registry, so the gate decides read or write; the loop
itself does not.

## The model runtime

Behind the `ports/model` seam (`Client { Complete / Stream / Embed / Caps }`), config sets the model
choice. `internal/modules/ai/` owns it:

- **`SelectBrain(cfg)`** turns one binding (the `ai.routing` setting) into a `Client`: "offline fake ↔
  API key ↔ local, one line." Providers:
  - **`anthropic`**, **`openai`**, **`gemini`** (the cloud default we ship), and **`openai_compatible`**
    (any vendor that speaks the OpenAI wire shape, bound by `base_url`): cloud **BYOK**. You give the
    key; the product runs no model of its own.
  - **`ollama`** and **`vllm`**: adapters for a local machine you run
 (`LocalOnly`, which may serve the
    sovereign profile, where no data leaves the machine).
  - **`fake`**: a fixed offline client that every test drives. It records each outbound payload
    *after* stripping, so tests can check what would leave the process.
- **The Router**: tasks name *tiers*; each tier points to a bound client; the budget guard changes the
  route *before* the call; every call is metered. **Callers never pick a model.**
- **The `SecretStripper`** runs over *every* outbound payload and removes secrets for good: API keys,
  tokens, private keys, password values (→ `[SECRET-REMOVED:<kind>]`). It guards secrets only and does
  no PII filtering. Names, emails and phone numbers pass through. The location ladder and the erasure
  engines handle privacy, and the sovereign profile blocks all data from leaving.
  - It covers the text lane only. An attached file goes in the payload as `base64`, and the rules
    match the text of a secret. So the stripper does not find a credential *inside an attached file*.
    The same file sent as text is cleaned. Attaching a document sends its bytes as they are.
- **Metering & budget**: `ai_usage` adds up counters per workspace, day, task and tier, against a
  **monthly token budget for the workspace**. This is a different limit from the per-run step and
  output token limits above, which stop one run that goes too far.
  - At 80% use or more, the router moves a tier one step down. At 100% or more, **background** tasks
    wait with a typed `BudgetDeferralError` before any provider call or trace row. It wraps
    `ErrBudgetDeferred`, and it holds `NextAttemptAt` (the next budget window).
  - **Tasks a user waits on** move down to `local_small` and do not block the user in the middle of
    work. Only model calls sit behind this error; core CRM work never does.

## Automations & MCP transports (in brief)

- **Automations** (`/v1/automations`) set up the closed catalog of the workflow engine per workspace.
  Only humans may change them, and the store checks that again on the `automation` RBAC object. (The
  workflow engine itself is in [who reads the events](write-backbone.md#5-the-consumer-side--groups--dedupe).)
- **MCP** serves the tool surface at `/mcp` on the api: one registry, one admission gate, one audit
  stream, one transport.
  `tools/list` is filtered on **scope** per caller. So the list shows the passport's scopes,
  and does not promise that every listed tool will run. The seat limit and object RBAC are checked
  again at each call, so either one can still refuse a listed tool.
  The catalog itself: [reference/agent-tools.md](../reference/agent-tools.md).
  Connecting: [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md);
  making the passport: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).

## The Margince skill: the contract an AI tool reads

An AI tool that calls the REST API with a passport learns the API from the Margince skill. The API
serves it as a ZIP at `GET /v1/agent-bundle`; the steps are in
[how-to/mint-a-passport.md](../how-to/mint-a-passport.md#give-the-passport-to-an-ai-tool). Its
`openapi.yaml` and `INDEX.md` are cut from `backend/api/crm.yaml` by `backend/tools/gen-agentcontract`,
which `make gen` runs and `make drift` checks. The cut follows the gate, so the file lists only what a
passport can reach:

- An operation stays when its `x-agent-access` is neither `human-only` nor `auth-bootstrap`, and its
  security admits `bearerAuth` alone. Any other operation refuses every passport, so listing it would
  send an agent to a refusal. The gate lets an agent change data only through an operation with a
  tool policy, its `x-mcp-tool`. Every such operation outside those two classes has one, because
  `backend/tools/gen-agentpolicy` refuses to generate without it.
- Its security keeps only `bearerAuth`, and so does `components.securitySchemes`, because an agent
  that reads the skill holds a passport and nothing else.
- A description keeps only its text up to the first blank line. A sentence that reads as a note to
  the contract's developers is cut, and so is such a note inside `( )`. Such a note is a decision
  label, a spec path, an `x-*` key, SQL or a storage table name. An agent reads every description as
  a fact about the API, and those notes are about the build.
- What the `x-*` keys say about a call is written into its description in words. That is the
  passport permission it needs, whether it waits for approval, and whether it waits on a model. Then
  every `x-*` key is stripped.
- `servers` is left out. The API fills it in when it serves the ZIP, from the address in
  [configuration.md](../reference/configuration.md#public-base-url), so one build serves every install.

The two guides, `README.md` and `SKILL.md`, are templates in `backend/internal/compose/agentbundle/`.
`TestTheEmbeddedFilesCarryNoDeveloperNote` there fails when a developer note reaches the generated
contract or index.

## Open gaps

- **A passport's share of model tokens only warns.** The admission gate meters each passport's reads,
  writes, outbound data and calls (`platform/agentvolume`). Going past one of those stops the agent
  or asks the human who granted the passport. The cost counter, its share of the workspace AI budget,
  refuses nothing: past the share, the answer has a warning. So only the workspace budget limits what
  one passport spends on the model.

## Where to go next

- The gate, autonomy tiers, and what a passport is: [authorization.md](authorization.md).
- Where the resume trigger of the runner comes from (the `approval.decided` event):
  [write-backbone.md](write-backbone.md).
- What each module owns (`agents`, `ai`): [reference/modules.md](../reference/modules.md).
