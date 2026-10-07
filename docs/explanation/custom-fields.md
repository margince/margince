<!-- prose:plain -->
# Custom fields: the governed add-field engine

How a workspace admin adds a field to `contact` at runtime without anyone shipping code. And why so many
rules hold that right in. `customfields` is the only module allowed to run an
`ALTER TABLE` at runtime; every other module is barred from it. A custom field is a real, typed column
(`cf_<slug>`) on the core object's own table. The `custom_field` catalog is the system of record that
says what it is.

**The rejected choice names the design.** The obvious way to ship fields that users make is an EAV
value store: a `(record_id, key, value)` table beside the record, for any key. Margince refuses it, for
a smaller reason than "EAV is slow". One value column for every field cannot carry a type or constraint
per field. One `text` column cannot be both a date and a `picklist` with three values.

So every read
turns rows back into columns through `JOIN` and `CAST` steps. The planner estimates those turns wrong on
the report queries that custom fields exist to serve.

An EAV table *can* be indexed. What is missing is constraints and indexes for each type, and plain
`GROUP BY` over typed values. A custom field is a real column so that reports stay plain SQL, with no
extra metadata layer on the path every read takes. One governed DDL path is the price. The values of a
`picklist` live in the catalog's own `options` `jsonb` column; that is field *metadata*, not a value store.

## What a custom field may be: the closed sets

The types are `text`, `number`, `date`, `currency`, `picklist`, `multiselect` and `boolean`
(`FieldTypes` in `engine.go`). The objects are `contact`, `company`, `deal`, `lead`, `project` and
`contract` (`FieldObjects` in `engine.go`). The catalog's `CHECK` on `object` allows more than that
list. The engine is the only writer of the table, so the list is what binds. There is no cap on how many
fields an object carries, and no way to add to what a field may be.

Each type maps to one column type:

- `number` → `numeric`, passed both ways as a string and never a `float`, so the value stays correct.
- `currency` → `bigint` in the smallest money unit, with the `ISO-4217` code kept in the catalog row
  instead of the column.
- `picklist` → `text` plus a generated `CHECK` constraint.
- `multiselect` → a `text[]` that may be `NULL` (see [Fields with more than one choice](#fields-with-more-than-one-choice)).

The type names are written in three places: the engine's constants, the catalog's `CHECK` constraints
(in `migrations/core/`), and `ports/fieldcatalog`. They must never drift apart. The port carries its own
copy because `shared` may not import `modules` without breaking the DAG.

The admin never names the column. `label` is the only text a caller supplies. The engine derives the
slug from it, and the column from the slug.

To make the slug, it runs `strings.ToLower` and turns
each run of signs other than `a-z` and `0-9` into one `_`. Then it caps the length at 40. That keeps `cf_<slug>_check` under the
63-byte name limit of Postgres. The server derives the column identity and never changes it: rename moves
the label and nothing else.

### Fields with more than one choice

`multiselect` is a governed custom field type, next to `picklist`. Its values are JSON lists of
strings, stored in a `text[]` column that may be `NULL`. Admins supply the allowed choices when they create the
field; the product ships no fields or choice lists made for one install. The generated `CHECK`
refuses unknown choices. An edit to the choice list refuses to remove any choice still stored on a
record, retired records included.

Create and edit forms offer one checkbox per choice. A label with a `,` in it stays a single choice.
Leaving a key out of a `PATCH` keeps its value, and `null` clears the field. `[]` stores an empty
choice set that the user asked for. Duplicate choices become one on record writes.

The `collections` filter words and query plans use `eq` for a match on the whole set and `neq` for no
such match. They use `in` for a match on any listed choice. These match whole choices, with case, never
a part of the label text.

The review questions a user answers when a deal closes can also use `multiselect`, apart from custom
fields. Admins edit the questions in Settings with `custom_field:update`. Question set versions stop two
edits at the same time from writing over each other. Text answers keep the existing `answers` map, while
`choice_answers` carries lists keyed by question key. Each sent review keeps a fixed copy of its
questions and allowed choices. So a later edit to the question set never changes the review of a past
closing.

## Refusing a change to the data model

A label that looks like a new object, a relationship, a formula, or a check rule is refused. It is never
accepted as a text column instead. It answers a 422 `structural_change_refused` that names
the way out (`details.route: source_development_path`). Runtime custom fields add small single-value
facts to objects that already exist; anything that changes the data model ships as a reviewed source change.

The check looks for key words in the label. It refuses some labels that are right, because accepting a
wrong label costs far more than refusing a right one. A "Linked Contract" text column would look like a
relationship but would not be one. Refusing only makes an admin change the words of a label.

## Two pools with different rights

The engine uses two pools with different authority. The app pool (`margince_app`, DML only) serves
every operation that only touches the catalog. The **schema pool**, which has owner rights, serves two
paths only: create, and an edit to the options of a `picklist`. Only those two run DDL.

Inside one transaction on that pool:

1. Cap every lock wait (`SET LOCAL lock_timeout = '2s'`), and take an **advisory lock keyed on the
   target table** that lasts for the transaction.
2. Check that the column name is free, then run the statement that needs the owner role: the
   `ALTER TABLE`.
3. Write the catalog `INSERT` (or options `UPDATE`) and the audit row, still as the owner role.

Step 3 runs as the owner role. A `SET ROLE` to `margince_app` would need the owner to be a member of
that role, which nothing sets up and hosted Postgres does not grant. Step 3 is two statements with their values
passed apart from the text, which the app role could run too. So they run under the authority of the
owner for one short transaction and add no surface.

Postgres runs DDL inside a transaction. So the column, the catalog row, and the audit entry land together
or not at all. A half-added field is not a state anyone can reach.

Two points count for more than their size shows:

- **DDL comes only from the checked input**, never from raw request text. Names go through
  `pgx.Identifier.Sanitize`, and `picklist` values go through the module's own `quoteLiteral`. Both
  check again where the DDL is built, instead of trusting the earlier check on the request.
- **The `lock_timeout` guards the whole platform.** One wait must not block it. Say an `ACCESS EXCLUSIVE`
  request is queued behind one reader that runs for a long time. It then holds up every later DML on a
  shared core table. Timing out instead answers a 409 that can be retried (`ErrTableBusy`).

  The advisory lock closes the gap between two creates at the same time that would add the same column.
  Every path that holds both locks takes the row lock first and the advisory lock second. So the two
  DDL paths cannot block each other for good.

The schema pool is **not wired by default**. Without `--schema-dsn`, create and options edit answer
501. They declare the gap by leaving it out, instead of following a `nil` value at request time. That is the
same rule as the blob store when it is not wired. Once wired, it also gets a `/readyz` check. A slug
already used by another field, a retired one included, answers 409 with the fix (`choose another
label`).

## Retire, never drop

**Retire is a status change.** The column and every value in it are kept; the engine never runs a
`DROP COLUMN`. The field leaves record payloads and the sort and filter words; the row can still be
fetched; `archived_at` stays `null` (retire is not an archive). Because the column lives on, the slug
stays in use: the catalog's `UNIQUE` indexes cover retired rows too. The admin list does not leave them
out by default, because it is the one surface that still shows a retired field.

Retiring is **for good**: a retired field refuses rename and options edits with a 409. Retiring it again
does nothing. It returns the row as it is and writes nothing to the audit log.

An options edit generates the `CHECK` of the `picklist` again from the new set, and `ADD CONSTRAINT` checks
existing rows. Removing an option that records still use refuses the edit (`migrate them first`). It does not
leave data outside its own constraint. A `picklist` always keeps one value or more.

Changing the catalog is **owned by admins and operators**, and every role may read it, as with pipeline config. A
field changes what the system stores for every user's records. The catalog is config shared
by the workspace, with no `owner_id`. So the object grant is the whole authority question, and there is
no row scope to compose. Creating a field is 🟡: an agent caller stages it for approval first, and a
human's direct call is itself the approval.

The field is put down to the human or, behind an agent, to the human who granted the agent
(*agent ≤ human*). Changes to the catalog are **audit only**: the closed event catalog has no
`custom_field.*` type. A catalog change over many objects has no single event stream to go on.

## How record stores see `cf_*` columns

A record store never imports this module; that would be an edge to another module, which the module
DAG does not allow. It
depends on `ports/fieldcatalog.Reader`, a seam with one method that answers *which `cf_*` columns are
live on this object, and of what type*. Compose puts in the real service. A `nil` `Reader` passes
everything through at no cost, for tests and installs that never wired the module.

`Column` is small: name and type, nothing else. The slug, label, retire status, and `picklist` options stay
inside `customfields`, because a record store has no need for admin metadata. The store calls the
custom column helpers in `storekit`. They are code that builds SQL parts and values, and they touch no database.
The store puts the result into the same `Patch` that carries core columns. So custom fields go through
the ordinary audit before and after, and the update guarded by version, with no extra records to keep.

The service behind the seam is wired with a **`nil` schema pool**, because reading the catalog never
needs DDL authority.

`ActiveColumns` runs no RBAC check, by design: it is called from inside a store's own gated
`Get`/`List`/`Create`/`Update`. What it exposes is the shape of the schema, which the whole workspace
may see (the same thing the admin list already answers). It exposes no row data. The store's row-level
gate guards the values.

One rule catches developers out: a custom field value is **dropped when it does not match**. A request body's
`additionalProperties` carries no shape contract per key. So a value whose shape does not match its
column's type is dropped without an error, instead of answered with a 422.

## No `cf_*` column is indexed

The engine writes `ADD COLUMN` and, for a `picklist`, its `CHECK`. It creates no index. Custom fields are
still full members of the list words. `?sort=cf_contract_end` and `?cf_region=emea` are both accepted
and checked against the live catalog. Then they are turned into real SQL.

Such a query first finds the
workspace through a core index. It then scans that workspace's rows to filter or sort on the `cf_` column that has no
index.

At small sizes no user sees this. For a large workspace the work goes up with the workspace's row count
*per page*. Paging by key runs the sort again on every page.

An index of the shape a report would need looks like this:

```sql
CREATE INDEX idx_company_renewal_risk
  ON company (workspace_id, renewal_risk)
  WHERE renewal_risk IS NOT NULL AND archived_at IS NULL;
```

Add-field is the right moment to index. The column is new, so every value is `NULL`, and an index with
`WHERE <col> IS NOT NULL` starts empty. The transaction already holds `ACCESS EXCLUSIVE`
for its `ALTER`. An index added later costs more. `CREATE INDEX CONCURRENTLY` is the normal way to
build one without blocking writes. It cannot run inside a transaction block, so it cannot meet this
module's promise of one transaction.

The open question is which policy to choose. An index per field on create needs no new step. But it
costs extra size and write work on every field nobody filters. A named admin "index this field" operation makes
the cost visible, but it is new surface to govern. The choice is an open product decision.

## Where the code lives

| | |
|---|---|
| The engine with no database calls (checks, making the slug and DDL, `quoteLiteral`) | `internal/modules/customfields/engine.go` |
| The service seam, the typed errors for a refused call, catalog scan | `internal/modules/customfields/service.go` |
| The two DDL paths | `internal/modules/customfields/create.go`, `options.go` |
| Rename + retire (catalog only, app pool) | `internal/modules/customfields/lifecycle.go` |
| The `fieldcatalog` provider half | `internal/modules/customfields/catalogreader.go` |
| The seam between modules | `internal/shared/ports/fieldcatalog/` |
| The record store code | `internal/platform/database/storekit/customcolumns.go` |
| The sort and filter words that `cf_*` columns are part of | `internal/platform/database/storekit/listquery.go` |
| The catalog table + its RBAC grants | `backend/migrations/core/` (the `custom_field` table is in the `0001` migration) |
| The admin UI | `frontend/src/screens/customfields.tsx` |

The owner pool flag and its `/readyz` check: [reference/configuration.md](../reference/configuration.md).
The write shape these changes still go through: [write-backbone.md](write-backbone.md). The role table
behind the admin and operator rule: [rbac-roles-and-teams.md](rbac-roles-and-teams.md).
