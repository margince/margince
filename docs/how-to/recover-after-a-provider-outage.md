<!-- prose:plain -->
# Recover after a provider outage

When an AI provider stops answering for all users, Margince stops asking it. The
router skips the blocked provider, and the lanes that run with no user waiting defer and wait for the next
probe. A request from a user fails at once, with a message to contact the system
admin. This page gives the steps for the operator after that. The design is in
[AI provider health](../explanation/ai-provider-health.md).

Most work starts again by itself. You are needed for the work that used up its tries
**before** the system marked the provider as blocked. Sender questions are in the review
queue with no usable verdict from the model, and company enrichments used up their tries.

## 1. Read the cause

Open **Settings** → **System health** and read the **AI provider status** card, or call
`GET /v1/ai/provider-health`. The card lists only providers that do not answer the normal
way, so an empty list is the good answer. The view is made
from the calls of both the API and the worker, so an outage that only the worker sees also shows.
Without Redis, each program shows only its own view. The most serious status counts.

| Health | Fix |
|---|---|
| `out of credit` | Add credit, or more quota, at the vendor. |
| `unauthorized` | Change the key under Settings → AI models → **Providers**. |
| `down` | Fix the host, or wait until the vendor has fixed its problem. |
| `degraded` | Does not block work; look into it if it goes on. |

## 2. Fix it and let the probe run

The probe checks a provider that is `out of credit` or `unauthorized` every 15 minutes. It checks a provider
that is down after 30 seconds, then waits twice as long each time, up to 5 minutes. When a **Test** of the key passes, it
removes the failures of the provider at once.

When you save a key or the routing, the failures go at the next
routing check, about every 30 seconds. Only one probe runs at a time. Calls while it runs
are refused, and they are asked to try again soon, at no cost. Deferred work needs nothing from you
once the probe passes.

## 3. Open again what the outage parked

1. Choose the window of time the outage covered.
   `--from` is when it started, and `--to` is when the
   provider answered again. Both are RFC3339, and `--to` is not part of the window.
2. Set `MARGINCE_DSN` to the database of the installation.
3. Run the `reopen-parked` command of the worker program, with a dry run first:

```sh
export MARGINCE_DSN=postgres://...
worker reopen-parked --from 2026-03-01T08:00:00Z --to 2026-03-01T11:30:00Z --dry-run
worker reopen-parked --from 2026-03-01T08:00:00Z --to 2026-03-01T11:30:00Z
```

The dry run prints how many sender questions and company enrichments it would
open again, and it changes nothing. A real run opens again at most `--batch` rows of each kind
for each workspace. It prints how many are still in the window, so run it again until the window
holds nothing.

The window includes its start but not its end. The run cannot tell an outage from a message the validator
refused, because both close a row with the same reason. So take the window from the **Started**
line of the **AI provider status** card in System health, and do not go past it. The card shows the
start in minutes before now (`Started 12 minutes ago`). Take that from the time now to get the
start of the window. The end is when the provider answered again.

It opens two kinds of work again, each with its tries set back to 0, and ready to run at once:

- sender questions closed as `unsure` because the model gave no usable verdict in the window. This also
  takes back their open review offer, and it skips any that someone already decided.
- company enrichments that used up their tries or failed in the window. It skips
  companies in the archive, and companies whose site the system has already read.

Each run writes an audit row for each row it opens again. A second run finds nothing new.

`--batch` is 200 by default. You must give both `--from` and `--to`. The run refuses a window that
does not start before it ends.

Run it after the provider answers. A row it opens again is ready to run at once, so the batch is
the most model calls that one run lets go.

## Check

`worker reopen-parked` with `--dry-run` over the same window reports no rows,
and the sender questions leave the review queue as the lanes work through them.
