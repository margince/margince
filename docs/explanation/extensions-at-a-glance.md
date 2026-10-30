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
 build/composition/             generated wiring (ignored by git)
        |
        |  extensions/<name>/manifest.generated.json is written beside each unit
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

## The tool bag: what a unit can do to each core record

Read a row as "can a unit do this to that record?" A ✅ is a door the core offers. A · is no door. Every table
in this section is written from the source of `backend/pkg/extension` by `make extdoc-gen`, and a gate fails a
stale page.

<!-- generated:extensions:start -->
<!-- Written by tools/gen-extdoc from backend/pkg/extension: change the code, then run `make extdoc-gen`. -->

| A unit can… | Company | Deal | Contact | Activity | Signal |
|---|:-:|:-:|:-:|:-:|:-:|
| **Land** it from its own system (create or update, by the id that system gave it) | ✅ | ✅ | · | ✅ | ✅ |
| **Move** it to another stage | · | ✅ | · | · | · |
| **Create** it inside the unit's own transaction (`Tx.Core()`) | · | · | · | ✅ | ✅ |
| **Read** and list it, as the user who asked | ✅ | ✅ | · | ✅ | ✅ |
| **Find** it by the id from its own system | ✅ | ✅ | · | · | · |
| **Filter** it by an admin's custom field | ✅ | ✅ | · | ✅ | · |
| **Tell** a member or a role about it (a notice) | ✅ | ✅ | ✅ | · | · |
| **Hear** its events | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Hear** that it was erased | ✅ | ✅ | · | · | · |
| **Hear** that it was merged | ✅ | ✅ | ✅ | · | · |
| **Add a button** to its page | ✅ | ✅ | ✅ | ✅ | · |
| **Add a small note** to its page | ✅ | ✅ | ✅ | · | · |
| **Add a card** to its page | ✅ | ✅ | ✅ | · | · |

### What else a unit can add

Every field of `Extension` that brings something to the core.

| Field | What the core says it is |
|---|---|
| `Jurisdictions` | Unit's jurisdiction packs (policy suppliers to the core retention engine, never actors). |
| `Messaging` | Unit's outbound-messaging rule sets, the country rules the core authorization engine consults before a message goes out. |
| `SubjectRights` | Unit's data-subject-request deadlines. |
| `Documents` | Compliance documents the unit ships for its jurisdictions. |
| `Tools` | Governed agent tools the unit contributes: named operations running at a requested risk tier. |
| `Channels` | Messaging providers this unit supplies TRANSPORT for. |
| `CompanyTypes` | Company types this unit adds beside core's own. |
| `RecordKinds` | Kinds of record this unit adds beside core's own, stored as `ext:<unit>:<kind>`. |
| `LinkTypes` | Ways this unit says two core records relate, stored as `ext:<unit>:<type>`. |
| `ReportCounts` | Named numbers this unit adds to Reports and Home. |
| `ReportBreakdowns` | Counts split by one field, added to Reports and Home. |
| `SignalTypes` | Kinds of signal this unit adds beside core's own, each with an evidence layout declared as data. |
| `FinanceSources` | Accounting systems this unit can read into the core's finance mirror. |
| `Secrets` | Secret keys the unit declares it will use, by name and scope. |
| `Jobs` | Scheduled background jobs the unit contributes: named cadenced passes that fan out over the fleet, one workspace per tick. |
| `Subscriptions` | Events the unit reacts to: named listeners over the installation's own event bus, each naming the types it wants and the function one delivery runs. |
| `Ingress` | Providers this unit brings records IN from, and the record kinds it lands through the core's own capture pipeline. |
| `Inbound` | Session-less HTTP edges this unit asks the core to mount: a signed POST from a party that holds no session and no seat. |
| `OAuthApps` | Vendors whose consent flow the core runs for this unit. |
| `SystemReads` | System reads the unit's jobs and subscriptions make. |
| `Migrations` | Unit's SQL schema layer: a read-only filesystem holding the MigrationsDir directory of NNNN_name.up.sql/.down.sql pairs, which a unit supplies with `//go:embed migrations`. |
| `FailureClasses` | Ways this unit's background jobs fail, named in the unit's own vocabulary, each with what an operator does about it. |
| `Entries` | Unit's ways into its own screen from core screens: the Log activity form and the contact, company and deal headers. |
| `Settings` | Values an admin can change for the unit, drawn by the core on the unit's card of the Extensions page. |
| `Panels` | Unit's components drawn inline on core pages, such as a deal page's side column. |

### Slots on a core page

| Slot | Kind | Where it shows |
|---|---|---|
| `log_activity` | button | Log activity form, on any record it is opened on. |
| `record_header.contact` | button | Verb row in a contact page's header. |
| `record_header.company` | button | Verb row in a company page's header. |
| `record_header.deal` | button | Verb row in a deal page's header. |
| `record_mark.contact` | small note | Small note beside a contact page's header verbs. |
| `record_mark.company` | small note | Small note beside a company page's header verbs. |
| `record_mark.deal` | small note | Small note beside a deal page's header verbs. |
| `record_panel.contact` | card | End of a contact page's research drawer, after its lists. |
| `record_panel.company` | card | End of a company page's Contacts tab, after the reach card. |
| `record_panel.deal` | card | End of a deal page's Edit details drawer, after its lists. |
<!-- generated:extensions:end -->

What stops a unit, in each case:

- **Landing and creating** run as a named member, and that member's rights apply. A field a user edited is left
  alone, and a value that is already filled is never overwritten.
- **Reading** is done as the user who asked. A record outside that user's scope answers "not found".
- **A contact** is never landed by a unit. The core makes one when it resolves the other party of an activity.
- **`Tx.Core()`** carries the same rights check, audit row and event as a core write. It is refused on a job
  tick, because nobody is behind a tick to check the write against.
- **Keeping its own data** beside a record needs no door. A unit writes its own `ext_<name>_…` table and joins it
  by the record's id.
- **Delete, a new column and a custom field** have no door. An admin adds custom fields, and a unit reads and
  filters on them.

### Where the slots sit

```text
 deal page
 +--------------------------------+---------------------------+
 | header        [ unit button ]  | Details                   |   record_header.deal   Entries
 |                                | ...                       |
 | overview, offers, brief ...    | -- Extensions ----------- |
 |                                | [ unit card ]             |   record_panel.deal    Panels
 +--------------------------------+---------------------------+
```

## How far the core reaches into an extension

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

Whole new record kinds, typed links between records, signal types and report counts are declared the same way.
The state of each feature is in
[an-extension-cannot-add-its-own-record-k.md](../reference/an-extension-cannot-add-its-own-record-k.md).

## What every extension gets for free

- **Its own names.** Tables begin with `ext_<name>_` and API paths with `/v1/ext/<name>/`, so a unit's objects
  cannot be mistaken for the core's or for another unit's. This is a naming rule, not access control: units share
  the application database role today, and the Stability note in `backend/pkg/extension/extension.go` says what
  will narrow that.
- **A manifest.** `make composition` writes `manifest.generated.json` beside the unit from what it declares and
  requests, such as its capabilities and risk tiers. An operator can read that list without opening the code. It
  does not describe everything the code does. A gate fails if the file is stale.
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
