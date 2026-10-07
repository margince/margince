<!-- prose:plain -->
# Roles, teams, and record sharing

This page goes with [authorization.md](authorization.md). That page says **where** the access check
lives: at the store, with the transaction seam and the app role's own grants below it. It also says
how the three transports reach one gate. This one covers the **data that gate reads**. That is what a
role grants and how row scope narrows it. It is also how teams make it wider, and how a share of
one record adds to that.

If a new user gets `permission denied` on every screen, skip to
[A user with no role sees nothing](#a-user-with-no-role-sees-nothing). In most cases, that is why.

## Three separate questions

A read or write is allowed only when all three pass. They are separate gates; opening one wider does
not stand in for another.

1. **Admission**: *may this caller act at all?* Scope ∧ seat limit ∧ autonomy tier. (See
   [authorization.md](authorization.md); not covered here.)
2. **Object RBAC**: *may this role do this action on this **type** of record?* Such as "may a
   `rep` `read` a `deal`?" The **role permissions** of the caller decide. If not → **403**.
3. **Row scope**: *may this caller see this **one** record?* Such as "may this rep read *deal
   #42*?" **Row scope + record grants** decide. If not → **404**, which hides that the row exists. A
   row you cannot see looks the same as one that does not exist.

The thing the whole feature turns on: **a record share only answers question 3.** It never grants
question 2. Sharing a deal with someone whose role has no `deal.read` still refuses them. The share
does nothing for them until they have a role that passes the object gate.

## Roles

A role is a row in the `role` table (`backend/migrations/core/0001_baseline.up.sql`), scoped to one
workspace. Its `permissions` JSON holds two things:

- **`objects`**: for each object type, a grant of `{create, read, update, delete}` over the core
  objects (`contact`, `company`, `deal`, `lead`, `activity`, `pipeline`, `list`, `custom_field`,
  `offer_template`, …). The closed set is `policy.coreObjects`, published in full in
  [reference/rbac-matrix.md](../reference/rbac-matrix.md).
- **`row_scope`**: `own` | `team` | `all` (see below).

A new workspace starts with 6 **system roles** (`is_system = true`). Their grants are compiled in
and are the one true source, so do not copy the full table into another place; the copy will drift.
Read it in **`backend/internal/modules/identity/internal/policy/defaults.go`** (`defaults`), or in
full in [reference/rbac-matrix.md](../reference/rbac-matrix.md), which a test builds from those same
values. The shape:

| Role | What it may do | Row scope |
|---|---|---|
| `admin` | Full CRUD on everything (config included). | `all` |
| `management` | The `manager` object grants, over every row: the sales leader. | `all` |
| `ops` | The same CRUD reach as admin: the ops side. | `all` |
| `manager` | CRUD on records; **read-only** on most config (pipeline, `automation`, `custom_field`); **no access at all** to the admin-only tables (`fx_rate`, `ai_model_rate`, `embedding_reindex`, `import_run`). | `team` |
| `rep` | Create, read and update records (delete only where it is normal work, such as to drop a lead that will not buy); **read-only** on config. | `own` |
| `read_only` | Reads every kind of record and every config surface a rep can see; writes nothing but its own saved views. The four admin-only tables (`fx_rate`, `ai_model_rate`, `embedding_reindex`, `import_run`) are closed to it, even for reading. | `all` |

Three things new readers do not expect:

- **`read_only` is `row_scope: all`, and `rep` is `row_scope: own`.** Scope and object reach are
  separate. A read-only reviewer is *there* to see the whole workspace and write none of it. A rep
  reads every record and writes only their own. On the customer record tables, scope does
  not decide the read tier at all (see *Reads* below); scope decides writes.
- **Of seeded roles, only `manager` is `team`.** A Team Lead writes their team members'
  records as well as their own, worked out through live team membership. The seat above it,
  `management`, has the same object grants at `all`: the sales leader over every row.
- **Config objects are read-only below admin and ops.** That covers pipeline, `custom_field` and
  `automation`. So a `rep` sees an error such as `pipeline.read: permission denied` only when they
  have **no role at all**. With the `rep` role they *can* read pipelines; they only cannot edit them.

Custom roles add to the same shape. An admin makes one in **Settings → Roles and permissions** by
copying a role that exists. Then they rename it, move its row scope and switch its object grants
there. Archiving takes it out of use, while nobody who can sign in holds it. When a user holds
several roles, permissions **merge to the widest** one held: object grants join, and row scope
takes the widest, `all` > `team` > `own`. See `policy.Merge`.

## Row scope: which rows of an allowed object

SQL checks row scope at every list or read over a table with owners (`platform/auth/rowscope.go`).
It means different things for reads and for writes, and for two classes of table
(`platform/auth/tableclass.go`).

### Reads: customer identity is shared, sales work is scoped

**Identity tables are open to every granted seat.** Any seat that holds the object grant reads
them, with any row scope, for `contact`, `company`, `lead`, `deal` and `project`. Hiding
customer records per team makes a rep miss that a company is already a customer of another team,
and contact it again. So a rep finds the company, sees who owns it and when someone last touched it,
and cannot edit it. Deals are in this class because a deal a rep cannot see leads to a rep
contacting the same customer twice.

`project` is in it because someone put on a project must still open it. They may not own it, and
may have no grant to it. `platform/auth/tableclass.go` records the reason. Who may change sales work
is scoped; everyone with the grant may see it.

Two rules narrow reads on identity tables:

- **Capture privacy**: a row a connector made with `visibility = 'owner'` answers to its owner
  alone until a `promote` action runs on it, even for `row_scope: all`. It is a setting on the row,
  not on the scope tier.
- A **record grant** can still open up a private row of one owner. That is a direct share by someone
  who could already read it.

**The personal tables (`list`, `saved_view`, `automation`, `voice_profile`) keep the old row
scope.** They are a seat's own work, not a record of the business, so the scope check applies to
reads as well. Once the object gate has passed:

- **`all`**: no row filter. Sees every row in the workspace. (`Unbounded`; also the system actor.)
- **`team`**: sees rows they **own**, rows a **team member** owns (any member of a team they are in,
  through `team_membership`), and rows with **no owner**.
- **`own`**: sees rows they own, and rows with no owner.

### Writes: the owner, a direct share, or a seat with no limit

Row scope decides **who may change** an identity row. The write check
(`platform/auth/writescope.go`, `EnsureWritable`) is the owner check or a live `write` grant. A
rep who can read the deal of a colleague and tries to edit it gets **403**, not 404. The rep can see the
row, so there is nothing left for a 404 to hide.

**Team membership grants nothing to a `rep`.** The seeded `rep` has `own` scope. So for them, the
record of a colleague takes a direct share or a seat with no limit. The share is a `record_grant`
that names the user or one of their teams. Being in someone's team does not by itself allow you to change their
records.

**A `manager` gets their team members.** A Team Lead has `team` scope, so the owner check covers
them plus everyone who shares a live team with them. A lead who cannot work the records of their
team is a lead in name only. Membership sets the limit of that reach, not the reporting lines of the
company. An archived team grants nothing, and the check does not walk `parent_team_id`. A lead of a
team above reaches the members of a team below it only by being in that team too.

A record grant may still name a **team**, so sharing with a group is one act, not one per member.
It stays the way to reach across teams, and the way for every seat other than `manager`.

A row with **no owner** (`owner_id IS NULL`) is nobody's to change until someone claims it
(`EnsureClaimable`, `POST /v1/records/{record_type}/{id}/claim`). Claiming makes the claimer the
owner. Everyone can read the row the whole time.

A record carries the answer on the wire: `writable` on a contact, company, lead, deal or project
says whether **this** caller may change **this** row. So a client builds its edit buttons from the
same question the server answers. It is a signal for the screen, and never the check itself.

### Activities: who can find them and who can read them

An activity has no owner. It takes its visibility from the records it links to (the walk over every
link). A note with no link is shared with the workspace. On top of that sits an **audience** on
each activity (`activity.audience`, `activity_audience_member`):

- `workspace`: everyone who can find the row reads it;
- `participants`: the humans on it (the owner of the mailbox that captured it, and every seat
  marked as a participant);
- `selected`: the participants plus the users and teams a human named.

`workspace` is the default for a row a human logged. For a row a mailbox captured, it is worked
out. `activities.RecomputeAudienceTx` looks at the `capture_import` row of every seat that imported
it. It takes the setting that holds back the most: the mailbox's setting, the thread's verdict, and
that seat's counterparty holds. So a colleague whose mailbox shares cannot publish a message another
importer is holding, in any order the two imports run.

`activity.audience_reason` names the setting that holds back the most. It is held back with the
content, because the reason says what the message is about.

A direct `PATCH /activities/{id}/audience` on a captured row is refused (`audience_is_derived`), and
points at `POST /activities/threads/{key}/audience`. That drops the own setting of the caller, and reports
how many other seats still hold the thread: a count, never a name.

`auth.ActivityDiscoverClause` answers "may this seat learn this row exists" (date, direction, kind,
and who owns it: the last-touch marker). `auth.ActivityContentClause` answers "may this seat read
it". That covers
the subject, body, participants, attachments, and everything built from them: search, briefs,
exports, webhooks. A reader that serves content uses the content clause. A limited activity the
caller may find but not read is held back, and shows only the safe markers. The audience does
**not** give way to `row_scope: all`; only the system principal reads past it.

`owner_id` **may be empty**. A create by hand makes the user who creates it the owner when the
caller names nobody. It asks who to assign it to when they name someone else
(`storekit.NewRecordOwner`). A record cannot start on a seat that nobody could later hand it to. A
record can still come in with no owner, from an import or a connector that has no seat to name.

Everyone can read such a row, and **nobody** can write it until a seat claims it. If every seat could change a customer record with no
owner, two teams would edit one company past each other.

The seeded `rep` is `row_scope: own`, `manager` is `team`, and `read_only`, `ops`, `admin` and
`management` are `all`. The `team` tier is also what a record grant to a team is checked against,
and a custom role may claim it.

## Field masks: one column of a row you can read

A role can read a kind of record and still not read every column of it. `field_mask`
(`backend/migrations/core/…_field_mask.up.sql`) names, per **role key**, an object, a field and a
rule. The rule is `always`, or `outside_write_authority`: the caller can read the row but may not
change it. The masks load into the principal at sign-in with the grants; a seat carries the masks of
all its roles together.

They apply where a store maps the row on the wire (`platform/auth/fieldmask.go`,
`deals/fieldmask.go`). The field goes out as `null`, and the record
names it in `masked_fields`, so a reader can tell held-back from empty. Ordering or filtering a list
by a masked column is refused (422), because ordering by a value is reading it.

A seat with no limit (`row_scope: all`) carries no mask.

No seeded role carries a mask: deal values are open to every seat that may read the deal. A rep who
cannot see the value of a colleague's deal cannot judge their own pipeline against it. Operators can
still write masks on custom roles, and every path that applies one is tested.

**Masks name only fields in the catalog.** Holding back is written per field. A column with no rule
to hold it back is dropped, not applied. So a mask that names such a column would be accepted,
stored, read back with no change, and hide nothing. `maskable_field` holds the pairs this build can really
hold back: today the deal's money and its three references, which is the whole of
`deals/fieldmask.go`.

`field_mask` points at `maskable_field`, so the database refuses an operator who names anything else,
at the time they write it. Without that, a screen would later still show the number. So adding a mask
to a new column takes two parts in one change. One is the rule to hold it back. The other is the
migration that offers the pair. `migrations/testdata/maskable_fields.txt` is what fails when only one of them moves.

## Teams

A **team** (`team` table) is a named group; **`team_membership`** joins users to teams (many to
many: a user can be in several). Teams do several jobs:

1. **They are a share target.** A record grant can name a team instead of a user. Then everyone in
   it, members now and later, gets the wider access. Sharing with a group is one act, not one
   per member.
2. **They answer `row_scope: team`** for a role that carries it. Of the seeded roles only
   `manager` does. So putting a rep in a team does not by itself let them edit that team's records.
   An operator who wants colleagues to have write access to each other's records writes a custom
   role at `team` scope. The scope check still builds the arm for it.
3. **They decide who may coach which colleague.** They answer "may this seat coach the work of
   this colleague?". Row scope cannot answer it, because it is not about which rows may be read. Two surfaces ask it: a
   coaching notice into someone's Worklist, and the coaching layer on their meeting brief. Both ask
   it the same way and in the same order.

   First `auth.RequireCoach` checks the seat: a human who holds `team_lead.create`, seeded to
   `admin`, `management` and `manager`. `rep` holds nothing on it, or a rep on a team would coach
   their team members. Then a live shared team decides the edge, through one membership seam, so
   the two cannot drift. Membership comes from `team_membership` and live teams only; the check does
   not walk the tree of teams, the same as row scope.

   No surface makes wider what the asker may read. The coaching layer on a meeting brief adds to
   the brief that lead would get in any case. A lead and their rep still see two briefs of one
   meeting with different scopes, because every read here is scoped to the caller.

4. **They decide who reads a coaching week.** They answer "may this seat read this team's
   coaching week?". That is the closed week that names each member with a verdict their lead
   should take up with them. Row scope does not answer it, because a `read_only` seat reaches
   every record and leads nobody.

   `auth.TeamWeekReachOf` answers it once, for both `GET /weekly-reviews/team` and the
   `team_week` field of the Worklist. That field is what Home offers the week on, so Home never
   offers a week the server refuses. The `team` scope of the Worklist is the team's live work, and
   stays on row scope.

   Every arm needs `deal.read`, because the week carries the money on deals. A seat that holds
   `team_oversight.read` (seeded to `admin` and `management`) opens every team. A human seat that
   holds `team_lead.read` opens a team it is a live member of. `team_membership` records who is on a
   team, not who leads it, so the `team_lead` grant is what says "lead". Because it is a grant, a
   custom role can lead a team.

   Every other seat, and every seat with `own` scope, is refused with 403. A lead who asks about a
   team they are not on gets 404, so no one can test a team id to learn whether it exists.

**Only a real admin changes team membership.** That means adding or removing a member, archiving or
restoring the team, or inviting a member to it (`identity/teams.go`,
`refuseTeamMembershipUnlessAdmin`, 403 `team_membership_requires_admin`). Membership makes wider or
ends a member's team reach, and decides who leads and coaches them. That is role authority, and
`team_admin` does not have it. A holder of `team_admin` creates and renames teams; both leave the
reach of every member as it is.

Teams do **not** carry their own permissions; a team is not a role. (An *assigned* role can be
scoped to a team, but the grants still come from the role.)

## A user with no role sees nothing

`role_assignment` links a user to a role. **A user with no assigned role has no object
permissions.** Every object gate (question 2) fails closed, so every list and record answers 404 or
403, even the pipeline board. Row scope has no part in it; the user cannot pass the object gate for
anything.

The workspace setup gives the first admin the `admin` role (`identity/service.go`,
`seedSystemRoles`). Any user made by another path, such as a SQL seed or an invite path still to
come, **must get a role directly**. If not, they sign in to a screen full of permission errors. `scripts/seed-dev.sql`
gives `rep` to its second user for this reason.

## Record sharing: a grant per record on top of scope

Row scope has only three steps (own / team / all). **Record sharing** works on one record at a time.
It grants **one record** to **one user or team**, at **read or write**, with a reason. It can also
carry an end date.
This is the Share screen (`frontend/src/screens/share.tsx`, `#/share/<type>/<id>`) and the
`record_grant` table and `/v1/record-grants` API.

How it works with everything above:

- **It only opens up question 3 (row visibility).** It never opens up question 2 (object RBAC). The
  user who gets the grant still needs a role that grants the action on that object type. Share a deal
  with a user whose role has no `deal.read`, and they still cannot open it. The grant does nothing
  until their role passes the object gate.
- **It applies only to tables you can share**: `contact`, `company`, `deal`, `lead`, `project`.
  See `shareableTables` in `rowscope.go`; the `record_grant` CHECK is the same rule on the database
  side. Config and other objects have no share per record. On an identity table, a `read` grant only
  counts for a private captured row of one owner; a `write` grant is what opens up editing.
- **A `write` grant covers a read** (write ⊇ read).
- **SQL checks it live on every query**: the visibility check
  `OR EXISTS (…record_grant… AND (expires_at IS NULL OR expires_at > now()))`. So **a share that is
  revoked or past its end date stops at the next read**, with no session to wait out.
- **A grant cannot go past its granter.** The server refuses a grant wider than the
  own access of the granter to that record (shown as `approval_required` / 422 in the UI). So sharing
  cannot be a way to get more access than you have.

In SQL, a read over a table you can share is `ownerPredicate OR liveGrantExists`. The grant is
a second way in, checked in the same statement as the scope filter (`VisiblePredicate` in
`rowscope.go`).

## A worked case: the dev seed

The dev seed (`scripts/seed-dev.sql`) sets up three seats, so you can see every branch above:

- **Demo Admin**: `admin` role, `row_scope: all`, member of **DACH Sales**. Owns the seeded contacts
  and deals; sees everything.
- **Rep One**: `rep` role, `row_scope: own`, member of **DACH Sales** (with Demo Admin). *The
  seat that gets a share.*
  - Object gate: `rep` grants `deal.read` and `pipeline.read` (read-only) → the deals board loads.
    It shows Demo Admin's records the same as everyone else's: every seat can read customer identity.
  - Row scope: `own` → being in Demo Admin's team gives Rep One nothing. Every one of Demo Admin's
    records is **open to read, not to edit**. Saving answers 403, and the record's `writable` flag
    says so before you try to save.
  - The seed shares **one** of Demo Admin's contacts with Rep One at `write`. That record, and only
    that record, is theirs to change, which makes the grant the reason you can see.
- **Rep Two**: `individual` role (a copy of `rep`), **in no team**. *The seat with no shares.*
  - The object gate passes (the same object grants as `rep`) → the board loads and shows every
    deal, read-only.
  - Row scope: `own` → owns nothing and holds no grant → may **edit nothing**.
  - What Rep One has and Rep Two does not is the grant. Both seats have `own` scope, and one of
    them has a record shared with it.

Remove every role a user holds and every read fails at the object gate (403 or 404 across the
board). That sign means "no role", which is not the same as "role held, but scope hides the row".

## Where this is enforced

- Role setup: `backend/internal/modules/identity/internal/policy/defaults.go`; merge:
  `policy.go`
- The object gate: `backend/internal/platform/auth/rbac.go`
  (`Require`, `RequireAny`, `UpsertAction`, `RequireHuman`, `RequireAdmin`)
- The SQL for row scope and record grants: `backend/internal/platform/auth/rowscope.go`
  (`OwnerPredicate`, `VisiblePredicate`, `ScopeClauseFor`, `shareableTables`). The read classes are
  in `tableclass.go`, and the activity find and content gates in `inheritedscope.go`
- Tables: `role`, `role_assignment`, `team`, `team_membership`, `record_grant`
  (`backend/migrations/core/`)
- How the checks are built (one gate, three transports, and a code check behind them):
  [authorization.md](authorization.md)
