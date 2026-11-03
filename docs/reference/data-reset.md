<!-- prose:plain -->
# The data reset

What `POST /v1/admin/reset-data` keeps and clears, and the setting that arms it.

## `POST /v1/admin/reset-data`: the armed data reset

`operations.allow_data_reset` in `margince.yaml` gates it, and its compiled default is **false in
every posture, dev included**. An installation that did not arm it has no such operation. The
switch is checked before auth, so a deployment that never asked for it answers 404 (never a 403),
and nobody can learn that the endpoint exists.

It is not a `setting` row, and Margince does not guess it from `MARGINCE_ENV`. An admin who could arm
it through the API could arm the purge of the data of their own tenant. A deployment labelled
`staging` (real internal users, real records) has not agreed to be wiped just by that label. `/me`
reports the same value as `data_reset_available`, so a client never shows an action the server
would refuse.

```yaml
operations:
  allow_data_reset: true   # dev/test only; omit or false everywhere else
```

Once armed:

1. **Human-only** (`auth.RequireHuman`): Margince refuses an agent or passport principal with 403.
2. **Admin-only** (`auth.RequireAdmin`): only the `admin` role itself. `ops` and every other role
   get 403.
3. **Typed name**: the request body `{"confirmation": "<company name>"}` must be the same as
   the company name of the workspace. If it is not, the answer is `422`, checked before
   anything is touched.

When it works, it wipes the workspace domain data and the starter config data back to the state of
the first boot. Then it runs the module seed steps again: pipeline and stages, consent purposes and
retention, AI defaults, starter automations, the booking page. That is the same seed path the
installation bootstrap of `identity` uses.

It **keeps** the identity and auth layer, so login keeps working. That is every `app_user`, roles,
who has each role, teams, who is in each team, sessions, passports and tokens. It also keeps the
append-only ledgers `audit_log` / `system_log`. The reset itself is recorded as an `audit_log` row
(action `reset_data`).

The `workspace` row stays too, since it carries the company. Only its **installation
identity** is kept. That is the primary key and `created_at`. It is also the name, slug, base currency
and timezone that bootstrap set from `margince.yaml`. (`updated_at` moves, as it does for any write.)
Every other column on it is a workspace **setting**, and each goes back to the default its migration
declared. The columns come from the catalog, so a setting added later is restored the day its column
exists.

A column that belongs to the identity of the installation must be declared kept, or the reset
clears it. Settings in the `setting` table are restored on the same path, by the same rule. The config
goes back to its registered default, and the identity of the installation stays.

The sweep runs as the app role (no superuser, no triggers turned off). So it finds a safe delete
order at runtime: a savepoint per table per pass, and a retry of each row a still-live FK blocks.
An FK cycle that cannot be broken shows up as an error. After that, Margince drops `cf_*`
custom-field columns that no field owns any more, through the owner schema pool (`--schema-dsn`).
With no schema pool set up, that step is skipped and logged, and the reset itself still works.

### It resets the runtime, not only the rows

Queued jobs, bus entries, Redis counters, the caches each process holds in memory, and the stored
object bytes all live on after a row sweep. To stop there would leave work running against records
that no longer exist. So the endpoint also does these steps:

1. **Pauses every job queue and drains it**, for at most 10 seconds. `river_queue` carries the
   pause, so the api quiets the queues the *worker* process owns.

   A drain that is not done in time never fails the reset, so a long pass cannot block a reset. It
   sets `drain_timed_out` in the response, the audit evidence and the log. The write that would
   mark the job done then fails against the wiped rows.
2. **Drains the staged outbox**: the `event_outbox` rows of this workspace, in a separate
   transaction *before* the streams are purged. The outbox relay is not part of the jobs the pause
   stopped.

   Without this step, staged rows would be shipped into the streams just after they were emptied.
   This cuts that window down to one relay batch still on its way; it does not close it.
3. **Purges job rows** (`river_job`): the rows of this workspace plus the dispatchers for all jobs.
   The scheduled ticks add those back on their next run. Every state goes, including the history
   River keeps of done, dropped and cancelled jobs. An installation wiped back to the first boot
   state must not carry that history.
4. **Purges the event bus**: the catalog streams and their consumer groups. Margince deletes the
   groups and creates them again, so live readers keep reading. It also purges the dedupe marks of
   events already handled.
5. **Deletes the stored objects of the workspace** under its `<workspace>/` prefix.

   It also redeems the **sealed credentials** the swept connection rows pointed at. `vault_secret`
   carries no `workspace_id` (the tenant lives inside the ref and inside the `AES-256-GCM` `AAD`), so
   the sweep cannot see it. The handles are read inside the sweep transaction, before the rows that
   name them go.

   The tables that hold one come from the catalog, on the `credential_ref` column. So a connection
   table added later is covered the day its column exists.
6. **Restores every workspace setting.** The table sweep reaches none of them. Its target list
   comes from the tables that carry a `workspace_id` column, and `workspace` keys on `id`.
7. **Sends a reset message** on the `gw:control:reset` Redis `pub/sub` channel. Then the api and
   the worker each drop the caches they hold. Those are model results and the system-of-record mode.
   No HTTP call reaches the worker process; this channel is the only path to it.

   The message clears caches and nothing else. The channel carries no signature. So any client that
   can reach that Redis can publish on it, and a dropped cache costs only work done again.

   This path cannot clear the auth lockout buckets. They slow down login guessing and too many
   password reset mails. So the process that ran the audited, gated reset clears its own, and no
   message clears those of another process.

The queues start again on every exit path, a failure and a panic included. That runs on a context
cut off from the request. So an operator whose client drops part way through the reset does not
leave the jobs paused. To end the process with SIGKILL runs no exit path, and does leave the pause
in place; to run the reset again removes it.

The Redis half covers the **whole installation**, from a declared key list: the stream catalog, the
`gw:dedupe:` namespace and `ovb:<workspace>:`. It never runs `FLUSHDB`, so anything else that shares
that Redis stays. The whole installation is right, because one installation serves one
company. On a laptop, two `DEV_SLUG` stacks side by side share one Redis database, so a reset
in one stack clears the bus of the other.

The 200 body reports what was cleared:

- `tables_cleared`;
- `jobs_deleted`: job rows in every state, history included, not how many jobs wait;
- `streams_purged`: stream *keys*, not entries;
- `cache_keys_deleted`: dedupe marks plus budget counters;
- `objects_deleted`;
- `drain_timed_out`.

The same counts go into the `audit_log` evidence, but not `objects_deleted`. The object purge cannot
join the transaction that writes the audit row.

When any purge step fails, the whole request fails with a plain 500 that shows no details (the cause
goes to the server log). What a failure leaves behind turns on which side of the commit it happened.

- The queue, bus and budget purges run *before* the database transaction. A failure there leaves a
  safe state: those parts are clear, and the data is whole. To run the reset again fixes it.
- The object purge cannot join that transaction, so it runs *after* the commit. So a 500 from the
  object store reports failure with the rows already wiped, and some stored bytes still there. To
  run the reset again fixes this too.

The `data_reset_available` field of `GET /v1/me` carries the same switch the endpoint gates on. So
the SPA shows the action only where it will work: Admin settings → *data* tab → Danger zone →
*Reset data*. It asks the operator to type the company name before it calls the endpoint; only
the server checks that string.
