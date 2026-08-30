# Settings

Settings is reached from the **account menu**, not from the main navigation.

It has two groups. **You** — five pages every member gets. **Admin settings** —
ten pages that appear only for an Admin or an Ops seat.

If you are not an operator, the admin group is simply absent. It does not show
you a row of locked cards, because a page announcing that it exists and is not
yours is the one thing an absent section is chosen to avoid saying.

---

# Your own settings

## Account

Your account as one card: who you are, and the three answers that belong to you —
how you sign in, how you sign off, and which language the product speaks to you
in. English, German and Vietnamese are available.

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
`.md`, `.vtt`, `.srt`, `.json`, several at once). The card says beside the zone
what teaches the voice (sent emails first, then proposals and posts, then call
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

Every member gets this page, ungated. A passport is lent by a person, so making
it administrator-only would mean only administrators could lend one.

You also see the governed tool list and connected agents here. Disconnecting an
agent ends the whole connection, not one credential: "the agent loses access on
its next call and cannot renew. Reconnecting means lending a passport again."

See [What the AI does](what-the-ai-does.md#passports-how-an-agent-is-connected).

## Connections

Your own mailbox and calendar connections, and your LinkedIn import.

The distinction between this page and **Integrations** is real and deliberate:
Connections is what *you* connected; Integrations is what the *installation* is
wired to.

Full detail in [Capture](capture.md#what-you-can-connect).

## Capture activity

"What happened to your messages in the last 24 hours." Your own by default, with
a switch for shared channels.

Use this when a message you expected did not show up. It tells you which step it
stopped at and why, in plain words.

---

# Administrator settings

## General

**Installation settings** — the organization's name, timezone and base currency.

**Currency rates** — "Exchange rates that convert foreign-currency amounts to
your base currency. New rates take effect today or later; past rates are never
changed." That last clause is the point: setting a rate today cannot rewrite what
last quarter reported.

**Company context** — what Margince knows about your own company, and where it
read it from. You can also tell it directly.

## Users & teams

Two cards.

**Users** — "Everyone who holds a seat here, deactivated accounts included."
Invite, change role, deactivate, reactivate. Reading the roster is open to every
user; managing it is administrators only.

**Teams** — named groups you can share records with. Create one, archive it, and
open a team to add or remove the users in it. Being in a team grants no access on
its own: no shipped role is team-scoped, so a team matters when a record is
shared with it.

See [Seats, roles and who can see what](seats-roles-and-access.md).

## Integrations

What the installation is wired to, as opposed to what one person connected.

- **Contact data** — a licensed person-data provider, and its budget and refresh
  policy.
- **Webhooks** — "Outbound subscriptions that receive signed HTTP POSTs for
  chosen events." Deliveries can be inspected and replayed.
- **HubSpot mirror** — connecting an existing HubSpot portal in read-and-sync
  mode, and the one-way switch to running natively.

## Extensions

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

Admin only, and the admin role specifically: Ops administers the rest of this
half of settings and not this page.

## Capture

The posture that decides what enters the CRM at all. In the order the page puts
them:

**Own email domains.** The domains that belong to your company. "When colleagues
write to each other, that message is not stored. Not even for you."

Be careful: mail skipped while a domain was registered is never offered again by
any mailbox.

**Enrichment.** Whether captured companies are enriched automatically.

**Consumer mail domains.** Which domains count as personal mailboxes. "Mail from
a consumer mailbox still creates the person — it just never creates a company."

**Keep out of capture.** Addresses and domains whose messages never enter the
CRM. Rules you set bind only your own mailboxes; the organization's rules bind
everyone. Takes effect from the next message; what is already captured stays.

**Refused domains.** Which domains this installation refuses a company, and what
decided each one — a model verdict, a heuristic, or a person. "Letting a domain
back in re-opens the company question rather than merely clearing a flag."

## Data model

The shape a record takes: which fields it carries, which stages it moves through,
and the priced things that go on an offer.

- **Custom fields** — extra fields on your records.
- **Pipelines** — see [The pipeline](the-pipeline.md#pipelines-and-stages).
- **Lead sources** and **Lead disqualify reasons** — the lists your team picks
  from.
- **Lead handling** — including the first-response target, which is off by
  default.
- **Products** and **Offer templates**.

## AI

**Model routing** — which model serves each tier. "Changes take effect without a
restart, and every process picks them up within a minute."

**Provider keys** — your own keys for whichever provider you use. Margince can
also run entirely against a local model with no cloud key at all.

**Automations** — the trigger-and-action catalogue.

**AI usage**, **Model costs** and **AI calls** — what has been spent and on what.

## Knowledge

**Document sets**: "Bodies of text this organization can be asked questions of.
An answer comes only from what is filed here, and a question they do not cover is
refused rather than guessed at."

See [Documents and files](documents-and-files.md#document-sets--asking-your-documents-questions).

## Privacy & audit

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

## License

**License and seats.** How many seats are in use, how many are granted, and
whether the licence is present and valid.

See [Seats, roles and who can see what](seats-roles-and-access.md#the-licence).

## Maintenance

- **Import a file** — a CSV of companies, up to 10 MB.
- **Search index** — rebuilding the index behind search and the AI's retrieval.
- **Job health** — "What the background system is holding, and whose work
  failed." Administrator only.
- **Reset data** — returns an installation to its first-boot state. It only
  appears where the installation has deliberately armed the capability. Treat it
  as what it is.

---

## Which settings need which role

| Page | Who |
|---|---|
| Account, Writing voice, Agents, Connections, Capture activity | every user |
| General, Users & teams, Integrations, Capture, Data model, AI, Knowledge, License, Maintenance | Admin or Ops |
| Privacy & audit | Admin or Ops; the audit log and privacy inbox inside it are Admin only |
| Extensions | Admin only — Ops does not reach it |
| Job health, Reset data, user administration | Admin only |

A few things sit outside the role system entirely and need the Admin role, full
stop: user administration, privacy erasure, and reading the audit log.

## Two things administrators should decide early

**Your own email domains.** Get these right before you connect mailboxes. They
decide what counts as internal, and the decision is not fully reversible.

**Your retention posture.** Your organization already has six retention rules
running. Read them before your first deletion happens rather than after.
