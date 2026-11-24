# Declare an app in the home side drawer

Use this page to give a unit a row in the home side drawer, with its own icon and name. It assumes the unit already has a frontend package, as in [add-an-extension.md](add-an-extension.md). For why the drawer is open to a unit, read [Why a unit may have a drawer entry](../explanation/extension-apps-in-the-drawer.md).

## What a unit may declare

| Part | What the unit decides | What the core decides |
|---|---|---|
| Count | At most one `extension.App` | The row appears once, for every reader who may open it |
| Icon | One icon from `extension.EntryIcons()` | How the row is drawn |
| Name | A copy key from the unit's own `frontend/i18n/` files, in `en`, `de` and `vi` | Which language the reader sees |
| Heading | `AppGroupRecords`, `AppGroupWork` or `AppGroupIntelligence` | The unit cannot add a heading |
| Position | Nothing | The row sits after the product's own rows under its heading |
| Who sees it | One of the unit's own RBAC objects | The row shows only to a role with `read` on that object |
| The screen | A named export of the unit's frontend package | The app opens inside the home layout |

The core refuses anything outside this table at `make gen` and again when the server starts.

## The two addresses

An app and a setting page are two doors to one unit. They draw different components and sit in different places.

| | App address | Setting address |
|---|---|---|
| Path | `#/app/<name>` | `#/ext/<name>` |
| Draws | The named export `Export` | The default export, or the card of published operations when there is none |
| Offered in | A row in the home side drawer, under `Group` | Settings, on the page that matches the unit's `Secrets` scope |
| For | The members who use the unit | The admin who sets the unit up |
| Shown to | A role with `read` on `Object` | As before: the settings rules |
| Without it | A unit with no `App` has no drawer row and no `#/app/<name>` | A unit with no default export still gets the card |

`<name>` is the unit's name. The app address holds only the name, with nothing after it. A reader without `read` on `Object` gets the unknown-page card at `#/app/<name>`, so the page does not say that the app exists. The app links to its setting page, and the unit's row in Settings links to the app.

## Steps

1. **Pick the RBAC object.** Use one of the unit's own objects, named `<namespace>_<object>`, that at least one of the unit's operations gates on. The generator refuses any other.
2. **Add the label.** Put a key in `frontend/i18n/en.json`, `de.json` and `vi.json` of the unit, inside the unit's `ext<CamelUnit>.` space. A language without it fails `make gen`.
3. **Export the component.** In the unit's frontend package, add a named export beside the default one:

   ```tsx
   export function OpenchannelApp() {
     return <OpenchannelScreen />;
   }
   ```

4. **Declare it in Go.** Add one `App` to the `Extension`:

   ```go
   App: &extension.App{
   	Icon:   extension.IconPlug,
   	Label:  "extOpenchannel.title",
   	Group:  extension.AppGroupWork,
   	Object: "ext_openchannel_endpoint",
   	Export: "OpenchannelApp",
   },
   ```

5. **Build.** `make gen` checks the declaration and writes `extscreens.gen.ts` and `extensions.gen.ts`, which are generated and never edited by hand.
6. **Look at it.** Start the app with `make dev`, sign in as a role that has `read` on the object, and find the row under its heading. Then sign in as a role without it, and check that the row is gone and `#/app/<name>` shows the unknown-page card.

## What the build refuses

| Mistake | Where it fails |
|---|---|
| An icon outside the list, a group that is not a drawer heading | `App.Validate` |
| A label that is not a key of the unit's own copy, or is missing from a language | `make gen` |
| An object that is not the unit's own, or that no operation gates on | `App.Validate` and `make gen` |
| An export that is not a component name, or is missing from the frontend package | `App.Validate` and `make gen` |
