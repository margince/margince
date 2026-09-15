# What the AI does, and what it does not

This page is what the AI actually produces for you, and how to read it. The
rules that bound it — the two tiers, what waits for a human, passports and
allowances — are on
[Agents, passports and what they may do](agents-and-passports.md).

## What the AI actually does for you

**Drafting.** It writes email drafts. It does not send them. Your email
signature is yours: the app notes that "the AI never writes a sign-off — this
is the one that goes out."

**Reading documents for deal fields.** You can ask it to read a file attached
to a deal. It comes back with the fields it can ground in that file — deal
name, amount, currency, expected close date — each shown with the passage it
read them from, and staged for you to accept. Nothing is written to the deal
until you press accept. If the file states none of those fields, it says so
rather than inventing them: "AI read this file and it states none of the deal
fields." A field it is unsure of is left out and marked "omitted (this file
says something, but not clearly enough to accept)".

Note that a document filed against **a deal** can be read for deal fields; one
filed against **the company** cannot.

**Enriching a record from the web.** Reading a company's website, matching a
LinkedIn connection, filling in a new account. Every one of these stages a card
rather than writing straight to the record.

**Answering questions about your documents.** See
[Documents and files](documents-and-files.md). The important property is that
it answers only from the set of documents you filed, and a question that set
does not cover is refused rather than guessed at.

This one is **yours to ask, not an agent's.** Asking a document set is refused
outright to an agent, however wide its passport — see "Things the AI is refused
outright" below. The reason is the same one that earns the text box in the first
place: a reader of a grounded answer can see which passage each sentence
rests on and go and check it, and an agent acting on that answer unattended
cannot.

**The overnight brief.** The product looks at your accounts overnight and
ranks what deserves your first hour. If it found nothing, it says so plainly:
"The overnight brief found nothing worth your first hour. That is the answer,
not an omission."

Where the overnight pass has something to say about a ranked deal, it writes it
onto the item itself, beside the rank — so the reason is in the same place as
the thing it is a reason for, rather than in a second list you have to hold
against the first. Every finding names a deal that is already in your queue; the
pass cannot add a deal to your brief by writing about it.

**A deal you dismissed, coming back.** Waving a deal away holds it out of every
later brief — it does not come back tomorrow because tomorrow is a new day. It
comes back only when something actually happens on it: a linked activity after
the moment you dismissed it. So a returning deal always carries the pair that
explains it — the day you dismissed it, and the activity that brought it back.
That is not the software guessing why it is showing you something twice. It is
the rule that put it back, stated: no activity, no return.

## Every derived claim carries its evidence

Nothing the AI writes on screen is presented as a bare fact. A generated value
carries the records it was written from, and you can open them. An enriched
field on a contact carries the verbatim snippet it was read from. A number in a
report carries the rows it reconciles to.

If you disagree with something the system derived, you can record that verdict.
Your correction is what the next re-derivation has to respect — it is not just a
dismissal that gets overwritten next time.

## Who did what: the trust marks

Everywhere a value can be attributed, the app says who put it there. The
labels are:

- "Typed by a person", "Typed by {name}", "typed by you", "typed by a buyer"
- "Automated by {agent}" — or "Automated by an agent" when the credential
  carries no readable name, because printing an opaque id at you would not help
- "System task {job}", or plain "System task" where the job has no readable
  name — the installation's own housekeeping: a scheduled sweep, a backfill. Deliberately named apart from an agent, because "a model decided
  this" and "the system did its housekeeping" are different answers to the
  question "who do I ask about this?"
- "via {connector}" — it arrived from a connected mailbox
- "source not recorded" — when honestly nothing is known

## Watching work in progress

While the AI is working for you, you can see it. Twelve kinds of work are
narrated: your morning brief, the overnight risk sweep, reading a document,
reading a company's site, summarising, scanning an account, drafting a reply,
drafting an offer, your weekly review, the learnings behind it, proposing next
steps from a transcript, and building your writing voice.

Each shows as queued, running, done, degraded, failed — or **stalled**.

There is a sixth state those five do not name: work can be **deferred** because
the company's monthly AI allowance is spent. That is not a failure and not a
stall — the request is kept whole, with its original authority and attempt
limits, and a sweep picks it up when the allowance is raised or the month rolls.
See [Settings](settings.md).

"Stalled" means the work has been running unusually long and may have stopped.
The app says exactly that: "Reading your document has taken unusually long. It
may have stopped."

Where there is a way out, the same line says it. An account whose read stalled
is read again when you open it again: the open starts a fresh attempt, and the
stalled one gives way to it. A document whose read stalled offers "Try reading
it again" where it showed "Reading this file…", and pressing it does the same.
A read still inside its time is joined rather than restarted, so opening a page
twice, or pressing twice, never reads anything twice.

It is worked out fresh each time you look, and never written down. That is
deliberate. Nothing has to remember to mark it, which is what stops a job that
died halfway from being shown as working forever.

A run that is waiting on a *human* is never called stalled. It is waiting, which
is a different thing, and it may wait as long as it needs to.

Two limits worth knowing. Work with nobody behind it — a nightly sweep — reaches
nobody's list, not by choice but because there is no one to show it to. And some
tasks only report when they are finished, so you may press "read this site", see
nothing for forty seconds, and then find it already done.

## Where to see what the AI is doing

The **Worklist** is the day's shape: what needs a decision, today's meetings,
deals going quiet, promises you made, what ran on its own overnight.

The **Home** brief ranks accounts and shows the factors behind each ranking —
winnability, revenue, timing, momentum, warmth — with the evidence rows behind
them.

The **AI** group in Settings holds four pages — Models & routing, Automations,
AI usage and Model calls — and **Settings → Agents** is where you mint your own
passports. **Settings → Audit log** holds the full audit trail: every action,
attributed to a human, an agent or a connector.
