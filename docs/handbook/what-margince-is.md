# What Margince is

Margince is a CRM — a place to keep track of the people you sell to, the
companies they work for, the deals you are working on, and the work you deliver
afterwards.

It is different from most CRMs in three ways, and the differences are the reason
to use it.

## 1. Your AI agents work inside it, not beside it

Most CRMs bolt AI on as a panel that summarises what you typed in yourself.

In Margince, an AI agent connects to your organization properly and gets a set of
governed tools. It can look things up, draft replies, read a document for you,
enrich a company, move a deal along. Everything it does is recorded and
attributed.

The governing rule is one sentence:

> An agent can do what the person behind it could do unaided — and nothing more.
> It is checked against that person on every call.

An agent has no identity of its own. It acts on behalf of a person, using a
credential that person minted and can revoke. If you cannot see a record, neither
can your agent. If you are not allowed to do something, it cannot do it for you.

And there is a short list of things it can never do at all, however it is
configured — chief among them: **an agent may never approve an action, including
its own.**

[What the AI does, and what it does not →](what-the-ai-does.md)

## 2. It fills itself in

You should not have to type into a CRM what you already wrote in an email.

Connect your mailbox and your calendar, and Margince files what arrives against
the right people, companies, deals and projects — by exact rules, not by
guesswork. Where the rules cannot decide, it asks you rather than picking.

[How conversations get in →](capture.md)

## 3. It tells you the truth about itself

This is the least obvious property and, over time, the most useful one.

Margince is unusually careful to distinguish between "no" and "I do not know",
and it says which one it means:

- Asked a question its documents do not cover, it answers **"Not covered by this
  set"** rather than composing something plausible.
- A morning brief that found nothing worth your attention says so: "The overnight
  brief found nothing worth your first hour. **That is the answer, not an
  omission.**"
- A record with no edit history says: "Set on create and never changed — the
  audit log records no edits. **An empty history is honest, not a gap.**"
- A count taken from a partial read says it was partial, so you do not read a
  blank cell as a fact.
- Where a value has no known source, it says "source not recorded" instead of
  leaving it blank.

Every generated sentence carries the records it was written from. Every enriched
field carries the passage it was read from. Every number in a report reconciles
to rows you can open.

You are meant to be able to check it. That is the design.

## What it holds

Five kinds of record:

- **Contacts** — people
- **Companies** — the organizations they work for
- **Leads** — prospects, kept deliberately apart from contacts
- **Deals** — pieces of business moving toward won or lost
- **Projects** — the work itself, which starts during the deal and outlives it

Plus the timeline of emails, calls, meetings, notes and tasks that runs through
all of them, and the documents that accumulate along the way.

[What each one is, and how they connect →](records.md)

## Getting around

**Home** sits on its own at the top. Below it the navigation is grouped in three:

**Records** — Contacts, Companies, Leads, Filtered lists

**Work** — Worklist, Pipeline, Projects

**Intelligence** — Reports, Ask Margince

**Settings** is in the account menu, not on the navigation rail.

Two screens are worth knowing on day one.

**Worklist** is your day, and the only door to work that waits on a person:
decisions to answer, tasks to finish, duplicates to merge, today's meetings,
deals going quiet, promises you made, and what ran on its own overnight. When
there is nothing, it says "Your day is clear" rather than showing you an empty
grid.

**Ask Margince** is the question box, reachable from anywhere with a keyboard
shortcut.

You will notice no counters or badges on the navigation. That is deliberate: the
queues that would carry them are lanes inside Worklist, which reports its own
numbers on the page rather than nagging from the edge of the screen.

## Words this handbook uses

**Organization** — your tenant. The whole of your company's data in Margince. One
installation serves one organization.

**Installation** — the running deployment. Whoever operates it decides things
like upload limits and whether sends need confirming.

**Company** — a record type. The word "organization" is the underlying name for
it and shows through on a few screens, which is confusing; when a search result
group says "Organizations", it means companies.

**Passport** — the credential a person mints so an AI agent can act as them.

**Staged** — an action that has not happened, and is waiting for a person to
decide.

**Archived** — removed from the live lists, still there. Almost everything in
Margince is archived rather than deleted. What actually destroys data is a short,
explicit list.

## Where it runs

Margince can run as a hosted service, on your own servers, or entirely on one
machine — including the AI model — for teams whose data cannot leave the
building. The behaviour described in this handbook is the same in all three.

It is licensed under BUSL-1.1, and you get the source.

## Where to go next

If you are a new user, read [Contacts, companies, leads, deals and
projects](records.md), then [The pipeline](the-pipeline.md).

If you are setting the product up, read [Settings](settings.md) and
[Seats, roles and who can see what](seats-roles-and-access.md) first, then
[Capture](capture.md).

If you are responsible for compliance, go straight to
[What is kept, what is destroyed](retention-exports-and-deletion.md).
