# Run a project

This guide is for the person working a project day to day — no code, no API.
It is the lookup page: where things are on the project page, how phases move,
and the full set of rules by which an email finds (or fails to find) its
project. The things you *cannot* do from the UI are at the end.

New to this? Start with
[tutorials/run-your-first-project.md](../tutorials/run-your-first-project.md).
For who may do what, key conventions and the fixed vocabularies, see
[set-up-projects.md](set-up-projects.md).

## What a project is in Margince

A project is **the body of work a deal is about**: started on a company,
carrying several deals over time, alive from the first conversation through
delivery to a deliberate close. Its page is the one place where everything
filed under that work — mail, notes, tasks, deals, contracts, documents, the
people seated on it — is read together, with a phase history that says when
the work moved and why.

Most of what lands on the page lands there without anyone filing it by hand.
The rest of this guide is about reading the page, moving the phase, and
knowing how the automatic filing decides.

## The Projects list

**Projects** in the left navigation lists every live project: name and key,
company, phase, owner, last activity.

- **All / In delivery / A–Z** at the top switch between every project, the ones
  in the **Delivering** phase, and an alphabetical listing.
- **Phase** narrows by phase (**All phases** or one of the four).
- **Search** matches names and keys.
- **Show archived** includes archived projects, marked **Archived**.
- **New project** opens the create form: **Project name**, **Company**
  (required), **Description**, **Target end date**. There is no key field —
  the key is minted from the name — and no owner field: a new project belongs
  to whoever creates it, and is reassigned later on the edit form.

## The project page, section by section

**Header** — name, the customer company (click it to open the company), owner,
phase and key. The verbs beside it: **Edit project**, **Archive project**, **Share**.

**Phase** — the four phases in a row with the current one highlighted. Press
another to move; see *Moving the phase* below.

**Figures** — **Open deal value**, **Won deal value**, **Open commitments**,
**Last activity**, **Activities**.

**Companies** (right, first card) — every company on this project with its
role, and the **Attach company** / **Detach** verbs. See
[set-up-projects.md](set-up-projects.md#putting-companies-and-people-on-a-project).

**Phase history** (right) — every phase the project has been in, with time
spent, and each move with date, who moved it and the reason. A move made by
the product itself — a deal win — shows the person whose action caused it.

**Stakeholders**, **Contracts**, **Documents** (right) — the people seated on
the project with their role, the agreements that name it, the files attached
to it. Files attached to the project's deals stay on the deals.

**Deals** — every deal on this project with status and value. **New deal**
here starts one on the same company and project.

**Open commitments** — open tasks filed under the project, soonest due first;
an **overdue** badge marks the late ones.

**Timeline** — what is filed under the project. **Activities / Changes / All**
switches between mail and activities, the record's own changes (fields and
phases), or both. **Activity kind**, **Search this timeline**, **From** and
**To** narrow it.

## Moving the phase

Press any phase other than the current one. A dialog titled **Move to
<phase>** opens with a **Reason** field:

- For **Initiative**, **Pursuing** and **Delivering** the reason is optional
  (*The move is recorded in the phase history with the reason you give.*).
  Press **Move**.
- For **Closed** the reason is required — *A closed project needs a reason* —
  and the button reads **Close project**. The dialog says what closing means:
  *Closing ends the project's delivery. It can be reopened later, and the
  reason stays on record.*

Any direction is allowed, including back from **Closed**. Two moves happen
without anyone pressing a phase:

- **Winning a deal** on the project moves it to **Delivering**, if it is in
  **Initiative** or **Pursuing**. A project already delivering is left alone,
  and a closed one is never reopened by a win.
- **Start delivery** on a won deal that names no project (offered only when
  the deal's company has exactly one live project) attaches the deal and moves
  the project to **Delivering** in one step.

An archived project takes no phase moves and no edits: the page says *This
project is archived and takes no changes.*

## Editing, archiving, sharing

**Edit project** changes name, owner, description and target end date. The key
is **not** editable: it was minted from the name at creation and stays as it
is, even if you rename the project. **Owner** offers *Me*, *Unassign*, and —
when somebody else owns it — *Keep current owner*.

The project's **customer** company cannot be changed after creation, but the
companies working the project can: attach and detach them in the **Companies**
section on the project page.

**Archive project** asks for confirmation: *Archiving removes this project
from the live list and frees its key. This cannot be undone from the UI.* The
deals and mail stay where they are; the project simply leaves the live list.
Archiving is a manager, management, admin or ops verb.

**Share** opens the sharing page for this one record: grant **Read** or
**Write** to a person or a team, optionally with an expiry and a reason, and
see who already has access. A share never widens anything beyond this project.

## Referencing a project from email

### Sending

Two composers, two behaviours.

**Replying (from a project, a deal or any timeline).** The composer works the
filing out and tells you, above the Subject field:

> ☑ **File under this project**
> This will be filed under *Name*, this deal's project.
> [KEY] is added to the subject so their reply files itself here.

It derives the project in the same order the inbound ladder uses: the thread's
own project first — the line then reads *…like the rest of this conversation* —
and the **deal's** project when the thread has none. If neither names one,
nothing is shown at all.

The **Subject** field is stamped with `[KEY]`. Untick the box and both lines go
and the tag is removed; tick it again and the tag comes back in front of
whatever you have typed. A tag you delete by hand stays deleted.

If the subject already carries a **different** project's key, sending under this
one removes it — two keys in a subject make the inbound rule ambiguous, so it
files under neither. Bracketed text that could not be a key (`[FYI]`) is left
alone.

> **On a reply, the subject tag is the only thing carrying the project.** The
> reply itself inherits the links of the message it answers, because the reply
> endpoint takes none of its own. So your sent copy may not appear on the
> project's timeline until the customer's tagged reply comes back and is filed.
> Tracked as [issue #2422](https://github.com/margince/margince/issues/2422).

**Writing to an account (company page → Write email).** A new conversation has
no thread to inherit from, so this composer asks. Under **Draft to** — and
under **Related to**, when the account has deals — is the **Project** picker:
**No project**, then the account's live projects as `KEY · Name`. One live
project is pre-selected; several start at **No project**. Closed projects are
not offered.

Picking one shows **Scoped to KEY** and does two things:

1. **Draft with AI** reads only what is filed under that project or under no
   project. Mail filed under another project on the same account is left out of
   the draft and of its **Based on:** line.
2. The sent message is **linked** to the project — this composer does send the
   link — so it appears on the project's timeline straight away.

You can also file without either control: put the key in square brackets in the
subject — `[NER-1] kickoff agenda` — and the rule below does the rest.

### Receiving

Every captured email is run through a fixed ladder, in this order, and the
first rule that answers wins. **No model is involved in rungs 1–3; these rules
are exact.** A rung that finds a project the reader may not use — archived, or
outside their access — counts as no match, and the ladder carries on to the
next rung rather than stopping.

The ladder runs just after the message is captured, not during. A message that
arrives before its project exists is not re-filed later on its own; relink it,
or let the next message in the thread carry the filing.

1. **The thread.** A reply to a conversation already filed under a project is
   filed under the same project. Matched **within one medium only** — a mail
   thread cannot inherit from a Telegram conversation, which stops a forged
   `References` header from filing mail onto a chat the sender was never in.
   Siblings under legal hold or archived settle nothing, and where siblings
   disagree the most recently filed one wins.
2. **The deal.** A message linked to a deal is filed under the deal's project.
   Two deals on the message rolling up to two *different* projects cancel out,
   the same way two keys do.
3. **The key in the subject.** A subject carrying the key **in square
   brackets** — `[ERP-27] status` — is filed under that project. A bare
   `ERP-27` in the subject is not a reference; the brackets are what stops a
   project keyed `RE` from swallowing every reply in the installation. Two
   different keys in one subject cancel out: the message says nothing reliable
   and is not filed.
4. **An offer to a person.** When none of the above answers, Margince does not
   file the message. It stages an approval — **File under a project** — in
   **Approvals**, and a person confirms or declines. Confirm files the message exactly as a manual
   relink would; decline means the same message-and-project pairing is not
   offered again. An offer nobody answers expires, and the next message in the
   thread opens a fresh one.

Most mail belongs to no project, and that is the correct answer. A message
that reaches none of the four rungs stays on the contact's and company's
timeline unfiled, which is where it belongs.

### When nothing matched, or the wrong thing did

Press **Relink** on the message in any timeline. The dialog **Relink this
activity** searches across people, companies, deals, leads and projects; pick
the project. Two checkboxes:

- **Move instead of also-link** — replace the existing link of the same type
  rather than adding another. Use it when the message was filed under the
  wrong project.
- **Also move the rest of this conversation** — shown when the message is
  part of a thread. *Every message in this thread you can edit moves with it,
  in one step.* A mis-filed conversation is usually mis-filed whole.

When a deal and a project share a name, the search shows both and the list
does not say which is which. Name projects so that this does not happen.

### What filing does to retention

> **Filing an email under a project is permanent for retention.** Under the
> German pack, an email linked to a project is business correspondence and is
> kept for six years counted from the end of the calendar year in which the
> email was sent or received (its own date, not the day it was filed). The
> mark is written the moment the link is made,
> whether by the ladder above, by the composer's picker, by a confirmed
> offer, or by **Relink**. Moving the email off the project afterwards does
> **not** remove the mark. An erasure request that reaches such a message
> keeps it under a restriction instead of deleting it; it shows on the
> **Restricted records** page with the project's name as the reason.

This is why the product never files by guesswork: every rule above is either
exact or confirmed by a person.

## Reports

Three project reports live in **Reports**:

- **Projects by phase** — how many projects sit in each phase, and their won
  and open deal value.
- **Project commitments** — open tasks across projects, by project and due
  date, with the overdue ones first.
- **Projects gone quiet** — projects in delivery with no filed activity for a
  while, so a stalled engagement is seen before the customer says so.

## Agents and MCP

An agent connected over MCP sees a project exactly as the person whose
passport it holds would. `read_project_360` returns the page above, section by
section. `catch_me_up_on` with a `project_id`
answers "what has been happening?" from what is filed under the project or
under none. `prepare_handoff` assembles the sales-to-delivery handover and
names each gap the records leave. `advance_project_phase` moves a phase under
the same rules as the page, and runs straight away under a passport whose
scopes admit it — the passport and the seat behind it are the confinement,
not an approval step.

## What you can't change from the UI

- **The retention mark** on a filed email — permanent, as above.
- **Legal hold** on a project — set by an operator in the database; no
  switch in the app.
- **Bulk owner transfer** — `POST /projects/transfer-ownership`, API only and
  human-only.
- **Un-archiving** a project.
- **The phase and stakeholder-role lists** — fixed vocabulary; see
  [set-up-projects.md](set-up-projects.md#the-fixed-vocabularies).
