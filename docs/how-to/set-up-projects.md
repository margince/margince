# Set up projects

This guide is for the person who decides how projects are used in the team — no
code, no API. It says who can do what with a project, how to choose keys, when
to create a project, how visibility and sharing work, and what the fixed
vocabularies are. The things you *cannot* do from the UI are listed at the end.

New to projects? [Walk one from the deal to close](../../user-guide/run-your-first-project.md)
first; this page is the reference you come back to. For the day-to-day page — the
project page section by section, and how email finds its project — see
[run-a-project.md](run-a-project.md).

## What a project is in Margince

A project is **the body of work a deal is about**. It carries several deals
over time and outlives any one of them: it exists before the first deal is won
and keeps running through delivery after it.

It is not a folder you create at close-won. It is born in the **Initiative**
phase while the deal is still open, so the early conversations are already
filed where the delivery team will look. Everything filed under it — mail,
notes, tasks, contracts, documents, stakeholders — shows on one page, and the
page carries the project's whole phase history with the reason for every move.

**A project is worked by several companies.** Typically one is its customer and
the others are the partners and subcontractors delivering it. Each company on
the project carries a role — **Customer**, **Partner** or **Subcontractor** —
and a project keeps at least one company at all times.

The roles are a description, not a constraint: nothing stops two companies both
holding **Customer**, or none of them holding it. Where the product needs a
single customer — the company shown in the project list's column — it takes the
first one holding that role.

A deal has at most one project, and the deal's company must be **one of the
companies on that project** — in any role. A partner's deal on a project it
subcontracts is ordinary; a deal on a company the project has never heard of is
refused.

## Who can do what

Permissions come from the role's grant on the **project** object, plus the
role's row scope for writes. The seeded roles:

| Role | Create | Read | Edit, move phase, seat stakeholders | Archive | May edit |
|---|---|---|---|---|---|
| **Rep** | yes | yes | yes | no | own, team-owned and unowned projects |
| **Manager** | yes | yes | yes | yes | own, team-owned and unowned projects |
| **Management** | yes | yes | yes | yes | every project |
| **Admin** / **Ops** | yes | yes | yes | yes | every project |
| **Read only** | no | yes | no | no | nothing |

Two consequences:

- **Every seat that holds the read grant reads every project**, whatever its
  row scope. A project is a customer-identity record like a contact, company,
  lead or deal: the consultant delivering a project they neither own nor were
  granted still opens it. Row scope restricts **writes** only — a rep edits
  the projects they own, a teammate owns, or nobody owns; the **Owner** field
  is what that check reads. The reasoning is in
  [explanation/rbac-roles-and-teams.md](../explanation/rbac-roles-and-teams.md).
- A rep can create and run a project but cannot archive one. Archiving removes
  the project from the live list and frees its key, and it cannot be undone
  from the UI, so it stays with manager, management, admin and ops.

An admin assigns roles in **Settings → Users & teams**; the user's card
there shows, under **What this user sees**, what their role grants on
**Projects**. The grants themselves are seeded per role and are not editable
in the app.

## Keys

Every project gets a key — a short handle like `NER-1` that appears beside its
name and, in square brackets, in email subjects. **You do not choose it.**
Margince mints it from the project's name when the project is created, and it
cannot be edited afterwards. An API caller that sends a `key` is refused with
**422 `read_only`**.

### How a key is built

The **stem** comes from the name: the initials of a multi-word name, or the
first eight letters of a single-word one. Then a hyphen and the **lowest free
number** for that stem.

| Project name | Key |
|---|---|
| `Nordwind ERP rollout` | `NER-1` |
| `ERP rollout Acme` | `ERA-1` |
| a second `ERP rollout Acme` | `ERA-2` |
| `Datenmigration` | `DATENMIG-1` |
| `7 Eleven Rollout` | `ER-1` |
| `2026` | `PRJ-1` |

Only ASCII letters and digits contribute, and leading digits are dropped so a
name opening with a year still yields a usable stem. A name leaving fewer than
two usable characters falls back to the stem `PRJ`.

The number is the lowest one **free**, not the next one up — archiving a
project releases its key, and the next project with that stem takes the number
back.

### The rules the product enforces

- **Every project created through the product has one**, and there is no way to
  create one without. The field is nullable in the schema, so a row from before
  minting existed could still carry none; nothing you create today will.
- **Unique among live projects**, compared case-insensitively: `ner-1` and
  `NER-1` are the same key.
- **Read-only** on create and on update.
- **2–24 characters**, starting with a letter. This is the storage shape; since
  you cannot type a key, it only matters when you read one.

### Naming a project so its key reads well

The key is the one thing your customer sees in every subject line, and the
project's **name** is your only lever on it. Two habits are worth having:

- **Lead with the customer, then the work.** `Nordwind ERP rollout` gives
  `NER-1`. `ERP rollout` gives `ER-1`, which tells a reader nothing about
  whose rollout it is once a second customer buys the same thing.
- **Three or four words beats one.** A single-word name spends its whole key on
  the opening letters of that word — `Datenmigration` becomes `DATENMIG-1`,
  which is long and still says nothing about the customer.

You cannot change a key by renaming the project later: the key is minted once,
at creation. If a key is genuinely wrong, the honest fix is to archive the
project and create it again under a better name — which also frees the old key.

### Telling the customer

The key only files their mail when it is in the subject, and the surest way to
get it there is to let them reply to a message that already carries it.
Margince stamps the key into the subject when you **reply** from a deal or a
project (see [run-a-project.md](run-a-project.md#sending)); the account-started
composer on a company page files by link instead and does not touch the
subject, so type the key yourself when you start a conversation there. Mail
clients generally keep the subject on a reply, but nothing guarantees it — one
line in the kickoff mail, *please keep `[NER-1]` in the subject*, covers the
case where somebody rewrites it.

## When to create a project

**At deal creation (recommended).** The deal form's **Project** field offers
every live project on the deal's company and a **New project…** entry that
creates one on the spot, with a name and a key. The project starts in
**Initiative**; move it to **Pursuing** once the deal is genuinely in play.
The benefit is that proposal-stage mail is already filed under the project on
the day delivery starts.

**At close-won.** When a deal is won and names no project, and its company has
exactly one live project, the deal page shows a **Start delivery** callout
offering to attach the deal to that project and move the project into
delivery. Press **Start delivery** to accept; you land on the project page with
the phase already moved. This covers the deal that was opened before anyone
thought of a project.

**From the Projects list.** **Projects → New project** creates one directly,
with a company picker. Use this for work that is not (yet) tied to a deal.

Whichever way it is created, the moment a deal on the project is won the
project moves into **Delivering** by itself — but only from **Initiative** or
**Pursuing**. A project already delivering stays as it is, and a **Closed**
project is never reopened silently by a win; reopening is a human move with a
reason.

## Putting companies and people on a project

The same section appears on three pages, with the same two verbs, so the flow
is learned once:

| Page | Section | What it attaches |
|---|---|---|
| A project | **Companies** | companies onto this project |
| A company | **Projects** | this company onto a project |
| A contact | **Projects** | this person onto a project |

A deal is the exception, and deliberately so — see below.

**To attach**, press **Attach project** (or **Attach company** on a project
page), choose the role under **As**, then search and pick. **Picking is what
attaches** — there is no separate confirm — so set the role first. The role
list opens on **Partner** rather than Customer, because a project already has
its customer by the time you are adding anyone.

**To detach**, press **Detach** on the row. The dialog is explicit that nothing
is destroyed: *{name} stays as it is. Only its link to this record ends —
nothing is deleted.* Detaching is not archiving; archiving a project is done on
the project itself.

**Attaching a company that is already on the project changes its role** rather
than adding it twice. That is how you promote a subcontractor to a partner:
attach it again with the new role.

**Two removals are refused**, both with an explanation on screen:

- **The last company.** *A project keeps at least one company; add another
  before taking this one off.* A project belonging to nobody has no customer to
  bill and no timeline that means anything.
- **A company that still has deals here.** *This company still has N deal(s) on
  the project; move or close them before taking the company off.* The count is
  of deals that still exist, so winning or losing one does not clear it: point
  the deal at another project, or archive it.

**A deal is different** and deliberately so: a deal carries at most **one**
project, so it names it as a field on the deal form and shows it as a chip on
the deal page. There is no attach/detach section, because a second control
writing the same single value would be two ways to do one thing.

### Seating a stakeholder

Open the **contact**, not the project. The contact page's **Projects** section
attaches them with a role: **Sponsor**, **Project lead**, **Delivery lead**,
**Subject-matter expert** or **User**. The project page's **Stakeholders** card
shows the result and is read-only.

## Visibility and sharing

Every seat with the project read grant already sees every project, so a share
is not how somebody gets to *read* one. A share does two other things: it
lets somebody **edit** a project their row scope does not reach — a delivery
lead on another team who has to move the phase or seat stakeholders — and it
reaches a seat whose role carries no project grant at all. Open the project
and press **Share**. The sharing page grants **Read** or **Write** on exactly
this one record, to a person or a team, with an optional expiry and a reason.
A share is capped at your own access and never widens anything else about
that person's scope.

**Ownership** is set on the project form (**Owner**: *Me*, *Unassign*, or
keep the current owner). Reassigning one project at a time is an edit. Moving
*every* live project one person owns to another — a handover or a leaver —
exists only in the API today: `POST /projects/transfer-ownership`, which a
signed-in person calls (agents may not). Each moved project gets its own
audit entry with the owner before and after.

## The fixed vocabularies

**Phase** — where the project is. Four values, fixed; movement in either
direction is allowed; only closing needs a reason.

| Phase | You are here when… |
|---|---|
| **Initiative** | the work is an idea — a deal may exist, but nothing has been agreed to pursue it. The birth phase. |
| **Pursuing** | a deal for this work is genuinely in play: proposal out, evaluation running. |
| **Delivering** | a deal is won and the work is owed. Winning a deal puts the project here by itself. |
| **Closed** | the work has ended, with a reason on record. Can be reopened later. |

**Company role** — what a company is to this project. The picker offers three;
a project keeps at least one company at all times.

| Role | You are here when… |
|---|---|
| **Customer** | they are buying the work. Usually one, and set when the project is created. |
| **Partner** | they are delivering part of it alongside you, on their own commercial footing. |
| **Subcontractor** | they are delivering part of it under you. |

**Stakeholder role** — what a person is to this project. Seat somebody from the
**contact's** page, in its **Projects** section; the project page's
**Stakeholders** card shows the result and is read-only.

| Role | You are here when… |
|---|---|
| **Sponsor** | they pay for it or own the outcome on the customer side. |
| **Project lead** | they run it on the customer side. |
| **Delivery lead** | they run it on your side. |
| **Subject-matter expert** | they are consulted on a part of it. |
| **User** | they will use what is delivered, and are consulted as one. |

Neither list has a settings screen. Both are enforced end to end — in the API
contract ([`backend/api/crm.yaml`](../../backend/api/crm.yaml), the `Project`
and `ProjectStakeholder` schemas) and as database constraints — so adding a
value is a small code change plus a migration, not a workaround.

## Agents and MCP

An agent connected over MCP works under the same grants as the person whose
passport it carries. It can read a project (`read_project_360`, `read_record`
with record type `project`), create or update one through the generic record
tools, move a phase with `advance_project_phase`, and archive one through
`archive_record`.

Those writes **execute immediately** under a passport whose grants admit them —
they are not staged for approval. What confines an agent here is the passport's
own scopes and the seat behind it, not a confirmation step. It cannot transfer
ownership; that endpoint is human-only.

## What you can't change from the UI

- **Legal hold.** A project can be placed under legal hold, which freezes the
  correspondence filed under it against erasure and retention sweeps. The hold
  is set by an operator directly in the database, the same as for a person,
  company or deal; there is no switch in the app.
- **Bulk owner transfer** — API only, as above.
- **Un-archiving.** Archiving is final from the UI.
- **The phase and stakeholder-role lists** — code and migration, as above.
- **Retention of filed mail.** Filing an email under a project marks it as
  business correspondence under the German pack — six years from the end of
  the calendar year it was sent or received — and
  moving it off the project later does not undo that. Nothing in the UI
  shortens it; see the callout in
  [run-a-project.md](run-a-project.md#referencing-a-project-from-email).
