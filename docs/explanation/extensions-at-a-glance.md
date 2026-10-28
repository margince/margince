<!-- prose:plain -->
<!-- describes: backend/pkg/extension extensions -->
# Extensions at a glance

An extension is a small add-on that lives in its own folder, `extensions/<name>/`, and gives Margince
something new without a change to any file the core owns. Turn it on by putting the folder there. Turn it
off by taking the folder away.

This page is the short version: what an extension can bring, and where to go to build each part. The
reasons behind the design are in [extensibility.md](extensibility.md).

## How it works

```text
 extensions/<name>/            you write one Go value: what the unit brings
   New() Extension                   (jobs, ingress, panels, secrets, ...)
        |
        |  make composition          reads every folder under extensions/
        v
 build/composition/             generated wiring, plus manifest.generated.json
        |
        |  api and worker start
        v
 +-----------------------------------------------+
 |  1. check the whole set    bad unit: no boot  |
 |  2. apply it to the core   only if all pass   |
 +-----------------------------------------------+
        |
        v
 the core runs the unit's parts      jobs tick, events reach it,
                                     panels draw on record pages
```

Nothing in the declaration reaches the core by itself. The core reads the list, checks it, and only then
applies it.

## Add one in four steps

```text
 1. make the folder      extensions/<name>/   its own Go module, name = folder name
        |
 2. declare              New() returns Extension{ Name, Version, Description, ...fields }
        |
 3. generate and check   make composition     writes the manifest; a stale one fails a gate
        |
 4. run                  make dev             the unit is on; remove the folder to turn it off
```

[add-an-extension.md](../how-to/add-an-extension.md) walks through each step with code.

## Where a service member fits

A feed that runs on a timer needs someone to land its records as. A colleague is a bad choice, because the
feed stops when they leave. A service member is the account made for this. No human signs in as it.

```text
 admin                          unit                               core
   |  declare member for "relay"  |                                  |
   |----------------------------------------------------------------->|  member made, no login,
   |                              |                                  |  role service_ingest
   |  file the feed secret under the member                          |
   |----------------------------->|                                  |
   |                              |  timer tick: rt.Ingest(member)   |
   |                              |--------------------------------->|  secret on file, and member
   |                              |                                  |  declared for this unit?
   |                              |        record lands, audited     |  yes: lands with the member's
   |                              |<---------------------------------|  rights. no: refused.
   |  archive the member  -------------------------------------------->  the feed stops here, only
```

The member is not counted against the seat limit and is never deleted, only archived. The steps and the
call are in [declare-a-service-member.md](../how-to/declare-a-service-member.md).

## The tool bag

An extension is one Go value, `Extension`, returned by its `New()`. Each field is one kind of thing the core lets a
unit plug in. A tick in a unit's column means that unit uses the field today, so the four units that ship are
worked examples to copy from.

| What a unit can add | What it is for | [`de`](../../extensions/de) | [`finance-demo`](../../extensions/finance-demo) | [`openchannel`](../../extensions/openchannel) | [`vn`](../../extensions/vn) |
|---|---|:-:|:-:|:-:|:-:|
| `Ingress` | Bring records in from another system, under a member's rights | · | · | ✅ | · |
| `Inbound` | Accept a signed call from a party with no session | · | · | ✅ | · |
| `Jobs` | Run something on a timer | · | · | ✅ | · |
| `Subscriptions` | React to events in Margince | · | · | ✅ | · |
| `Channels` | Send and receive through a messaging provider | · | · | ✅ | · |
| `OAuthApps` | Sign an admin in to a vendor | · | ✅ | · | · |
| `Secrets` | Name the secrets the unit keeps | · | · | ✅ | · |
| `Tools` | Give people and agents a governed operation | · | · | ✅ | · |
| `Entries` | A button on a core page | · | · | ✅ | · |
| `Panels` | A card on a record page | · | · | · | · |
| `Settings` | A switch or a line of text an admin can change | · | · | · | · |
| `FinanceSources` | Read an accounting system | · | ✅ | · | · |
| `SystemReads` | Name the reads that jobs and subscriptions make | · | · | · | · |
| `Migrations` | Keep its own tables, `ext_<name>_…` | · | · | ✅ | · |
| `FailureClasses` | Sort job errors into those worth a retry | · | · | ✅ | · |
| `Jurisdictions` | Legal retention floors for a country | ✅ | · | · | ✅ |
| `Messaging` | Outbound messaging rules for a country | ✅ | · | · | ✅ |
| `SubjectRights` | Data-subject request deadlines | ✅ | · | · | · |
| `Documents` | Compliance documents for staff | ✅ | · | · | · |

`Name`, `Version` and `Description` say who the unit is, and every unit sets them.

### Where a unit's screen can appear

```text
 deal page
 +--------------------------------+---------------------------+
 | header        [ unit button ]  | Details                   |   record_header.deal   Entries
 |                                | ...                       |
 | overview, offers, brief ...    | -- Extensions ----------- |
 |                                | [ unit card ]             |   record_panel.deal    Panels
 +--------------------------------+---------------------------+
```

| Slot | Kind | Where it shows |
|---|---|---|
| `log_activity` | button | The Log activity form |
| `record_header.contact`, `.company`, `.deal` | button | The verb row in the page header |
| `record_mark.contact`, `.company`, `.deal` | small note | On the record page, for readers who may read the unit's data |
| `record_panel.deal` | card | The side column of a deal page |

## How far an extension reaches into the core, and how far the core reaches into it

A unit is a guest with a short list of doors. Every door runs under the rights of a member, and a write through
it is audited like any other. A unit never touches a core table with its own SQL.

### What an extension can do to core records

| Action | Allowed | How, and what stops it |
|---|:-:|---|
| Create or update a company, deal or activity under its own outside id | ✅ | `Ingest`, `IngestBatch`. It lands as a named member, and that member's rights apply. |
| Move a deal to another stage | ✅ | `Ingest`, with the stage's id. A lost stage needs a reason. |
| Overwrite a field a user edited | 🚫 | The core leaves it alone and names it in the result. |
| Overwrite a value that is already filled | 🚫 | Adopting and updating fill empty fields only. |
| Read deals, companies, signals, activities, the custom-field list | ✅ | The read doors. It reads as the caller, and a record outside that caller's scope answers "not found". |
| Make the core's own write inside the unit's transaction | ✅ | `Tx.Core()`. Same rights check, audit row and event as a core write. |
| Record what its own SQL did | ✅ | `Tx.Record`. It is offered, not forced. |
| Tell a member or a role about a record | ✅ | Notices. The unit names no address, and the core picks who may open the record. |
| Keep its own data | ✅ | Its own `ext_<name>_…` tables, joined by the record's id. |
| Add a column to a core record | 🚫 | A unit carries its fields in its own table and its outside id. |
| Create a custom field | 🚫 | An admin adds them. A unit reads and filters on them. |
| Delete a core record, change a role, touch another unit's data | 🚫 | No door exists. |

### What the core does with an extension

| The core… | Happens | When it happens |
|---|:-:|---|
| Runs the unit's job | ✅ | On the cadence in the unit's contract fragment. No caller is behind a tick. |
| Delivers an event | ✅ | For each event type the unit declared. Erase and merge events carry ids only. |
| Passes on an outside call | ✅ | After the core checked the workspace, the size and the rate. |
| Serves the unit's agent tool | ✅ | At the risk tier the contract declares, through the same admission gate as core tools. |
| Draws the unit's button or card | ✅ | In the slot the unit named, with the record's type and id. |
| Reads the unit's declaration | ✅ | At boot and on the Extensions settings page. A bad set stops the boot. |
| Reads a country pack | ✅ | When retention or messaging rules apply. A pack is data, not an actor. |
| Waits for the unit before saving a record | 🚫 | Never. A failing unit does not change a core record. |
| Runs the unit's SQL or shares its memory | 🚫 | Never. |

### How a unit carries its own fields

```text
 core record (company, deal, activity)
   |  custom fields an admin adds   listed by the read door; a unit reads and filters, never creates
   |  the unit's outside id         source_system + external id, so the unit finds its record again
   |  the unit's own table          ext_<name>_<table>, joined by the record's id,
   |                                shown on the page by a Panel or a mark
```

Whole new record kinds, typed links between records, signal types and report counts are being added to the
declaration one feature at a time. The state of each is in
[an-extension-cannot-add-its-own-record-k.md](../reference/an-extension-cannot-add-its-own-record-k.md).

## What every extension gets for free

- **Its own address space.** Tables begin with `ext_<name>_`, API paths with `/v1/ext/<name>/`, and no unit can
  reach another's.
- **A manifest.** `make composition` writes `manifest.generated.json` from the declaration, so an operator can read
  what a unit does without reading its code. A gate fails if the file is stale.
- **The core's checks.** A write through the core is checked for rights, audited and published as an event. A
  member's permissions apply to what the unit lands.
- **A clean exit.** A bad declaration stops the boot before anything is applied.

## A worked example

`extensions/finance-demo` is the reference unit. It declares one accounting source with generated data and one
sign-in app, in about forty lines. `extensions/openchannel` shows an entry into a screen. Read them next to the
first how-to below.

## Where to go next

- [add-an-extension.md](../how-to/add-an-extension.md): build a unit from an empty folder.
- [land-companies-and-deals-from-an-extension.md](../how-to/land-companies-and-deals-from-an-extension.md): bring
  data in under a member's rights.
- [read-core-records-from-an-extension.md](../how-to/read-core-records-from-an-extension.md): read the core's
  records.
- [declare-a-service-member.md](../how-to/declare-a-service-member.md): the account a timed job runs as.
- [send-a-notice-from-an-extension.md](../how-to/send-a-notice-from-an-extension.md): tell a user about a record.
- [extensibility.md](extensibility.md): why the tier is built this way, and what it guards.
