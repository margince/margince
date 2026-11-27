# Customize a deployment

Use this page to make one installation look and behave your way without forking the core. Four parts do
this, and each has its own page for the steps. Pick the parts you need, in this order.

## 1. Choose the extension set

A folder under `extensions/` does not turn a unit on. `instance/go.mod` names the set that the API, the worker and
the migration program build with. Add a unit to `require` there, with a local `replace` or a released version, and
run `make build`. The steps are in [build-a-custom-margince.md](build-a-custom-margince.md) and
[install-extensions-as-go-modules.md](install-extensions-as-go-modules.md). To write a unit of your own, start at
[add-an-extension.md](add-an-extension.md).

## 2. Change the wording

A unit can replace core text, such as "Deals" with "Opportunities". It ships one flat JSON file per language at
`extensions/<name>/frontend/i18n-overrides/<locale>.json`, and `make gen` reads the files of every enabled unit. The
steps and the limits are in [override-core-wording-from-an-extension.md](override-core-wording-from-an-extension.md).

## 3. Add an app to the home side drawer

A unit may declare one `extension.App`. The drawer then shows a row with the unit's icon and name, to the roles that
have `read` on the RBAC object the unit names. The steps are in
[declare-an-app-in-the-home-drawer.md](declare-an-app-in-the-home-drawer.md).

## 4. Set values

Two kinds of value exist, and an admin or an operator sets each in a different place:

| Value | Who sets it | Where | Steps |
|---|---|---|---|
| A unit's own setting, declared as `extension.Setting` (a switch or one line of text) | An admin | Settings, Extensions, on the unit's card | [Declare settings an admin can change](add-an-extension.md#declare-settings-an-admin-can-change) |
| A setting of the programs | An operator | `margince.yaml`, the environment or a flag | [change-a-setting.md](change-a-setting.md) |

The core stores a unit's setting and does not hand it to the unit's code. The settings of the programs are all in
the [configuration reference](../reference/configuration.md#every-setting).

## Check the result

Run `make check-composition`. It compares the generated files with the next run, and a build with another set of
units fails it.
