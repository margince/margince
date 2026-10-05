# The agent surface & the model runtime

How AI agents *act* inside Margince, and what *runs the model* behind them. This is the read/react
counterpart to the write path: how a proposal becomes a governed action. The governance (the
autonomy tiers, passports, and the one admission gate) is explained in
[authorization.md](authorization.md). What follows is what the agent *does* and how the model runtime
is wired.

## Two surfaces, one gate

There are two ways an agent reaches the tool surface, and both go through the same governed
registry and the same admission gate. There is no privileged back door.

- **Surface A**: an *external* agent over MCP (`/mcp`, Streamable HTTP on the api), acting under a passport.
- **Surface B**: *our own* runner, the proactive reason-act-observe loop (e.g. the overnight passes).

Both call every action through `agents.Registry.Invoke`, which admits each call through `platform/auth`
(**scope ∧ seat ∧ tier**) before any handler runs. A 🟢 call executes and is audited; a 🟡 call stages a
confirm-first approval. Both surfaces share the gate because both call the same registry; no convention
is needed to keep them aligned.

Most mutating verbs auto-execute (🟢); `enrich` stages for approval (🟡) because the model names the URL
the server fetches. The reasoning, and how an installation floors a verb back to 🟡, is in
[authorization.md](authorization.md#autonomy-tiers--how-agent-actions-are-governed--).

A passport is also a REST Bearer credential, governed as it is over MCP: 🟢 mutations auto-execute, 🟡
ones stage for confirm-first approval, and both stay capped by the granting human's live seat and RBAC.
Every call re-authenticates, so a revoked passport stops working on the next tool call rather than at
the next login. What a passport carries: [authorization.md](authorization.md#what-a-passport-is).

## The reason-act-observe loop (Surface B)

The runner (`internal/modules/agents/runner/`) is where the model proposes and the governed tool
surface decides. Each iteration:

1. **Guarantee checks first**: three hard per-run ceilings, namely wall-clock, a **step budget**
   (`MaxSteps`, default 40), and an **output-token budget** (`MaxOutputTokens`, default 50 000).
   Hitting one ends the run with a degraded status, so one unattended run cannot use the whole
   workspace's model budget.
2. **Reason**: one model call (`brain.Complete`).
3. **Parse** the proposed step. The protocol requires one of `tool` or `final`, never both; malformed
   output retries with feedback, and after 3 consecutive invalid steps the run ends with a degraded
   status instead of returning a partial result.
4. **Terminal**: a `final` step completes the run.
5. **Act**: `registry.Invoke(tool, args)` (the runner's *only* path to an action).
6. **Observe** the tool's result:
   - a **🟡 refusal** *suspends* the run on the staged approval (`awaiting_approval`); it never blocks;
   - a **scope/budget refusal** is fed back as an *observation*, so the model re-plans within its
     authority;
   - **success** is observed and the loop continues.

**Resume:** when a human approves, the run re-submits the *identical* staged call carrying the approval
id. When rejected, it observes "re-plan without it." **Grounding** content seeded into the run is
spotlighted as data to read, never as instructions (a prompt-injection guard). The runner reaches
records only through the registry, so the gate governs read versus write; the loop itself does not.

## The model runtime

Behind the `ports/model` seam (`Client { Complete / Stream / Embed / Caps }`), model choice is set by
config. `internal/modules/ai/` owns it:

- **`SelectBrain(cfg)`** turns one binding (the `ai.routing` setting) into a `Client`: "offline fake ↔
  API key ↔ local, one line." Providers:
  - **`anthropic`**, **`openai`**, **`gemini`** (the shipped cloud default), and **`openai_compatible`**
    (any vendor speaking the OpenAI wire shape, `base_url`-bound): cloud **BYOK**. You supply the
    key; the product runs no inference of its own.
  - **`ollama`** and **`vllm`**: local / self-host adapters (`LocalOnly`, eligible for the zero-egress
    sovereign profile).
  - **`fake`**: a fully deterministic offline client that every test drives (records each outbound
    payload *after* stripping, so tests assert what would have left the process).
- **The Router**: tasks name *tiers*; tiers resolve to bound clients; the budget guardrail bends the
  route *before* the call; every call is metered. **Callers never pick a model.**
- **The `SecretStripper`** runs over *every* outbound payload and irreversibly removes secrets: API
  keys, tokens, private keys, password assignments (→ `[SECRET-REMOVED:<kind>]`). It is hygiene and
  does no PII filtering. Names, emails, and phone numbers pass through; privacy is handled by the
  location ladder and the erasure engines. The sovereign profile blocks egress entirely.
  It covers the text lane only. An attachment rides the payload base64-encoded, and the rules match a
  secret's literal text, so a credential *inside an attached file* is not found. The same file arriving
  as text is scrubbed. Attaching a document sends its bytes as they are.
- **Metering & budget**: `ai_usage` accumulates per-(workspace, day, task, tier) counters against a
  **workspace monthly token budget** (distinct from the per-run step/output-token ceilings above,
  which stop a single runaway run). At ≥80% utilization the router soft-degrades a tier. At ≥100%,
  **background** tasks are deferred with a typed `BudgetDeferralError` before any provider attempt or
  trace row; it unwraps to `ErrBudgetDeferred` and carries `NextAttemptAt` (the next budget window).
  **Interactive** tasks degrade to `local_small` instead of blocking a user mid-flow. Only model calls
  sit behind this error; core CRM never does.

## Automations & MCP transports (in brief)

- **Automations** (`/v1/automations`) parameterize the workflow engine's closed catalog per workspace;
  mutations are human-only, re-gated at the store on the `automation` RBAC object. (The workflow engine
  itself is covered in [write-backbone.md → who consumes the events](write-backbone.md#5-the-consumer-side--groups--dedupe).)
- **MCP** serves the tool surface at `/mcp` on the api: one registry, one admission gate, one audit
  stream, one transport.
  `tools/list` is filtered on the **scope axis** per caller, so the list reflects the passport's
  scopes and does not promise that every listed tool will run. The seat ceiling and object RBAC are
  re-derived at each invoke, so a listed tool can still be refused by those.
  The catalog itself: [reference/agent-tools.md](../reference/agent-tools.md).
  Connecting: [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md);
  minting the passport: [how-to/mint-a-passport.md](../how-to/mint-a-passport.md).

## Known gaps

- **The per-agent volume budget is not enforced.** The admission gate binds scope ∧ seat ∧ tier
  today; a per-agent budget ceiling is designed but not wired.

## Where to go next

- The gate, autonomy tiers, and what a passport is: [authorization.md](authorization.md).
- Where the runner's resume trigger comes from (the `approval.decided` event):
  [write-backbone.md](write-backbone.md).
- What each module owns (`agents`, `ai`): [reference/modules.md](../reference/modules.md).
