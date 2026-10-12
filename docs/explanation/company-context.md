<!-- prose:plain -->
# Company context: cold-start onboarding and governed AI grounding

One installation serves one company, and Margince keeps a confirmed, lasting picture of that company:
what it sells, who buys it, and what proves it. That picture starts in first run onboarding (from a website
read, a form filled by hand, or both). It sits as rows with provenance in the `contacts` module. It
reaches AI tasks as governed data with scopes, never as free prompt text. The model runtime it feeds is
in [ai-runtime.md](ai-runtime.md).

## The shape

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

## The four layers

There is no copied "AI profile" that can drift from the company data. The governed source of truth is
four separate parts, all with workspace scope and all with provenance:

1. **Identity**: the standard `company` columns and the domain of the company's site.
2. **Business profile**: statements that a human can confirm, of one value each. They sit in `company_profile_field` (offer summary, ICP, value proposition, USP, buyer roles, …). Each has evidence, a confidence, a source and a capture actor.
3. **Evidence facts**: findings that can show up more than once, each grounded in a source. They sit in `company_fact` (locations, services, products, certifications, named customers, technologies, …). Each links back to its `site_read`.
4. **Operational dossier**: the record of the read itself (`site_read`: progress, pages read or skipped, the reason it stopped). It is never prompt context on its own.

The standing rule across all of them: a human edit wins over a machine refresh. A website read run again
may *propose* a change; it never replaces a value a human holds without asking.

Over these rows, the contacts module offers one typed **`CompanyContext`** read model
(`GET /company/context`), which only reads. Its fields come in a fixed order. It has named scopes
(`identity`, `positioning`, `sales`, `offer`, `market`, `proof`, `administrative`), and a fixed fingerprint. Readers
get bounded views of this model; nothing builds its own company prompt from tables.

## The cold-start: five stops, one rail that tells the story

The first login lands in a work surface that fills the screen. One scene owns the screen at a time, and a
step rail beside it tells where the user is (`onboarding-conversation/`). That rail comes from a pure
reducer, and the reducer is the conversation, so the rail cannot disagree with it. The state is stored on
the server (`/onboarding/state`, `onboarding_wizard_state` in the identity module). So loading the page again, or an
OAuth round trip, builds the same scene again.

The rail of the user who set up the workspace reads **Read · Confirm · Basis · Voice · Connect**. Basis
is what the setup settles right after the company is confirmed, and before any step about the user. It is
the installation's base for reports: the base currency, and the `timezone` for reports. An admin can
change it later in Settings. A currency a deal has already frozen shows as locked there, as it does here.

Connect is the last stop. Leaving it writes that onboarding is complete, and shows the hand off. Leaving
the team act does the same for a creator who will not work in Margince.

A member invited later walks the personal stops. Their company and its basis are already settled: the
server refuses every new seat with 409 `company_not_described` until the company is saved. Their rail
reads Voice · Connect, and the plan that brings them back puts them in the voice act. The app's onboarding gate sends
here every human whose wizard state is missing or not finished. It does not send a read seat (it cannot
write the checkpoint the journey ends on). A member invited later sets up their voice and connects their
mailbox, as the creator did.

The server refuses a member checkpoint at any step only the creator takes (`read`, `confirm`, `basis`,
`invite`, `team`). It answers with the text `members begin at Voice`, which is now also the route.

The `step` enum on the wire (`read, confirm, basis, invite, team, voice, results, connect, complete`) is
the vocabulary of the checkpoint. The rail is a view over it that knows about side steps. So a question
to clear something up can take the whole screen without claiming a numbered slot.

- **Reading the website is optional.** `Enter it myself` needs three fields: the company name, *what do
  you sell?* (`offer_summary`) and *who do you sell it to?* (`icp`). It works with AI routing turned off
  and no egress. Legal, VAT and address fields depend on the case, and never block all users.
- **A website read is grounded and shows progress.** The onboarding dossier (`/company/site-reads`
  start → poll → confirm) is an *unbound* `site_read`, so it needs no company to exist first. It sends
  grounded findings as pages are read.
  - The standing read rules apply: the SSRF guard, the robots rules, hard limits on pages, bytes and
    time, and evidence or nothing. A value with no ground in the source stays empty, and is not made up.
- The default read covers up to **60 pages**, within the current byte and time limits. The lane for
  facts from the legal page keeps the heading breaks, and then builds the usual bounded blocks of text.
  It keeps short company blocks together. It does not make the evidence scope wider, or change the checks
  on where a fact is from.
- **A confirm takes part of the read.** It does so in one transaction. The confirm request binds the read
  version the user looked at. It writes only the selected fields and facts (audited, with outbox events),
  and treats any edited value as a statement by a human.
  - Nothing goes into domain tables before the confirm. Published team members are never company rows;
    each is staged as a separate lead proposal.

After first run, the **Company context** settings screen edits the same rows. "Refresh from website"
runs the same dossier pipeline. It sorts every proposed value as *new*, a *machine change*, or a *human
conflict*. Only the first two can be accepted all at once. A human conflict needs a clear decision: keep,
accept or edit.

## Injection: every task declares a policy

"Use it everywhere" means every AI task declares what it gets. No model call gets a general company
summary by default. The provider in the compose layer (`companycontextprompt.go`) holds a **closed
policy registry**. Each task in the [task contract](ai-runtime.md) declares one of two things.

- `none`, where context would push the task the wrong way: capture sorting, pulling facts from a
  website, embedding work.
- Named scopes with a token budget: the agent loop, reply drafting, offer drafting, the vocabulary of
  NL search.

A fitness test fails the build when a task declares nothing.

The guards:

- Context is rendered into a **`<company_context_data>`** block of user data, marked so it cannot
  break out of the block. It is data, never system orders. Text from a website is still not trusted,
  even after it is accepted.
- The selected scopes and the **context fingerprint** go into the `ai_call` trace and the answer cache
  key. So a profile edit changes both, and no stale cached answer lasts past it.
- The lane sits behind the ordered `company_context.rollout` switch in `margince.yaml`
  (`off < read < tasks < onboarding`, all the way on by default). Migration `0105` filled in installations
  that already existed from their anchor rows, without a read of the site and without touching provenance.

## Reference

| What | Where |
|---|---|
| Read model + policy | `GET /company`, `GET /company/context` (contacts module); scopes and fingerprint |
| Onboarding dossier | `/company/site-reads` start, poll, confirm; unbound `site_read` (contacts) |
| Wizard state | `/onboarding/state`, `onboarding_wizard_state` (identity; migration `0103`) |
| Provider + prompt block | `internal/compose/companycontextprompt.go` |
| The switch | `company_context.rollout` (`margince.yaml`, `platform/deployconfig`, migration `0105`) |
| Trace provenance | `ai_call` context columns (migration `0102`); see [ai-runtime.md](ai-runtime.md) |
| The UI | `frontend/src/screens/onboarding-conversation/` (the machine + acts), the scene modules `onboarding-gate.tsx` / `onboarding-backread.tsx`, and `company-context.tsx` (the settings screen). `onboarding.tsx` is the entry point and the shared vocabulary of the journey, not a screen |
| Deep read (companies that exist) | `POST /companies/{id}/deep-read`: the same engine, staged for approval |

**See also:**

- [ai-runtime.md](ai-runtime.md): the Router, tracing, the budget.
- [agent-surface.md](agent-surface.md): the agent loop that reads the short profile.
- [privacy-and-consent.md](privacy-and-consent.md): erasure and retention over the same rows.
