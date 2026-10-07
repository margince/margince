# Roles, teams, and record sharing

The companion to [authorization.md](authorization.md). That page explains **where** the access check
lives (at the store, with the transaction seam and the app role's own grants beneath) and how the
three transports resolve one gate. This one covers the **data model that gate reads**: what a role
grants, how row scope narrows it, how teams widen it, and how a single-record share layers on top.

If you just watched a freshly-created user get "permission denied" on every screen, skip to
[A user with no role sees nothing](#a-user-with-no-role-sees-nothing); that is almost always why.

## Three independent questions

A read or write is allowed only when all three pass. They are separate gates; widening one does not
substitute for another.

1. **Admission**: *may this caller act at all?* Scope ∧ seat ceiling ∧ autonomy tier. (See
   authorization.md; not covered here.)
2. **Object RBAC**: *may this role do this verb on this **type** of record?* e.g. "may a `rep`
   `read` a `deal`?" Decided by the caller's **role permissions**. Failure → **403**.
3. **Row scope**: *may this caller see this **particular** record?* e.g. "may this rep read *deal
   #42*?" Decided by **row scope + record grants**. Failure → **404**, which hides existence: a row
   you can't see is indistinguishable from one that doesn't exist.

The trap the whole feature hinges on: **a record share only answers question 3.** It never grants
question 2. Sharing a deal with someone whose role has no `deal.read` still denies them, and the
share is invisible until they have a role that clears the object gate.

## Roles

A role is a row in the `role` table (`backend/migrations/core/0001_baseline.up.sql`), scoped to one
workspace. Its `permissions` JSONB holds two things:

- **`objects`**: a per-object-type grant of `{create, read, update, delete}` over the core objects
  (`contact`, `company`, `deal`, `lead`, `activity`, `pipeline`, `list`, `custom_field`,
  `offer_template`, …). The closed set is `policy.coreObjects`, published cell-by-cell in
  [reference/rbac-matrix.md](../reference/rbac-matrix.md).
- **`row_scope`**: `own` | `team` | `all` (see below).

A fresh workspace is seeded with six **system roles** (`is_system = true`). Their grants are
compiled in and are the source of truth, so do not transcribe the full matrix elsewhere; it will
drift. Read it in **`backend/internal/modules/identity/internal/policy/defaults.go`** (`defaults`),
or cell by cell in [reference/rbac-matrix.md](../reference/rbac-matrix.md), which a test renders
from those same values. The shape:

| Role | Posture | Row scope |
|---|---|---|
| `admin` | Full CRUD on everything (config included). | `all` |
| `management` | The `manager` object grid, over every row: the sales leader. | `all` |
| `ops` | Same CRUD reach as admin: the operations counterpart. | `all` |
| `manager` | CRUD on records; **read-only** on most config (pipeline, automation, custom_field); **no access at all** to the admin-only sheets (`fx_rate`, `ai_model_rate`, `embedding_reindex`, `import_run`). | `team` |
| `rep` | Create/read/update records (delete only where it's routine, e.g. disqualify a lead); **read-only** on config. | `own` |
| `read_only` | Reads every record kind and every config surface a rep can see; writes nothing except its own saved views. The four admin-only sheets (`fx_rate`, `ai_model_rate`, `embedding_reindex`, `import_run`) are closed to it entirely, even for reading. | `all` |

Three things surprise newcomers:

- **`read_only` is `row_scope: all`, and `rep` is `row_scope: own`.** Scope and object reach are
  orthogonal. A read-only auditor is *meant* to see the whole workspace and write none of it, while
  a rep reads every record and writes only their own. On the customer-record tables scope does
  not decide the read tier at all (see *Reads* below); scope decides writes.
- **`manager` is the only `team`-scoped seeded role.** A Team Lead writes
  their teammates' records as well as their own, resolved through live team membership. The seat
  above it, `management`, is the same object grid at `all`: the sales leader over every row.
- **Config objects are read-only below admin/ops.** That covers pipeline, custom_field and
  automation. So a
  `rep` sees `pipeline.read: permission denied`-adjacent behaviour only when they have **no role at
  all**. With the `rep` role they *can* read pipelines; they just can't edit them.

Custom roles are additive on the same shape. An admin makes one in **Settings → Roles and
permissions** by copying an existing role, then renames it, moves its row scope and switches its
object grants there. Archiving takes it out of use while nobody who can sign in holds it. When a
user holds several roles, permissions **merge to the widest** held (object grants union; row scope
takes the widest, `all` > `team` > `own`); see `policy.Merge`.

## Row scope: which rows of a permitted object

Row scope is evaluated in SQL at every list/read over an owner-scoped table
(`platform/auth/rowscope.go`). It means different things for reads and for writes, and for two
classes of table (`platform/auth/tableclass.go`).

### Reads: customer identity is shared, commercial work is scoped

**Identity tables are readable by every granted seat.** Any seat holding the object grant reads them, whatever the
seat's row scope, for `contact`, `company`, `lead`, `deal` and `project`. Hiding customer records per team makes a rep miss
that a company is already a customer of another team, and contact it again. So a rep finds the
company, sees who owns it and when it was last touched, and cannot edit it. Deals are in this class
because a deal a rep cannot see causes the same duplicate outreach. `project` is in it because a
consultant staffed to a project they neither own nor were granted must still open it
(`platform/auth/tableclass.go` records the reasoning). Commercial work is scoped by who may change
it; everyone with the grant may see it.

Two narrowings apply on identity tables:

- **Capture privacy**: a row a connector minted as `visibility = 'owner'` answers to its owner
  alone until it is promoted, even for `row_scope: all`. It is a property of the row rather than of
  the scope tier.
- A **record grant** can still widen an owner-private row (an explicit share by someone who could
  already read it).

**The personal tables (`list`, `saved_view`, `automation`, `voice_profile`) keep the classic row
scope.** They are a seat's own working material rather than a record of the business, so the
predicate applies to reads as well. Given the object gate already passed:

- **`all`**: no row filter. Sees every row in the workspace. (`Unbounded`; also the system actor.)
- **`team`**: sees rows they **own**, rows owned by a **teammate** (any member of a team they belong
  to, via `team_membership`), and **ownerless** rows.
- **`own`**: sees rows they own, and ownerless rows.

### Writes: the owner, an explicit share, or an unbounded seat

Row scope decides **who may change** an identity row. The write-authority probe
(`platform/auth/writescope.go`, `EnsureWritable`) is the owner predicate or a live `write` grant. A
rep who can read a colleague's deal and tries to edit it gets **403** rather than 404: the row is
visibly theirs to read, so there is nothing left for a 404 to hide.

**Team membership grants nothing to a `rep`.** The seeded `rep` is `own`-scoped, so for them a
colleague's record takes an explicit share (a `record_grant` naming the user or one of their teams)
or an unbounded seat. Being in somebody's team is not by itself permission to rewrite their records.

**A `manager` gains their teammates.** A Team Lead is `team`-scoped, so the owner
predicate resolves to themselves plus everyone sharing a live team with them. A lead who cannot
work their team's records is a lead in name only. The reach is bounded by membership rather than by
the company chart: an archived team grants nothing, and `parent_team_id` is not walked. Leading a
parent team reaches a child team's members only by belonging to that team too.

A record grant may still name a **team**, so sharing with a group is one act rather than one per
member. It stays the mechanism for reaching across teams and for every seat other than `manager`.

An **ownerless** row (`owner_id IS NULL`) is nobody's to change until somebody claims it
(`EnsureClaimable`, `POST /v1/records/{record_type}/{id}/claim`); claiming makes the claimer the
owner. It stays readable by everyone throughout.

A record carries the answer on the wire: `writable` on a contact, company, lead, deal or project
says whether **this** caller may change **this** row, so a client draws its edit affordances from the
same question the server answers. It is a UX signal and never the enforcement.

### Activities: discoverable versus readable

An activity has no owner; it inherits visibility from the records it links to (the any-link walk),
and a link-less note is workspace-shared. On top of that sits a per-activity **audience**
(`activity.audience`, `activity_audience_member`):

- `workspace`: everyone who can discover the row reads it;
- `participants`: the humans on it (the capturing mailbox owner, anyone stamped as a participant
  by seat);
- `selected`: the participants plus the users and teams a human named.

`workspace` is the default for a row a human logged. For a row a mailbox brought in it is derived:
`activities.RecomputeAudienceTx` takes the strictest contribution across every importing seat's
`capture_import` row (the mailbox's posture, the thread's verdict, that seat's counterparty holds).
So a colleague whose mailbox shares cannot publish a message another importer is holding, in
whatever order the two syncs ran. `activity.audience_reason` names the strictest contributor, and
is withheld with the content, because the reason describes what the message is about.

A direct `PATCH /activities/{id}/audience` on a captured row is refused (`audience_is_derived`) and
points at `POST /activities/threads/{key}/audience`. That releases the caller's own contribution and
reports how many other seats still hold the thread: a count, never a name.

`auth.ActivityDiscoverClause` answers "may I learn this row exists" (date, direction, kind, who owns
it: the last-touch marker). `auth.ActivityContentClause` answers "may I read it" (subject, body,
participants, attachments, and everything derived from them: search, briefs, exports, webhooks).
A reader that serves content composes the content clause; a limited activity the caller may discover
but not read is withheld, with only the safe markers shown. The audience does **not** yield to
`row_scope: all`; only the system principal reads past it.

`owner_id` is **optional**. A manual create stamps the creator when the caller names nobody, and
asks the assignment question when they name somebody else (`storekit.NewRecordOwner`): a record
cannot be born on a seat no handover could hand it to. A record can still arrive without an owner,
from an import or a connector that had no seat to attribute. Such a row is readable by everyone and
writable by **nobody** until a seat claims it. If every seat could rewrite an ownerless customer
record, two teams would edit one company past each other.

The seeded `rep` is `row_scope: own`, `manager` is `team`, and `read_only`, `ops`, `admin` and
`management` are `all`. The `team` tier is also what a team-subject record grant resolves against,
and a custom role may claim it.

## Field masks: one column of a readable row

A role can read a kind of record and still not read every column of it. `field_mask`
(`backend/migrations/core/…_field_mask.up.sql`) names, per **role key**, an object, a field and a
condition: `always`, or `outside_write_authority` (the row is readable but not the caller's to
change). The masks are loaded into the principal at login with the grants (a seat carries the union
over its roles). They are applied where a store maps the row onto the wire (`platform/auth/fieldmask.go`,
`deals/fieldmask.go`): the field goes out `null` and the record names it in `masked_fields`, so a
reader can tell withheld from empty. Sorting or filtering a list by a masked column is refused
(422), because ordering by a value is reading it. An unbounded seat (`row_scope: all`) carries no
mask.

No seeded role carries a mask: deal amounts are open to every seat that may read the deal. A rep
who cannot see what a colleague's deal is worth cannot judge their own pipeline against it.
Operators can still author masks on custom roles, and every path that applies one is tested.

**Masks name only catalogued fields.** Withholding is written per field: a column with no
withhold rule is dropped rather than applied, so a mask naming one would be accepted, stored, read
back unchanged, and hide nothing. `maskable_field` holds the pairs this build can actually withhold:
today the deal's money and its three references, which is the whole of `deals/fieldmask.go`.
`field_mask` references it, so the database refuses an operator naming anything else where they
write it, instead of a screen later still showing the number. Extending a mask to a new column is
therefore two halves in one change: the withhold rule, and the migration that offers the pair.
`migrations/testdata/maskable_fields.txt` is what fails when only one of them moves.

## Teams

A **team** (`team` table) is a named group; **`team_membership`** joins users to teams (many-to-many:
a user can be in several). Teams do several jobs:

1. **They are a share target.** A record grant can name a team instead of a user, so everyone in
   it, present and future members, gets the widened access. Sharing with a group is one act rather
   than one per member.
2. **They resolve `row_scope: team`** for a role that carries it. Of the seeded roles only
   `manager` does: putting a rep in a team does not by itself let them edit that team's records. An
   operator who wants standing write access among colleagues authors a custom role at `team` scope,
   and the predicate still renders the arm for it.
3. **They decide who may coach whom.** They answer "may I speak into this colleague's work?",
   which row scope cannot answer because it is not about which rows may be read. Two surfaces ask it: raising a coaching notice into
   somebody's Worklist, and the coaching layer on their meeting brief. Both ask it the same way and
   in the same order. First `auth.RequireCoach` checks the seat: a human holding
   `team_lead.create`, seeded to `admin`, `management` and `manager`. `rep` holds nothing on it,
   or a rep on a team would coach their teammates. Then a live shared team decides the edge,
   through one membership seam so the two cannot drift. Membership resolves through
   `team_membership` and live teams only; the parent hierarchy is not walked, matching row scope.

   Neither surface widens what the asker may read. The coaching layer on a meeting brief attaches
   to the brief that lead would have got anyway. A lead and their rep still see two differently
   scoped briefs of one meeting, because every read here is caller-scoped.

4. **They decide who reads a coaching week.** They answer "may I read this team's coaching
   week?", the frozen week that names each member
   with a verdict their lead is meant to raise. Row scope does not answer it, because a `read_only`
   seat reaches every record and leads nobody. `auth.TeamWeekReachOf` answers it once, for both
   `GET /weekly-reviews/team` and the Worklist's `team_week` field, which is what Home offers the
   week on, so Home never offers a week the server refuses. The Worklist's `team` scope is the
   team's live work and stays on row scope. Every arm needs `deal.read`, because the week carries
   deal totals. A seat holding `team_oversight.read` (seeded to `admin` and `management`) opens
   every team. A human seat holding `team_lead.read` opens a team it is a live member of.
   `team_membership` records who is on a team, not who leads it, so the `team_lead` grant is what
   says "lead". Because it is a grant, a custom role can lead a team. Every other seat, and every
   own-scoped seat, is refused with 403. A lead asking about a team they are not on gets 404, so a
   team id cannot be probed for existence.

**Only a literal admin changes team membership**: adding or removing a member, archiving or
restoring the team, or inviting a member onto one (`identity/teams.go`,
`refuseTeamMembershipUnlessAdmin`, 403 `team_membership_requires_admin`). Membership widens or
ends a member's team reach and decides who leads and coaches them, which is role authority, and
`team_admin` is not. A holder of `team_admin` creates and renames teams; neither changes anybody's
reach.

Teams do **not** carry their own permissions; a team is not a role. (A role *assignment* can be
scoped to a team, but the grants still come from the role.)

## A user with no role sees nothing

`role_assignment` links a user to a role. **A user with zero role assignments has zero object
permissions.** Every object gate (question 2) fails closed, so every list and record 404/403s, even
the pipeline board. Row scope plays no part; the user simply cannot clear the object gate for
anything.

The workspace bootstrap assigns the founding admin the `admin` role (`identity/service.go`,
`seedSystemRoles`). Any user created by another path (a SQL seed, a future invite flow) **must be
given a role explicitly**, or they sign in to a wall of permission errors. `scripts/seed-dev.sql`
assigns `rep` to its second user for this reason.

## Record sharing: a per-record grant on top of scope

Row scope is coarse (own / team / all). **Record sharing** is the fine-grained layer:
grant **one specific record** to **one user or team**, at **read or write**, optionally expiring,
with a reason. This is the Share screen (`frontend/src/screens/share.tsx`, `#/share/<type>/<id>`) and
the `record_grant` table / `/v1/record-grants` API.

How it composes with everything above:

- **It only widens question 3 (row visibility).** It never widens question 2 (object RBAC). The grantee still
  needs a role granting the verb on that object type. Share a deal with a user whose role lacks
  `deal.read` and they still can't open it; the grant is inert until their role clears the object
  gate.
- **It applies only to shareable tables**: `contact`, `company`, `deal`, `lead`, `project`
  (`rowscope.go` `shareableTables`; the `record_grant` CHECK is the schema-side twin). Config and
  other objects have no per-record share. On an identity table a `read` grant only matters for an
  owner-private captured row; a `write` grant is what widens editing.
- **A `write` grant satisfies a read** (write ⊇ read).
- **It is evaluated live on every query**: the visibility predicate `OR EXISTS (…record_grant… AND (expires_at IS NULL OR expires_at > now()))`. So **revoking or expiring a share binds on the
  next read**, with no session to wait out.
- **A grant can't exceed the granter.** The server rejects a grant wider than the granter's own
  access to that record (surfaced as `approval_required` / 422 in the UI), so sharing can't launder
  privilege.

In SQL terms, a read over a shareable table is `ownerPredicate OR liveGrantExists`: the grant is a
second way in, checked in the same statement as the scope filter (`VisiblePredicate` in `rowscope.go`).

## Worked example: the dev seed

The dev seed (`scripts/seed-dev.sql`) sets up three seats so every branch above is observable:

- **Demo Admin**: `admin` role, `row_scope: all`, member of **DACH Sales**. Owns the seeded contacts
  and deals; sees everything.
- **Rep One**: `rep` role, `row_scope: own`, member of **DACH Sales** (with Demo Admin). *The
  shared-with seat.*
  - Object gate: `rep` grants `deal.read`, `pipeline.read` (read-only) → the deals board loads, and
    shows Demo Admin's records like everyone else's: customer identity is workspace-readable.
  - Row scope: `own` → being in Demo Admin's team buys Rep One nothing. Every one of Demo Admin's
    records is **readable and not editable**: pressing save answers 403, and the record's `writable`
    flag says so before you press it.
  - The seed shares **one** of Demo Admin's contacts with Rep One at `write`. That record, and only
    that record, is theirs to change, which makes the grant the observable cause.
- **Rep Two**: `individual` role (a clone of `rep`), **in no team**. *The nothing-shared seat.*
  - Object gate passes (same object grants as `rep`) → the board loads and shows every deal, read-only.
  - Row scope: `own` → owns nothing and holds no grant → may **edit nothing**.
  - The contrast with Rep One is the grant: two own-scoped seats, one of which has been handed a
    record.

Remove a user's role assignment entirely and every read fails at the object gate (403/404 across the
board). That symptom means "no role", which differs from "role present but scope hides the row".

## Where this is enforced (pointers)

- Role definitions: `backend/internal/modules/identity/internal/policy/defaults.go`; merge:
  `policy.go`
- Object-level gate: `backend/internal/platform/auth/rbac.go`
  (`Require`, `RequireAny`, `UpsertAction`, `RequireHuman`, `RequireAdmin`)
- Row-scope + record-grant SQL predicates: `backend/internal/platform/auth/rowscope.go`
  (`OwnerPredicate`, `VisiblePredicate`, `ScopeClauseFor`, `shareableTables`); the read classes in
  `tableclass.go`; the activity discover/content gates in `inheritedscope.go`
- Schema: `role`, `role_assignment`, `team`, `team_membership`, `record_grant`
  (`backend/migrations/core/`)
- The enforcement architecture (one gate, three transports, structural backstop):
  [authorization.md](authorization.md)
