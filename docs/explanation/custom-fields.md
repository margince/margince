<!-- prose:plain -->
<!-- describes: backend/internal/modules/customfields backend/internal/shared/ports/fieldcatalog backend/internal/platform/database/storekit/customcolumns.go backend/internal/platform/database/storekit/listquery.go backend/internal/platform/database/storekit/removedchoices.go frontend/src/features/records/customfields.tsx frontend/src/features/records/customfields.choices.tsx frontend/src/components/listcustomfields.tsx -->
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

The types are `text`, `number`, `date`, `currency`, `picklist`, `multiselect`, `boolean`, `record` and
`url` (`fieldcatalog.Types()`, which `FieldTypes` in `engine.go` is). The objects are `contact`, `company`, `deal`, `lead`, `project` and
`contract` (`FieldObjects` in `engine.go`). The catalog's `CHECK` on `object` allows more than that
list. The engine is the only writer of the table, so the list is what binds. There is no cap on how many
fields an object carries, and no way to add to what a field may be.

Each type maps to one column type:

- `number` → `numeric`, read back as a string and never a `float`. A request decodes a JSON number as
  a `float64` by default, so a caller that needs every digit sends the value as a string.
- `currency` → `bigint` in the smallest money unit, with the `ISO-4217` code kept in the catalog row
  instead of the column.
- `picklist` → `text` plus a generated `CHECK` constraint.
- `multiselect` → a `text[]` that may be `NULL` (see [Fields with more than one choice](#fields-with-more-than-one-choice)).
- `record` → a `text` that holds the id of one record. The field's single option names the record kind an
  extension declared, such as `ext:<unit>:<kind>`. Creating the field is refused with `invalid_record_kind`
  on `options` unless it names one registered kind and no other. A write is refused with `invalid_value` unless
  the value is a UUID. Core does not yet check that the row exists, and the lists filter the value as text.
- `url` → a `text` with a 2000-character `CHECK`. A write is refused with `invalid_value` unless the value is
  an absolute `http` or `https` address. It needs a host and no user name or password (`values.IsWebAddress`).
  The scheme matches in any case. The lists filter the value as text, and the record page draws it as a
  link through `OffsiteLink`.

The type names are written in three places: `ports/fieldcatalog`, the catalog's `CHECK` constraint (in
`migrations/core/`), and `CF_TYPES` in `frontend/src/lib/customfields.logic.ts`. The engine's constants
are the port's. `TestFieldTypes_IsTheCatalogCheckSet` holds the port against the `CHECK` in the head
schema, and `backend/gates/frontendvocabularymirrors_test.go` holds `CF_TYPES` against the port. Both fail
in either direction.

The admin never names the column. `label` is the only text a caller supplies. The engine derives the
slug from it, and the column from the slug. When a retired field already owns that column, the new
field takes the next free `cf_<slug>_<n>` instead.

To make the slug, it runs `strings.ToLower` and turns
each run of signs other than `a-z` and `0-9` into one `_`. Then it caps the length at 40. That keeps `cf_<slug>_check` under the
63-byte name limit of Postgres. The server derives the column identity and never changes it: rename moves
the label and nothing else.

### Fields with more than one choice

`multiselect` is a governed custom field type, next to `picklist`. Its values are JSON lists of
strings, stored in a `text[]` column that may be `NULL`. Admins supply the allowed choices when they create the
field; the product ships no fields or choice lists made for one install. The generated `CHECK`
refuses unknown choices. An edit to the choice list takes a removed choice off every record that holds
it, as [Removing a choice](#removing-a-choice) says.

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
the way out (`details.route: source_development_path`). Runtime custom fields add small facts (one value or a
list of choices) to objects that already exist. Anything that changes the data model ships as a reviewed source change.

The check looks for key words in the label. It refuses some labels that are right, because accepting a
wrong label costs far more than refusing a right one. A "Linked Contract" text column would look like a
relationship but would not be one. Refusing only makes an admin change the words of a label.

## Two pools with different rights

The engine uses two pools with different authority. The app pool (`margince_app`, DML only) serves
every operation that only touches the catalog. The **schema pool**, which has owner rights, serves two
paths only: create, and an edit to the options of a `picklist` or `multiselect`. Only those two run DDL.

Inside one transaction on that pool:

1. Cap every lock wait (`SET LOCAL lock_timeout = '2s'`), and take an **advisory lock keyed on the
   target table** that lasts for the transaction.
2. Pick the column: `cf_<slug>`, or the first `cf_<slug>_<n>` that no catalog row of the object owns.
   Check that it is free, then run the statement that needs the owner role: the
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
same rule as the blob store when it is not wired. Once wired, it also gets a `/readyz` check. A label
or slug held by another live field answers 409 with the fix (`choose another label`).

## Retire, never drop

**Retire is a status change.** The column and every value in it are kept; the engine never runs a
`DROP COLUMN`. The field leaves record payloads and the sort and filter words; the row can still be
fetched; `archived_at` stays `null` (retire is not an archive). Because the column lives on, its name stays in use: `uq_custom_field_column` covers retired rows.

The slug and the label are free again, because `uq_custom_field_slug` covers live fields only.
A new field can take a retired field's name and gets its own column. A screen that still shows the retired
field reads "Label (retired)". The admin list does not leave them
out by default, because it is the one surface that still shows a retired field.

Retiring is **for good**: a retired field refuses rename and options edits with a 409. Retiring it again
does nothing. It returns the row as it is and writes nothing to the audit log.

An options edit generates the `CHECK` of the field again from the new set, and `ADD CONSTRAINT` checks
existing rows. A choice the edit removes is first cleared from the records that hold it, so no data is
left outside its own constraint. A `picklist` always keeps one value or more.

### Removing a choice

The options edit clears a removed choice from every record of the field's object, archived ones too,
because the `CHECK` covers them. It runs in the same schema-pool transaction as the `ALTER`, after the
advisory lock and before the `CHECK` is made again. A `picklist` value becomes `NULL`. A `multiselect` keeps
its other choices in their order, and one left with none becomes `NULL`.

Removing a choice changes records, so the caller also needs the grant to update the field's object. Before
the clear, the edit takes the `ACCESS EXCLUSIVE` lock the `ALTER` needs. So no record can take a removed
choice between the clear and the `CHECK`.

Each record module owns its table, so it does the clearing. `customfields` holds a `ChoiceRemover` for
each object, and compose injects them (`CustomFieldChoiceRemovers` in
`internal/compose/customfieldchoices.go`). Most removers call `storekit.ClearRemovedChoices` with their own
table and their own `<object>.updated` payload. That helper locks the rows that hold a removed choice and
updates them. It writes the update, the audit rows and the events in one statement each per 1000 records.

The contract remover takes the locked rows from `storekit.LockRemovedChoices` and writes each through
`applyContractUpdate`, the one writer of a contract update. Both refuse a column the catalog does not list
on that table with that type. The table ownership gate counts
`ClearRemovedChoices` as a write by the module that calls it.

The cost grows with the records that hold the choice. The `ALTER` already reads the whole table to check
the new `CHECK`. The 2-second `lock_timeout` covers the row locks too, so a record being edited answers
`ErrTableBusy`, which a client may retry. A record that holds the choice and still belongs to a member who has left is
refused by the retired-member trigger. An activity under a retention hold refuses any change. The edit
then answers 409 and names the reason.

The answer carries `records_changed`. `GET /custom-fields/{id}/choices` answers how many records hold
each choice, for a reader of the field and its object. The **Edit choices** dialog shows those counts and the total a save
clears.

A Live List whose filter names the removed choice keeps working. A filter condition does not
refuse a value outside the set at read time (`storekit.RefuseUnknownPicklistValues` runs only when a
filter is saved). It matches no record by that choice. Saving a `picklist` filter that still names the choice is refused until the
choice is taken out of it.

Changing the catalog is **owned by admins and operators**, and every role may read it, as with pipeline config. A
field changes what the system stores for every user's records. The catalog is config shared
by the workspace, with no `owner_id`. So the object grant is the whole authority question, and there is
no row scope to compose.

An agent on a passport holds its user's grant and changes fields through `change_settings`. Its tool
marks creating a field, retiring one and changing its options for an ask first. It reads the live
catalog through `describe_query_vocabulary`.

The field is put down to the human who changed it. Changes to the catalog are **audit only**: the closed event catalog has no
`custom_field.*` type. A catalog change over many objects has no single event stream to go on.

## How record stores see `cf_*` columns

A record store never imports this module; that would be an edge to another module, which the module
DAG does not allow. It
depends on `ports/fieldcatalog.Reader`, a seam with one method that answers *which `cf_*` columns are
live on this object, and of what type*. Compose puts in the real service. A `nil` `Reader` passes
everything through at no cost, for tests and installs that never wired the module.

`Column` is small: the name, the type, a field's options, a currency field's currency and an
activity field's kinds. The slug, label and retire status stay inside `customfields`, because a record
store has no need for admin metadata. The store calls the
custom column helpers in `storekit`. They are code that builds SQL parts and values, and they touch no database.
The store puts the result into the same `Patch` that carries core columns. So custom fields go through
the ordinary audit before and after, and the update guarded by version, with no extra records to keep.

The service behind the seam is wired with a **`nil` schema pool**, because reading the catalog never
needs DDL authority.

`ActiveColumns` runs no RBAC check, by design: it is called from inside a store's own gated
`Get`/`List`/`Create`/`Update`. What it exposes is the shape of the schema, which the whole workspace
may see (the same thing the admin list already answers). It exposes no row data. The store's row-level
gate guards the values.

One rule catches developers out: a record write refuses what it cannot store. A request body's
`additionalProperties` carries no shape contract per key, so the write checks each extra key itself
(`storekit.CheckCustomValues`). A 422 names each key that fails.

Three things fail. One is a key that is not a `cf_` key. Another is a `cf_` key that names no active
field. The third is a value of the wrong type or outside the field's options. The codes are
`unknown_field` and `invalid_value`. Nothing is written, so a success means every key was stored.

An activity field may apply to some kinds only (`custom_field.activity_kinds`, null for every kind).
The activity writer refuses a value for a field that does not apply to the kind of the activity.
The code is `field_not_valid_for_kind`, on a create and on a patch, where it reads the stored kind.
A `null` always passes, so a value stored before the field was narrowed can still be cleared. The
check is `checkCustomValues` in `internal/modules/activities/customfields.go`, and the REST routes
and the agent tools all reach it.

A deal field may apply to some pipelines only (`custom_field.pipeline_ids`, null for every pipeline).
The deal store refuses a value for a field that does not apply to the deal's pipeline. This holds on a
create, a patch and an import row. The code is `field_not_valid_for_pipeline`, and a `null` always passes.

A patch never moves the deal, so it reads the stored pipeline. A deal that moves keeps its values.
An array column cannot carry a reference constraint, so the engine checks each id against the live
pipelines when it writes the list. A pipeline archived later matches nothing, and its id may stay on the list while only new ids must be live. The check is
`checkPipelineScope` in `backend/internal/modules/deals/pipelinefields.go`.

The forms show a field
only when it applies to the deal's pipeline or the deal holds a value for it. That rule is
`appliesToPipeline` in `frontend/src/components/customfields.form.ts`.

## List columns and filters

Company, contact, lead, deal and project lists offer active custom fields in **View → Columns**.
They start hidden. **View → Filters** offers the same fields by their catalog labels. The filter
uses the field's form control and stored type, including the currency's scale.
Filters for fields with several choices compare the selected set in any choice order. They have no sort control.

Masked values stay hidden when their column is shown. A retired field leaves the offered columns and
filters; an existing saved filter still follows the server's catalog rules.

Company and contact custom fields appear in the details column beside the standard record fields.

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
| Clearing a removed choice, and the count before it | `internal/modules/customfields/removedchoices.go`, `internal/platform/database/storekit/removedchoices.go` |
| Rename + retire (catalog only, app pool) | `internal/modules/customfields/lifecycle.go` |
| The `fieldcatalog` provider half | `internal/modules/customfields/catalogreader.go` |
| The seam between modules | `internal/shared/ports/fieldcatalog/` |
| The record store code | `internal/platform/database/storekit/customcolumns.go` |
| The sort and filter words that `cf_*` columns are part of | `internal/platform/database/storekit/listquery.go` |
| The catalog table + its RBAC grants | `backend/migrations/core/` (the `custom_field` table is in the `0001` migration) |
| The admin UI | `frontend/src/features/records/customfields.tsx`, `customfields.choices.tsx` |

The owner pool flag and its `/readyz` check: [reference/configuration.md](../reference/configuration.md).
The write shape these changes still go through: [write-backbone.md](write-backbone.md). The role table
behind the admin and operator rule: [rbac-roles-and-teams.md](rbac-roles-and-teams.md).
