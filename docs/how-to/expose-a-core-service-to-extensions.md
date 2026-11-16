<!-- prose:plain -->
# Expose a core service to extensions

Use this page when a unit needs something the core owns and holds a secret for or spends shared money on, such as the AI models or the object store. The unit asks for it by name, and the core makes the call. For why the design is this way, read [Why an extension declares what it uses from the core](../explanation/extension-capability-grants.md). To choose between this and the other doors into the core, read [The doors from a unit into the core](../explanation/extensions-at-a-glance.md#the-doors-from-a-unit-into-the-core).

The worked examples are `models` and `objects`. Copy their shape.

## Check that a capability is the right door

| The unit needs | Door | Page |
|---|---|---|
| A core record, read or written under a user's rights | `tx.Core()` | [read-core-records-from-an-extension.md](read-core-records-from-an-extension.md) |
| To tell a member about a record | `tx.Core().Notices()` | [send-a-notice-from-an-extension.md](send-a-notice-from-an-extension.md) |
| To use something the installation owns: a key, a budget, a bucket | A capability in `Extension.Needs` | This page |
| A pure function, such as a format or a check | Import it from `backend/pkg/**` | none |

Add a capability only when the core must hold a secret or a shared budget for the unit. A pure function needs no gate.

## Steps

1. **Name it.** Add a `Capability` constant in `backend/pkg/extension/need.go` and list it in the `switch` of `Need.Validate`. The boot refuses a need with a name that is not there.
2. **Publish the port.** Add the interface and its request and answer types in a new file under `backend/pkg/extension/`, and add one method to `Runtime` in `runtime.go`, as `Models()` and `Objects()` are. The port holds no core type, so a unit never imports one. A new method on `Runtime` breaks every unit test fake that implements it, so say so in the PR.
3. **Write the core side.** Add a file under `backend/internal/compose/extcapability/` that implements the port. It takes the core service, the unit's name and a `Gate`. Every method calls `Gate` first, validates its input, calls the core service, and maps each failure to a published error.
4. **Hand it over.** Add a method on `callRuntime` in `backend/internal/compose/extensiontx.go` that builds the core side with the unit's name and a gate that calls `r.needed(ctx, extension.Capability…)`. `needed` reads the composition again on each call, so a unit that stops declaring a need loses it on its next call.
5. **Bind the service.** Pass the core service into the `callRuntime` dependencies where the role is put together. A role that binds none answers an `…Unwired` error, as `ErrModelsUnwired` does.
6. **Document the facts.** Add a row to the capability table in [extension-capabilities.md](../reference/extension-capabilities.md), with its limits, refusals and audit row. Add a section for the unit author to [add-an-extension.md](add-an-extension.md). Update the capability line of `ComposedExtensionNeed` in `backend/api/crm.yaml` and run `make gen`.
7. **Test it.** The tests of `extcapability` call the real core side with a fake service behind the gate. Cover the call with no declaration, each refusal, and one failure of the service.

## The guards to decide for each capability

The plumbing is small. The review is about these questions, and each answer is in the core side, never in the unit.

| Question | The answer for `models` and `objects` |
|---|---|
| Who is the unit? | The core says so, from the `Runtime` it minted. The request carries no unit name. |
| Which workspace? | The caller's principal, never an argument. |
| What can one unit reach of another's? | Nothing. `objects` builds the key from the workspace and the unit's name, and a name has no separator. |
| Can the call spend without a trace? | No. `models` writes an audit row naming the unit before the call. |
| Does the data follow the rules about a person? | `models` needs `About` or `NoContactData`, and refuses a contact under a restriction of processing. |
| What does a failure show the unit? | An error with no provider, host or path. The core logs the real error with the unit's name. |
| What is the size or rate cap? | `objects` caps a body at `extension.MaxObjectBytes`. `models` has one shared budget and no cap for each unit. |
| What happens when the unit is removed? | `objects` deletes the unit's prefix at the next boot. A capability that stores anything needs this too. |
| Is the secret ever in the unit's reach? | Never. If the unit needs a vendor's own token, that is a different door. |

A capability that cannot answer one of these is not ready to expose.
