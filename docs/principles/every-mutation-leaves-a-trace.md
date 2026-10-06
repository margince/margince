# Every mutation leaves a trace

**A change to a domain row, the audit record of that change, and the event
announcing it commit together or not at all.** One transaction, three rows.

The binding form is *The write shape*
in the rulebook, spelled once in `platform/database/storekit` (`Audit` + `Emit`)
and called by every module store. What follows is why it has that shape.

## What the single transaction buys

Three failures become impossible rather than unlikely:

- **A change nobody can account for.** If the audit row could fail
  independently, the write someone later asks about (one that succeeded under
  unusual circumstances) would be the one most likely to be missing its
  record.
- **An orphan event**: an event for a row that does not exist, or a row that changed
  with no outbox record. The transaction guarantees that the *staging* is
  atomic: after the commit there is a durable outbox row or there is no change
  at all. Getting that row onto the bus is the relay's job, which is
  at-least-once with retries and does not guarantee delivery.
  Atomic staging is what makes the dual-write problem go away; monitoring the
  relay is a separate obligation.
- **A caller-supplied actor.** `captured_by` is stamped from the authenticated
  principal, never from the request body.

## The method

**Write domain rows only through a store.** A module store's entry point owns
the transactional shape. A handler that reaches for SQL has skipped both
the audit and the gate.

**Publishing is always through the outbox.** `platform/events.Relay` ships it;
no direct XADD from domain code. The bus is at-least-once, so consumers wrap
handlers in `events.Dedupe`: a consumer that assumes each event arrives once
breaks on the first redelivery.

**Trace the request end to end.** The HTTP layer mints one `correlation_id` per
request; `Audit()` returns the audit row id; `Emit()` links both. A trace that
starts at the event has lost the half that says who asked.

**Every store entry point is RBAC-gated**, and the two denials are different
facts:

| Denial | Sentinel | Wire | Why |
|---|---|---|---|
| object denied | `apperrors.ErrPermissionDenied` | 403 | the caller may not perform this verb |
| row out of scope | `apperrors.ErrNotFound` | 404 | existence-hiding: a 403 here would confirm the row exists |

**Anything that returns a record is a read** and carries the row-scope gate,
including replay, conflict and error paths. Error paths are where this rule is
usually broken: an error path that echoes the conflicting row has served it.

**A new seam owes the whole shape.** Audit-only mutations exist and are
allowed: installation configuration writes an audit row and no event, because
the closed event catalog defines no type for it and inventing one build-side is
forbidden. They are *enumerated*: `backend/gates/writeshape_test.go` carries each
one with the argument for why, and the gate refuses an audit-only function that
is not on the list, as well as a listed one that no longer exists. For a new
write path, audit implies event unless you can write the argument that puts it
on that list.

## What this does not ask for

- **Not an audit row per read.** Reads are gated, not journalled.
- **Not that evidence tables join the shape.** `raw_capture` is evidence, not a
  domain row; it neither audits nor emits, and that is correct.
- **Not a bespoke event per column.** The envelope is the
  `shared/kernel/events` contract; a new event kind is a catalog change, not a
  free-form payload.
