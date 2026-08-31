# What the AI does, and what it does not

Margince is built to be worked in by AI agents as well as by people. That is
the point of the product, and it is also the part most worth understanding
before you trust it.

There is one rule underneath everything on this page:

> **An agent can do what the person behind it could do unaided — and nothing
> more. It is checked against that person on every single call.**

Everything else follows from that sentence. It is worth understanding properly,
because it is not the rule most people assume.

## The two tiers

Every action an agent can take carries one of two labels, declared once in the
product's contract and enforced the same way whether the agent arrives over MCP
or over plain HTTP.

**auto-execute.** The action happens immediately, with the agent stamped on it
in the audit trail. This is the default, and it covers most of the surface:
looking records up, searching, summarising, drafting, ordinary field updates,
logging an activity, promoting a lead, archiving a record — and sending an email
or a message.

**confirm-first.** The action does **not** happen. The agent's intention is
written down as a card in the approval inbox, and a person decides.

### Why sending is not automatically confirm-first

This surprises people, so here is the product's own reasoning.

A passport carries the granting human's own seat, permissions and record
visibility. So a send an agent can make is one its holder could already make
unaided, sitting in the app. Requiring that same person to confirm it again made
the agent surface *weaker* than the person behind it, not safer — it added a
click without adding a check.

What is kept behind a confirmation is narrower and more specific: **the calls
whose destination the credential holder did not choose.**

### What actually waits for a human

**Enriching from the web, and reading a company's site.** The standing case. Here
the *model* names the address the server fetches. Persuading the model could
reach an address nobody holding the credential ever picked. That is a question
about where data goes, not about who is allowed to do what — so it waits.

**Creating or changing a custom field.** This changes the shape of your data for
everyone.

**Creating or changing a webhook subscription.** This decides where your events
are sent.

**Closing or reopening a deal.** Advancing a deal is normally immediate, but a
move with a won or lost stage at *either* end waits. Winning is irreversible and
touches money; reopening takes revenue back out of a quarter that has already
been reported. If the stage's meaning cannot be read with certainty, it waits —
the doubt falls toward the approval, not past it.

**Filing an activity under a project.** Relinking is normally immediate, but
filing under a project marks the message as commercial correspondence, which is
write-once and cannot be undone by relinking away. See
[Capture](capture.md#filing-under-a-project-is-permanent).

**Any field a person last wrote.** See the section below — this one catches more
in practice than all the others together.

### Your installation can be stricter

An installation that wants every send confirmed can set a floor on the send
action, and it then stages into the inbox exactly as anything else does. That is
an operator decision made in the deployment, not a switch in Settings.

If you need sends confirmed in your organization, ask whoever runs your
installation whether that floor is set. Do not assume it.

> **A note on wording.** Some screens in the app still describe an older, stricter
> rule — "Write & send wait for you", "we never send anything without your
> approval". The contract the server actually enforces is the one described
> above. Where a screen and this page disagree, the behaviour above is what
> happens.

See [Approvals](approvals.md) for what happens to a card once it is staged.

## An agent never has more rights than the person behind it

An agent does not have an identity of its own. It acts **on behalf of** a
person, using a credential that person minted, and it is checked against that
person's permissions on every call.

Concretely:

- If you cannot see a record, the agent working for you cannot see it either.
  It does not get a "permission denied" that reveals the record exists — it
  gets the same nothing you would get.
- If you are not allowed to do something, the agent cannot do it by asking
  nicely. Approving a staged action demands exactly the permissions the action
  itself demands.
- The agent's credential also carries its own narrower limits (see
  **Passports** below). Having your permissions is the ceiling, not the floor.

## Things the AI is refused outright

These are not "staged for approval". They are refused.

**An agent may never approve an action.** Not another agent's, and above all
not its own. The product states this as: a credential does not release the
proposal it made. Without that rule an agent could stage a confirm-first
action, approve its own card, and the confirmation would have confirmed
nothing. An agent *may* reject its own proposal — an agent that changes its
mind is allowed to take its own request off your desk.

**An agent may never write consent or data-subject decisions.** Granting or
withdrawing consent, and fulfilling or rejecting a privacy request, are for
people only.

**An agent may never ask a document set.** Asking a set, and defining or
changing one, are people's work. A grounded answer is only as good as the
reader's ability to open the passage under each sentence and disagree with it,
and an agent acting on that answer unattended is precisely the reader who
cannot. This is why the question box exists for a NAMED set of documents and
not over everything.

**An agent may never change pipeline or stage configuration.** The stage ladder
is the ground truth that the "advance a deal" approval is judged against. An
agent that could edit the ladder could change the meaning of the approval it
was asking for.

For these operations an agent's credential is not even an accepted way to sign
in. The refusal is at the door, not in a check further inside.

**A new capability is refused by default.** If someone adds an action to the
product and forgets to give it a tier, agents cannot use it at all. It fails
closed. This is deliberate: the failure mode of forgetting is "the agent can't
do it", never "the agent can do it unsupervised".

## What actually protects a send

Since sending is not held behind a confirmation by default, it is worth knowing
what *does* stand in the way. Four things, and none of them is a click.

**Consent, default-deny, per purpose.** A send is refused unless an active,
proven consent grant exists for the *purpose* that send falls under. A grant for
a different purpose does not authorise it. "Unknown" blocks. "Withdrawn" blocks.
Where a purpose requires double opt-in, a granted-but-unconfirmed record does not
send. Every recipient is checked, including anyone copied in.

**Your seat, re-read at the moment of transmission.** Not at the moment of
staging. Someone demoted to a read-only seat between writing a message and its
going out is refused, whatever staged it. The message parks rather than retrying,
because a demotion is an answer, not a hiccup.

**The passport's scope.** Sending spends a `send` scope. A passport that was
never granted that scope cannot send at all, regardless of tier. Scopes are
exact: holding `write` does not imply holding `send`.

**A hard ceiling on outward calls.** Each passport gets a fixed allowance of
calls that leave the building per 24 hours. Unlike the read and write
allowances — which a person can widen by approving a card — **this one no
approval lifts.** It ends when the window ends.

## Human edits win, field by field

This is a subtle rule and worth reading twice, because it is what stops an
agent from quietly undoing your work.

When an agent updates a record, the product looks at each field it is trying to
change and asks: **did a person last type this value?**

- Fields no person has touched are updated straight away.
- Fields whose current value a person last wrote are split off and staged for
  approval — those fields only.

So a mixed update partly applies and partly waits. The record comes back
updated, together with a note naming exactly which fields were withheld and
which approval card they went to. If *every* field in the update was one a
person had written, nothing applies and the whole thing waits.

When you approve that card, only the withheld fields are written. The agent's
original wider request is not replayed.

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
place: a person reading a grounded answer can see which passage each sentence
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

- "typed by you", "typed by a person", "typed by a buyer"
- "Automated by {agent}" — or "Automated by an agent" when the credential
  carries no readable name, because printing an opaque id at you would not help
- "System task {job}" — the installation's own housekeeping: a scheduled sweep,
  a backfill. Deliberately named apart from an agent, because "a model decided
  this" and "the system did its housekeeping" are different answers to the
  question "who do I ask about this?"
- "via {connector}" — it arrived from a connected mailbox
- "source not recorded" — when honestly nothing is known

## Passports: how an agent is connected

A **passport** is the credential that binds one agent to one person. You mint
it yourself in Settings, and you can revoke it yourself. Revoking is the kill
switch for that one binding.

A passport carries **scopes** — narrower rights than the person has. There are
five, and they are exact rather than nested: holding one never implies another.

- **read** — reads only. The only scope a read-only seat may spend at all.
- **draft** — proposes text. Note this is not read-only: one drafting action
  saves a draft on the deal's timeline.
- **write** — every change that stays inside your organization.
- **send** — the three actions that put something on the wire.
- **enrich** — the one action that fetches from a third party.

A passport with only `read` can read your approval inbox but is refused the
decision. A passport needs `write` to approve most things, and `send` on top of
that where approving puts a message on the wire.

A passport lasts **30 days by default**, at least an hour and at most 90 days.
The token is shown **once** and never again — the app says so: "Copy it now —
you'll only see this token once." Only the hash is stored, so nobody, including
an administrator, can recover it for you.

Revoking takes effect at the agent's next call. So does demoting the person
behind it: authority is re-derived every time, so a change binds mid-session
rather than at the next login.

### Letting an agent work overnight on your behalf

A scheduled agent runs while nobody is at a keyboard, so it cannot borrow your
authority from a session you are not in. It has to be given, in advance, by you:
Settings lists every scheduled agent this installation runs, and you answer for
each one — granted, declined, or not yet asked.

Granting **mints your own passport** in the same act. That is the whole point:
the overnight run carries a credential that is yours, bound to you as both the
person acted for and the person who granted it, so everything it does is limited
to what you could have done yourself and is attributed to you. Withdrawing the
grant revokes that credential rather than merely unlinking it — the authority
actually ends.

You may see a grant that says you agreed and is still not working. That is
honest rather than broken: a passport expires on its own schedule, and nothing
writes to your answer when it does. The screen tells you the credential behind
the grant is no longer live, and the remedy is to grant it again, which mints a
fresh one.

Nobody can do this on your behalf, and no agent can do it for itself. An agent
that could grant itself standing authority would be deciding its own rights.

### Volume allowances

Each passport gets a fixed allowance per 24-hour window: records read, changes
made, outward calls, and total calls.

Two of these behave differently when they run out. Reading and writing are
**step-ups** — the agent is refused, and a card goes to the person who
approved the connection, whose approval widens the window by one allowance.
Outward calls and total calls are **hard stops**: no approval lifts them, and
only the window ending clears them.

The window is fixed, not rolling, so every allowance in an installation resets at
the same moment. That is why a refusal says "when the window rolls" rather than
naming a number of hours.

Nobody but the person who approved the connection can answer a step-up. Not an
administrator, not the owner of the organization. An agent's ceiling is that
person's own authority.

The app's own summary: "Mint a passport in Settings and point any MCP-capable
agent at your organization. It reads only what you can see."

When an outside application asks to connect, you get a consent screen that
says plainly "{client} will be able to act in Margince as you, with the access
checked below", names the host the authorization is sent back to, and shows
`read draft write send enrich` as checkboxes, all ticked by default, for you
to untick before approving. You can deny it. There is nothing to set up
first: approving the screen is what creates the connection.

## One honest gap: attachments

Everything an agent sends outward is scrubbed for secrets first — API keys,
tokens, private keys, passwords are removed and replaced with a marker.

This is hygiene, not a privacy filter. **Names, email addresses and phone
numbers pass through.** And the scrub matches text, while an attached file rides
along encoded, so a credential *inside an attached document* is not something it
can find.

Attaching a document is a decision to send its bytes as they are. Treat it that
way.

## Watching work in progress

While the AI is working for you, you can see it. Six kinds of work are narrated:
your morning brief, the overnight risk sweep, reading a document, summarising,
drafting a reply, and drafting an offer.

Each shows as queued, running, done, degraded, failed — or **stalled**.

"Stalled" means the work has been running unusually long and may have stopped.
The app says exactly that: "Reading your document has taken unusually long. It
may have stopped."

It is worked out fresh each time you look, and never written down. That is
deliberate. Nothing has to remember to mark it, which is what stops a job that
died halfway from being shown as working forever.

A run that is waiting on a *person* is never called stalled. It is waiting, which
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

**Settings → AI** and **Settings → Agents** are where the models and the agent
registry are configured. **Settings → Privacy & audit** holds the full audit
trail: every action, attributed to a human, an agent or a connector.
