<!-- prose:plain -->
<!-- describes: backend/pkg/extension/need.go backend/internal/compose/extcapability backend/internal/compose/extruntime.go backend/api/ai-tasks.yaml -->
# Why an extension declares what it uses from the core

An extension uses the core's AI models and file store only when its manifest says so, and the core refuses the rest. This page records that decision and the choices that follow from it. The facts a reader looks up are in [extension-capabilities.md](../reference/extension-capabilities.md).

## Decision

A grant is declared and enforced. It is declared in `Extension.Needs`, so the installing admin can read it before the unit runs. It is enforced on every call, so a unit cannot use a capability it did not name.

A grant that was only declared would be a promise. A grant that was only enforced would be hidden from the admin who must decide whether to install the unit. Each half alone fails in a way the other covers.

## What follows

| Choice | Reason |
|---|---|
| The core calls the model for the unit | A unit that held a provider key could spend any budget and send any data. With the call in the core, the model choice, the secret stripper, the budget and the call record stay the installation's. |
| A unit never reads the core vault | The vault holds connector tokens and provider keys. A unit that needs a vendor token asks for it through `OAuthAccessToken`, one provider at a time. |
| Each unit declares its own tasks in `api/ai-tasks.yaml` | Settings, AI shows a task by name with its own budget, call log and overrides, and the call log says which unit spent. The unit declares the task the way it declares a job, so the core contract needs no edit for a new unit. `Needs: models` stays, because it tells the admin why the unit wants AI. |
| The unit is named in `system_log`, joined by correlation id | The call record carries the correlation id, and its task name starts with the unit's name. A column for the unit on every call row would change a table all core AI work writes, for a fact the task name already holds. |
| A request names its records or says it holds none | The task follows a restriction of processing. The core cannot see inside a prompt a unit wrote, so the unit must say which records it came from. |
| No cap for each unit yet | The workspace budget already stops spend that is too high. The call log now shows spend for each task, and a cap for each unit can follow once there is a number to set. |
| A unit's objects sit under `<workspace>/ext/<unit>/` | The key is built by the core from the workspace and the unit's name. A unit passes a flat name, so it cannot reach another key. |
| A unit cannot read core attachments | An attachment belongs to a record and its row-level rights. A unit reading the bytes by key would skip those rights. A unit that needs a file's content reads the record through the core, which checks them. |
| The core keeps no index of a unit's objects | Deleting all of a workspace's data and removing a unit each delete a prefix, which needs no index. Deleting after a set time would need one, so a unit deletes its own objects with `Delete` until then. |

## How to read this page

Each row in the table is one choice and the reason that supports it.
