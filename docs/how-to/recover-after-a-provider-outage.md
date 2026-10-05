# Recover after a provider outage

When an AI provider stops answering for everyone, Margince stops asking it: the
router skips the blocked provider, background lanes defer and wait for the next
probe, and interactive requests fail fast with a message to contact the system
administrator. This is the operator procedure for what happens next. The design is
[AI provider health](../explanation/ai-provider-health.md).

Most work resumes by itself. What needs you is the work that spent its attempts
**before** a provider was marked blocked: sender questions retired to the review
queue with no judgement, and company enrichments that ran out of attempts.

## 1. Read the cause

Open **Settings** → **System health** and read the **AI provider status** card, or call
`GET /v1/ai/provider-health`. Only providers that are not answering normally are
listed; an empty list is the healthy answer. The view is the serving process's own,
learned from its calls, so check the worker's too when the two could differ.

| Health | Fix |
|---|---|
| out of credit | Add credit or raise the quota at the vendor. |
| unauthorized | Replace the key under Settings → AI models → **Providers**. |
| down | Fix the host or wait out the vendor's incident. |
| degraded | Does not block work; investigate if it lasts. |

## 2. Fix it and let the probe run

An out-of-credit or unauthorized provider is probed every 15 minutes; a down one at
30 seconds doubling to 5 minutes. Saving a key, or saving the routing, probes at
once. Work that was deferred needs nothing from you once the probe succeeds.

## 3. Reopen what the outage parked

Pick the window the outage covered: `--from` is when it began, `--to` is when the
provider answered again, both RFC3339, `--to` exclusive. Run the worker
binary's subcommand with `MARGINCE_DSN` set to the installation's database, dry
run first:

```sh
export MARGINCE_DSN=postgres://...
worker reopen-parked --from 2026-03-01T08:00:00Z --to 2026-03-01T11:30:00Z --dry-run
worker reopen-parked --from 2026-03-01T08:00:00Z --to 2026-03-01T11:30:00Z
```

The dry run prints how many sender questions and company enrichments would be
reopened and changes nothing. A real run reopens at most `--batch` rows of each kind
per workspace and prints how many remain in the window; run it again until none do.
The window is half-open and cannot tell an outage from a message the validator
rejected, because both retire a row with the same reason; name it from the **Started**
time on the **AI provider status** card, not wider.

It reopens two kinds of work, each back to zero attempts and due at once: sender
questions retired to unsure for want of a verdict inside the window, which also
withdraws their standing review offer and skips any someone already decided; and
company enrichments that ran out of attempts or failed inside the window, skipping
archived companies and ones with a site read already done. Each reopen writes an
audit row. A repeat run finds nothing new.

`--batch` defaults to 200. Both `--from` and `--to` are required, and a window that
does not start before it ends is refused.

Run it after the provider answers: a reopened row is due at once, so the batch is
the most model calls one run releases.

## Check

`worker reopen-parked` with `--dry-run` over the same window reports nothing left,
and the sender questions leave the review queue as the lanes work through them.
