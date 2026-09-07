# Settings

Settings is reached from the **account menu**, not from the main navigation.

It has **seven groups** and 28 pages. Which of them you see depends on what your
role lets you do, and the rule has two halves worth knowing, because they answer
two different questions.

**The sidebar lists what you can change.** A page whose every control is closed
to you is not in it. That is about prominence, not permission — it keeps the list
you navigate past every day down to the work you can actually do.

**The settings home lists everything you can open**, in two parts: *What you can
change*, which is the sidebar again, and *What you can look up*, which is the
rest. Search finds both. So a page missing from your sidebar is still yours to
read: open the settings home, or search for it, or follow a link straight to it.

If a page is genuinely not yours, opening its address says so plainly and leaves
the address alone, so you can quote it to whoever can grant it.

A page you can read and not change says so once, at the top, instead of leaving
you to work it out from a screenful of greyed-out buttons.

## Whose settings am I changing?

Every page carries a badge beside its heading saying who a change there affects:

| Badge | What it means |
|---|---|
| **Only you** | Your own seat. Nobody else sees the difference. |
| **Your team** | The people on your team. |
| **Company** | Everyone in this organization. |
| **Installation** | Every organization on this deployment. |
| **Mixed** | The page holds settings of more than one kind. Read the card. |

Three pages are **Mixed**, and each for a named reason: *Connections* and
*Capture activity* sit among your own settings but each carries one card that
binds everybody, and *Integrations* holds an installation-wide provider setting
beside workspace-level wiring.

---

# Your own settings

## Account

Your account as one card: who you are, and the answers that belong to you alone
— how you sign in, how you sign off, which language the product speaks to you in
(English, German and Vietnamese), and how it looks. **Appearance** is here as
well as in the account menu; they are one setting with two doors, so changing it
in either place moves the other.

**Your email signature** lives here. It is appended below every message you send,
above the unsubscribe footer. Leave it empty to send unsigned.

One rule worth repeating: **the AI never writes a sign-off.** This is the one
that goes out.

## Writing voice

Your **Voice DNA**: "Your personal writing voice. It shapes drafts made for you,
stays private to you, and only learns from sources you add."

Three properties in one sentence. It affects your drafts. Nobody else sees it. It
learns only from what you give it.

Samples arrive as files: drop them on the zone or click it to choose (`.txt`,
`.md`, `.pdf`, `.docx`, `.vtt`, `.srt`, `.json`, several at once). A PDF or
Word document is read to its text in the browser before anything is sent; a
scanned PDF with no text layer counts as empty, and one that is
password-protected is named so you can paste its text instead. The card says
beside the zone what teaches the voice (sent emails first, then proposals and posts, then call
transcripts) and what to leave out (other people's writing, AI drafts). A file
whose words are at least half attributed to named speakers is a conversation:
the card asks "Which speaker is you?" and keeps only that speaker's turns. Below
that share the file is prose and is taken whole, so an email that opens a line
with a heading and a colon ("Frage: …") is not asked about.
A first build needs 800 words; the button reads "Build my Voice DNA" until a
version exists and "Rebuild Voice DNA" after.

## Agents

Where you mint and revoke **passports** — the credentials that let an AI agent
work as you.

Every member gets this page, ungated. A passport is minted by a person for
their own use, so making it administrator-only would mean only administrators
could mint one.

You also see the governed tool list and connected agents here. Disconnecting an
agent ends the whole connection, not one credential: "the agent loses access on
its next call and cannot renew. Reconnecting means approving access again."

See [What the AI does](what-the-ai-does.md#passports-how-an-agent-is-connected).

## Connections

Your own mailbox and calendar connections, and your LinkedIn import.

The distinction between this page and **Integrations** is real and deliberate:
Connections is what *you* connected; Integrations is what the *installation* is
wired to.

Full detail in [Capture](capture.md#what-you-can-connect).

## Capture activity

Two things on one page: the senders you keep out, and what the last 24 hours of
your mail turned into.

**Keep out of capture.** Addresses and domains whose messages never enter the
CRM. Rules you set bind only your own mailboxes; the organization's rules bind
everyone (and only an administrator may add or remove one of those). Takes
effect from the next message; what is already captured stays.

**Outcomes.** Five counters for the window — captured, dropped as internal, no
contact created, sent for a verdict, derivation failed. Click one to narrow the
list under it.

**Messages**, behind a disclosure, is the per-message log: which step a single
message stopped at and why. Open it when a message you expected did not show up.
Most installations record no sender and no subject for these rows, which the
page says once above them; that is the default and not a misconfiguration.

---

# The rest of settings

Six groups beyond your own. Which pages you see, and whether you can change them
rather than only read them, follows your role — see the table at the end.

## Company

*Group: Company.* Sign-in methods and OAuth applications moved to their own
**Sign-in & apps** page in the same group.

**Installation settings** — the organization's name, timezone and base currency.

**Currency rates** — "Exchange rates that convert foreign-currency amounts to
your base currency. New rates take effect today or later; past rates are never
changed." That last clause is the point: setting a rate today cannot rewrite what
last quarter reported.

**Company context** — what Margince knows about your own company, and where it
read it from. You can also tell it directly.

## Members, Teams, and Seats & license

*Group: People.* One page each now — the roster, the team list, and the seat
count with the licence beside it. **Roles & permissions** is not built yet, and
is deliberately absent from the navigation rather than present and empty.

**Users** — "Everyone who holds a seat here, deactivated accounts included."
Invite, change role, deactivate, reactivate. Reading the roster is open to every
user; managing it is administrators only.

**Teams** — named groups you can share records with. Create one, archive it, and
open a team to add or remove the users in it. Being in a team grants no access on
its own — with one exception: a **Team Lead** reads and works their team's
records, so for them a team is what "their team" means. For every other shipped
role a team matters when a record is shared with it.

See [Seats, roles and who can see what](seats-roles-and-access.md).

## Integrations

*Group: Data.*

What the installation is wired to, as opposed to what one person connected.

- **Contact data** — a licensed person-data provider, and its budget and refresh
  policy.
- **Webhooks** — "Outbound subscriptions that receive signed HTTP POSTs for
  chosen events." Deliveries can be inspected and replayed.
- **HubSpot mirror** — connecting an existing HubSpot portal in read-and-sync
  mode, and the one-way switch to running natively.

## Extensions

*Group: Governance.*

Every extension unit this installation was built with, and what each may reach.

A unit is software composed into the installation at build time, not something
installed from inside the app — so this page reports what is there rather than
offering anything to add or remove. Each unit says what it is for, the version it
declares, and the permission objects it registered.

Those permission objects are the reason the page exists. A unit that owns records
gates them on names no seeded role has ever heard of, so until somebody grants a
role read on them, every seat opens the unit's screen and sees nothing. The
switches here are that grant.

A unit that registers no permission objects — a jurisdiction pack, for instance,
which only supplies retention policy the core consults — is listed with nothing to
grant, which is the correct and common answer.

Admin and Ops both reach it: the page asks for the extension inventory read and
the role-directory read, and Ops holds both.

## Capture rules

*Group: Data.* The **Email sharing** rule now lives here, at the top: it decides
whether captured mail is shared with colleagues, which is everybody's business
rather than one seat's. Your own connections page states what it currently says
and links here to change it.

The posture that decides what enters the CRM at all. In the order the page puts
them:

**Own email domains.** The domains that belong to your company. "When colleagues
write to each other, that message is not stored. Not even for you."

Be careful: mail skipped while a domain was registered is never offered again by
any mailbox.

**Enrichment.** Whether captured companies are enriched automatically.

**Consumer mail domains.** Which domains count as personal mailboxes. "Mail from
a consumer mailbox still creates the person — it just never creates a company."

**Refused domains.** Which domains this installation refuses a company, and what
decided each one — a model verdict, a heuristic, or a person. "Letting a domain
back in re-opens the company question rather than merely clearing a flag."

## Pipelines, Lead handling, Fields, Tags, and Products & offers

*Group: Sales.* Five pages now, one per subject, where this was one page with a
tab strip.

Every seat can READ all five. Four of them — pipelines, lead handling, fields and
tags — are an operator's to change, so for most people they appear under *What
you can look up* on the settings home rather than in the sidebar. **Products &
offers** is the exception: a sales seat authors both, so it stays in their
sidebar.

The shape a record takes: which fields it carries, which stages it moves through,
and the priced things that go on an offer.

- **Custom fields** — extra fields on your records.
- **Pipelines** — see [The pipeline](the-pipeline.md#pipelines-and-stages).
- **Lead sources** and **Lead disqualify reasons** — the lists your team picks
  from.
- **Lead handling** — including the first-response target, which is off by
  default.
- **Products** and **Offer templates**.

## Models & routing, Automations, AI usage, and Model calls

*Group: AI.* Four pages, split from one.

**Model routing** — which model serves each tier. "Changes take effect without a
restart, and every process picks them up within a minute."

**Provider keys** — your own keys for whichever provider you use. Margince can
also run entirely against a local model with no cloud key at all.

**Automations** — the trigger-and-action catalogue.

**AI usage**, **Model costs** and **AI calls** — what has been spent and on what.

## Knowledge

*Group: Data.* **Data import** is its own page in the same group.

**Document sets**: "Bodies of text this organization can be asked questions of.
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
the privacy inbox are **Admin only**, because they name every actor and every
person who asked.

Full detail in
[What is kept, what is destroyed](retention-exports-and-deletion.md).

## Seats & license

*Group: People.*

**License and seats.** How many seats are in use, how many are granted, and
whether the licence is present and valid.

See [Seats, roles and who can see what](seats-roles-and-access.md#the-licence).

## System health, Data import, and Reset

*Group: Governance and Data.* Three pages, split so that a reindex, a queue
reading and "empty the installation" are no longer three buttons on one screen.
**Reset** appears only where the deployment has armed it.

- **Import a file** — a CSV of companies, up to 10 MB.
- **Search index** — rebuilding the index behind search and the AI's retrieval.
- **Job health** — "What the background system is holding, and whose work
  failed." Administrator only.
- **Reset data** — returns an installation to its first-boot state. It only
  appears where the installation has deliberately armed the capability. Treat it
  as what it is.

---

## Which settings you get

Pages follow **permissions**, not role names. A custom role holding the right
permission reaches the page with no change to the product, and an Admin whose
role lost a permission stops reaching the page that needs it.

Two questions, and they have different answers:

- **Can I open it?** Search finds it, the settings home lists it, and its address
  works.
- **Can I change it?** It is in the sidebar, and its controls are live.

A **read-only seat** is the clearest case of the two coming apart: it keeps every
page it could read before and loses every control that writes to the server.
Your own preferences still work — the language and the appearance are settings
about your browser, not about anyone's records.

| You are | You can change | You can also look up |
|---|---|---|
| A sales seat | Your own five pages, Products & offers, Company profile, Capture rules | Pipelines, Lead handling, Fields, Tags, Knowledge |
| A team lead | The same | The same |
| Management | The same | The above plus AI usage, model calls, seat counts and the sign-in status — all of them readable and none of them theirs to change |
| Ops | Most of the operational catalog, including the model rates and the extension inventory | The rest |
| Admin | Everything the deployment has armed | — |

That first row surprises people, so it is worth saying plainly: a sales seat can
edit **Capture rules** and the **company profile**, because those cards ask for a
permission every sales role holds. If that is not what you want, the fix is the
permission, not the page.

Nearly everything is a permission now, including the three that used to be role
checks: administering members answers to `user_admin`, reading the audit log to
`audit_log`, and the privacy queue to `privacy_request`. A custom role granted
one of those reaches the page, and an Admin whose role lost it does not.

A handful of paths still ask for the literal Admin role, and they are the ones
about recovering access rather than using the product: the last-admin rule, and
the deployment-level resets.

## Two things administrators should decide early

**Your own email domains.** Get these right before you connect mailboxes. They
decide what counts as internal, and the decision is not fully reversible.

**Your retention posture.** Your organization already has six retention rules
running. Read them before your first deletion happens rather than after.
