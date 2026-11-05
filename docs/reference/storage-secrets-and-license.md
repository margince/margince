<!-- prose:plain -->
# Storage, secrets and the license

Where attachment bytes, sealed credentials, custom-field columns and the license token come from,
and what each program does when one is missing.

## Object storage (api, worker): attachments and company logos

Set by env only, and shared by both roles; secrets never show on the command line, because any user
on the machine can read argv. There are two providers: an `S3`-compatible service
(`MARGINCE_BLOBSTORE_ENDPOINT`) or a local folder (`MARGINCE_BLOBSTORE_PATH`). Leave both unset, and
the `/attachments` endpoints answer 501. Set one of them to turn the endpoints on. With both set,
the endpoint wins, because it may already hold objects this installation wrote. To pick an empty
local folder in its place would make the attachments that are already there go missing.

- **The path provider** is for an installation with local storage and no object storage service.
  That is a deployment on one machine.
- Attachments, company logos and CSV import bodies all go through the same `Store` seam, so all of
  them work on both providers.
- The path provider is not a store shared by many machines. It keeps no copies, no versions and no
  signed URLs, and it holds bytes only for the machine it runs on. More than one api replica needs
  the endpoint provider, because two machines cannot share a folder they do not both mount.
- **Company logos** use the same store. With no store set up, the logo lane returns before it gets
  anything. So no logo object is written, and `GET /companies/{id}/logo` answers 404; every
  company shows its fixed letter mark. The 501 on that route is more narrow: a record that names an
  object on a deployment whose store no longer exists.
- Attachment rows may exist (uploaded while a store was set up) when the process that erases has no
  store. Then an Article 17 erase **fails and changes nothing**, and does not leave the bytes
  behind. It can be run again until a store is set up.
- The bucket is created on first connect, and the store waits for a backend that is still starting,
  with a limited retry.
- `MARGINCE_BLOBSTORE_REGION` is required when an endpoint is set, and has no default, because it
  decides where a bucket that holds attachments is created. For MinIO any value works. A missing
  endpoint, bucket and region are reported in one error.
- `MARGINCE_BLOBSTORE_USE_SSL` is read by `strconv.ParseBool`: `1`, `t`, `T`, `true`, `TRUE`,
  `True`, and the same forms of false. Any other value stops the boot, and is never read as off.
- The path provider creates the folder owner-only (`0700`). Bytes land under `<path>/blob/<key>`
  and their content type under `<path>/meta/<key>`. Each is written to a short-lived file and then
  renamed, so a stopped write never leaves a cut-off attachment a row still points at.


## Secret vault (api, worker): connector credentials

Set by env only, and shared by both roles. The root key never shows in any log or error, or on the
command line, where any user on the machine can read argv.

- A connector credential is sealed with `AES-256-GCM` under this key, and stored as ciphertext in
  the `vault_secret` table. The `connector_connection` row carries only a `credential_ref` that
  tells nothing and is scoped to the workspace, never the credential bytes.
- Leave `MARGINCE_KEYVAULT_ROOT_KEY` unset on an installation that has sealed nothing, and there is
  no vault. Then the connect path of every connector refuses with a clear error, and does not store
  a credential in plain text. Gmail, gcal, graph and IMAP all connect through the same operation,
  which seals to the vault.
- Set it, and the api gets the `/readyz` keyvault probe and the path through the vault. At boot the
  worker also moves any old `auth` bytea rows into the vault; to run that twice does no harm.
- `MARGINCE_KEYVAULT_ROOT_KEY` is 32 bytes in standard base64; `openssl rand -base64 32` makes one.
- A key that is set but is not 32 bytes (once read as base64) is a boot error. So is an unset key on
  an installation that already holds sealed ciphertext. A new deployment that dropped the variable
  is refused, not run in a weaker mode (see below).


### The vault may hold copies of the two deployment credentials

The deployment declares the **outbound relay password** (`email.smtp.password`) and the **license
token** (`license.token`). Margince reads each where it is declared, and never seals a declared
value into the vault.

An earlier release sealed each declared value into the vault, and recorded where. Such a copy stays.
While the deployment declares the credential, the declaration wins and the copy is not read. Once
nothing declares it, the copy is the answer. So an installation that removed its declaration
after that release keeps starting.

**Delete the declaration and its source together.** Say you drop the variable, or remove the file,
while the `license:` block or the `password:` line is still in `margince.yaml`. Then the boot fails
while it loads its settings, before Margince looks at the vault. A `${file:…}` that no longer
exists cannot be read. A `${env:…}` that is unset is a named source that gave nothing, which is an
error.

**Remove the relay password with `${none}`.** To delete the `password:` line does not switch the
installation to a relay with no password: a sealed copy keeps answering. Write the line as
`password: ${none}` instead. That is the case for a relay that takes no credential, and for a relay
password that is no longer secret. On the next boot, the api and the worker each do three things:

- They delete the sealed copy from the vault, and then the record of where it was. This happens
  even when `email.enabled` is false.
- With mail on, they send with no login, even if `username:` is still set.
- They log this line once, on the boot that removed it:

```
removed a sealed deployment credential on this boot; the deployment declares it absent
  credential_name="the outbound-mail password" declared_at=email.smtp.password
```

Keep `${none}` in the file for as long as the relay takes no password. A later boot that finds
nothing sealed does nothing and logs nothing. If the delete fails, the boot logs an error, sends
without a password anyway, and tries again on the next boot.

`${none}` is not a password. Each secret field refuses a literal value, so it cannot be one.
`email.smtp.password` is the only field that takes it, in `margince.yaml` or as the value of
`MARGINCE_SMTP_PASSWORD`. `license.token` and `bootstrap_admin.password` refuse it at boot,
and so does `email.smtp.password` next to `password_file`. A `${env:…}` or `${file:…}` reference
whose target holds the text `${none}` is refused as well. A removal is always written where the
setting is declared.

This does not matter for the license. An installation that removes its license has stopped paying,
and a production boot refuses a missing license in any case.

**A declaration does not replace the copy.** No part of the product changes either credential, and
no role Margince ships with holds the grant to write one. A declaration that differs from the copy
wins, and the copy is left in place. To remove it would let one bad boot, with an old variable or a
broken pipeline, delete for good a credential nobody picked to replace.

**A vault that cannot be opened says so**, in two places. There are two ways it can fail, and each
needs its own words.

*The root key is missing.* An installation that holds sealed ciphertext with
`MARGINCE_KEYVAULT_ROOT_KEY` unset **refuses to boot**, and names the variable. Margince checks this
once, where it builds the vault, not in each reader. The loss covers every credential the
installation holds at once, connector tokens included. An installation that has sealed nothing has
no problem, and starts with no vault. Nobody can get the key back from the ciphertext, or from us:
restore the one this installation sealed with.

*The root key is wrong.* A sealed reference that will not open stops the boot, and names the vault
and the root key. It does not report an installation that has a license as having none, which would
call someone out about another problem.

One result for the **worker**: its license check runs after its database pool, because a sealed
token lives in a table. It still runs before the worker does any work. So an operator error never
leaves a worker running on a license the api refuses to boot on.

## Custom-field schema pool (api) — runtime DDL

`--schema-dsn`/`MARGINCE_SCHEMA_DSN` is the owner-role DSN, for the api only, behind
`createCustomField` and `updateCustomFieldOptions`. It is the one chokepoint of the customfields
engine for an `ALTER TABLE` at runtime.

- Unset, both operations answer `501` (`ErrSchemaChangesUnavailable`), and do not touch a pool that
  was never opened. `renameCustomField`, `retireCustomField`, and `listCustomFields` need no schema
  pool, and always work.
- Set, the api opens a second pgxpool with `pool_max_conns=3`, unless the DSN already sets
  `pool_max_conns` itself. That follows the rule of `database.NewPool`: the DSN wins over the
  default. The app pool has `MaxConns=16` by default.
- Each schema change waits in line behind an advisory lock for one transaction, keyed on the target
  table. So this pool never runs more than one `ALTER` against the same table at a time. `ALTER`
  statements against separate tables do not wait for each other.
- The transaction runs the DDL and the catalog and audit write as the owner role. So the credential
  this DSN names must be the same owner role `cmd/migrate` uses. It does not need to be a member of
  `margince_app`: the transaction never switches role.
- When it is set up, the api also gets the `customfields-schema-pool` probe on `/readyz`.

### Turning on custom fields on an installation

`make dev` gives the owner DSN of the stack it picked to the schema pool of the API. In a linked
worktree, that DSN has the separate database name. The normal API pool still uses the app role; the
schema credential does not go to the worker or the frontend. Set up the dev database through
`OWNER_DSN`/`APP_DSN`, or their environment fallbacks, and do not set the schema database yourself.

The container API entrypoint sets `MARGINCE_SCHEMA_DSN` to `MARGINCE_OWNER_DSN` by default. An API
started by hand skips both launchers, and must set `MARGINCE_SCHEMA_DSN` itself. Use the owner role
for the same database as the app connection. The setting, with notes, is in
[`.env.example`](../../.env.example).

Adding a field may report
`operation custom-field schema changes is specified but not yet implemented`. That means the API was
started without this pool. Set it up and start the API again; you do not need to empty the database
or pick a new field name. The start-up log confirms
`api custom-field schema changes enabled (schema pool configured)`.

Then check `/readyz`. On a test installation, create a picklist through Settings, store a value, and
read it back. The normal ready answer alone does not prove that the pool is set up. An optional part
nobody set up has no ready probe. A pool that is set up is checked, and makes the ready check fail
if its connection fails.

## License

Gradion issues the license token.
[Get a license token](../how-to/deploy-margince.md#get-a-license-token) says how to ask for one.

The `license:` block points at the license token of the installation. Margince checks it
**offline**, in the process, against the license check WebAssembly module bundled at
`backend/internal/platform/licensecheck/module/`. It makes no call out of any kind. So an
installation with no network proves its license the same way a connected one does. The tools of
the publisher install the module, its pin and its digest together, and nobody edits them by hand.
A blob that stops matching its recorded digest fails `make check`.

| field | default | what it does |
|---|---|---|
| `token` | *(none)* | The token as a reference: `${file:/run/secrets/margince-license}` or `${env:SOME_VAR}`. The form Margince asks for, and the one it points to when it refuses. An inline value is refused when the file is read. |
| `token_file` | *(none)* | The older form, still accepted, so a deployment that has it starts with no change. A path to a file that holds the license token. A **file reference, never an inline value**, because it is a credential, and this file gets copied into support threads. A `MARGINCE_LICENSE` that is not empty wins over it. The license module itself reads that variable, so a container that already sets the license needs no `license:` block. |

There are three postures. This is what each one does at boot:

| posture | boot | reported as |
|---|---|---|
| **no token set up** | **refuses to boot in production**; starts with a warning when `MARGINCE_ENV` is `dev` or `test` | `margince_license_posture{state="absent"} 1` |
| **token checked and good** | starts | `margince_license_posture{state="valid"} 1`, plus `margince_license_seats` when the license grants a seat count |
| **token refused** | **refuses to boot** (api and worker both), naming the own reason of the module and the setting to fix | (none) |

Margince reads the token where it is declared, and never seals it. A token an earlier release sealed
in the key vault is read only when nothing declares one. See
[the vault may hold copies of the two deployment credentials](#the-vault-may-hold-copies-of-the-two-deployment-credentials).
It also covers the boot error that a vault Margince cannot reach causes, in place of an `absent`
posture.

**A production installation serves only on a license.** The posture decides it, and `MARGINCE_ENV`
fails closed. So an installation that names nothing is production, and must have a license. When
Margince refuses, it names both ways out: set up the token, or name the installation as not
production.

A refused license refuses the boot in *every* posture. To name yourself as not
production says you have no license. It does not let you run one the module has refused.

**What Margince refuses.** A signature it does not trust, the wrong issuer, or no grant for this
product at this generation. It also refuses a license whose end date is past the grace time the
module carries. A module that cannot run at all is refused the same way, because that is a package
error. To read it as no license would lower the installation without a word.

A `token_file` that is
set but cannot be read, or is empty, is also a boot error. So a wrong path does not count as a wish
to run with no license.

**An ended license never stops a running process.** A license that ends while the process
runs leaves it serving. The api checks again daily, and its `/metrics` posture gets worse; nothing
goes offline mid-month without a human there. Each daily check reads `token_file` (or the variable)
again. So a license replaced in place applies within a day, without starting the process again.

Some results are not a result about the license: a token it cannot read, or a module that failed to
run. Those keep the posture the process last worked out, and are logged as what they are. None of
them is evidence about the license.

**Seat counts apply at first use.** The granted seat count applies where a seat comes into use.

- Once every licensed full seat is in use, Margince refuses to add a member, and refuses to turn a
  turned-off full seat back on. It answers `403 seat_limit_reached`, with the granted and used
  counts.
- Nothing already in use is touched: no seat is moved down, and no session ends. A license that runs
  out mid-month refuses the next seat, and leaves the ones colleagues are working in.
- Read seats have no limit, and nobody counts them. A suspended or turned-off seat frees its own,
  so an admin at the limit can make room.
- A license that carries no seat count limits nothing, and the same goes for a dev installation with
  no license.
- Margince reads the limit live, so a license replaced in place raises it on the next daily check,
  without starting the process again. The number an admin is refused against is the number the
  license screen and `margince_license_seats` report.

**Which authority a license must come from.** A production installation accepts one:
`margince-license-authority`. Our licensers for test installs sign with keys the bundled keyset
carries. So the issuer is what keeps a license made for a test from licensing a customer.

An installation with `MARGINCE_ENV` set to `dev` or `test` also accepts
`margince-license-authority-test` and `margince-license-authority-dev`. That is how a developer runs
the product on a test license. A `MARGINCE_ENV` that is not set, or that Margince does not know, is
production. So an installation gets the narrow set unless someone picked another. The boot line
names the authority each time it is not the production one.

Margince reads a token up to 64 KiB. A file over that size is a boot error. A path that points at
something that is not a license (a log, an image) is an error to report. And everything after this
step copies the token whole.

Every dev and CI process in this repository runs with no license. That is why a missing license is
a posture Margince supports, and Margince does not refuse it.
