# Settings

Open Settings from the **account menu**. Its seven groups and 32 pages follow
your permissions:

**The sidebar lists what you can change.** A page whose every control is closed
to you is not in it — prominence, not permission.

**The settings home lists everything you can open**, in two parts: *What you can
change*, which is the sidebar again, and *What you can look up*, which is the
rest. Search finds both, so a page missing from your sidebar is still yours to
read. If a page is genuinely not yours, opening its address says so plainly and
leaves the address alone, so you can quote it to whoever can grant it.

A page you can read and not change says so once, at the top.

## Whose settings am I changing?

Every page carries a badge beside its heading saying who a change there affects:

| Badge | What it means |
|---|---|
| **Only you** | Your own seat. Nobody else sees the difference. |
| **Company** | Everyone in this company. |
| **Installation** | Every company on this deployment. |
| **Mixed** | The page holds settings of more than one kind. Read the card. |

Three pages are **Mixed**, each for a named reason: *Connections* and *Capture
activity* sit among your own settings but each carries one card that binds
everybody, and *Integrations* holds an installation-wide provider setting beside
company-level wiring.

---

# Your own settings

## Account

Your account as one card: who you are, how you sign in, how you sign off, which
language the product speaks to you in (English, German and Vietnamese), and how
it looks. **Appearance** is here as well as in the account menu — one setting
with two doors, so changing it in either moves the other.

**Your email signature** lives here. It is appended below every message you send,
above the unsubscribe footer. Leave it empty to send unsigned.

One rule worth repeating: **the AI never writes a sign-off.** This is the one
that goes out.

## Writing voice

Your **Voice DNA**: "Your personal writing voice. It shapes drafts made for you,
stays private to you, and only learns from sources you add."

Three properties in one sentence. It affects your drafts. Nobody else sees it.
It learns only from what you give it.

Samples arrive as files (`.txt`, `.md`, `.pdf`, `.docx`, `.vtt`, `.srt`,
`.json`, several at once). A PDF or Word document is read to its text in the
browser before anything is sent; a scanned PDF with no text layer counts as
empty, and a password-protected one is named so you can paste its text instead.
The card says what teaches the voice — sent emails first, then proposals and
posts, then call transcripts — and what to leave out: somebody else's writing,
and AI drafts.

A file at least half attributed to named speakers is treated as a conversation:
the card asks "Which speaker is you?" and keeps only your turns. Below that
share it is prose and is taken whole, so an email opening a line with a heading
and a colon ("Frage: …") is not asked about.

A first build needs 800 words. The button reads "Build my Voice DNA" until a
version exists, and "Rebuild Voice DNA" after.

## Agents

Where you mint and revoke **passports** — the credentials that let an AI agent
work as you.

Every member gets this page, ungated. A passport is minted by a colleague for
their own use, so making it administrator-only would mean only administrators
could mint one.

You also see the governed tool list and connected agents here. Disconnecting an
agent ends the whole connection, not one credential: "the agent loses access on
its next call and cannot renew. Reconnecting means approving access again."

See [What the AI does](what-the-ai-does.md#passports-how-an-agent-is-connected).

## Connections

Your own mailbox and calendar connections, and your LinkedIn import. The
distinction from **Integrations** is deliberate: Connections is what *you*
connected, Integrations is what the *installation* is wired to.

Full detail in [Capture](capture.md#what-you-can-connect).

## Capture activity

Two things on one page: the senders you keep out, and what the last 24 hours of
your mail turned into.

**Keep out of capture.** Addresses and domains whose messages never enter the
CRM. Rules you set bind only your own mailboxes; the company's rules bind
everyone (and only an administrator may add or remove one of those). Takes
effect from the next message; what is already captured stays.

**Outcomes.** Five counters for the window — captured, dropped as internal, no
contact created, sent for a verdict, derivation failed. Click one to narrow the
list under it.

**Messages**, behind a disclosure, is the per-message log: which step a single
message stopped at and why. Open it when a message you expected did not show up.
Most installations record no sender and no subject for these rows — the page says
so once above them, and that is the default, not a misconfiguration.

---

# The rest of settings

Six groups beyond your own. Which pages you see, and whether you can change them
rather than only read them, follows your role — see the table at the end.

## Company

*Group: Company.* Sign-in methods and OAuth applications have their own
**Sign-in & apps** page in the same group.

**Installation settings** — the company's name, timezone and base currency.

**Currency rates** — "Exchange rates that convert foreign-currency amounts to
your base currency. New rates take effect today or later; past rates are never
changed." That last clause is the point: setting a rate today cannot rewrite what
last quarter reported.

**Company context** — what Margince knows about your own company, where it read
it from, and a place to tell it directly.

## Members, Teams, and Seats & license

*Group: People* — the one place in the product where that word still means the
humans who work here rather than the record type. One page each: the roster,
the team list, and the seat count with the licence beside it. **Roles &
permissions** is not built yet, and is deliberately absent from the navigation
rather than present and empty.

**Members** — "Everyone with a seat, and what each one may reach." Invite,
change role, deactivate, reactivate. The settings *entry* is administrators',
because it follows those verbs rather than the roster underneath — every seat
can still look a colleague up, since the share and assignee pickers read the
same roster beside the record where you need it.

**Teams** — "Who works together, which is what row scope reads." Create one,
archive it, and open a team to add or remove its members. Being in a team grants
no access on its own — with one exception: a **Team Lead** reads and works their
team's records, so for them a team is what "their team" means. For every other
shipped role a team matters when a record is shared with it. See
[Seats, roles and who can see what](seats-roles-and-access.md#row-scope-which-records-not-which-kinds).

**Seats & license** — the installation's entitlement against what is in use. See
[the licence](seats-roles-and-access.md#the-licence).

See [Seats, roles and who can see what](seats-roles-and-access.md).

## Integrations

*Group: Data.*

What the installation is wired to, as opposed to what one colleague connected.

- **Contact data** — a licensed provider of contact data, and its budget and
  refresh policy.
- **Webhooks** — "Outbound subscriptions that receive signed HTTP POSTs for
  chosen events." Deliveries can be inspected and replayed.

## Extensions

*Group: Governance.*

Every extension unit this installation was built with, and what each may reach.

A unit is software composed in at build time, not installed from inside the app,
so this page reports what is there rather than offering anything to add. Each
unit names what it is for, the version it declares, and the permission objects
it registered.

Those permission objects are why the page exists. A unit that owns records gates
them on names no seeded role has heard of, so until somebody grants a role read
on them, every seat opens the unit's screen and sees nothing. The switches here
are that grant. A unit that registers none — a jurisdiction pack, say — is
listed with nothing to grant, which is the correct and common answer.

Admin and Ops both reach it.

## Capture rules

*Group: Data.* The posture that decides what enters the CRM at all, in the order
the page puts it:

**Email sharing**, at the top — whether captured mail is shared with colleagues.
It is everybody's business rather than one seat's, which is why it lives here;
your own connections page states what it currently says and links here.

**Own email domains.** The domains that belong to your company. "When colleagues
write to each other, that message is not stored. Not even for you." Be careful:
mail skipped while a domain was registered is never offered again by any mailbox.

**Enrichment.** Whether captured companies are enriched automatically.

**Consumer mail domains.** Which domains count as personal mailboxes. "Mail from
a consumer mailbox still creates the contact — it just never creates a company."

**Refused domains.** Which domains this installation refuses a company, and what
decided each one — a model verdict, a heuristic, or a human. "Letting a domain
back in re-opens the company question rather than merely clearing a flag."

## The Sales group

*Group: Sales.* Nine pages, one per subject, where this was one page with a tab
strip. The shape a record takes: which fields it carries, which stages it moves
through, which vocabularies it picks from, and the priced things that go on an
offer.

- **Pipelines** — the stages a deal moves through, for the whole company. See
  [The pipeline](the-pipeline.md#pipelines-and-stages).
- **Stage automation** — "The record each stage transition has earned, before it
  is trusted to move deals by itself." See
  [The pipeline](the-pipeline.md#stage-automation-the-evidence-before-trusting-a-move).
- **Lead handling** — lead sources, disqualify reasons, and the first-response
  target, which is off by default.
- **Acquisition sources** — "The business channels a deal can be attributed to.
  Separate from lead sources, which record how a record reached Margince."
- **Outcome reviews** — "The questions a rep is asked when a deal is won or
  lost." One set for won, one for lost.
- **Responsibility roles** — "What a colleague or team can be accountable for on
  a company, deal or project. **A role grants no access to the record.**" You
  choose what each one applies to and who it can be assigned to when you create
  it. A role is retired through its switch rather than deleted, so an assignment
  that carried it stays readable.
- **Fields** — the custom columns this company keeps beyond the ones every
  installation has. See
  [Contacts, companies, leads, deals and projects](records.md#custom-fields).
- **Tags** — "the shared vocabulary — renaming one here renames it on every
  record."
- **Products & offers** — what this company sells, and the templates an offer
  starts from.

Every seat can READ all nine. Seven are an operator's to change, so for most
seats they appear under *What you can look up* on the settings home rather than
in the sidebar.

Two stay in a sales seat's sidebar: **Products & offers**, which a sales seat
authors, and **Outcome reviews**, where reading the questions is itself the
point of opening the page.


## Models & routing, Automations, AI usage, and Model calls

*Group: AI.* Four pages, split from one.

**Model routing** — which model serves each kind of work, presented by activity
rather than by tier. What you see is the current policy; **Model calls** is what
actually ran. Shared bindings sit under Advanced, and changing one can move
several activities at once. Prices name input and output cost per million tokens
separately, rather than one unexplained arrow, and a tier name proves nothing
about where data is processed or what it costs — read the binding.

Changes take effect without a restart: a running process picks up a saved
binding within about a minute, and a call already in flight keeps the one it
started with.

**Provider keys** — your own keys for whichever provider you use. Margince can
also run entirely against a local model with no cloud key at all.

**Automations** — the trigger-and-action catalogue.

**Monthly AI allowance** — Admin and Ops set it; Management can read it and not
change it. The default is **12 million tokens per active full user each month**,
pooled across the company. It is not an individual quota and not a spending cap
in money. You can override the calculation with a fixed company total, which
does not delete the per-user figure underneath. With no eligible users, the
calculation counts one.

The allowance resets at the start of each calendar month, UTC. Two thresholds
matter:

- **At 80%**, routing moves to lower tiers. The actual model can stay the same
  where two tiers share one binding.
- **At 100%**, background completions wait and interactive calls use the lowest
  tier. Search embeddings keep running, and still count.

Raising the allowance does not just unblock the future: saved website reads,
account scans and voice builds that were waiting become runnable on the next
pass, each carrying its original request, authority and attempt limits — which
is not a promise a provider will answer. The waiting counts cover exactly those
saved carriers, not every AI task. A requester whose access was revoked stays
parked until it is restored.

Preview an allowance or model change before saving. A save is refused if the
stored configuration has moved since you previewed it, so you never save against
an answer you did not see.

**AI usage**, **Model costs** and **AI calls** — what has been spent and on
what. Anything that failed outright stays visible in **Job health** instead.

## Knowledge

*Group: Data.* **Data import** is its own page in the same group.

**Document sets**: "Bodies of text this company can be asked questions of.
An answer comes only from what is filed here, and a question they do not cover is
refused rather than guessed at."

See [Documents and files](documents-and-files.md#document-sets--asking-your-documents-questions).

## Privacy & retention

*Group: Governance.* The **Audit log** is its own page now, beside it: the two
answer to different permissions, so a reader could hold one and not the other.

Five things, and they are the heart of the compliance story:

- **Consent purposes** — append-only. A purpose cannot be renamed or removed once
  created.
- **Retention** — how long each kind of record is kept, and what happens when its
  window runs out.
- **Restricted records** — what a statutory obligation is holding after an
  erasure.
- **Privacy inbox** — data-subject requests with their deadlines.
- **Audit log** — every action, attributed.

Retention and Restricted records are visible to Admin and Ops. The audit log and
the privacy inbox are **Admin only**, because they name every actor and
whoever asked.

Full detail in
[What is kept, what is destroyed](retention-exports-and-deletion.md).

## System health, Data import, and Reset

*Group: Governance and Data.* Three pages, split so that a reindex, a queue
reading and "empty the installation" are no longer three buttons on one screen.
**Reset** appears only where the deployment has armed it.

- **Import a file** — a CSV of companies, up to 10 MB.
- **Search index** — rebuilding the index behind search and the AI's retrieval.
- **Job health** — "What the queue is holding, and whose work failed." Admin
  and Ops.
- **Reset data** — returns an installation to its first-boot state. It only
  appears where the installation has deliberately armed the capability. Treat it
  as what it is.

---

## Which settings you get

Pages follow **permissions**, not role names. A custom role holding the right
permission reaches the page with no change to the product, and an Admin whose
role lost a permission stops reaching the page that needs it.

- **Can I open it?** Search finds it, the settings home lists it, and its address
  works.
- **Can I change it?** It is in the sidebar, and its controls are live.

A **read-only seat** is the clearest case of the two coming apart: it keeps every
page it could read and loses every control that writes. Your own preferences
still work — language and appearance are about your browser, not anyone's
records.

| You are | You can change | You can also look up |
|---|---|---|
| A sales seat | Your own five pages, Products & offers, Outcome reviews, Company profile, Capture rules | The rest of the Sales group, and Knowledge |
| A team lead | The same | The same |
| Management | The same | The above plus AI usage, model calls, seat counts and the sign-in status — all of them readable and none of them theirs to change |
| Ops | Most of the operational catalog, including the model rates | The rest, the extension inventory among them — Ops reads it and cannot change the grants |
| Admin | Everything the deployment has armed | — |

That first row surprises readers, so it is worth saying plainly: a sales seat can
edit **Capture rules** and the **company profile**, because those cards ask for a
permission every sales role holds. If that is not what you want, the fix is the
permission, not the page.

The company-profile half also depends on the deployment: on an installation
without the company-context capability, a sales seat has no writable card
there.

Nearly everything is a permission now, including the three that used to be role
checks: administering members answers to `user_admin`, the audit log to
`audit_log`, and the privacy queue to `privacy_request`. Only a handful of paths
still ask for the literal Admin role, and they are about recovering access
rather than using the product: the last-admin rule, and deployment-level resets.

## Two things administrators should decide early

**Your own email domains.** Get these right before you connect mailboxes. They
decide what counts as internal, and the decision is not fully reversible.

**Your retention posture.** Your company already has six retention rules
running. Read them before your first deletion happens rather than after.
