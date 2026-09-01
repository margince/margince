# Roles, teams, and record sharing

The companion to [authorization.md](authorization.md). That page explains **where** the access check
lives (at the store, with the transaction seam and the app role's own grants beneath) and how the
three transports resolve one gate. This
page explains the **data model that gate reads**: what a role grants, how row scope narrows it, how
teams widen it, and how a single-record share layers on top.

If you just watched a freshly-created user get "permission denied" on every screen, skip to
[A user with no role sees nothing](#a-user-with-no-role-sees-nothing) — that is almost always why.

## Three independent questions

A read or write is allowed only when all three pass. They are separate gates; widening one does not
substitute for another.

1. **Admission** — *may this caller act at all?* Scope ∧ seat ceiling ∧ autonomy tier. (See
   authorization.md; not covered here.)
2. **Object RBAC** — *may this role do this verb on this **type** of record?* e.g. "may a `rep`
   `read` a `deal`?" Decided by the caller's **role permissions**. Failure → **403**.
3. **Row scope** — *may this caller see this **particular** record?* e.g. "may this rep read *deal
   #42*?" Decided by **row scope + record grants**. Failure → **404** (existence-hiding — a row you
   can't see is indistinguishable from one that doesn't exist).

The trap the whole feature hinges on: **a record share only answers question 3.** It never grants
question 2. Sharing a deal with someone whose role has no `deal.read` still denies them — and the
share is invisible until they have a role that clears the object gate.

## Roles

A role is a row in the `role` table (`migrations/core/0002_identity.up.sql`), scoped to one
workspace. Its `permissions` JSONB holds two things:

- **`objects`** — a per-object-type grant of `{create, read, update, delete}` over the 29 core
  objects (`person`, `organization`, `deal`, `lead`, `activity`, `pipeline`, `list`, `custom_field`,
  `quota`, …). The closed set is `policy.coreObjects`, published cell-by-cell in
  [reference/rbac-matrix.md](../reference/rbac-matrix.md).
- **`row_scope`** — `own` | `team` | `all` (see below).

A fresh workspace is seeded with five **system roles** (`is_system = true`), whose exact grants are
compiled in and are the source of truth — do not transcribe the full matrix elsewhere, it will
drift. Read it in **`backend/internal/modules/identity/internal/policy/policy.go`** (`defaults`), or
cell by cell in [reference/rbac-matrix.md](../reference/rbac-matrix.md), which is rendered from those
same values by a test and so cannot drift from them. The shape:

| Role | Posture | Row scope |
|---|---|---|
| `admin` | Full CRUD on everything (config included). | `all` |
| `ops` | Same CRUD reach as admin — the operations counterpart. | `all` |
| `manager` | CRUD on records; **read-only** on most config (pipeline, automation, custom_field, quota); **no access at all** to the admin-only sheets (`fx_rate`, `ai_model_rate`, `embedding_reindex`, `import_run`). | `team` |
| `rep` | Create/read/update records (delete only where it's routine, e.g. disqualify a lead); **read-only** on config. | `own` |
| `read_only` | Reads every record kind and every config surface a rep can see; writes nothing except its own saved views. The four admin-only sheets (`fx_rate`, `ai_model_rate`, `embedding_reindex`, `import_run`) are closed to it entirely — not even read. | `all` |

Two things surprise people:

- **`read_only` is `row_scope: all`, and `rep` is `row_scope: own`.** Scope and object reach
  are orthogonal — a read-only auditor is *meant* to see the whole workspace and write none of it,
  while a rep reads every record and writes only their own. On the five customer-record tables the
  read tier is not what scope decides at all (see *Reads* below); scope decides writes.
- **`manager` is `row_scope: team`, and it is the only seeded role that is.** A Team Lead writes
  their teammates' records as well as their own, resolved through live team membership. The seat
  above it, `management`, is the same object grid at `all` — the sales leader over every row.
- **Config objects (pipeline, custom_field, automation, quota) are read-only below admin/ops.** This
  is why a `rep` gets `pipeline.read: permission denied`-adjacent behaviour only when they have **no
  role at all** — with the `rep` role they *can* read pipelines; they just can't edit them.

Custom roles are additive on the same shape. When a user holds several roles, permissions **merge to
the widest** held (object grants union; row scope takes the widest — `all` > `team` > `own`); see
`policy.Merge`.

## Row scope — which rows of a permitted object

Row scope is evaluated in SQL at every list/read over an owner-scoped table
(`platform/auth/rowscope.go`). It means different things for READS and for WRITES, and for two
classes of table (`platform/auth/tableclass.go`).

### Reads: customer identity is shared, commercial work is scoped

**Identity tables — `person`, `organization`, `lead`, `deal` — are readable by every seat that
holds the object grant, whatever its row scope.** The decision behind this (2026-08-19): the model
that hid customer records per team made a rep miss that a company was already a customer of another
team and contact it again. A rep now finds the company, sees who owns it and when it was last
touched, and cannot edit it. Deals are in this class deliberately — a workspace-wide deal count was
already the rule, and a deal a rep cannot see is the duplicate-outreach failure in a different coat.
Two narrowings survive on identity tables:

- **Capture privacy** — a row a connector minted as `visibility = 'owner'` answers to its owner
  alone until it is promoted, even for `row_scope: all`. It is a property of the row, not of the
  scope tier.
- A **record grant** can still widen an owner-private row (an explicit share by someone who could
  already read it).

**Commercial tables — `project` — keep the classic row scope.** Given the object gate already
passed:

- **`all`** — no row filter. Sees every row in the workspace. (`Unbounded` — also the system actor.)
- **`team`** — sees rows they **own**, rows owned by a **teammate** (any member of a team they belong
  to, via `team_membership`), and **ownerless** rows.
- **`own`** — sees rows they own, and ownerless rows.

The personal tables (`list`, `saved_view`, `automation`, `voice_profile`) keep the owner predicate
as well.

### Writes: the owner, an explicit share, or an unbounded seat

Row scope decides **who may change** an identity row: the write-authority probe
(`platform/auth/writescope.go`, `EnsureWritable`) is the owner predicate OR a live `write` grant. A
rep who can read a colleague's deal and tries to edit it gets **403**, not 404 — the row is visibly
theirs to read, so there is nothing left for a 404 to hide.

**Team membership grants nothing to a `rep`.** The seeded `rep` is `own`-scoped, so for them a
colleague's record takes an explicit share — a `record_grant` naming the user or one of their teams —
or an unbounded seat. Being in somebody's team is not by itself permission to rewrite their records.

**For a `manager` it grants exactly one thing: their teammates.** A Team Lead is `team`-scoped, so the
owner predicate resolves to themselves plus everyone sharing a live team with them. That is the seat's
purpose — a lead who cannot work their team's records is a lead in name only — and it is bounded by
membership rather than by the org chart: an archived team grants nothing, and `parent_team_id` is not
walked, so leading a parent team reaches a child team's members only by belonging to that team too.

A record grant may still name a **team**, so sharing with a group is one act rather than one per
member, and it stays the mechanism for reaching ACROSS teams and for every seat that is not this one.

An **ownerless** row (`owner_id IS NULL`) is nobody's to change until somebody claims it
(`EnsureClaimable`, `POST /v1/records/{record_type}/{id}/claim`); claiming makes the claimer the
owner. It stays readable by everyone throughout.

A record carries the answer on the wire: `writable` on a person, organization, lead, deal or project
says whether **this** caller may change **this** row, so a client draws its edit affordances from the
same question the server answers. It is a UX signal and never the enforcement.

### Activities: discoverable versus readable

An activity has no owner; it inherits visibility from the records it links to (the any-link walk),
and a link-less note is workspace-shared. On top of that sits a per-activity **audience**
(`activity.audience`, `activity_audience_member`):

- `workspace` — everyone who can discover the row reads it;
- `participants` — the humans on it (the capturing mailbox owner, anyone stamped as a participant
  by seat);
- `selected` — the participants plus the users and teams a human named.

`workspace` is the default for a row a human logged. For a row a MAILBOX brought in it is derived,
not defaulted: `activities.RecomputeAudienceTx` takes the strictest contribution across every
importing seat's `capture_import` row — the mailbox's posture, the thread's verdict, that seat's
counterparty holds — so a colleague whose mailbox shares cannot publish a message another importer
is holding, in whatever order the two syncs ran. `activity.audience_reason` names the strictest
contributor, and is withheld with the content: the reason describes what the message is about.

A direct `PATCH /activities/{id}/audience` on a captured row is refused (`audience_is_derived`) and
points at `POST /activities/threads/{key}/audience`, which releases the caller's own contribution
and reports how many other seats still hold the thread — a count, never a name.

`auth.ActivityDiscoverClause` answers "may I learn this row exists" (date, direction, kind, who owns
it — the last-touch marker); `auth.ActivityContentClause` answers "may I read it" (subject, body,
participants, attachments, and everything derived from them: search, briefs, exports, webhooks).
A reader that serves content composes the content clause; a limited activity the caller may discover
but not read is withheld, with only the safe markers shown. The audience does **not** yield to
`row_scope: all` — only the system principal reads the arm away.

Note that `owner_id` is **optional**. A manual create stamps the creator
(`storekit.OwnerOrActor`), but a record can still arrive without an owner — an import, a connector
that had no seat to attribute. Such a row is readable by everyone and writable by **nobody** until a
seat claims it, which is the opposite of what this paragraph used to say: an ownerless customer
record every seat could rewrite is how two teams edit one company past each other.

The seeded `rep` is `row_scope: own`, `manager` is `team`, and `read_only`, `ops`, `admin` and
`management` are `all`. The `team` tier is also what a team-subject record grant resolves against,
and a custom role may claim it.

## Field masks — one column of a readable row

A role can read a kind of record and still not read every column of it. `field_mask`
(`backend/migrations/core/…_field_mask.up.sql`) names, per **role key**, an object, a field and a
condition: `always`, or `outside_write_authority` — the row is readable but not the caller's to
change. The masks are loaded into the principal at login with the grants (a seat carries the union
over its roles) and applied where a store maps the row onto the wire (`platform/auth/fieldmask.go`,
`deals/fieldmask.go`): the field goes out `null` and the record names it in `masked_fields`, so a
reader can tell withheld from empty. Sorting or filtering a list by a masked column is refused
(422), because ordering by a value is reading it. An unbounded seat (`row_scope: all`) carries no
mask.

**No seeded role carries a mask.** The baseline shipped one — `rep` → `deal` → `amount_minor` →
`outside_write_authority`, so a rep read the value of a deal outside their write authority as
withheld — and it has been removed. Deal amounts are open to every seat that may read the deal.

It was written when a rep's write authority covered their whole team, so it hid other teams' numbers
and left the rep's own team visible. Once the seeded rep became `own`-scoped that same mask would
have blacked out every deal a rep does not personally own, which is a decision about what people may
SEE arrived at as a side effect of a decision about what they may WRITE. The product answer is that
deal values are open: a rep who cannot see what a colleague's deal is worth cannot judge their own
pipeline against it.

The machinery above stays and is not dead — an operator may author a mask on a custom role, and every
path that applies one is tested. What went is the row the product shipped.

## Teams

A **team** (`team` table) is a named group; **`team_membership`** joins users to teams (many-to-many
— a user can be in several). Teams do two jobs:

1. **They are a share target**, and this is now the primary job. A record grant can name a team
   instead of a person, so everyone in it — present and future members — gets the widened access.
   Sharing with a group is one act rather than one per member.
2. **They resolve `row_scope: team`** for a role that carries it. No seeded role does any more:
   putting a rep in a team does not by itself let them edit that team's records. An operator who
   wants standing write access among colleagues authors a custom role at `team` scope, and the
   predicate still renders the arm for it.

Teams do **not** carry their own permissions — a team is not a role. (A role *assignment* can be
scoped to a team, but the grants still come from the role.)

## A user with no role sees nothing

`role_assignment` links a user to a role. **A user with zero role assignments has zero object
permissions** — every object gate (question 2) fails closed, so every list and record 404/403s, even
the pipeline board. This is not a row-scope subtlety; the user simply cannot clear the object gate
for anything.

The workspace bootstrap assigns the founding admin the `admin` role (`identity/service.go`,
`seedSystemRoles`). Any user created by another path — a SQL seed, a future invite flow — **must be
given a role explicitly**, or they log in to a wall of permission errors. (This is exactly what bit
the dev seed's second user before it assigned `rep`; see `scripts/seed-dev.sql`.)

## Record sharing — a per-record grant on top of scope

Row scope is coarse (own / team / all). **Record sharing** is the fine-grained layer:
grant **one specific record** to **one person or team**, at **read or write**, optionally expiring,
with a reason. This is the Share screen (`frontend/src/screens/share.tsx`, `#/share/<type>/<id>`) and
the `record_grant` table / `/v1/record-grants` API.

How it composes with everything above:

- **It only widens question 3 (row visibility), never question 2 (object RBAC).** The grantee still
  needs a role granting the verb on that object type. Share a deal with a user whose role lacks
  `deal.read` and they still can't open it — the grant is inert until their role clears the object
  gate.
- **It applies only to shareable tables** — `person`, `organization`, `deal`, `lead`, `project`
  (`rowscope.go` `shareableTables`; the `record_grant` CHECK is the schema-side twin). Config and
  other objects have no per-record share. On an identity table a `read` grant only matters for an
  owner-private captured row; a `write` grant is what widens editing.
- **A `write` grant satisfies a read** (write ⊇ read).
- **It is evaluated live on every query** — the visibility predicate `OR EXISTS (…record_grant…
  AND (expires_at IS NULL OR expires_at > now()))`. So **revoking or expiring a share binds on the
  next read**, no session to wait out.
- **A grant can't exceed the granter.** The server rejects a grant wider than the granter's own
  access to that record (surfaced as `approval_required` / 422 in the UI), so sharing can't launder
  privilege.

In SQL terms, a read over a shareable table is `ownerPredicate OR liveGrantExists` — the grant is a
second way in, checked in the same statement as the scope filter (`VisiblePredicate` in `rowscope.go`).

## Worked example — the dev seed

The dev seed (`scripts/seed-dev.sql`) sets up three seats so every branch above is observable:

- **Demo Admin** — `admin` role, `row_scope: all`, member of **DACH Sales**. Owns the seeded people
  and deals; sees everything.
- **Rep One** — `rep` role, `row_scope: own`, member of **DACH Sales** (with Demo Admin). *The
  shared-with seat.*
  - Object gate: `rep` grants `deal.read`, `pipeline.read` (read-only) → the deals board loads, and
    shows Demo Admin's records like everyone else's: customer identity is workspace-readable.
  - Row scope: `own` → being in Demo Admin's team buys Rep One nothing. Every one of Demo Admin's
    records is **readable and not editable** — pressing save answers 403, and the record's `writable`
    flag says so before you press it.
  - The seed shares **one** of Demo Admin's people with Rep One at `write`. That record, and only
    that record, is theirs to change — which is what makes the grant the observable cause.
- **Rep Two** — `individual` role (a clone of `rep`), **in no team**. *The nothing-shared seat.*
  - Object gate passes (same object grants as `rep`) → the board loads and shows every deal, read-only.
  - Row scope: `own` → owns nothing and holds no grant → may **edit nothing**.
  - The contrast with Rep One is now the GRANT rather than the team: two own-scoped seats, one of
    which has been handed a record.

Remove a user's role assignment entirely and every read fails at the object gate (403/404 across the
board) — the symptom that means "no role," distinct from "role present but scope hides the row."

## Where this is enforced (pointers)

- Role definitions + merge: `backend/internal/modules/identity/internal/policy/policy.go`
- Object-level gate: `backend/internal/platform/auth/rbac.go`
  (`Require`, `RequireAny`, `UpsertAction`, `RequireHuman`, `RequireAdmin`)
- Row-scope + record-grant SQL predicates: `backend/internal/platform/auth/rowscope.go`
  (`OwnerPredicate`, `VisiblePredicate`, `ScopeClauseFor`, `shareableTables`); the read classes in
  `tableclass.go`; the activity discover/content gates in `inheritedscope.go`
- Schema: `role`, `role_assignment`, `team`, `team_membership`, `record_grant`
  (`backend/migrations/core/`)
- The enforcement architecture (one gate, three transports, structural backstop):
  [authorization.md](authorization.md)
