# Company context: cold-start onboarding and governed AI grounding

One installation serves one company, and Margince keeps a confirmed, durable
understanding of that company: what it sells, to whom, and what proves it. That
understanding starts in first-run onboarding (from a website read, a manual
form, or both). It lives as provenance-bearing rows in the `contacts` module and
reaches AI tasks as governed, scoped data, never as ad-hoc prompt prose. The
model runtime it feeds is [ai-runtime.md](ai-runtime.md).

## The shape at a glance

```text
 FIRST RUN (the wizard)                 THE PROFILE (contacts module)       AI TASKS (compose + ai)
 ─────────────────────                 ───────────────────────────      ───────────────────────
 Read → Confirm → Basis → Voice         company        (identity)   CompanyContextProvider
      → Connect                         company_profile_field         task → scopes | none
   │                                    company_fact   (evidence)        │  (closed policy,
   ├─ "Read my website"                 site_read           (dossier)         │   fitness-gated)
   │    progressive, evidence-or-omit        ▲                                ▼
   │    confirm = accept-subset ─────────────┘                        <company_context_data>
   └─ "Enter it myself"                 human edits outrank                data block in the
        3 required fields                machine refreshes                 prompt → ai.Router
                                                                       (fingerprint → trace + cache)
```

## The four canonical layers

There is no denormalized "AI profile" that can drift from company data. The
governed source of truth is four distinct concepts, all workspace-scoped and
provenance-bearing:

1. **Identity**: canonical `company` columns and the primary domain.
2. **Business profile**: human-confirmable single-value statements
   (`company_profile_field`: offer summary, ICP, value proposition, USP,
   buyer roles, …) with evidence, confidence, source, and capture actor.
3. **Evidence facts**: repeatable, source-grounded findings
   (`company_fact`: locations, services, products, certifications, named
   customers, technologies, …), linked back to their `site_read`.
4. **Operational dossier**: the crawl record itself (`site_read`: progress,
   pages read/skipped, stop reason). Never prompt context by itself.

The standing rule across all of them: human edits outrank machine refreshes. A
website re-read may *propose* a change; it never replaces a human-held value
without asking.

Over these rows the contacts module exposes one typed, read-only
**`CompanyContext`** read model (`GET /company/context`), with deterministic
field ordering, named scopes (identity, positioning, sales, offer, market,
proof, administrative), and a deterministic fingerprint. Consumers get
bounded views of this model; nothing assembles its own company prompt from
tables.

## The cold start: five stops, one narrating rail

First login lands in a full-viewport work surface: one scene owns the board at a
time and a derived step rail narrates beside it
(`onboarding-conversation/`, whose pure reducer is the conversation, so the
rail cannot disagree with it). State persists server-side (`/onboarding/state`,
`onboarding_wizard_state` in the identity module), so a reload or an OAuth
round-trip reconstructs the same scene.

The creator's rail reads **Read · Confirm · Basis · Voice · Connect**. Basis is
what the setup settles right after the company is confirmed and before any step
about the user answering: the installation's reporting basis (base currency
and reporting timezone). An admin can change the reporting basis later in
Settings; a currency a deal has already frozen is shown locked there as it is
here. Connect is the last stop: leaving it writes completion and plays the
handoff, as leaving the team act does for a creator who will not work in Margince
themselves.

An invited member walks the personal stops. Their company and its basis are
already settled: the server refuses every new seat with 409
`company_not_described` until the company is saved. Their rail reads
Voice · Connect and the restore plan lands them in the voice act. The app's
onboarding gate sends every human whose wizard state is absent or unfinished
here, except a read seat (it cannot write the checkpoint the journey ends on).
A member invited later trains their voice and connects their mailbox as the
creator did.

The server refuses a member checkpoint at any creator-only step (`read`,
`confirm`, `basis`, `invite`, `team`) with the text "members begin at Voice",
which is now also the route.

The `step` enum on the wire (`read, confirm, basis, invite, team, voice, results,
connect, complete`) is the checkpoint vocabulary. The rail is a detour-aware
view over it, so a clarify question can take the whole screen without claiming
a numbered slot.

- **Website ingestion is optional.** "Enter it myself" needs three fields:
  company name, *what do you sell?* (`offer_summary`) and *who do you sell it
  to?* (`icp`). It works with AI routing disabled and zero egress. Legal/VAT/address fields are conditional,
  never a universal block.
- **A website read is progressive and grounded.** The onboarding dossier
  (`/company/site-reads` start → poll → confirm, an *unbound* `site_read` that
  needs no pre-existing company) streams grounded findings as pages are read.
  The standing crawl guarantees apply: SSRF guard, robots handling, hard
  page/byte/time bounds, and evidence-or-omit (an ungrounded value stays empty
  instead of guessed).
- The default crawl reads up to **60 pages**, within the existing byte and
  time limits. The legal page-facts lane preserves heading boundaries before
  packing the usual bounded passages, keeping short company blocks together
  without widening the evidence scope or changing the attribution checks.
- **Confirmation is accept-subset in one transaction.** The confirm request
  binds the inspected read version, writes only the selected fields/facts
  (audited, outbox-evented), and treats any edited value as a human assertion.
  Nothing persists to domain tables before confirmation. Published team
  members are never company rows; each is staged as a separate lead proposal.

After first run, the **Company context** settings screen edits the same
canonical rows. "Refresh from website" runs the same dossier pipeline and
classifies every proposed value as *new*, a *machine change*, or a *human
conflict*. Only the first two can be bulk-accepted; a human conflict needs an
explicit keep/accept/edit decision.

## Injection: every task declares a policy

"Use it everywhere" means every AI task declares what it gets; no model call
receives a generic company blurb by default. The compose-layer provider
(`companycontextprompt.go`) holds a **closed policy registry**. Each task in
the [task contract](ai-runtime.md) declares either `none` (context would bias
the task: capture classification, website extraction, embeddings) or named
scopes with a token budget (agent loop, reply drafting, offer drafting,
NL-search vocabulary). A fitness test fails the build when a task has no
explicit declaration.

The safety frame:

- Context is rendered into a delimiter-escaped **`<company_context_data>`
  user-data block**: data, never system instructions. Website-derived text
  stays untrusted even after acceptance.
- The selected scopes and the **context fingerprint** ride the `ai_call`
  trace and the response-cache key, so a profile edit changes both and no
  stale cached answer survives it.
- The lane sits behind the ordered `company_context.rollout` kill switch in
  `margince.yaml` (`off < read < tasks < onboarding`, default fully on);
  migration `0105` backfilled existing installations from their anchor rows
  without crawling or touching provenance.

## Reference

| Concern | Where |
|---|---|
| Read model + policy | `GET /company`, `GET /company/context` (contacts module); scopes/fingerprint |
| Onboarding dossier | `/company/site-reads` start/poll/confirm; unbound `site_read` (contacts) |
| Wizard state | `/onboarding/state`, `onboarding_wizard_state` (identity; migration `0103`) |
| Provider + prompt block | `internal/compose/companycontextprompt.go` |
| Rollout switch | `company_context.rollout` (`margince.yaml`, `platform/deployconfig`, migration `0105`) |
| Trace provenance | `ai_call` context columns (migration `0102`); see [ai-runtime.md](ai-runtime.md) |
| The UI | `frontend/src/screens/onboarding-conversation/` (the machine + acts), the scene modules `onboarding-gate.tsx` / `onboarding-backread.tsx`, and `company-context.tsx` (the settings screen). `onboarding.tsx` is the entry point and the journey's shared vocabulary, not a screen |
| Deep read (existing companies) | `POST /companies/{id}/deep-read`: same engine, approval-staged |

**Related:** [ai-runtime.md](ai-runtime.md) (the Router, tracing, budget) ·
[agent-surface.md](agent-surface.md) (the agent loop that consumes the compact
profile) · [privacy-and-consent.md](privacy-and-consent.md) (erasure/retention
over the same rows).
