<!-- prose:plain -->
# Every mutation leaves a trace

**A change to a domain row, the audit record of that change, and the event that reports it commit
together or not at all.** One transaction, three rows.

The rule a change must follow is *The write shape* in the rulebook. It exists once, in
`platform/database/storekit` (`Audit` + `Emit`), and every module store calls it. What follows is
why it has that shape.

## What one transaction buys

Three problems can never happen, where without it they happen once in a while:

- **A change no one can account for.** Say the audit row could fail on its own. The write that
  someone asks about later is one that worked in a case out of the normal. That write then has the
  most risk of missing its record.
- **An event with no row**: an event for a row that does not exist, or a row that changed with no
  outbox record. The transaction makes the write to the outbox atomic. After the commit there is an
  outbox row that lasts, or there is no change at all. Getting that row to the bus is the job of the
  relay. The relay sends each row at least once and sends it a second time after a failed send, but it
  does not promise delivery. The atomic outbox write is what ends the problem of writing to two
  places; watching the relay is a separate obligation.
- **A caller that says who it is.** `captured_by` comes from the signed-in user or agent, never
  from the request body.

## How to use it

**Write domain rows only through a store.** The entry point of a module store owns the transaction
shape. A handler that reaches for SQL skips both the audit and the gate.

**Every event goes out through the outbox.** `platform/events.Relay` ships it; domain code never
sends an XADD of its own. The bus sends each event at least once, so every handler that reads events
runs through `events.Dedupe`. A handler that expects each event to come once breaks the first time an
event comes twice.

**Trace the request from start to end.** The HTTP code makes one `correlation_id` per request;
`Audit()` returns the audit row ID; `Emit()` links both. A trace that starts at the event is missing
the part that says who asked.

**Every store entry point has an RBAC gate**, and the two denials mean two other things:

| Denial | Error value | HTTP status | Why |
|---|---|---|---|
| object denial | `apperrors.ErrPermissionDenied` | 403 | the caller may not take this action |
| row out of scope | `apperrors.ErrNotFound` | 404 | it keeps secret that the row exists: a 403 here confirms the row is there |

**A path returning a record is a read** and has the row-scope gate, including replay,
conflict and error paths. Error paths are where this rule breaks most: an error path that returns
the conflicting row has served it.

**A new seam must have the whole shape.** A write with an audit row and no event may happen in
some cases. Installation config is one. It writes an audit row and no event, because the closed
event catalog has no type for it and the build may not add one. These cases are *listed*:
`backend/gates/writeshape_test.go` names each one with the reason for it. The gate refuses an
audit-only function that is not on the list, or a listed one that does not exist any more.

For a new write path, an audit row means an event too, unless you can write the reason that puts it
on that list.

## What this does not ask for

- **Not an audit row for each read.** Reads have a gate, not a log.
- **Not that evidence tables follow the shape.** `raw_capture` is evidence, not a domain row. It
  writes no audit row and no event, and that is right.
- **Not a custom event for each column.** The outer shape of an event is the
  `shared/kernel/events` contract. A new kind of event is a catalog change, not a free-form payload.
