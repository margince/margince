# An extension cannot add its own record kinds, links, signals or reports

This package of work lets an extension bring its own record kinds, links between records, signals and reports into
Margince. It is for those who build or run extensions. Each feature ships on its own, and the table says which ones
are done.

## How to read this page

| Column | Meaning |
|---|---|
| **Feature** | One piece of the package, in build order. |
| **What someone can do now** | What works today. A planned feature says nothing yet. |
| **Page with the details** | Where the facts are. |
| **State** | **done** is built and merged. **planned** has not started. |

| # | Feature | What someone can do now | Page with the details | State |
|---|---|---|---|---|
| 1 | Audited fix of signal evidence | Correct a signal's type and evidence. The old values stay in the audit log. | [Audited fix of signal evidence](#audited-fix-of-signal-evidence) | done |
| 2 | Company type field | Declare a company type in an extension and set it on a company. See it in the company list and filter by it. | [Company type field](#company-type-field) | done |
| 3 | Extension record kinds and link registry | Declare a record kind in an extension. Tag it, put it on a Shortlist and refuse access to it by default. Read the kinds you may see from `GET /v1/record-kinds`. | [Extension record kinds and link registry](#extension-record-kinds-and-link-registry) | done |
| 4 | Extension report breakdowns and counts | Declare a count or a breakdown in an extension. See it on Reports and Home, under the seat's row scope, field masks and the group floor. A multi-select custom field can be split by. | [Extension report breakdowns and counts](#extension-report-breakdowns-and-counts) | done |
| 5 | Extension signal types with structured evidence | Declare a signal type in an extension, with an evidence layout that marks personal data. File a signal of it. The same event reported twice leaves one signal. Erasing a contact removes the personal members. | [Extension signal types with structured evidence](#extension-signal-types-with-structured-evidence) | done |
| 6 | Panels and tabs on record pages | Nothing yet. | None yet | planned |
| 7 | "Pick a record" field type | Nothing yet. | None yet | planned |
| 8 | Record page with merged history for extension kinds | Nothing yet. | None yet | planned |
| 9 | Search and lists for extension kinds | Nothing yet. | None yet | planned |
| 10 | Suggested-link resolver | Nothing yet. | None yet | planned |
| 11 | Typed links between core records | Nothing yet. | None yet | planned |
| 12 | Let an unattended extension run land a signal through Ingest | Nothing yet. | None yet | planned |

```mermaid
flowchart LR
    F1[1 Audited fix of signal evidence] --> F5[5 Extension signal types]
    F1 --> F12[12 Land a signal through Ingest]
    F2[2 Company type field] --> F3[3 Record kinds and link registry]
    F3 --> F6[6 Panels and tabs]
    F3 --> F7[7 Pick a record field]
    F3 --> F8[8 Merged history]
    F3 --> F9[9 Search and lists]
    F3 --> F10[10 Suggested-link resolver]
    F3 --> F11[11 Typed links between core records]
    F3 --> F4[4 Report breakdowns and counts]
    F5 --> F12
```

## Audited fix of signal evidence

A signal can be corrected after it is filed. `PATCH /v1/signals/{id}` now accepts `kind` and `evidence` next to
`status`, `note` and `severity`. The request is `UpdateSignalRequest` in `backend/api/crm.yaml`.

| Field | What it does |
|---|---|
| `kind` | Changes the signal's type. Only the six kinds filed by hand are accepted: `stalled_deal`, `champion_left`, `reengagement`, `buying_intent`, `risk` and `other`. |
| `evidence` | Replaces the whole evidence list. |

The edit uses the existing `update` audit action, so no new action exists. The audit row's before image holds the old
kind and evidence, and its after image holds the new ones. An edit that changes nothing writes no audit row.

The old values live only in that audit row. The audit log view shows them to a user who may read it.
An erasure of a contact writes an erase row against each signal about that contact. The history views then hold back
every image written before that row, so the old kind and evidence are no longer served.
The data-access export for a contact does not list signals, so these old values are not in it yet.

The request contract accepts only those six kinds. The store also accepts the signal's current `kind` unchanged, even
when it is outside the six, so a client can send back what it read. Any other change of `kind` is refused with a 422 on
the field `kind`, code `invalid_kind`, and nothing changes. The maintainers decide which of the other signal kinds may be
corrected. Until then they are refused.

## Company type field

An extension can add company types beside core's fixed list. The steps to declare and set one are in
[Declare a company type](../how-to/add-an-extension.md#declare-a-company-type).

The extension's types live in their own field, `extension_relationship_types`, and core's `relationship_types` list
is unchanged. A published list that grows cannot be narrowed again, and a client that switches on the old list would
meet a value it never saw. The list filter follows the same split. The two fields share one table, so one company can
carry both.

| Part | What it does |
|---|---|
| Value | `ext:<unit>:<type>`, built from the declaring unit's name. A unit cannot set another unit's value. |
| `Extension.CompanyTypes` | The unit declares the types and a label for each. The boot refuses a bad name or a duplicate. |
| `SetExtensionTypes` | The unit sets its own types on a company. It leaves other units' types alone. |
| `PATCH /v1/companies/{id}` | A user or an agent sets `extension_relationship_types`. A value no unit declares is a 422 with the code `invalid_enum`. |
| `GET /v1/company-types` | Lists every declared value and its label, for any signed-in user. |
| `extension_relationship_type` list filter | Takes a declared extension value. The `relationship_type` filter stays core's closed list, because narrowing a published parameter later breaks clients. |
| Agent tools | The company update shape carries `extension_relationship_types`, the same as the REST body. |

`partner` works as before: it needs a partner row, and removing it while a partner row exists is a 422. Setting an
extension type does not touch it. The database check on `company_relationship_type` accepts the shape
`ext:<unit>:<type>`; which pairs exist is decided by the composed set, not by the database.

## Extension record kinds and link registry

An extension can declare a kind of record of its own, such as a visit or a ledger account. Core then treats the kind as
a record in tags, Shortlists, the audit log and the places that name a kind, under rights that start denied. The steps to
declare one are in [Declare a record kind](../how-to/add-an-extension.md#declare-a-record-kind).

A kind is named `ext:<unit>:<kind>`. The unit is the extension's name. The kind is a lower-case letter followed by up to
31 lower-case letters, digits or underscores. The spelling is permanent once a row carries it, because every column
that names a kind accepts it as a pattern.

| Part | What it does |
|---|---|
| `Extension.RecordKinds` | The unit declares each kind: a display name, its own table, fields, a natural key, optional starting grants and a delete policy. The boot and `make gen` refuse a bad declaration. |
| Registry | The core keeps the declared kinds in memory, filled at boot. Tags, lists and the rights check ask it, so none of them imports a unit. |
| Rights | Each kind is gated on its own RBAC object, `ext_<unit>_<kind>` with hyphens turned into underscores. A role holds nothing on it until the unit's starting grants or an admin give it. |
| Row check | A link, a tag or a Shortlist member needs the kind's `read` right and a live row in the unit's table. A caller without the right gets not found, so the id is not disclosed. |
| Natural key | The unit's table keeps a unique index over the declared fields and no others. The readiness probe fails while a declared table, a column or that index is missing. |
| Delete policy | `archive_only` (the default) keeps the row. `erasable` lets the row be erased through the erasure path. |
| `GET /v1/record-kinds` | Lists the kinds the signed-in seat may read, with fields, natural key, delete policy and the actions the seat holds. |
| Audit | A change to an extension record is written with the `ext:<unit>:<kind>` name as its entity type and the existing actions. No new action is needed. |
| Frontend | `isExtensionEntityKind` reads the spelling and `useRecordKindDirectory` loads the declared kinds. An extension kind has no record page, so `recordRoute` names none. |

The database accepts the spelling in every column that named a kind. The columns are listed in the table below. A gate
fails a new column that restates a closed list.

| Table | Column |
|---|---|
| `activity_link` | `entity_type` (with `ext_entity_id` for the record's id) |
| `attachment`, `embedding`, `list`, `list_member`, `list_live_member`, `list_member_event`, `taggable`, `user_record_view`, `signal` | `entity_type` |
| `field_provenance` | `object_type` |
| `record_grant` | `record_type` |
| `dedupe_candidate` | `entity_type` (with `left_ext_id` and `right_ext_id`) |
| `saved_view` | `resource` |

No column gains a foreign key to an extension table, because an extension role cannot be referenced from core.

What this feature does not do yet:

- A Live List over an extension kind is refused. Only a Shortlist takes one.
- The activity writers still refuse an extension kind, so no activity is filed on an extension record yet. The column is ready.
- The record page, search, and the REST wire enums keep core's kinds. Tag counts do not include extension records.

## Extension report breakdowns and counts

An extension can add named counts and breakdowns to Reports and Home. A count is how many live records a population
holds. A breakdown splits that count by one field. The steps to declare one are in
[Declare a report count or breakdown](../how-to/add-an-extension.md#declare-a-report-count-or-breakdown).

A declaration is data, never SQL. Core builds the query and runs it through the same report engine as its own reports,
so a unit cannot register a report that skips a gate. Every name is `ext:<unit>:<name>`, built from the declaring unit's
name.

| Part | What it does |
|---|---|
| `Extension.ReportCounts` and `Extension.ReportBreakdowns` | The unit declares a name, a label and a population. A breakdown also names the field it splits by. The boot and `make gen` refuse a bad name, a duplicate, or a population that is neither one of the unit's own kinds nor a core report population. |
| Population | The short name of a record kind the unit declares, or the key of a core report population. A core key has a hyphen and a kind name never does, so the two cannot be confused. |
| Field of a unit's kind | One of the fields the kind declares. |
| Field of a core population | A custom field of the deal, written `cf_<name>`, or of its company, written `company_cf_<name>`. Only the deal populations take one. |
| Multi-select fields | Core's own rule keeps a multi-select field out of reports, because a record would fall in several groups and the groups would not add up. A registered breakdown admits one and splits by the sorted combination of values a record holds. One record is in one group and the groups add up to the whole. |
| Row scope | A unit's kind has no owner, so the kind's `read` right is its whole scope: a seat without it gets nothing and the report is left out. A core population keeps its own row scope. |
| Field masks | A mask that reaches the kind or the population takes the rows out of every group. The answer carries how many in `excluded_by_permission`. |
| Group floor | A group below the analytics group floor is withheld. The floor is the one the analytics questions use, five by default. The smallest remaining group goes with it, and so does the total. A count under the floor is withheld too. |
| `GET /v1/extension-reports` | Answers every report the seat may read. A report whose custom field this installation does not define is left out. |
| Reports and Home | The Reports Pipeline page and the Home side column draw each answer in a panel. Both read the same endpoint. |

The frontend holds no list of registered names. The label and the groups come from the answer, so the screens cannot
drift from the registry. The contract's `kind` values `count` and `breakdown` are held equal to the registry's by a
unit test.

What this feature does not do yet:

- A breakdown by a field of a signal's evidence is not offered. Evidence members are named now, but no report reads
  them yet.
- A report is not an entity of the analytics question builder, and has no drill-through.
- A core population other than the deal's takes counts only.

## Extension signal types with structured evidence

An extension can add its own kinds of signal, each with an evidence layout declared as data. The steps to declare one
are in [Declare a signal type](../how-to/add-an-extension.md#declare-a-signal-type).

The maintainers chose one database kind per extension type, spelled `ext:<unit>:<kind>`, over one generic `ext` kind
with the type inside the evidence. Every filter, count and screen already switches on `kind`, and the spelling matches
record kinds and company types. The spelling is permanent once a row carries it.

```mermaid
flowchart LR
    U[Unit calls Signals Create] --> C{Kind declared by this unit?}
    C -- no --> R[Refused]
    C -- yes --> L{Evidence fits the layout?}
    L -- no --> R
    L -- yes --> F{Fingerprint already filed?}
    F -- yes --> E[Answer the signal already filed]
    F -- no --> W[Write signal, audit row and event, with provenance]
```

| Part | What it does |
|---|---|
| `Extension.SignalTypes` | The unit declares each kind: a name, a label and the evidence members. The boot and `make gen` refuse a bad name, a duplicate member or an unknown type. |
| Evidence layout | Each member has a name, a label, a type (`text`, `number` or `bool`), `Required` and `Personal`. A member the layout does not name, a missing required one or a wrong type is refused with `invalid_evidence`. |
| `Signals().Create` | Files one signal about a deal, company, contact or project, under the caller's own rights. It writes the signal, its audit row and its `signal.detected` event in the unit's transaction. A scheduled job or a bus delivery is refused. |
| Fingerprint | Required. The stored fingerprint is the kind plus the unit's value, so two kinds or two units cannot swallow each other's signals. The existing unique index on live fingerprints holds the repeat out, and the call answers the first signal with `created` false. The answer is a read, so a signal the caller cannot see is not found. |
| Provenance | Core writes the `provenance` member of the evidence claim with the unit and the signed-in seat. A unit cannot set it. The `fields` and `provenance` members of a claim are also refused on a request to create or edit a signal. |
| Only the unit files it | `POST /v1/signals` refuses an `ext:` kind with the field `kind`, because only the declaring unit's call checks the layout, the owner and the fingerprint. |
| Evidence is fixed | The evidence of an extension signal cannot be replaced by `PATCH /v1/signals/{id}`, because a replacement would drop the layout members and the provenance. |
| `signal_kind_check` | Keeps core's sixteen kinds as a closed list and also admits the `ext:<unit>:<kind>` spelling. Which names exist is decided by the composed set at boot. |
| Contract | `kind` on a signal is a string that matches core's kinds or the extension spelling. A client must treat a kind it does not know as a kind of its own. The list filter `kind` keeps core's sixteen values. |
| Screens | A kind with no catalog text shows its own name, without the `ext:<unit>:` prefix. |

### Erase

Erasing or anonymizing a contact rewrites the extension signals whose subject is the contact, and those the resolver
attributed to them. The summary and each claim's snippet are
free text a unit wrote, so both become `[erased]`. A layout member marked `Personal` is removed. A signal whose kind no
unit declares loses all its members, because nothing says what is safe to keep. The provenance stays, because it names a
unit and a seat and never the subject. Signals of core's own kinds are not rewritten by this path.

What this feature does not do yet:

- A report does not read evidence members. See [the reports section](#extension-report-breakdowns-and-counts).
- Signals about a contact are not in the data-access export, as noted for the audited fix of evidence.
- The list filter on `kind` does not take an extension kind.
- Erase reaches the signals whose subject is the contact, and those the resolver attributed to the contact. A `Personal`
  member on a signal about a company or deal that is not attributed to the contact is not reached, because nothing links
  it to one.
- An extension signal cannot be moved to another kind by an edit.
- Subjects stay deal, company, contact and project.
