<!-- prose:plain -->
# Run your first project

## In short

A deal ends the day it is won. The work it was sold for does not. The rollout
runs for months, a second deal lands on the same work a year later, and the
email about it never stops. A **project** is where Margince keeps that body of
work, from the first conversation, through the deal and delivery, to the day
you close it.

In Margince you can:

- Start a project while the deal is still being pursued, so the early
  conversations are already filed where the delivery team will look for them.
- Have Margince give it a key such as `NER-1`. Every email whose subject
  carries `[NER-1]` is filed under it automatically. That includes the replies you
  send from the deal or the project, which carry the key without you typing it.
- Win the deal and watch the project move into delivery by itself.
- Write email from inside the project, with the AI reading only what belongs
  to it.
- Close it with a reason, and reopen it when the scope grows.

Below, one project is followed from start to finish. For who may do what, the
key rules and the vocabulary, see
[handbook/leads-deals-and-projects.md](../docs/handbook/leads-deals-and-projects.md). For how
email finds its project and what that means for retention, see
[handbook/capture.md](../docs/handbook/capture.md).

No code, no API. You need a sign-in, a company in Margince, and one contact at that
company. The walkthrough uses a fictional customer, *Nordwind Logistik*, recorded as a company.

## 1. Why a project exists at all

Open **Pipeline** and look at any deal. It has a value, a stage, a close date.
When it is won it becomes a line in a report and stops moving.

Now think about what an ERP rollout at Nordwind involves. There is a scoping
workshop before the proposal, then the proposal, the contract and six months of
delivery. Then come a go-live and hypercare and, if it goes well, a phase-2 deal
for the warehouse module. One deal cannot hold that. Two deals cannot hold it either, because the
conversation in between belongs to neither.

A project holds it. It is started on a company, and it carries several deals over
time. Everything filed under it (mail, notes, tasks, contracts) stays
together on one page.

Open **Projects** in the left navigation. On a fresh installation you see the
empty state:

> **No projects yet**
> A project is the body of work a deal is about. It starts during the deal,
> in the initiative phase, and outlives close-won: once the deal is won,
> delivery is tracked here.
> Every project gets a short key. Any email whose subject carries it in
> brackets is filed under that project automatically.

The rest of this page puts that paragraph into practice.

## 2. Create the project during the deal

You do not need to win anything first. The moment Nordwind says "send us a
proposal", the project exists.

1. Open **Pipeline** and press **New deal**.
2. Fill in **Deal name** (`Nordwind ERP licences and rollout`) and **Value**
   (`180000`).
3. Pick the **Company**: *Nordwind Logistik*. Until you do, the **Project**
   field is disabled. The projects a deal may be filed under depend on
   its company.
4. Open **Project** and choose **New project…**. One more field appears:
   **Project name**.
5. Enter the project name: `Nordwind ERP rollout`. Give it a different name
   from the deal. When you search for the project later, a deal with the same
   name sits next to it in the results. The two are then hard to tell apart.
6. Press **Create**.

You land on the deal page. Beside the company name you should see a chip
reading **Nordwind ERP rollout**: the project this deal belongs to. Click it.

The project page opens. Under the name you should see the company, the owner
(**you**, because a project belongs to whoever creates it) and the phase:
**Initiative**. On the right, the **Phase history** reads *Started in
Initiative* with today's date and your name.

A project created with its deal starts in **Initiative**: an idea, not yet a
pursuit. The project exists before you know whether the deal will happen.

## 3. The key it was given

Look under the project's name. Beside the phase is a short chip, something like
**NER-1**. No one typed it: Margince made it from the project's name when the
project was created, and it cannot be edited. Hover it and the tooltip says
what it is for:

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

Only ASCII letters and digits are used. A name with too few of them (`工事`,
`2026`) falls back to `PRJ-1`. The number is the lowest one free, not the next
one up, so a number released by archiving a project is used again.

**What this means for you:** the only control you have over a key is the
project's name. A name whose initials read well gives a key that reads well,
and your customer will see the key in every subject line. For more naming
advice, see
[handbook/leads-deals-and-projects.md](../docs/handbook/leads-deals-and-projects.md#how-do-i-choose-a-good-project-key).

Two properties worth knowing now, because both come up later:

- **Keys are unique among live projects, ignoring case.** `ner-1` and `NER-1`
  are the same key.
- **Archiving a project frees its key.** So Margince never puts an archived
  project's key into a subject line: by then the key may belong to another
  project.

## 4. Work the deal phase

The proposal goes out; Nordwind is evaluating. Move the project from
**Initiative** to **Pursuing** so the page says what is true.

1. On the project page, in the **Phase** row, press **Pursuing**.
2. A dialog titled **Move to Pursuing** opens. It says: *The move is recorded
   in the phase history with the reason you give.* Type a reason
   (`Scoping workshop booked; proposal in progress.`) and press **Move**.

The phase under the name now reads **Pursuing**. The **Phase history** on
the right shows *Initiative → Pursuing* with the date, your name and your
reason in quotation marks. The list of phases above it shows how long the
project spent in each one.

While the deal is pursued, log what happens on the **deal**. Open the deal,
use **Log activity** (a note *Kickoff call with Nordwind IT*, for example) and
press **Log**. The note lands on the deal's timeline.

It does not appear on the project's timeline yet. A note on a deal is filed
under the deal; the project's **Timeline** shows only what is filed under the
project. To move it, press **Relink** on the note and search for *Nordwind ERP
rollout*. Pick the **project** (not the deal of the same name) and press
**Relink**. Now open the project: the timeline shows the note and the
**Activities** figure reads 1. Email does this filing by itself once the key is
in the subject; see step 7.

## 5. Win the deal

Nordwind signs on a purchase order.

1. Open the deal and press **Won** in the **Stage** row.
2. A confirmation opens: **Move to Won?** Press **Confirm**.
3. If the deal has no signed contract attached, the dialog stays open and
   asks **How was it won?** Pick **On a purchase order** and press **Confirm**
   again. ([the-pipeline.md](../docs/handbook/the-pipeline.md#winning)
   explains why winning asks this.)

The deal now reads **won**. Click the project chip.

The project's phase reads **Delivering**. You did not press anything on the
project. Winning a deal moves its project into delivery at the same moment.
So no report ever sees a won deal on a project still being pursued. The **Phase
history** shows *Pursuing → Delivering* with your name. The figures at the
top have moved: **Open deal value** is `€0.00`, **Won deal value** is
`€180,000.00`.

Two limits on this automatic move:

- It moves only from **Initiative** or **Pursuing**. A project already in
  **Delivering** stays there, because a second deal landing on running work is
  not a restart.
- It never reopens a **Closed** project. A renewal won years later does not
  bring back an engagement someone chose to end. The deal reads won, the
  project stays closed, and reopening is a decision a human makes, with a
  reason (step 6).

## 6. Live in delivery, then close and reopen

Delivery runs for months. The project page is where it is tracked:

- **Companies** (first on the right) lists every company working this project
  and what each one is to it. More on this below.
- **Deals** lists every deal on this project, won and open, with its value.
  **New deal** here starts another deal on the same project and company.
- **Open commitments** lists open tasks filed under the project, soonest due
  first, with an **overdue** badge where it applies.
- **Stakeholders**, **Contracts** and **Documents** on the right fill in as
  contacts are seated on the project, agreements name it, and files are attached
  to it.
- **Timeline** is the mail and activity filed under the project. The filter
  row above it (**Activity kind**, **Search this timeline**, **From**, **To**)
  narrows it. The **All / Changes** switch shows the mail and activity, or
  the project's own field and phase changes.

### Nordwind brings in a partner

Two months in, Nordwind subcontracts the warehouse integration to
*DACHPartner GmbH*. The project is now more than one company's work, and
Margince can show that.

1. On the project page, in **Companies**, press **Attach company**.
2. Under **As**, leave **Partner**. The list opens on Partner, not Customer,
   because the project already has its customer. Set the role before you pick
   the company, because picking is what attaches it.
3. Search for *DACHPartner GmbH* and pick it from the results. The dialog
   closes and the company is on the project.

The Companies card now shows both: *Nordwind Logistik: Customer* and
*DACHPartner GmbH: Partner*.

Now a deal on **DACHPartner** can be filed under this project. A deal may name
any project one of its companies is on, whatever role that company holds. So
the partner's own commercial work sits on the same project as the customer's.

Margince refuses the following, and shows the reason on screen:

- **Removing the last company.** *A project keeps at least one company; add
  another before taking this one off.*
<!-- prose:allow sentence the refusal message is quoted as the screen shows it -->
- **Removing a company with deals here.** *This company still has 1
  `deal(s)` on the project; move or close them before taking the company off.*
  Winning or losing a deal does not clear this, because the count is of deals
  that still exist. Point them at another project, or archive them.

Attaching a company that is already on the project changes its role instead of
adding it twice. That is how you move DACHPartner from subcontractor to partner
later.

When go-live is signed off:

1. Press **Closed** in the **Phase** row.
2. The dialog **Move to Closed** says: *Closing ends the project's delivery.
   It can be reopened later, and the reason stays on record.* The **Reason**
   field is required here (*A closed project needs a reason*).
   **Close project** stays disabled until you type one. Enter
   `Go-live signed off by Nordwind on 22 Aug; hypercare handed to support.`
   and press **Close project**.

The phase reads **Closed**. The history records *Delivering → Closed* with
your reason.

Three months later Nordwind orders the warehouse module. Press **Delivering**
in the **Phase** row, give the reason (`Warehouse module phase 2 added to
scope.`) and press **Move**. The project is back in delivery. The history keeps
both the close and the reopen, so the gap between them stays visible.

Every phase move works like this, in either direction. The four phases are
fixed (**Initiative**, **Pursuing**, **Delivering**, **Closed**), but you can
move between them in any order. Only closing demands a reason; the other moves
record one if you give it.

## 7. Email, and how it finds the project

Open the project (or the deal) and press **Reply** on a message in the
timeline. Above the Subject field is one control:

> **Project**  `NER-1 · Nordwind ERP rollout` ▾

And the **Subject** field already contains `[NER-1]`.

Open the picker and you get **No project**, then every live project Nordwind
is on. That includes any where it is a partner or a subcontractor and not the
customer.

- **Choosing a project** puts its `[KEY]` at the front of the Subject.
- **Choosing No project** takes the tag out.
- **Switching projects** swaps the tag.

The tag is in the Subject field, so you can see what will go out.

Write the rest of the subject however you like; the tag stays at the front
while a project is chosen. Deleting it from the text does not unset the
project, because the picker still names one and the send follows the picker.
To send without a tag, choose **No project**.

### What it starts on

The picker suggests a project; you decide:

1. **The thread's own project**, when the conversation is already filed. A
   conversation is one body of work, and an earlier message in it settled
   which.
2. **The deal's project**, when the thread has none and you are replying
   from a deal. This is the usual case for a conversation that started before
   the project was attached.
3. **The company's only live project**, when it has one and neither of the
   above applies.

Otherwise it starts on **No project**. You can change any of these. When the
company is on no live project at all, the picker does not appear.

> **The tag files the customer's replies.** Your customer's mail client keeps
> `[NER-1]` in the subject when they reply. Margince reads it when the mail
> arrives and files their answer under the project. That works even if the
> thread is broken by a forward, a new subject, or a colleague brought in on a
> fresh message.

One limit: on a reply, the tag may be the only thing carrying the project. A
reply is filed under whatever the message you are answering was filed under,
and it cannot add a project of its own. So on a conversation the project never
reached, your sent copy does not appear on the project's timeline. The
customer's tagged answer is what brings the whole thread in. A conversation
already filed does not have this problem, and neither does the composer below.
This is [issue #2422](https://github.com/margince/margince/issues/2422).

### Writing to an account from the company page

Open the company page (**Companies** → *Nordwind Logistik*) and press
**Write email**. A new mail is filed under a project the same way. The
**Project** picker is the same control, in the same place, filling the Subject
with the same tag. There is no thread to inherit from, so it starts on the
company's only live project when it has one, and on **No project** otherwise.

Two more pickers sit above it here, because a new conversation needs what a
reply already knows:

- **Draft to**: which contact.
- **Related to**: which deal, when the account has any.

Choosing a project shows **Scoped to NER-1** beneath the picker. Here it also
limits what the AI reads.

**What the AI reads.** Press **Draft with AI**. The draft uses only what is
filed under this project or under no project at all. Mail filed under a
*different* project on the same account is left out. The **Based on:** line
lists what the draft drew from.

The sent message is also filed under the project by this composer, so it
appears on the project's timeline straight away.

### Sending removes other project keys from the subject

If the subject already carries another project's key, sending under this
project removes it. With two keys in one subject, Margince cannot tell which
project a reply belongs to and files it under neither. Margince removes any
bracketed word shaped like a key, without checking whether that project
exists, so `[FYI]` is removed too. Only a group that could never be a key,
like `[2026]`, stays.

> **Filing under a project is permanent for retention.** Under the
> German pack, an email filed under a project counts as business
> correspondence. It is kept for six years from the end of the calendar year it
> was sent or received. Moving it off the project later does not undo that, so
> relink with care.

## 8. Agents over MCP

Everything above is available to an agent connected over MCP, with the same
permissions the signed-in user holds:

- `read_project_360` reads the whole project page: phase history with time
  per phase, deals, stakeholders, contracts, documents, open commitments,
  timeline.
- `catch_me_up_on` with a `project_id` answers "what has been going on?" for
  the project, reading only what is filed under it or under no project.
- `prepare_handoff` collects what the delivery side needs from the sales side:
  owner, client contacts, what was sold, by when, what is promised. It names
  each gap the records leave.
- `advance_project_phase` moves a phase, with the same closing-needs-a-reason
  rule. It runs straight away when the agent's access allows it; the agent's
  passport and the seat behind it limit it, and no approval step applies.
- The general record tools (`create_record`, `read_record`, `update_record`,
  `list_records`, `search_records`) accept `project` as a record type, and a
  deal can be listed by its `project_id`.

The tool catalog is [reference/mcp-info.md](../docs/reference/mcp-info.md).
Connecting a client is [how-to/connect-an-mcp-client.md](../docs/how-to/connect-an-mcp-client.md).

## Where to next

- [handbook/leads-deals-and-projects.md](../docs/handbook/leads-deals-and-projects.md):
  who can create, edit and archive a project; the key; the project page; starting
  delivery.
- [handbook/capture.md](../docs/handbook/capture.md): every rule by which email
  finds its project, and what filing does to retention.
