# Run your first project

## In short

A deal ends the day it is won. The work it was sold for does not: the rollout
runs for months, a second deal lands on the same work a year later, and the
email about it never stops. A **project** is where Margince keeps that body of
work — from the first conversation, through the deal, through delivery, to the
day you close it.

In Margince you can:

- **Start a project while the deal is still being pursued**, so the early
  conversations are already filed where the delivery team will look for them.
- **Have Margince give it a key** such as `NER-1`, and every email whose subject
  carries `[NER-1]` is filed under it automatically — including the ones you
  send, which carry the key without you typing it.
- **Win the deal and watch the project move into delivery** by itself.
- **Write email from inside the project**, with the AI reading only what
  belongs to it.
- **Close it with a reason, and reopen it** when the scope grows.

**This page walks one project from start to finish.** For who may do what,
the key rules and the vocabulary, see
[how-to/set-up-projects.md](../how-to/set-up-projects.md). For the day-to-day
lookup — including how email finds its project and what that means for
retention — see [how-to/run-a-project.md](../how-to/run-a-project.md).

No code, no API. You need a sign-in, a company in Margince (the walkthrough
uses a fictional customer, *Nordwind Logistik*, recorded as a company), and
one contact at that company.

## 1. Why a project exists at all

Open **Pipeline** and look at any deal. It has a value, a stage, a close date.
When it is won it becomes a line in a report and stops moving.

Now think about what an ERP rollout at Nordwind actually is: a scoping
workshop before the proposal, the proposal, the contract, six months of
delivery, a go-live, hypercare, and — if it goes well — a phase-2 deal for the
warehouse module. One deal cannot hold that. Two deals cannot hold it either,
because the conversation in between belongs to neither.

A project holds it. It is started on a company, it carries several deals over
time, and everything filed under it — mail, notes, tasks, contracts — stays
together on one page.

Open **Projects** in the left navigation. On a fresh installation you see the
empty state:

> **No projects yet**
> A project is the body of work a deal is about. It starts during the deal,
> in the initiative phase, and outlives close-won: once the deal is won,
> delivery is tracked here.
> Give a project a key and any email whose subject carries [KEY] is filed
> under it automatically.

That paragraph is the whole model. The rest of this page is it in practice.

## 2. Create the project during the deal

You do not need to win anything first. The moment Nordwind says "send us a
proposal", the project exists.

1. Open **Pipeline** and press **New deal**.
2. Fill in **Deal name** (`Nordwind ERP licences and rollout`) and **Value**
   (`180000`).
3. Pick the **Company** — *Nordwind Logistik*. Until you do, the **Project**
   field is disabled: which projects this deal may be filed under is a question
   about its company, and there is no company yet.
4. Open **Project** and choose **New project…**. One more field appears:
   **Project name**.
5. Enter the project name — `Nordwind ERP rollout`. Give it a different name
   from the deal: later, when you search for the project, a deal with the same
   name sits next to it in the results and the two are hard to tell apart.
6. Press **Create**.

You land on the deal page. Beside the company name you should see a chip
reading **Nordwind ERP rollout** — the project this deal belongs to. Click it.

The project page opens. Under the name you should see the company, the owner —
**you**, because a project belongs to whoever creates it — and the phase:
**Initiative**. On the right, the **Phase history** reads *Started in
Initiative* with today's date and your name.

Created at deal creation like this, the project is in **Initiative** — an
idea, not yet a pursuit. That is deliberate: the project exists before you
know whether the deal will happen.

## 3. The key it was given

Look under the project's name. Beside the phase is a short chip — something
like **NER-1**. Nobody typed it: Margince minted it from the project's name
when the project was created, and it cannot be edited. Hover it and the
tooltip says what it is for:

> Margince gives each project a short key. Write [NER-1] in an email subject
> and the mail is filed under this project.

**How the key is built.** The initials of a multi-word name, or the opening
letters of a single-word one, then a hyphen and the lowest free number:

| Project name | Key |
|---|---|
| `Nordwind ERP rollout` | `NER-1` |
| `Datenmigration` | `DATENMIG-1` |
| `ERP rollout Acme` | `ERA-1` |
| a second `ERP rollout Acme` | `ERA-2` |

Only ASCII letters and digits are used, and a name with too few of them —
`工事`, `2026` — falls back to `PRJ-1`. The number is the lowest one free, not
the next one up, so a number released by archiving a project is used again.

**What this means for you:** the only lever you have over a key is the
project's **name**. A name whose initials read well gives a key that reads
well, and the key is what your customer will see in every subject line. That
is the whole of the naming advice — see
[how-to/set-up-projects.md](../how-to/set-up-projects.md#naming-a-project-so-its-key-reads-well).

Two properties worth knowing now, because both surface later:

- **Keys are unique among live projects, compared case-insensitively.** `ner-1`
  and `NER-1` are the same key.
- **Archiving a project frees its key.** Which is why Margince will never stamp
  an archived project's key into a subject line — by then the key may belong to
  somebody else.

## 4. Work the deal phase

The proposal goes out; Nordwind is evaluating. Move the project from
**Initiative** to **Pursuing** so the page says what is true.

1. On the project page, in the **Phase** row, press **Pursuing**.
2. A dialog titled **Move to Pursuing** opens. It says: *The move is recorded
   in the phase history with the reason you give.* Type a reason —
   `Scoping workshop booked; proposal in progress.` — and press **Move**.

The phase under the name now reads **Pursuing**, and the **Phase history** on
the right shows *Initiative → Pursuing* with the date, your name and your
reason in quotation marks. The list of phases above it shows how long the
project spent in each one.

While the deal is pursued, log what happens on the **deal**: open the deal,
use **Log activity** (a note *Kickoff call with Nordwind IT*, for example) and
press **Log**. The note lands on the deal's timeline.

It does **not** appear on the project's timeline yet. A note on a deal is
filed under the deal; the project's **Timeline** shows only what is filed
under the project. To move it, press **Relink** on the note, search for
*Nordwind ERP rollout*, pick the **project** (not the deal of the same family)
and press **Relink**. Now open the project: the timeline shows the note and the
**Activities** figure reads 1. Email does this filing by itself once the key is
in the subject; see step 7.

## 5. Win the deal

Nordwind signs on a purchase order.

1. Open the deal and press **Won** in the **Stage** row.
2. A confirmation opens: **Move to Won?** Press **Confirm**.
3. If the deal has no signed contract attached, the dialog stays open and
   asks **How was it won?** Pick **On a purchase order** and press **Confirm**
   again. ([work-your-pipeline.md](../how-to/work-your-pipeline.md#close-a-deal)
   explains why winning asks this.)

The deal now reads **won**. Click the project chip.

The project's phase reads **Delivering**. You did not press anything on the
project: winning a deal moves its project into delivery inside the same
change, so no report ever sees a won deal on a project still being pursued.
The **Phase history** shows *Pursuing → Delivering* with your name, and the
figures at the top have moved — **Open deal value** is `€0.00`, **Won deal
value** is `€180,000.00`.

Two limits on this automatic move are worth knowing now:

- It fires only from **Initiative** or **Pursuing**. A project already in
  **Delivering** stays there — a second deal landing on running work is not a
  restart.
- It never reopens a **Closed** project. A renewal that closes years later
  does not silently resurrect an engagement somebody deliberately ended; the
  deal reads won, the project stays closed, and reopening is a decision a
  person makes with a reason (step 6).

## 6. Live in delivery, then close and reopen

Delivery runs for months. The project page is where it is tracked:

- **Deals** lists every deal on this project, won and open, with its value.
  **New deal** here starts another deal on the same project and company.
- **Open commitments** lists open tasks filed under the project, soonest due
  first, with an **overdue** badge where it applies.
- **Stakeholders**, **Contracts** and **Documents** on the right fill in as
  people are seated on the project, agreements name it, and files are attached
  to it.
- **Timeline** is the mail and activity filed under the project. The filter
  row above it (**Activity kind**, **Search this timeline**, **From**, **To**)
  narrows it; the **Activities / Changes / All** switch shows the mail, the
  record's own field and phase changes, or both.

When go-live is signed off:

1. Press **Closed** in the **Phase** row.
2. The dialog **Move to Closed** says: *Closing ends the project's delivery.
   It can be reopened later, and the reason stays on record.* The **Reason**
   field is required here — *A closed project needs a reason* — and
   **Close project** stays disabled until you type one. Enter
   `Go-live signed off by Nordwind on 22 Aug; hypercare handed to support.`
   and press **Close project**.

The phase reads **Closed**. The history records *Delivering → Closed* with
your reason.

Three months later Nordwind orders the warehouse module. Press **Delivering**
in the **Phase** row, give the reason (`Warehouse module phase 2 added to
scope.`) and press **Move**. The project is back in delivery; the history
keeps both the close and the reopen, so the gap reads as what it was.

Every phase move works like this, in either direction. The four phases are
fixed — **Initiative**, **Pursuing**, **Delivering**, **Closed** — but the
order is not enforced. Only closing demands a reason; the other moves record
one if you give it.

## 7. Email, and how it finds the project

There are two composers, and they behave differently on purpose.

### Replying from the deal or the project

Open the project (or the deal) and press **Reply** on a message in the
timeline. Above the Subject field you will see:

> ☑ **File under this project**
> This will be filed under Nordwind ERP rollout, this deal's project.
> [NER-1] is added to the subject so their reply files itself here.

And the **Subject** field already contains `[NER-1]`.

Nothing was asked of you. The composer worked out where this message belongs
and said so, in this order:

1. **The thread's own project**, if the conversation is already filed. Then the
   first line reads *…like the rest of this conversation* instead.
2. **The deal's project**, when the thread carries none. This is the ordinary
   case for a conversation that started before the project was attached, and
   the line names the reason — *this deal's project* — because you never put
   that project on this conversation and deserve to be told where it came from.

If neither names a project, **nothing appears at all**: no line, no tickbox, no
tag. A message that belongs to no project is the common case and says so by
staying quiet.

**To decline**, untick the box. The two lines disappear and `[NER-1]` is taken
back out of the Subject — the tag would otherwise promise a routing that will
not happen. Tick it again and the tag returns, in front of whatever you have
since typed. You can also just delete the tag yourself; it stays deleted.

> **The tag is doing real work, not decoration.** Your customer's mail client
> keeps `[NER-1]` in the subject when they reply. Margince reads it back on the
> way in and files their answer under the project — even if the mail thread is
> broken by a forward, a new subject, or a colleague brought in on a fresh
> message. This is what makes the filing survive leaving your installation.

One limit, stated plainly: **on a reply, the tag is the only thing carrying the
project.** The reply itself is filed under whatever the message you are
answering was filed under, because the send inherits its links. So the copy of
your own reply may not appear on the project's timeline until the customer
answers and their tagged reply comes back. This is
[issue #2422](https://github.com/margince/margince/issues/2422); the
account-started composer below does not have this limitation.

### Writing to an account from the company page

Open the company page (**Companies** → *Nordwind Logistik*) and press
**Write email**. This composer starts a new conversation rather than continuing
one, so there is no thread to inherit from and it **asks** instead of stating:

- **Draft to** — which contact.
- **Related to** — which deal, when the account has any.
- **Project** — **No project**, then the projects on this account as
  `NER-1 · Nordwind ERP rollout`. With exactly one live project it is
  pre-selected; with several it starts at **No project**.

Choosing one shows **Scoped to NER-1** beneath it, and that does two things:

- **What the AI reads.** Press **Draft with AI**. The draft is grounded only in
  what is filed under this project or under no project at all; mail filed under
  a *different* project on the same account is left out, and the **Based on:**
  line lists what it did draw from.
- **Where the mail is filed.** The sent message is linked to the project, so it
  appears on the project's timeline and replies inherit that filing.

### One more thing the composer does quietly

If the subject already carries **another** project's key, sending under this
project removes it. Two keys in one subject make the inbound rule ambiguous —
it files under neither — so leaving both would silently break the filing you
just asked for. Bracketed text that could not be a key, like `[FYI]`, is left
alone.

> **Filing an email under a project is permanent for retention.** Under the
> German pack an email filed under a project counts as business
> correspondence and is kept for six years from the end of the calendar year it
> was sent or received. Moving it off the project later does not undo that.
> Relink deliberately.

## 8. Agents over MCP

Everything above is available to an agent connected over MCP, governed by the
same permissions the signed-in person holds:

- `read_project_360` reads the whole project page — phase history with time
  per phase, deals, stakeholders, contracts, documents, open commitments,
  timeline.
- `catch_me_up_on` with a `project_id` answers "what has been going on?" for
  the project, reading only what is filed under it or under no project.
- `prepare_handoff` assembles what the delivery side needs from the sales
  side — owner, client contacts, what was sold, by when, what is promised —
  and names each gap the records leave.
- `advance_project_phase` moves a phase, with the same closing-needs-a-reason
  rule. It runs straight away under a passport whose scopes admit it — what
  confines the agent is the passport and the seat behind it, not an approval
  step.
- The generic record tools (`create_record`, `read_record`, `update_record`,
  `list_records`, `search_records`) accept `project` as a record type, and a
  deal can be listed by its `project_id`.

The tool catalog is [reference/mcp-info.md](../reference/mcp-info.md).
Connecting a client is [how-to/connect-an-mcp-client.md](../how-to/connect-an-mcp-client.md).

## Where to next

- [how-to/set-up-projects.md](../how-to/set-up-projects.md) — who can create,
  edit, archive and share; key conventions; when to create a project.
- [how-to/run-a-project.md](../how-to/run-a-project.md) — the lookup page for
  daily work, including every rule by which email finds its project.
