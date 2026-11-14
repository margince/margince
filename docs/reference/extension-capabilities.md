<!-- prose:plain -->
<!-- describes: backend/pkg/extension/need.go backend/pkg/extension/models.go backend/pkg/extension/objects.go backend/internal/compose/extcapability backend/internal/compose/extruntime.go -->
# What an extension can use from the core

An extension can use the core's AI models and its file store, but only after it says so in its declaration. This page is for the author of an extension and for the admin who installs one. It lists the two capability names, the refusals, and where each limit lives in the code.

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

The boot refuses a need with an unknown capability, a reason that is empty or runs over one line, or a capability listed twice. The core reads the declaration again on each call. A unit that stops declaring a need loses it on its next call.

The declared needs are in `manifest.generated.json` and in the `needs` list that `GET /v1/extensions` returns. Settings, Extensions shows them under "Uses from the core", each with the unit's reason, to the admin who installs the unit.

## Use the models

`Models().Complete` takes a system text and messages, and returns text. The unit never sees a provider, a model name or a key.

| What the core does | Where |
|---|---|
| Picks the model from the installation's own model choice | `ai-tasks.yaml`, task `extension_call` |
| Removes secrets from the request before it leaves | the secret stripper of the AI service |
| Counts the call against the budget and writes the call record | the AI service |
| Names the unit beside the call's correlation id in the audit trail | `system_log`, action `extension_model_call` |
| Refuses a contact under a restriction of processing | the request's `About` list |

A request names the core records its text was built from in `About`, or says in `NoContactData` why it holds no contact's data. A request with neither is refused with `ErrInvalid`. A request that names a contact under a restriction of processing is refused with `ErrForbidden`.

| Call | Answer |
|---|---|
| The unit did not declare `models` or `objects` | `ErrNotDeclared` |
| The request or the name is wrong in form, or an object is over the size cap | `ErrInvalid` |
| The request names a contact under a restriction of processing | `ErrForbidden` |
| `Get` of an object that is not there | `ErrNotFound` |
| The model or the store fails, or this process has none bound | an error with no detail, so no provider or path shows |

A failed model call is not answered from the unit's own facts: the unit gets the error at once.

The core keeps no payload of an extension's call, because it cannot know what the unit put in its own prompt. One shared budget serves all units and there is no cap for each one. The audit rows say which unit spent it.

## Use the file store

Each unit has a key prefix of its own in each workspace: `<workspace>/ext/<unit>/`. The core builds the key from the call's workspace and the unit's name. The unit passes only a flat name, so it cannot name another unit's key or a core attachment.

| Rule | Value |
|---|---|
| Name characters | lower-case letters, digits, `.`, `-`, `_`; at most 128; no leading dot |
| Size of one object | `extension.MaxObjectBytes`, the same cap as an inbound file |
| Content type | found by the core from the bytes, never taken from the unit |
| Core attachments | not readable by a unit |

An object is deleted when:

| Event | How |
|---|---|
| An admin runs the data reset | the reset deletes the whole workspace prefix, and `ext/` is under it |
| The unit is removed from the composed set | each boot deletes the prefix of every unit recorded earlier and no longer composed. A boot with no file store, or a failed delete, leaves it for the next boot and does not stop the start |
| The unit deletes it | `Objects().Delete` |

Erasing one contact deletes none of a unit's objects, because no object is tied to a contact. A unit that stores personal data in an object deletes it with `Delete`. No object is deleted by age. The core keeps no index of a unit's objects, so a unit that wants an object removed after a time deletes it itself.

## How to read this page

`ErrNotDeclared`, `ErrInvalid` and `ErrForbidden` are values in [pkg/extension](../../backend/pkg/extension). The reasons for this design are in [extension-capability-grants.md](../explanation/extension-capability-grants.md).
