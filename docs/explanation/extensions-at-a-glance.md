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

An extension is one Go value. Each thing it can bring is one field of that value
(`Extension` in `backend/pkg/extension/extension.go`). It lists what it wants, and the core does the
wiring.

| I want to… | The field | What you write |
|---|---|---|
| Bring data in from another system | `Ingress` | A named source. The unit hands the core a company, a deal, a signal or an activity, and the core stores it under the rights of a member. |
| Run something on a timer | `Jobs` | A function. The cadence and the account it runs as are set outside the code. |
| React to what happens in Margince | `Subscriptions` | A list of events and a function that runs for each. |
| Accept a call from outside | `Inbound` | A named endpoint. The core checks the caller and the size, and the unit decides what the call means. |
| Send a message through a provider | `Channels` | The send and receive functions of one messaging provider. |
| Connect a vendor account | `OAuthApps` | The vendor's sign-in flow, which the core runs for an admin. |
| Hold a secret | `Secrets` | The names of the secrets the unit needs. It never sees the vault. |
| Give agents a tool | `Tools` | A function behind a verb that the unit's contract declares. |
| Add a screen | `Entries`, `Panels` | A way into the unit's screen, or a card drawn on a record page. |
| Let an admin tune it | `Settings` | One value an admin can change. |
| Read accounting data | `FinanceSources` | Three read-only functions for one accounting system. |
| Read core records | `SystemReads` | The reads that the unit's jobs and subscriptions make. A read that is not listed is refused. |
| Keep its own tables | `Migrations` | SQL files. The tables are named `ext_<name>_…`. |
| Ship legal rules for a country | `Jurisdictions`, `Messaging`, `SubjectRights`, `Documents` | A pack of retention floors, messaging rules, data-subject rights or compliance documents. |

Two more fields are not a capability: `FailureClasses` sorts a job's errors into the ones worth a retry, and
`Name`, `Version` and `Description` say who the unit is.

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
