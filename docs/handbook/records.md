# Contacts, companies, leads, deals and projects

Margince holds five kinds of record. This page explains what each one is, and —
more usefully — how they connect, and which connections are required.

## The five, in one line each

**Contact** — a person. In the app's navigation this is **Contacts**; on a few
screens the underlying word "person" shows through.

**Company** — an organization you deal with. Navigation calls this
**Companies**.

**Lead** — a prospect you have not qualified yet. Deliberately kept apart from
your contacts.

**Deal** — a specific piece of business, moving through stages toward won or
lost.

**Project** — the body of work a client relationship is actually made of. It
starts while a deal is still open and outlives the close.

Around these sit **activities** — the timeline of emails, calls, meetings, notes
and tasks — and **documents**, covered on their own pages.

## What connects to what

This is the part worth reading carefully, because Margince is looser here than
most CRMs, on purpose.

| Link | Required? |
|---|---|
| Contact → Company | **Optional.** It lives on an employment relationship, not a field. |
| Lead → Company | **There is none.** A lead carries a company *name* as free text. |
| Deal → Pipeline and stage | **Required.** |
| Deal → Company | **Optional.** |
| Deal → Contact | **Optional.** People sit on a deal as stakeholders. |
| Deal → Project | Optional, at most one, and both must name the same company. |
| Project → Company | **Required — at least one, always.** |
| Activity → any record | Optional. An activity with no links is shared with everyone. |

Two of these catch people out.

**A deal does not need a company.** Deal name, currency and stage are required.
Value, company and expected close date are optional. Qualifying a lead creates a
deal with no company attached at all, because a lead has no company to attach.

**A lead has no company record behind it.** That free-text name is not a
pointer. This is the whole reason leads are segregated: bulk-sourced prospects do
not get to create half-formed companies in your CRM.

## Contacts

A contact needs a name and a source. Everything else is optional.

Emails are typed **work, personal or other**. Phones are **work, mobile, home or
other**.

A contact's job title appears in two places and they are not the same thing. The
title on the contact record is a convenience copy; the authoritative one lives on
the employment relationship with the company.

A contact can have exactly one current primary employer, or none.

Other things a contact carries: an owner, a consent record per purpose, a
relationship strength, when they were last active, and — if they came from a
lead — a pointer back to that lead.

## Companies

A company needs a display name and a source.

Two separate fields describe a company, and mixing them up is a common mistake.

**Lifecycle** — where the account stands with you. One value only:
**unknown, target, prospect, opportunity, customer, former customer,
disqualified.** New companies start at unknown.

**Relationship types** — what the company *is* to you. Several at once:
**customer, partner, supplier, investor, portfolio company, competitor, other.**

So a company can be a customer *and* a supplier. It cannot be both a prospect and
a customer, because that is one question with one answer.

Size bands are: 1-10, 11-50, 51-200, 201-500, 501-1000, 1001-5000, 5000+.

A company may have a parent company, one level only — no chains, no cycles. The
website address is derived from the primary domain rather than typed.

One company in your installation is marked as the **anchor** — that is your own
company, the one Margince knows things about on your behalf.

## Leads

> "Leads are kept apart from Contacts. A lead becomes a contact only when you
> qualify it."

A lead needs only a source. Email and LinkedIn address are the fields used to
spot duplicates; a second live lead with the same email is refused.

### The ladder

Five statuses. The first three are open, the last two are terminal.

| What the screen says | What it means |
|---|---|
| **New** | Nothing has happened yet |
| **Contacted** | We reached out |
| **Engaged** | They answered, or a meeting is booked or held |
| **Qualified** | A contact now exists for this person |
| **Disqualified** | Closed, with a reason |

Contacted and engaged are set automatically from captured activity, and can also
be set by hand. The record tells you which: "set automatically from captured
activity" or "set by hand".

A person may move an open lead to any open step, forwards or back. The system
only ever moves it forwards.

### Qualifying a lead

The button says **Qualify**. A lead needs an email address and an open status
before it can be qualified.

You must say what triggered it — an inbound reply, a meeting booked, a meeting
held, or your own judgement. **Cold outbound with no reply is refused.**
Qualification is engagement, not import and not an outbound touch.

Then one of two things happens, in a single step:

- If the lead's email matches an existing contact, it **merges** into that
  contact. No duplicate is created.
- Otherwise a new contact is created.

Either way the lead's history and activities come with it — nothing is orphaned —
and the lead is marked qualified and archived. You can preview which of the two
will happen before you confirm.

One caution on that preview: if the matching contact is one you are not allowed
to see, the preview still says "merge" but does not show you the person. **An
absent contact never means "no match".**

You can open a deal in the same step. The deal takes the lead's owner and is left
with no company.

### Reversing it

A qualification can be reversed, with a reason that is recorded.

- If it **created** a contact: the contact is archived and the lead comes back to
  the queue. Activities captured since the qualification stay on the contact's
  timeline.
- If it **merged** into an existing contact: that contact is untouched. Only the
  lineage pointers are cleared. A field-level un-merge is never attempted,
  because it is lossy and ambiguous.
- **If the contact owns a deal, it cannot be reversed at all.** The product
  blocks rather than orphaning the deal.

### Disqualifying

Disqualifying archives the lead, records a reason from an administrator-managed
list, and an optional note. The record stays fetchable. It becomes read-only:
"This lead is closed and takes no changes."

There is no reverse-disqualify action.

### Scoring

Leads carry a score from 0 to 100. You can override it by hand, but a reason is
required — an override with no reason is refused, and the override stops the
score being recomputed until you clear it.

The score decays: the screen shows it as a base "halving every 14 days".

There is an optional first-response target, **off by default**, set between 15
minutes and 7 days. When it is on, leads show as **On time**, **Due soon** or
**Overdue**.

## Projects

> "A project is the body of work a deal is about. It starts during the deal, in
> the initiative phase, and outlives close-won: once the deal is won, delivery is
> tracked here."

A project is **not** a folder you create when you win. It is born while you are
still selling.

### Phases

Four, fixed: **Initiative → Pursuing → Delivering → Closed.**

Movement is allowed in either direction — a closed project can be reopened — and
only closing needs a reason. You cannot set the phase by editing the record; you
move it.

**A won deal moves its project to Delivering by itself** — but only from
Initiative or Pursuing. A project already delivering stays put, and a closed
project is never silently reopened by a win. Reopening is a human move with a
reason.

Every phase move is recorded, including the automatic one, which is attributed to
the person whose action caused it.

### The key

Every project gets a short key, minted by the server. You cannot choose it.

It comes from the project's name — initials for a multi-word name, the first
eight letters for a single-word one — plus the lowest free number. So "Nordwind
ERP rollout" becomes `NER-1`, and a second "ERP rollout Acme" becomes `ERA-2`.

The key is what files email automatically. Any subject carrying it **in square
brackets** goes under that project. See [Capture](capture.md).

Keys are unique among live projects only. Archiving a project frees its key.

### Companies and stakeholders

A project always has at least one company. Trying to remove the last one is
refused: "A project keeps at least one company; add another before taking this
one off." A company with deals still on the project cannot be removed either.

Companies on a project are offered three roles — **Customer, Partner,
Subcontractor** — but these are descriptive labels, not an enforced list. Two
companies can both be Customer, or none can be.

Stakeholders on a project use five roles: **Sponsor, Project lead, Delivery lead,
Subject-matter expert, User.**

One thing that trips people up: **you seat someone on a project from the
contact's page, not the project's.** The project's Stakeholders card shows the
result and is read-only.

### Who may do what

| Role | Create | Read | Edit and move phase | Archive |
|---|---|---|---|---|
| User | yes | yes | yes | **no** |
| Team Lead | yes | yes | yes | yes |
| Management | yes | yes | yes | yes |
| Admin / Ops | yes | yes | yes | yes |
| Read-only | no | yes | no | no |

Archiving is final from the app: "Archiving removes this project from the live
list and frees its key. This cannot be undone from the UI." It does **not**
archive the deals or activities underneath — the grouping dies, the history does
not.

## Notes and activities

There is no separate "notes" feature. A note is an activity, and activities come
in six kinds: **email, call, meeting, note, task, message.**

Use **Log activity** to add a note or a task straight onto a timeline.

One activity can link to several records at once — a contact and a deal, for
example. An activity with no links at all is visible to everyone in the
organization.

A meeting carries a status: **booked, held, no-show, canceled.**

**Activities are never hard-deleted.** Deleting archives them. If something is
filed against the wrong record, the fix is **Relink**, not delete.

On top of visibility inherited from linked records, an activity carries an
audience: everyone in the organization, the participants, or a named few. That
audience is not overridden by seniority — someone who can see every record still
does not read a message they were not an audience for.

Where the audience comes from depends on how the row arrived. A note or a call
you log is shared with the organization unless you say otherwise. **A message
captured from a mailbox is not**: its audience is derived from what each
importing mailbox asks for, and a new mailbox holds its mail until a classifier
judges the thread ordinary. You change a captured message's audience by sharing
its thread, not by editing the row — the row refuses a direct edit and says so.

## Putting a change back

Every record keeps a history of what changed, who changed it and when. From that
history you can put a single change back — one entry, not the whole record to a
point in time.

Putting a change back is an ordinary edit, not a special power. It re-applies
what the field held before that entry, goes through the same rules as if you had
typed the old value yourself, and appears in the history as its own entry naming
the change it reversed. So an undo is visible, and an undo can itself be undone.

Not every entry can be put back, and the history says which and why before you
press anything:

- **The field moved again since.** Putting the entry back would silently discard
  whatever was written after it. The reason names the field, so you can look at
  what happened in between and decide.
- **The record was archived.** Restore the record first; a change cannot be put
  back onto something that is not there.
- **The change did not come from an editable path.** Some entries record things
  the record's own edit path cannot write — an entry that would have to clear a
  field nothing can clear. These are shown as history, not as something to undo.

One thing to expect: the button can be honest when you read the screen and the
answer can still change by the time you press it, because somebody else may have
edited the record in between. When that happens the change is refused and
nothing is written, rather than being applied to a record that has moved. Read
the reason, look at the record again, and decide from what is there now.

**An agent cannot put a change back.** It is a person's authority on purpose:
otherwise an agent could reach a change it was never allowed to make directly by
making it, and then undoing the undo.

## Money

Amounts are stored as whole minor units plus a currency code. There is no
floating-point money anywhere in the product.

Most currencies have two decimal places. Some have none — Japanese yen, Korean
won, Vietnamese dong and others — and the product carries its own table rather
than trusting the browser, because the two standards involved disagree on about
ten currencies.

**Two currencies are never added together.** A column holding more than one
currency shows no total at all — it says "several currencies — no single total".
Adding euros to dollars produces a number that is not money, so Margince refuses
rather than guessing a rate.
