<!-- describes: backend/pkg/extension extensions backend/internal/compose/extcapability -->
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

## The doors from a unit into the core

A unit runs in the core's process, so its code could reach anything. The design gives it three doors instead. Each is a published port in `backend/pkg/extension`, and the core builds the other side.

```text
 unit code (extensions/<name>)
   │
   ├─ 1  rt.Tx ─────────▶ the unit's OWN tables      as role ext_runtime;
   │                      ext.ext_<name>_*            a core table is refused
   │
   ├─ 2  tx.Core() ─────▶ core records               the caller's live RBAC, row-scope check,
   │     read doors       (contacts, deals, ...)     audit row, outbox event; the core stamps
   │                                                  the unit's name
   │
   └─ 3  rt.Models()  ──▶ what the installation OWNS  gate: did the unit declare the need?
         rt.Objects()     (AI key and budget,         then validate, trace, call, map the
                          S3 credentials)             error. No secret crosses the line.
```

| Door | Use it for | What guards it | Page |
|---|---|---|---|
| 1. `rt.Tx` | The unit's own data | A database role with no core table | [add-an-extension.md](../how-to/add-an-extension.md) |
| 2. `tx.Core()` and the read doors | A core record | The write path of the product, under the caller's rights | [read-core-records-from-an-extension.md](../how-to/read-core-records-from-an-extension.md) |
| 3. `rt.Models()` and `rt.Objects()` | A service the installation pays for or holds a secret for | `Extension.Needs`, checked on each call | [expose-a-core-service-to-extensions.md](../how-to/expose-a-core-service-to-extensions.md) |

Door 3 differs from door 2 in who the call acts as. Door 2 acts as the signed-in member, so the member's rights decide it. A model or file call has no record behind it, so the installation's own choice decides: the model the admin picked, the shared budget, a key prefix of the unit's own. The admin reads each declared need and the author's reason under Settings, Extensions before the unit runs. The reasons are in [extension-capability-grants.md](extension-capability-grants.md).

A new core service for units is a new method on `Runtime`, a published port and a core side that holds the gate. The steps are in [expose-a-core-service-to-extensions.md](../how-to/expose-a-core-service-to-extensions.md).

### What the core lends a unit, and what is built

One row for each thing a unit can ask of the core. The tool bag below lists which records `tx.Core()` reaches.

| Core capability | A unit calls | Declared in | Decided by | State |
|---|---|---|---|---|
| Its own tables | `rt.Tx`, `tx.Exec`, `tx.Query` | `Migrations` | The `ext_runtime` role: unit tables only | Built |
| Core records, read and write | `tx.Core()` | none | The caller's live rights | Built |
| Tell a member about a record | `tx.Core().Notices()` | none | The member's own notification settings | Built |
| Propose where a message belongs | `tx.Core().LinkSuggestions()` | none | The caller's live rights | Built |
| Read for a job tick or a delivery | `rt.Deals`, `Authority`, `Directory`, `Activities`, `SetAside` | `SystemReads` | Nobody; an attended call is refused | Built |
| Land records from its own system | `rt.Ingest`, `IngestBatch`, `IngestPosition` | `Ingress` | The member whose credential produced the record | Built |
| Run its own job soon | `rt.SyncNow` | `Jobs` | The unit's own declared job | Built |
| A vendor's access token | `rt.OAuthAccessToken`, `OAuthDisconnect` | `OAuthApps` | The admin's consent | Built |
| Its own secrets | `rt.Secrets` | `Secrets` | The unit's own namespace | Built |
| The installation's AI models | `rt.Models().Complete` | `Needs`, `models` | The installation's model choice and shared budget | Built |
| The installation's file store | `rt.Objects().Put`, `Get`, `Delete` | `Needs`, `objects` | A key prefix of the unit's own | Built |

What a unit does not get:

| Not offered | State | Why |
|---|---|---|
| The core vault | Never | It holds connector tokens and provider keys. A vendor token comes through `OAuthApps`. |
| The bytes of a core attachment | Never | An attachment follows its record's row-level rights, which a read by key skips. |
| Custom field values on a core write | Not yet | The write is refused with `ErrInvalid` until the field catalog is checked against it. |
| A spend cap for each unit's model calls | Not yet | One shared budget serves all units, and the audit rows say which unit spent it. |
| Expiry or an index of a unit's objects | Not yet | A unit deletes its own objects with `Delete`. |

### What a unit can change in the screens

| Capability | Declared in | Where it shows | Decided by | State |
|---|---|---|---|---|
| Its own screen under Settings | The default export of its frontend package | `#/ext/<name>` | The settings rules of the page | Built |
| An app in the home side drawer | `App` | A drawer row and `#/app/<name>` | `read` on the unit's RBAC object | Built |
| A button or a note on a core screen | `Entries` | The log form, and the contact, company and deal pages | `read` on the unit's RBAC object | Built |
| Other wording for a core key | Files in `frontend/i18n-overrides/` | Any frontend text of the core | The build checks only | Built |

| Not offered | State | Why |
|---|---|---|
| A second app, a new drawer heading, or a row position | Never | The core draws the drawer, so a unit cannot restyle it. |
| A path below `#/app/<name>` | Not yet | The app route holds the unit's name and nothing else. |
| Overriding a record type's name, AI prompts, agent tools or text the backend writes | Not yet | An override changes frontend wording only. |

How to do each: [declare-an-app-in-the-home-drawer.md](../how-to/declare-an-app-in-the-home-drawer.md), [Override core wording](../how-to/add-an-extension.md#override-core-wording) and [Offer an entry on a core screen](../how-to/add-an-extension.md#offer-an-entry-on-a-core-screen).

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
| `Reach` | Reach declares the unit's tables holding a core company, contact or lead id. |
| `Privacy` | Privacy declares, for every table the unit owns, which core record its rows belong to, which columns are personal data and the retention class. |
| `Documents` | Compliance documents the unit ships for its jurisdictions. |
| `Tools` | Governed agent tools the unit contributes: named operations running at a requested risk tier. |
| `Channels` | Messaging providers this unit supplies TRANSPORT for. |
| `CompanyTypes` | Company types this unit adds beside core's own. |
| `RecordKinds` | Kinds of record this unit adds beside core's own, stored as `ext:<unit>:<kind>`. |
| `LinkTypes` | Ways this unit says two core records relate, stored as `ext:<unit>:<type>`. |
| `ReportCounts` | Named numbers this unit adds to Reports. |
| `ReportBreakdowns` | Counts split by one field, added to Reports. |
| `SignalTypes` | Kinds of signal this unit adds beside core's own, each with an evidence layout declared as data. |
| `FinanceSources` | Accounting systems this unit can read into the core's finance mirror. |
| `Secrets` | Secret keys the unit declares it will use, by name and scope. |
| `Jobs` | Scheduled background jobs the unit contributes: named cadenced passes that fan out over the fleet, one workspace per tick. |
| `Subscriptions` | Events the unit reacts to: named listeners over the installation's own event bus, each naming the types it wants and the function one delivery runs. |
| `Ingress` | Providers this unit brings records IN from, and the record kinds it lands through the core's own capture pipeline. |
| `Inbound` | Session-less HTTP edges this unit asks the core to mount: a signed POST from a party that holds no session and no seat. |
| `OAuthApps` | Vendors whose consent flow the core runs for this unit. |
| `SystemReads` | System reads the unit's jobs and subscriptions make. |
| `Needs` | Core capabilities the unit uses, with a reason each. |
| `Migrations` | Unit's SQL schema layer: a read-only filesystem holding the MigrationsDir directory of NNNN_name.up.sql/.down.sql pairs, which a unit supplies with `//go:embed migrations`. |
| `FailureClasses` | Ways this unit's background jobs fail, named in the unit's own vocabulary, each with what an operator does about it. |
| `Entries` | Unit's ways into its own screen from core screens: the Log activity form and the contact, company and deal headers. |
| `Settings` | Values an admin can change for the unit, drawn by the core on the unit's card of the Extensions page. |
| `Panels` | Unit's components drawn inline on core pages, such as a deal page's side column. |
| `App` | Unit's own screen offered in the home side drawer, apart from its sub-setting under Settings. |
| `Instrumentation` | Instrumentation observes the whole process and the browser: logs, requests, jobs, panics, errors and navigation. |

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
[extension-record-kinds-links-signals-and-reports.md](../reference/extension-record-kinds-links-signals-and-reports.md).

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
- [declare-an-app-in-the-home-drawer.md](../how-to/declare-an-app-in-the-home-drawer.md): give a unit a row in the
  home side drawer.
- [expose-a-core-service-to-extensions.md](../how-to/expose-a-core-service-to-extensions.md): let units use a core
  service that holds a secret.
- [override-core-wording-from-an-extension.md](../how-to/override-core-wording-from-an-extension.md): change the
  words the core shows.
- [extensibility.md](extensibility.md): why the tier is built this way, and what it guards.
