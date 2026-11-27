<!-- prose:plain -->
<!-- describes: backend/pkg/extension/members.go backend/internal/compose/extmembers.go backend/pkg/extension/need.go backend/pkg/extension/models.go backend/pkg/extension/objects.go backend/internal/compose/extcapability backend/internal/compose/extruntime.go -->
# What an extension can use from the core

An extension can use the core's AI models, its file store and the list of its members. It must say so in its declaration first. This page is for the author of an extension and for the admin who installs one. It lists the three capability names, the refusals, and where each limit lives in the code.

## Declare what you need

A unit lists each capability in `Extension.Needs`, with one sentence that says what it does with it. The core grants what is listed. A call to anything else returns `extension.ErrNotDeclared`.

```go
Needs: []extension.Need{
    {Capability: extension.CapabilityModels, Reason: "Writes a one-line summary of a deal's changes."},
    {Capability: extension.CapabilityObjects, Reason: "Keeps the summaries it wrote."},
},
```

| Name | Grants | Published type |
|---|---|---|
| `models` | `Runtime.Models().Complete` | [models.go](../../backend/pkg/extension/models.go) |
| `objects` | `Runtime.Objects().Put`, `Get` and `Delete` | [objects.go](../../backend/pkg/extension/objects.go) |
| `members` | `tx.Core().Members().List` | [members.go](../../backend/pkg/extension/members.go) |

The boot refuses a need with an unknown capability, a reason that is empty or runs over one line, or a capability listed twice. The core reads the declaration again on each call. A unit that stops declaring a need loses it on its next call.

The declared needs are in `manifest.generated.json` and in the `needs` list that `GET /v1/extensions` returns. Settings, Extensions shows them under "Uses from the core", each with the unit's reason, to the admin who installs the unit.

## Read who holds a role

`tx.Core().Members().List(ctx, extension.MemberQuery{Role: "admin"})` answers the active members who hold the role: `ID`, `DisplayName` and `IsAgent`. An empty `Role` lists every active member. The list is sorted by name and ends at `extension.MaxRosterMembers`. A service member is not listed, because nobody signs in as it. The answer carries no email and no roles.

| Case | Answer |
|---|---|
| The unit did not declare `members` | `ErrNotDeclared` |
| A scheduled job, or a delivery with no service member | `ErrForbidden` |

## Use the models

`Models().Complete(ctx, task, req)` takes the name of a task the unit declared, a system text and messages, and returns text. The unit never sees a provider, a model name or a key.

### Declare the task

A unit declares each AI task in `extensions/<unit>/api/ai-tasks.yaml`, the way it declares a job in `api/jobs.yaml`. The task is named `ext_<unit>_<task>`, and the unit passes that name to `Complete`.

```yaml
overlay: 1.0.0
info: {title: finance, version: "1.0.0"}
actions:
  - target: $.tasks['ext_finance_summarise_order']
    update:
      display_name: "Order summary"
      summary: "Writes a one-line summary of an order."
      ladder: [cheap_cloud, premium]
      execution_mode: interactive
      on_budget_exhausted: degrade
      status: shipped
      subject_restriction: honour
      company_context: none
      no_payload: true
      sites: [summarise_order]
```

`make gen` reads the merged contract and refuses a task that breaks a rule:

| Rule | Why |
|---|---|
| The name starts with the unit's `ext_<unit>_` | The call log and Settings show whose task it is |
| Every tier in `ladder` is a tier the contract declares | A task with an unknown tier could not be routed |
| `interactive` pairs with `degrade` and `background` with `queue` | An `interactive` task answers someone who waits, and a `background` task waits for the next budget period |
| `subject_restriction` is `honour`, `no_payload` is `true`, `company_context` is `none` | The core cannot see inside a prompt the unit wrote, so it keeps none of it and adds none |
| `display_name`, `summary` and at least one site are set | Settings shows them |

A unit that declares a task must also declare the `models` need, or the boot refuses it. At boot the core registers the tasks in the router. Settings, AI then shows each task by name, with its own budget, call log and overrides. Certification does not walk them, because it covers the core tasks only.

| What the core does | Where |
|---|---|
| Picks the model from the installation's own model choice, for the unit's own task | the unit's `api/ai-tasks.yaml` |
| Removes secrets from the request before it leaves | the secret stripper of the AI service |
| Counts the call against the budget and writes the call record | the AI service |
| Names the unit and the task beside the call's correlation id in the audit trail | `system_log`, action `extension_model_call` |
| Records the record a request is about, when `About` names one | the call record's subject, which erasure reads |
| Refuses a contact under a restriction of processing | the request's `About` list |

`Tier` pins a request to the cheap or the premium model. The installation still decides which model that is. A tier with no model bound fails the call.

`Attachments` sends images (`image/png`, `image/jpeg`, `image/webp`, `image/gif`) or `application/pdf` files, up to 8 files of 20 MB each. A model that cannot read a file's type fails the call with `ErrInvalid`. It never drops the file. A filled-in form is contact data, so name its record in `About`.

`JSONSchema` makes the model answer one JSON value of that schema, and nothing around it.

A request names the core records its text was built from in `About`, or says in `NoContactData` why it holds no contact's data. A request with neither is refused with `ErrInvalid`. A request that names a contact under a restriction of processing is refused with `ErrForbidden`.

| Call | Answer |
|---|---|
| The unit did not declare `models` or `objects`, or the task is not one it declared | `ErrNotDeclared` |
| A scheduled job or a bus delivery calls an `interactive` task | `ErrForbidden` |
| The request or the name is wrong in form, or an object is over the size cap | `ErrInvalid` |
| The request names a contact under a restriction of processing | `ErrForbidden` |
| `Get` of an object that is not there | `ErrNotFound` |
| `Put` past the unit's object count or byte quota | `ErrObjectQuotaReached` |
| The model or the store fails, or this process has none bound | an error with no detail, so no provider or path shows |

A scheduled job or a bus delivery has nobody waiting, so it may call `background` tasks only. A `background` task queues when the budget runs out, and an `interactive` task degrades. When `NoContactData` is set, the core writes its reason to the log.

A failed model call is not answered from the unit's own facts: the unit gets the error at once.

The core keeps no payload of an extension's call, because it cannot know what the unit put in its own prompt. The workspace budget serves all units and there is no cap for each one yet. Each task has its own line in `AI Usage`, so the spend of each unit shows.

## Use the file store

Each unit has a key prefix of its own in each workspace: `<workspace>/ext/<unit>/`. The core builds the key from the call's workspace and the unit's name. The unit passes only a flat name, so it cannot name another unit's key or a core attachment.

| Rule | Value |
|---|---|
| Name characters | lower-case letters, digits, `.`, `-`, `_`; at most 128; no leading dot |
| Size of one object | `extension.MaxObjectBytes`, the same cap as an inbound file |
| Objects of one unit | `extension.MaxObjectsPerUnit` |
| Bytes of one unit's objects together | `extension.MaxObjectBytesPerUnit` |
| Content type | found by the core from the bytes, never taken from the unit |
| Core attachments | not readable by a unit |

The core counts each unit's objects and bytes in the table `extension_object_usage`. A `Put` that replaces an object counts only the change in its size. So a unit at its quota can still replace or shrink an object.

Each `Put` and `Delete` writes an audit row on that table in the same transaction, before the bytes are stored or deleted. The row names the unit, the call, the object's name and its size. A call whose audit row cannot be written stores nothing and answers an error with no detail. The audit log keeps these rows, so an object's name must not hold personal data.

An object is deleted when:

| Event | How |
|---|---|
| An admin runs the data reset | the reset deletes the whole workspace prefix, and `ext/` is under it |
| The unit is removed from the composed set | each boot deletes the prefix of every unit recorded earlier and no longer composed, and then that unit's count. A boot with no file store, or a failed delete, leaves it for the next boot and does not stop the start |
| The unit deletes it | `Objects().Delete` |

Erasing one contact deletes none of a unit's objects, because no object is tied to a contact. A unit that stores personal data in an object deletes it with `Delete`. No object is deleted by age. The core keeps a count of a unit's objects and no list of them. A unit that wants an object removed after a time deletes it itself.

## How to read this page

`ErrNotDeclared`, `ErrInvalid` and `ErrForbidden` are values in [pkg/extension](../../backend/pkg/extension). The reasons for this design are in [extension-capability-grants.md](../explanation/extension-capability-grants.md).
