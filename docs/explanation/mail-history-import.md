# Mail history import: the bounded backward scan and its cost

Connecting a mailbox starts *standing sync*: from then on, new mail flows in. A fresh connection is
also offered one **bounded backward scan**, the history import, so the CRM does not start with an empty
history. The import spends model budget on mail the user has not seen yet, so it is built around an
estimate the user accepts *before* anything runs.

The sections below follow one import end to end: what the user is shown, where that estimate comes
from, what the run does per page, and what keeps happening after the progress bar fills. For how a
connector works (the normalize/Sink split, credentials, the OAuth flow), see
[capture-connectors.md](capture-connectors.md). For the pricing formula the estimate uses, see
[ai-runtime.md](ai-runtime.md#cost--the-meter-collects-tokens-a-rate-table-prices-them). To
connect a mailbox and try this, see [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md).

## The screen the whole design serves

After a mailbox connects, the user picks a window and sees this:

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

Setup and Settings share a dropdown offering 3 months, 6 months, 1 year, 2 years, 3 years, 5 years,
7 years and 10 years. Six months is selected by default. The preview names the actual starting date.
The choice *none* is expressed by not starting: it short-circuits to zero with no provider call at all.

The starting date names the day of the preview boundary; Microsoft also filters by the time within that day. Windows are measured back from today using calendar-month arithmetic. Starting later recalculates that rolling window. Extending history preserves existing emails; overlapping messages are scanned again and deduplicated.

For a capped preview, both screens say the count is a lower bound and explain that the estimate covers only the counted messages: the full import may contain more and cost more.

Every section below answers a question that screen raises.

## "~3,436 messages" — counting the scope without reading the mail

The count is a real provider call, but it reads **ids only**. The preview pages
`messages.list?q=after:<date>` and counts what comes back: a list of `{id, threadId}` pairs, with no
headers, snippet or body. The call that fetches a message is used by the import loop, never by the
preview.

Two decisions shape the count:

- **The provider's own count is refused.** Gmail returns a `resultSizeEstimate`, and it is off by
  multiples: a 1,300-message window can read as ~200. The count also feeds the spend estimate, so a
  wrong count means the user consents to the wrong spend. An id count costs a handful of calls and is
  accurate.
- **It is bounded.** 500 ids per page, 40 pages, so up to 20,000 messages are counted one by one. A
  larger mailbox reports the counted floor instead of turning a preview into a long scan. The user
  consents to a bound on the scope; the bound is not a promise of the final count.

## "~0.84 USD" — pricing work that hasn't happened

An imported message lands in the timeline and also draws AI work. Three passes, each with its own
unit:

| pass | one unit is | what it produces |
|---|---|---|
| classify | a message | the attention label: commitment / meeting / noise |
| enrich | a newly created contact | contact fields read from the mail signature |
| embed | an entity | the vector that makes it searchable |

So the estimate is `Σ per-pass (expected units × per-unit cost)`, and both factors are measured:

```text
  expected units ◀── this connection's last completed import
                     (how many messages one scan captured,
                      how many contacts it created)

  per-unit cost  ◀── this workspace's last 7 days of real model calls,
                     each repriced at the model that will actually run it now
```

Each factor falls back independently. With no completed import, the units come from a built-in ratio;
with no call history, the cost comes from a **work-shape floor** derived from the size of the actual
prompt templates. Either fallback marks the whole estimate `heuristic` instead of `observed`. The API
returns this label so a reader can tell a measured number from a cold-start guess.

Rules for missing or zero prices:

- If nothing can be priced at all, the cost field is **omitted**. It is never rendered as `0`, because
  the user would consent to a spend figure that was made up.
- A local model's `$0` is a real price and *is* shown.
- If the estimate read fails outright, the preview degrades to a plain message count. It never blocks
  the flow.

Nothing in the import path refuses to run because the estimate is large or missing.

## Starting: one page at a time, resumable

The run is a row with a state machine, paged by the worker:

```text
   queued ──▶ running ──▶ done
                 │
                 ├──▶ cancelled   (the human stopped it, or the connection changed under it)
                 └──▶ error       (a non-transient fault, or too many consecutive transient ones)
```

The worker calls one step at a time; each step pulls **one provider page of 100 messages**, pushes
every message through the same Sink that standing sync uses, and commits the page's outcome together
with the cursor that resumes it. Because the **message** counters and the cursor land in one statement,
a page that fails to commit has scanned nothing the resume point will redo. The **counterparty** counts
are kept outside that statement (see below).

Two conditions make that commit conditional. Both mean *this page no longer belongs to the run being
written*:

- the run reached a terminal state concurrently (a cancel), or
- the connection was disconnected and reconnected while the page was out at the provider.

A page fetched under a grant its owner has since withdrawn is discarded.

**Failure is sorted by kind.** A rate limit or an unreachable provider is waited out on a doubling
backoff. The backoff honours the provider's own `Retry-After` whenever it asks for longer, since coming
back early only spends the next refusal. Anything else ends the run immediately. The transient ladder is
capped at 10 **consecutive** failures, and a committed page resets it to zero, so faults the import
already recovered from never end it.

## What the run learns about itself

While it pages, the run measures its own yield, which makes the *next* preview accurate.

The counterparty resolver reports what each ensure did. `contacts_created` counts contacts
**created**; a sender matched to an existing contact is not counted, and triggers no enrich call
either. `companies_created` counts something different: **domains this run queued for a company
verdict**, because capture creates no companies at all. A run that met twelve new domains did that
work whether or not the crawls have answered yet, and reporting zero would hide it.

A counterparty is counted **the moment it is created**, in its own write, outside the page's commit.
Counting each creation in its own write avoids losing or doubling counts when a page is cancelled,
retried or fails. Capture is idempotent, so a replayed message never reaches the resolver again;
there is nothing to rebuild a lost page total from.

What this guarantees and what it does not:

- It never **double-counts**. A row is created once, the write runs on that one outcome, and nothing
  retries it.
- It is not **once-and-only-once**. Creation and counting are different transactions, so a failed
  counter write loses that creation's count permanently (logged at ERROR).
- The loss is **per failure and uncapped**: a database fault spanning a page loses one count for every
  creation inside it.

So read the committed columns as a **floor** on what the run created, never an overcount. Closing the
gap needs a ledger keyed on the created row's id, which is a design decision rather than a cleanup.

The yields are an **under-count** by design. A sender the tier gate defers is resolved by the verdict
engine long after the page that saw it, and no page claims the contact it may eventually create. So a
run reporting zero contacts created is read as **"ratio unavailable"** instead of "zero contacts". A
window whose senders were all already known, suppressed or deferred reads zero, while a wider window
would create plenty. Quoting `$0` for enrich off that zero would understate the spend, so the estimate
floors instead and says `heuristic`.

## After the bar fills

AI spend continues after the import finishes. The three passes the estimate priced are periodic
sweeps over whatever backlog exists, outside the paging loop:

- **classify** runs hourly,
- **enrich** runs daily,
- **embeddings** ride their own lane.

So a freshly imported mailbox has its timeline immediately, and its labels, contact fields and search
vectors fill in behind it. Two consequences: the estimate covers work that lands after the progress bar
completes, and the live meter (the preview is only an estimate) is the source of truth for what was
spent.

One thing does fire on the completing edge: the same-day digest, so a freshly imported mailbox surfaces
on the morning screen instead of waiting for the nightly pass. It fires only on the single step that
moves a live run to `done`, so a lost race can never produce a spurious digest.

While it runs, the status surface reports `messages_scanned`, `captured`, `skipped`, `contacts_created`
and `companies_created`, alongside the estimate the run started with as the progress denominator.

## Known limitations

- **The contacts/company yields under-count** deferred senders, as above. This is by design, and the
  zero case is handled explicitly.
- **The scope count is capped** at 20,000 messages; beyond that the preview reports a floor.
- **The estimate assumes new mail resembles recent mail.** Longer mail costs more. The line under the
  estimate says it is an estimate and that usage is metered.
- **The web UI ignores the `observed`/`heuristic` label.** It renders the cost whenever the API
  returns one, `$0` included, but the quality signal never reaches the user even though the API
  returns it.

## Where the code lives

| concern | where |
|---|---|
| run control (start, status, cancel), scope count | `internal/modules/capture/backfill.go` |
| the paging loop, commit, failure ladder | `internal/modules/capture/backfillpager.go` |
| yield measurement | `internal/modules/capture/backfillyields.go` |
| the provider-side count and page fetch | `internal/modules/capture/gmail/backfill.go`, `.../graph/backfill.go` |
| the cost estimator | `internal/compose/costestimate/` |
| HTTP surface | `internal/compose/backfilltransport.go` |
| the consent screen | `frontend/src/screens/backfill.tsx` |

## Where to go next

- [capture-connectors.md](capture-connectors.md): how a connector works, and the one Sink every
  captured record goes through.
- [ai-runtime.md](ai-runtime.md#cost--the-meter-collects-tokens-a-rate-table-prices-them): the model
  routing, the token meter, and the rate table the estimate prices against.
- [privacy-and-consent.md](privacy-and-consent.md): what happens to imported personal data afterwards.
- [how-to/connect-a-mailbox.md](../how-to/connect-a-mailbox.md): connect a mailbox and run one.
