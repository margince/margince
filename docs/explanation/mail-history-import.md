<!-- prose:plain -->
# Mail history import: the capped scan back in time and its cost

Connecting a mailbox starts *standing sync*: from then on, new mail comes in. A new connection is also
offered one **capped scan back in time**, the history import, so the CRM does not start with an empty
history. The import spends model budget on mail the user has not read yet. So it is built around an
estimate the user accepts *before* anything runs.

The sections below follow one import from start to end. They cover what the user sees and where that
estimate comes from. Then they cover what the run does per page, and what keeps happening after the
progress bar fills.
For how a connector works (the split between normalize and the Sink, credentials, the OAuth sign-in), see
[capture-connectors.md](capture-connectors.md). For how the estimate works out a price, see
[ai-runtime.md](ai-runtime.md#cost--the-meter-collects-tokens-a-rate-table-prices-them). To
connect a mailbox and try this, see [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md).

## The screen the whole design serves

After a mailbox connects, the user chooses a window and sees this:

```text
  Import your mail history
  Choose how far back to import. You'll see the scope and estimated cost
  before anything runs — and you can skip this entirely.

    Import window: [ 6 months ▾ ]
    Import emails since 14 March 2026.

    ┌──────────────────────────────────────────────────────────┐
    │ Messages in this window: ~3,436                          │
    │ Estimated AI cost: ~0.84 USD                             │
    │ An estimate, not a bill — actual usage is metered and    │
    │ visible as it happens.                                   │
    │                          [ Start the import ]            │
    └──────────────────────────────────────────────────────────┘

  Skip the history import
```

Setup and Settings share one drop-down list. It offers 3 months, 6 months, 1 year, 2 years, 3 years,
5 years, 7 years and 10 years. 6 months is the default. The preview names the real start date. The
choice *none* is made by not starting: it goes to zero at once, with no provider call at all.

The start date names the day of the preview edge; Microsoft also filters by the time within that day.
Windows are measured back from today in calendar months. Starting later works out that moving window
again. Going back more keeps the emails that already exist. Messages the old and new windows share
are scanned again, and the duplicates are dropped.

When the preview reached its cap, both screens say the count is a floor. They explain that the estimate
covers only the counted messages: the full import may hold more and cost more.

Every section below answers a question from that screen.

## "~3,436 messages": counting the scope without reading the mail

The count is a real provider call, and on Gmail it reads **ids only**. The preview pages through
`messages.list?q=after:<date>` and counts what comes back. That is a list of `{id, threadId}` values, with
no headers, `snippet` or body. The call that fetches a message is used by the import loop, never by the
preview.

Two decisions shape the count:

- **The provider's own count is refused.** Gmail returns a `resultSizeEstimate`, and it can be wrong many
  times over: a window of 1,300 messages can read as ~200. The count also goes into the spend estimate,
  so a wrong count means the user consents to the wrong spend. An id count costs a small number of calls and
  is correct.
- **It is capped.** 500 ids per page, 40 pages, so up to 20,000 messages are counted one by one. A
  larger mailbox reports the counted floor instead of turning a preview into a long scan. The user
  consents to a cap on the scope; the cap is not a promise of the last count.

Microsoft mail takes neither path. Graph answers a correct `@odata.count` for the date filter, so the
preview uses it directly. There is no cap, and the count is never a floor.

## "~0.84 USD": pricing work that has not happened

An imported message lands in the timeline and also starts AI work. There are three passes, each with its
own unit:

| pass | one unit is | what it produces |
|---|---|---|
| classify | a message | the label for how much it needs a human: `commitment` / `meeting` / `noise` |
| enrich | a newly created contact | contact fields read from the mail signature |
| embed | a record | the vector that lets search find it |

So the estimate is `Σ per-pass (expected units × per-unit cost)`, and both parts are measured:

```text
  expected units ◀── this connection's last completed import
                     (how many messages one scan captured,
                      how many contacts it created)

  per-unit cost  ◀── this workspace's last 7 days of real model calls,
                     each repriced at the model that will actually run it now
```

Each part falls back on its own. With no completed import, the units come from a built-in ratio. With
no call history, the cost comes from a **work-shape floor**, derived from the size of the real prompt
text. Either fallback marks the whole estimate `heuristic` instead of `observed`. The API returns
this label, so a reader can tell a measured number from one made up from defaults.

Rules for missing or zero prices:

- If nothing can be priced at all, the cost field is **dropped**. It never shows as `0`, because
  the user would consent to a spend number that was made up.
- A local model's `$0` is a real price, and it *does* show.
- If the estimate read fails, the preview falls back to a plain message count. It never blocks the user.

Nothing in the import path refuses to run because the estimate is large or missing.

## Starting: one page at a time, and it can go on later

The run is a row with a state machine, paged by the worker:

```text
   queued ──▶ running ──▶ done
                 │
                 ├──▶ cancelled   (the human stopped it, or the connection changed under it)
                 └──▶ error       (a non-transient fault, or too many consecutive transient ones)
```

The worker calls one step at a time. Each step pulls **one provider page of 100 messages**, and pushes
every message through the same Sink that standing sync uses. It then commits what the page did together
with the cursor that lets the run go on from there. The **message** counts and the cursor land in one
statement. So a page that fails to commit scans nothing the next try will do again. The
**counterparty** counts are kept outside that statement (see below).

Two cases make that commit fail by design. Both mean *this page no longer belongs to the run
being written*:

- the run reached an end state at the same time (a cancel), or
- the connection was closed and opened again while the page was out at the provider.

A page fetched under a grant its owner has since revoked is discarded.

**Each failure is handled by its kind.** A rate limit or a provider that cannot be reached is waited out, with a
wait that is twice as long each time. The wait keeps to the provider's own `Retry-After` when that asks for
longer, since coming back early only gets refused again. Anything else ends the run at once. The
ladder of failures that may pass is capped at 10 **in a row**. A committed page sets it back to zero, so
faults the import already passed never end it.

## What the run learns about itself

While it pages, the run measures its own yield, and that makes the *next* preview correct.

The counterparty resolver reports what each call did. `contacts_created` counts contacts **created**. A
sender matched to an existing contact is not counted, and it starts no enrich call either.
`companies_created` counts something different: **domains this run queued for a company verdict**,
because capture creates no companies at all. A run with 12 new domains did that work whether or not the
site reads have answered yet. Reporting zero would make that work look like none.

A counterparty is counted **the moment it is created**, in its own write, outside the page's commit.
Counting each new row in its own write means a count is not dropped or counted twice when a page is cancelled,
retried or fails. Capture is idempotent, so a replayed message never reaches the resolver again. There is
nothing to build a missing page total from again.

What this promises and what it does not:

- It never **counts twice**. A row is created once, the write runs on that one result, and nothing
  retries it.
- It is not **once and only once**. Creating and counting are different transactions, so a failed count
  write loses the count for that row for good (logged at `ERROR`).
- The loss is **per failure and has no cap**: a database fault over a whole page loses one count for
  every row created inside it.

So read the committed columns as a **floor** on what the run created, never as too many. Closing the gap
needs a ledger keyed on the created row's id. That is a design decision, not a small fix.

The yields count short by design. A sender the tier gate defers is resolved by the verdict engine long
after the page that carried it. No page claims the contact it may create later.

So a run that reports
zero contacts created is read as **"ratio not known"** instead of "zero contacts". A window whose senders
were all known, blocked or deferred reads zero, while a longer window would create many. Showing `$0`
for enrich off that zero would make the spend look too small. So the estimate uses the floor instead and
says `heuristic`.

## After the bar fills

AI spend goes on after the import ends. The three passes the estimate priced are sweeps on a schedule
over whatever work is waiting, outside the paging loop:

- **classify** runs every hour,
- **enrich** runs daily,
- **embed** runs have their own lane.

So a newly imported mailbox has its timeline at once, and its labels, contact fields and search
vectors fill in behind it. That has two results. The estimate covers work that lands after the progress
bar completes. And the live meter, not the preview, says what the import cost.

One thing does run on the completing edge: the same-day digest. So a newly imported mailbox shows on the
morning screen instead of waiting for the daily pass. It runs only on the single step that moves a live
run to `done`. So two steps that both try to complete the run can never produce a wrong digest.

While it runs, the status surface reports `messages_scanned`, `captured`, `skipped`, `contacts_created`
and `companies_created`. Next to them it reports the estimate the run started with, as the progress
total.

## Known limits

- **The contacts and company yields count short** on deferred senders, as above. This is by design, and
  the zero case is handled by name.
- **The scope count is capped** at 20,000 messages; beyond that the preview reports a floor.
- **The estimate expects new mail** to be like past mail. Longer mail costs more. The line under the
  estimate says it is an estimate and that use is metered.
- **The web UI does not use the label.** It does not read `observed`/`heuristic`. It shows the cost each time the API
  returns one, `$0` included. But the signal of how good the estimate is never reaches the user, even
  when the API returns it.

## Where the code lives

| part | where |
|---|---|
| run control (start, status, cancel), scope count | `internal/modules/capture/backfill.go` |
| the paging loop, commit, failure ladder | `internal/modules/capture/backfillpager.go` |
| yield measure | `internal/modules/capture/backfillyields.go` |
| the count and page fetch at the provider end | `internal/modules/capture/gmail/backfill.go`, `.../graph/backfill.go` |
| the cost estimate code | `internal/compose/costestimate/` |
| HTTP surface | `internal/compose/backfilltransport.go` |
| the consent screen | `frontend/src/screens/backfill.tsx` |

## Where to go next

- [capture-connectors.md](capture-connectors.md): how a connector works, and the one Sink every
  captured record goes through.
- [ai-runtime.md](ai-runtime.md#cost--the-meter-collects-tokens-a-rate-table-prices-them): the model
  routing, the token meter, and the rate table the estimate prices against.
- [privacy-and-consent.md](privacy-and-consent.md): what happens to imported personal data later.
- [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md): connect a mailbox and run one.
