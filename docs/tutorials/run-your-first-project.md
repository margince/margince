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
  carries `[NER-1]` is filed under it automatically — including the replies you
  send from the deal or the project, which carry the key without you typing it.
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
> Every project gets a short key. Any email whose subject carries it in
> brackets is filed under that project automatically.

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

- **Companies** (first on the right) lists every company working this project
  and what each one is to it. More on this below.
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

### Nordwind brings in a partner

Two months in, Nordwind subcontracts the warehouse integration to
*DACHPartner GmbH*. The project is no longer one company's work, and Margince
does not make you pretend otherwise.

1. On the project page, in **Companies**, press **Attach company**.
2. Under **As**, leave **Partner** — the list opens here rather than on
   Customer, because the project already has its customer. **Set the role
   before you pick the company**, because picking is what attaches it.
3. Search for *DACHPartner GmbH* and pick it from the results. The dialog
   closes and the company is on the project.

The Companies card now shows both: *Nordwind Logistik — Customer* and
*DACHPartner GmbH — Partner*.

What this buys you: a deal on **DACHPartner** can now be filed under this
project. A deal may name any project one of its companies is on, whatever role
that company holds, so the partner's own commercial work sits on the same body
of work as the customer's.

Two things Margince will refuse, both with the reason on screen:

- **Taking off the last company.** *A project keeps at least one company; add
  another before taking this one off.*
- **Taking off a company that still has deals here.** *This company still has 1
  deal(s) on the project; move or close them before taking the company off.*
  Note that winning or losing a deal does not clear this — the count is of
  deals that still exist. Point them at another project, or archive them.

Attaching a company that is already on the project **changes its role** rather
than adding it twice — which is how you promote DACHPartner from subcontractor
to partner later.

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

Open the project (or the deal) and press **Reply** on a message in the
timeline. Above the Subject field is one control:

> **Project**  `NER-1 · Nordwind ERP rollout` ▾

And the **Subject** field already contains `[NER-1]`.

Open the picker and you get **No project**, then every live project Nordwind
reaches — including any it works as a partner or a subcontractor, not only the
ones it is the customer of.

- **Choosing a project** puts its `[KEY]` at the front of the Subject.
- **Choosing No project** takes the tag out.
- **Switching projects** swaps the tag.

Nothing explains this on screen, and nothing needs to: the tag is right there
in the Subject field, so you can see exactly what will go out.

Write the rest of the subject however you like — the tag stays at the front
while a project is chosen. Deleting it out of the text does not unset the
project, because the picker still names one and the picker is what the send
follows. To send without a tag, choose **No project**.

### What it starts on

The picker suggests rather than decides:

1. **The thread's own project**, when the conversation is already filed. A
   conversation is one body of work, and a sibling message settled it.
2. **The deal's project**, when the thread carries none and you are replying
   from a deal. This is the ordinary case for a conversation that started
   before the project was attached.
3. **The company's only live project**, when it has exactly one and neither of
   the above applies.

Otherwise it starts on **No project**. Every one of those is a starting point
you can change — and when the company reaches no live project at all, the
picker does not appear.

> **The tag is doing real work, not decoration.** Your customer's mail client
> keeps `[NER-1]` in the subject when they reply. Margince reads it back on the
> way in and files their answer under the project — even if the mail thread is
> broken by a forward, a new subject, or a colleague brought in on a fresh
> message. This is what makes the filing survive leaving your installation.

One limit worth knowing: **on a reply, the tag may be the only thing carrying
the project.** A reply is filed under whatever the message you are answering
was filed under, and it cannot add a link of its own. So on a conversation the
project never reached, your sent copy does not appear on the project's timeline
— the customer's tagged answer is what brings the whole thread in. A
conversation already filed does not have this problem, and neither does the
composer below. This is
[issue #2422](https://github.com/margince/margince/issues/2422).

### Writing to an account from the company page

Open the company page (**Companies** → *Nordwind Logistik*) and press
**Write email**. A fresh mail is filed under a project exactly the same way: the
**Project** picker is the same control, in the same place, filling the Subject
with the same tag. There is no thread to inherit from, so it starts on the
company's only live project when it has one, and on **No project** otherwise.

Two more pickers sit above it here, because a new conversation needs what a
reply already knows:

- **Draft to** — which contact.
- **Related to** — which deal, when the account has any.

Choosing a project shows **Scoped to NER-1** beneath the picker, and here it
does one extra thing beyond filing the mail:

**What the AI reads.** Press **Draft with AI**. The draft is grounded only in
what is filed under this project or under no project at all; mail filed under a
*different* project on the same account is left out, and the **Based on:** line
lists what it did draw from.

The sent message is also **linked** to the project — this composer does send the
link — so it appears on the project's timeline straight away.

### One more thing the composer does quietly

If the subject already carries **another** project's key, sending under this
project removes it. Two keys in one subject make the inbound rule ambiguous —
it files under neither — so leaving both would silently break the filing you
just asked for. The rule goes by SHAPE rather than by looking each one up, so
any bracketed word that could be a key is removed — `[FYI]` included. Only a
group that could never be one, like `[2026]`, survives.

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
