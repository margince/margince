<!-- prose:plain -->
<!-- describes: backend/internal/platform/config/spec.go backend/internal/compose/configregistry.go backend/tools/gen-config/main.go -->
# Add, rename or remove a setting

A setting is a value an operator gives Margince through a flag, the environment or `margince.yaml`.
A stored setting can also be changed by an admin in the app. To change the value of a setting that
exists, read [change-a-setting.md](change-a-setting.md). The
[configuration reference](../reference/configuration.md#every-setting) lists every setting.

The worked example on this page is `activities.follow_up_after_days`, in
`backend/internal/modules/activities/followupsetting.go`.

## Choose `Define` or `DefineStored`

- `config.Define` declares a setting that only a boot source sets: a flag, the environment, an env
  file or a YAML file. A program reads it once at the start, with `config.Val`.
- `config.DefineStored` declares a setting that an admin may also change in the app. The value lives
  in a row of the `setting` table. A boot source that sets it wins and locks it in the app.

Use `DefineStored` only when an admin should change the value without a new start. A stored
setting needs an endpoint that reads and writes it, and most have a screen under **Settings**.

## Add a setting

1. **Declare it** as a package-level `var` in the package that reads it.
   Write the `Key` as a string in quotes, so the census gate can read it.

   ```go
   var FollowUpAfterDays = config.DefineStored(config.Spec[int]{
       Key:     "activities.follow_up_after_days",
       Env:     "MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS",
       Type:    config.Int,
       Default: config.Some(2),
       Min:     config.Some(followUpMinDays),
       Max:     config.Some(followUpMaxDays),
       Doc:     "Days a sent message may go without a reply before its sender is asked to follow up.",
   }, config.Store{
       RBACObject: "installation_settings", AuditVerb: settingsAuditVerb,
       MachineryApplied: true,
   })
   ```

2. **Fill the `Spec`.** The fields are in `backend/internal/platform/config/spec.go`.
   `Key` has no capitals, is split by dots, and is also the path in YAML.
   `Env` is in capitals, with `_` between words, and starts with `MARGINCE_`.
   `Doc` is one line, and the reference and both example files print it.
   `Min`, `Max` and `Validate` check a value from every source.
3. **Name the programs** in `Roles`: `config.RoleAPI`, `config.RoleWorker` or `config.RoleMigrate`.
   A `Define` setting with no `Roles` is read by no program, and a gate fails it.
   A stored setting with no `Roles` is read by the api and the worker.
   The reference shows `Roles` as the **Programs** column.
4. **List it** in the package's settings function, such as `ConfigSettings()`.
   A module's function returns `[]config.Any`.
5. **Register the function** in `allSettings()` in `backend/internal/compose/configregistry.go`.
   Do this once per package. A module that already has settings is already there.
6. **Read it.** A `Define` setting is read with `config.Val(cfg, Setting)`, mostly in `compose` or `cmd`.
   A stored setting is read with `config.StoredVal` or `config.StoredValTx`.
   It is written with `config.Save`.
7. **Run `make config-gen`.**
   It writes again every file that `backend/tools/gen-config/main.go` lists as an output.
   These are the settings tables of the configuration reference, `config/margince.example.yaml`,
   `.env.example` and `config/margince.schema.json`. Never edit those parts by hand.
8. **Test it.** Use `backend/internal/platform/config/configtest`.
   `configtest.MustLoad` loads a YAML text over your settings.
   `configtest.MustLoadEnv` loads them from the environment alone.
   `configtest.StoredOver` reads and saves stored settings through a test database.
9. **Check a changed file** with `make check-config`. It runs `--check-config` on the api and the
   worker. Then run `make check`.

## What a stored setting also needs

The `config.Store` fields say who may change the setting and how its row is read.

- `RBACObject` is the RBAC object whose grant guards the setting. A read needs `read` on it, and a
  save needs `update`. It must be an object in the closed set. To add one, follow
  [add-an-rbac-object.md](add-an-rbac-object.md).
- `AuditVerb` is the action that the audit row of each save records, such as `update`.
- `MachineryApplied` lets code read the row without the read grant, with `config.StoredApplyTx`.
  Set it only when the value must apply to a write by any user. The follow-up window is one case:
  the Worklist applies it to rows the reader may already see. A secret can never set it.
- `SecretReference` keeps a secret sealed in the key vault. The row holds only a reference to it.
- `Freeze` refuses a change once the data depends on the value.
- `SurvivesDataReset` keeps the row through a data reset.

The key of a stored setting starts with the name of the module that owns it, such as `activities.`.
The settings response of its endpoint carries `locks`, so the screen can show a locked control.

## Secrets

Set `Secret: true` for a password, key or token. A secret has no `Default`, because the reference
prints every default. Its value shows in no log, trace, answer or error.

In a YAML file, a secret takes a reference, never the value: `${file:/path}` or `${env:NAME}`.
`${none}` says the secret is absent, and only a setting with `AllowNone` takes it.
A stored secret also needs `SecretReference`, so its row never holds the value. The
[secrets section](../reference/configuration.md#secrets) of the reference states each rule.

## Which gate checks which step

| Step you missed | What fails |
|---|---|
| Bad `Key`, `Env` or `Flag` form, no `Doc`, a default out of its own bounds | `config.NewRegistry`, at every start and in `TestEverySettingIsInSomeProgram` |
| A key, variable or flag that two settings declare | `config.NewRegistry` names both |
| Not in a settings function, or that function not in `allSettings()` | `TestEverySettingIsInSomeProgram` in `backend/gates/configregistry_test.go` |
| `Key` not written as a string in quotes, or `Define` inside a function | the same test |
| A stored key without its module's name, an unknown RBAC object, no audit verb | `backend/gates/settingscatalog_test.go` |
| A stored key that is also a seed key or the key of a `Define` setting | `TestNoSettingKeyIsAlsoADeploymentConfigKey` in the same file |
| `make config-gen` not run | `TestGeneratedConfigurationFilesAreCurrent` in `backend/gates/configgenerated_test.go`, and `make drift` |
| A `MARGINCE_` variable the reference does not name | `backend/gates/envcontract_test.go` |
| Bounds in `crm.yaml` or on the screen that differ from `Min` and `Max` | `backend/gates/settingbounds_test.go` |
| A settings response that does not report its locks | `TestEverySettingsResponseReportsItsLocks`, in the integration lane |

## Rename or remove a setting

An operator's `margince.yaml` may still name the old key. A YAML key that no program reads stops the
boot, and so does a flag no program knows. A `MARGINCE_` variable that no program reads only logs a
warning. A key written twice in one YAML mapping also stops the boot. So an operator cannot keep the old
line and the new line side by side.

The code has no field that marks a key as old. To remove a key, keep it declared:

1. Keep the `config.Define` with its `Key`, `Env`, `Flag` and `Type`. Remove what read its value.
2. Change its `Doc` to `Ignored; kept so existing files load.`. For a setting with a flag, write
   `Ignored; kept so existing command lines parse.`, as `ai_routing` does in
   `backend/internal/compose/programsettings/shared.go`.
3. When an operator may still expect it to work, log a warning at the start while it is set.
   Say what replaced it and that they should remove it. `WarnStaleRates` in
   `backend/internal/compose/capture.go` does this for `rates.model_pricing`.
4. Run `make config-gen`. The key stays in the reference with the new `Doc`.

A rename removes the old key and adds a new one. Add the new key, and keep the old one as above.
Its warning names the new key.

Nothing in the code says when a kept key may go. Deleting it breaks every file that still names it.
So give that pull request a `!` in its title, and it lists under **Breaking** in the release notes.

### A stored setting keeps its row

A saved value lives in a `setting` row under the old key. After a rename, no code reads that row, so
the saved value is lost. Move it in a new core migration, as
`backend/migrations/core/1792418861_each_provider_key_is_its_own_setting.up.sql` does:

```sql
INSERT INTO setting (key, value)
SELECT 'module.new_key', value FROM setting WHERE key = 'module.old_key'
ON CONFLICT (key) DO NOTHING;
DELETE FROM setting WHERE key = 'module.old_key';
```

For a removed stored setting, delete its row the same way. The table holds at most one row per key,
so the change is small. Still, `backend/gates/migrationrewrite_test.go` asks for a line in
`backend/migrationrewrites.txt` that says why. Follow [apply-migrations.md](apply-migrations.md#write-a-migration)
for the rest.
