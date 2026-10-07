# Custom fields: the governed add-field engine

How a workspace admin adds a field to `contact` at runtime without anyone shipping code, and why that
power is fenced in as tightly as it is. `customfields` is the only module allowed to run a runtime
`ALTER TABLE`; every other module is forbidden it. A custom field is a real, typed, physical column
(`cf_<slug>`) on the core object's own table, and the `custom_field` catalog is the system-of-record
describing it.

**The rejected alternative names the design.** The obvious way to ship user-defined fields is an EAV
value store: a generic `(record_id, key, value)` sidecar. Margince refuses it, for a reason narrower
than "EAV is slow". A generic value column cannot carry a per-field type or constraint (one `text`
column cannot be both a date and a three-way picklist). So every read pivots rows back into columns
through joins and casts, and the planner estimates those pivots poorly on the reporting queries custom
fields exist to serve. An EAV table *can* be indexed. What is lost is type-specific constraints and
indexes, and plain `GROUP BY` over typed values. A custom field is a real column so that reporting
stays plain SQL with no metadata-engine indirection on the hot path. One governed DDL path is the
price. Picklist values live in the catalog's own `options` jsonb column; that is field *metadata*, not
a value store.

## What a custom field may be: the closed sets

The types are `text`, `number`, `date`, `currency`, `picklist`, `multiselect` and `boolean`
(`FieldTypes` in `engine.go`). The objects are `contact`, `company`, `deal`, `lead`, `project` and
`contract` (`FieldObjects` in `engine.go`). The catalog's `CHECK` on `object` is wider than that list;
the engine is the only writer of the table, so the list is what binds. There is no cap on how many
fields an object carries, and no way to widen what a field may be.

Each type maps to one storage type:

- `number` → `numeric`, round-tripped as a string and never a float, so precision survives.
- `currency` → `bigint` minor units, with the ISO-4217 code held in the catalog row instead of the
  column.
- `picklist` → `text` plus a generated `CHECK` constraint.
- `multiselect` → a nullable `text[]` (see [Multiple-choice fields](#multiple-choice-fields)).

The type literals are spelled in three places: the engine's constants, the catalog's `CHECK`
constraints (in `migrations/core/`), and `ports/fieldcatalog`. They must never drift apart. The port
carries its own copy because `shared` may not import `modules` without inverting the DAG.

The admin never names the column. `label` is the only text a caller supplies. The engine derives the
slug from it (lowercased, non-alphanumeric runs collapsed, capped at 40 chars so that
`cf_<slug>_check` stays under Postgres's 63-byte identifier limit) and the column from the slug.
Column identity is server-derived and immutable: rename moves the label and nothing else.

### Multiple-choice fields

`multiselect` is a governed custom-field type alongside `picklist`. Its values
are JSON arrays of strings, stored in a nullable `text[]` column. Administrators
provide the allowed choices when creating the field; the product ships no
installation-specific field definitions or vocabularies. The generated CHECK
refuses unknown choices. Editing the vocabulary refuses removal of any choice
still stored on a record, including retired records.

Create and edit forms offer independent checkboxes. Labels containing commas
remain single choices. Omitting a key on PATCH preserves its value; `null` clears
the field, and `[]` stores an explicitly empty selection. Duplicate selections
are normalized on record writes.

The collections filter vocabulary and query plans use `eq`/`neq` for whole-set
membership and its negation, and `in` for overlap with any listed choice. These
compare complete, case-sensitive choices, never substrings of a flattened label.

Outcome-review questions can separately use `multiselect`. Administrators edit
the questions in Settings with `custom_field:update`; template versions prevent
concurrent overwrites. Text answers keep the existing `answers` map, while
`choice_answers` carries arrays keyed by question key. Each submitted review
freezes its questions and allowed choices, so a later template edit never
rewrites a previous closing's review.

## The structural refusal

A label that looks like a new object, a relationship, a formula, or a validation rule is refused; it
is never accepted as a text column instead. It answers a 422 `structural_change_refused` that names
the route out (`details.route: source_development_path`). Runtime custom fields add bounded scalar
attributes to objects that already exist; anything structural ships as a reviewed source change. The
check is a keyword heuristic over the label. It is blunt and biased toward refusing, because the
failure it prevents is far worse than the one it causes. A "Linked Contract" text column would become
a fake relationship; a refusal only makes an admin reword a label.

## The privilege boundary

The engine rides two pools with different authority. The app pool (`margince_app`, DML-only) serves
every catalog-only operation. The owner-privileged **schema pool** serves two paths only, create and a
picklist's options edit, because only those run DDL.

Inside one transaction on that pool:

1. Bound every lock wait (`SET LOCAL lock_timeout = '2s'`), and take a
   transaction-scoped **advisory lock keyed on the target table**.
2. Pre-check the column namespace, then run the statement that needs the owner role: the
   `ALTER TABLE`.
3. Write the catalog `INSERT` (or options `UPDATE`) and the audit row, still as the owner role.

Step 3 runs as the owner role. A `SET ROLE` to `margince_app` would need the owner to be a member of
that role, which nothing provisions and managed Postgres does not grant. Step 3 is two parameterized
statements the app role could issue too, so they run under the owner's authority for one short
transaction and widen no surface.

Postgres's transactional DDL makes the column, the catalog row, and the audit entry land or roll back
together, so a half-added field is not a reachable state.

Two details matter more than their size suggests:

- **DDL comes only from the validated spec**, never from raw request text. Identifiers go
  through `pgx.Identifier.Sanitize`, picklist literals through the module's own quoter, and both
  re-validate at the DDL boundary instead of trusting the request-side check that already ran.
- **The `lock_timeout` prevents a platform-wide stall.** An `ACCESS EXCLUSIVE` request queued behind
  one long-running reader parks every later DML on a shared core table behind it. Timing out instead
  answers a retryable 409 (`ErrTableBusy`). The advisory lock closes the duplicate-column race between
  two concurrent creates. Lock order is row-then-advisory in every flow holding both, so the two DDL
  paths cannot deadlock each other.

The schema pool is **unwired by default**. Without `--schema-dsn`, create and options-edit answer 501
and declare the gap by omission instead of dereferencing nil at request time (the unwired-blobstore
posture); wired, it also gains a `/readyz` probe. A slug already used by another field, including a
retired one, answers 409 with the remedy ("choose another label").

## The lifecycle: retire, never drop

**Retire is a status flip.** The physical column and every value in it are preserved; the engine
never issues a `DROP COLUMN`. The field leaves record payloads and the sort/filter vocabulary; the
row stays fetchable; `archived_at` stays null (retire is not an archive). Because the column survives,
the slug stays reserved: the catalog's unique indexes cover retired rows too. The admin list does not
default-exclude them, because it is the one surface that still shows a retired field.

Retirement is **terminal**: a retired field refuses rename and options edits with a 409. Re-retiring
is a no-op that returns the row unchanged and writes nothing to the audit trail.

An options edit regenerates the picklist's `CHECK` from the new set, and `ADD CONSTRAINT` validates
existing rows. Removing an option that records still use refuses the edit ("migrate them first")
instead of stranding data outside its own constraint. A picklist always keeps at least one value.

Changing the catalog is **admin/ops-owned; every role may read it**, as with pipeline config, because
a field definition reshapes what the system stores for everyone's records. The catalog is
workspace-shared config with no `owner_id`, so the object grant is the whole authority question and
there is no row scope to compose. Creation is 🟡: an agent caller stages for approval upstream, and a
human's direct call is itself the approval. The field is attributed to the human or, behind an agent,
to the granting human (*agent ≤ human*). Catalog changes are **audit-only**: the closed event catalog
defines no `custom_field.*` type, and a cross-object catalog change has no single family stream to
ride.

## How record stores see `cf_*` columns

A record store never imports this module; that would be the sibling edge the module DAG forbids.
It depends on `ports/fieldcatalog.Reader`, a one-method seam answering *which `cf_*` columns are
active on this object, and of what type*. Compose injects the concrete service; a nil Reader is the
zero-cost pass-through for tests and deployments that never mounted the module.

`Column` is thin: name and type, nothing else. Slug, label, lifecycle status, and picklist options
stay inside `customfields`, because a record store has no business with admin metadata. The store
drives `storekit`'s custom-column helpers, which are pure SQL-fragment and value mechanics that touch
no database. It folds the result into the same `Patch` that carries core columns, so custom fields
ride the ordinary audit before/after and version-guarded update with no extra bookkeeping. The
seam-side service is wired with a **nil schema pool**, because reading the catalog never needs DDL
authority.

`ActiveColumns` runs no RBAC check, by design: it is called from inside a store's own gated
`Get`/`List`/`Create`/`Update`. What it exposes is workspace-visible schema shape (the same thing the
admin list already answers), not row data. The store's row-level gate protects the values.

One rule surprises developers: custom-field values convert **drop-on-mismatch**. A request body's
`additionalProperties` carries no per-key shape contract, so a value whose shape does not match its
column's type is dropped without an error instead of answered with a 422.

## No `cf_*` column is indexed

The engine emits `ADD COLUMN` and, for a picklist, its `CHECK`. It creates no index. Custom fields
are still first-class in the list vocabulary: `?sort=cf_contract_end` and `?cf_region=emea` are both
accepted, validated against the active catalog, and compiled into real SQL. Such a query narrows to
the workspace on a core index and then scans that workspace's rows to filter or sort on the
unindexed `cf_` column. At small volumes this is invisible. For a large workspace it is work
proportional to the workspace's row count *per page*, since keyset paging repeats the sort on every
page.

An index of the shape a reporting workload would want looks like this:

```sql
CREATE INDEX idx_company_renewal_risk
  ON company (workspace_id, renewal_risk)
  WHERE renewal_risk IS NOT NULL AND archived_at IS NULL;
```

Add-field is the cheapest moment to index. The column is brand new, so every value is NULL and a
partial index `WHERE <col> IS NOT NULL` starts empty. The transaction already holds
`ACCESS EXCLUSIVE` for its `ALTER`. Indexing later is the expensive path. `CREATE INDEX CONCURRENTLY`,
the usual way to avoid the write-blocking build, cannot run inside a transaction block, so it cannot
be reconciled with this module's one-transaction guarantee.

The open question is which policy to adopt. An index per field on create is simple, but pays storage
and write amplification on every field nobody filters. An explicit admin "index this field" operation
makes the cost visible, but is new surface to govern. The choice is an open product decision.

## Where the code lives

| | |
|---|---|
| The pure engine (validation, slug/DDL generation, quoting) | `internal/modules/customfields/engine.go` |
| The service seam, typed refusals, catalog scan | `internal/modules/customfields/service.go` |
| The two DDL paths | `internal/modules/customfields/create.go`, `options.go` |
| Rename + retire (catalog-only, app pool) | `internal/modules/customfields/lifecycle.go` |
| The `fieldcatalog` provider half | `internal/modules/customfields/catalogreader.go` |
| The cross-module seam | `internal/shared/ports/fieldcatalog/` |
| The record-store mechanics | `internal/platform/database/storekit/customcolumns.go` |
| The sort/filter vocabulary `cf_*` columns join | `internal/platform/database/storekit/listquery.go` |
| The catalog table + its RBAC grants | `backend/migrations/core/` (the `custom_field` table is in the `0001` baseline) |
| The admin UI | `frontend/src/screens/customfields.tsx` |

The owner-pool flag and its `/readyz` probe: [reference/configuration.md](../reference/configuration.md).
The write shape these mutations still ride: [write-backbone.md](write-backbone.md). The role matrix
behind the admin/ops posture: [rbac-roles-and-teams.md](rbac-roles-and-teams.md).
