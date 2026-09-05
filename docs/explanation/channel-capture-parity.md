# Channel capture and mail capture are not the same product yet

A Telegram message and a Gmail message travel the same pipeline — one
`connector.Sink`, one activity row, one audit + outbox commit, the same RBAC
gates. They do **not** get the same treatment afterwards. Everything the mail
work built to answer *who may read this message* is gated to `kind = 'email'`,
and a channel message inherits none of it.

This page states where the two paths part, so a change to either side is made
knowing what the other side does. It describes the tree as it stands; the code
is the authority, and the two gates below are where to check.

## The two gates

Everything downstream follows from these, and neither is a per-feature flag.

**1. The birth ladder refuses a non-mail kind.**
`capture/birthdecision.go`, first statement of `decideBirthTx`:

```go
// Non-mail kinds keep the workspace default: a meeting or a channel message
// is not correspondence a mailbox posture was ever asked about.
if fields.Kind != "email" {
    return birthDecision{}, nil
}
```

So a channel message is decided by none of the five rungs — the workspace
mail-sharing floor, a counterparty hold, an explicit `[Vertraulich]` marker, an
inherited thread verdict, the delivering mailbox's own posture. It is born
`workspace` unless `limitLinkLessAudience` catches it for filing under no record.

**2. The import row needs an address.**
`recordThisImport` (`capture/importrow.go`) writes a `capture_import` row only
once `mailboxWasARecipientTx` finds one of the acting seat's **own email
addresses** among `rec.Addresses` — the anti-forgery evidence that a provider
delivered the message rather than that somebody typed its `Message-ID`. A
channel connector supplies no addresses at all (`capture/telegram/normalize.go`
sets `Counterparty.ChannelIdentity` and nothing address-shaped), so a channel
message never gets an import row for any seat.

That single absence removes the rest, because `capture_import` is the row every
later decision hangs on: it carries the seat's posture and verdict, it is what
`activities.RecomputeAudienceTx` derives from, and it is the join in
`capture.ThreadActivityIDsTx` that the owner's share/hold decision selects
through.

## What each side gets

| Capability | Mail (`kind = 'email'`) | Channel (`kind = 'message'`) | Why they differ |
| --- | --- | --- | --- |
| Activity row, links, audit image, `activity.captured` event | yes | yes | one sink, one write shape; the event names `channel_provider` |
| Per-seat import row (`capture_import`) | yes | **no** | `mailboxWasARecipientTx` matches an address; a channel record carries none |
| Workspace mail-sharing floor | yes | **no** | `decideBirthTx` returns before reading the setting |
| Mailbox posture (`shared` / `classified` / `held`) | yes | **no** | posture lives on `capture_connection` and is read per mail connection |
| Counterparty hold (this seat holds their mail) | yes | **no** | `capture_counterparty_hold` is keyed on address and domain |
| `[Vertraulich]` / confidentiality marker in the subject | yes | **no** | a channel message has no subject to mark |
| Thread verdict, inherited by the next message | yes | **no** | `capture_thread_verdict` is opened only for a held import row |
| Owner shares or re-holds a thread (`ThreadAudienceSetter.Decide`) | yes | **no** — answers 404 | it selects `activity JOIN capture_import`, which finds nothing |
| `held_by_others` count reported back to the sharer | yes | n/a | derived from the same import rows |
| Audience recompute across every importing seat | yes | **no** | reached only from inside `recordThisImport` |
| Held-to-participants when filed under no record | yes | yes | `limitLinkLessAudience` reads the kind and the counterparty shape, not the transport |
| Auto-create the counterparty | tiered ladder T0–T4, disposition ledger | own seam, always creates, ownerless | a human messaging the workspace's own bot *is* the affirmative intent the ladder hunts for; the ledger is address-keyed |
| Pending-counterparty question and the verdict engine | yes | **no** | same address-keyed ledger |
| Attachments | yes | **no** | the connector fetches no media; `recordParts` is transport-agnostic and simply gets nothing |
| Waiting queue / worklist | yes | yes | `activities/waitingsql.go`: `kind IN ('email','message')`, matched within one `channel_provider` |
| "Not sales" disposition | yes | yes | `activity_sales_state` is keyed `(thread_key, kind, channel_provider)` |
| Reader state — snooze, not mine, pin | yes | yes | keyed on the activity and the reader, never on the transport |
| Replying from the CRM | yes | yes | `capture/channelsend.go` with the outbound staging row |
| Art. 17 erasure | address suppression | channel-identity suppression | `refuseErasedChannelAccount` takes the account's advisory lock inside the capture transaction, because a channel record with no person link and no address would otherwise be findable by no later sweep |

## What a channel message is therefore born as

Workspace-readable, unless it is filed under no record at all. There is no seat
posture that holds it, no verdict that judges it, and no control anywhere that
lets the receiving human hold it afterwards — the share endpoint cannot see a
message the seat has no import row for, in either direction.

The same conversation arriving by email is gated by the workspace floor, then by
that mailbox's posture, and is shareable or re-holdable by its owner at any time.

## If the gap is to be closed

`capture_import` is the seam. Key the row on *how the message named its human*
rather than on an address:

- `counterparty_shape` already answers that question totally
  (`capture/sinkchannel.go`), and `shapeChannel` is the arm that needs the
  delivery evidence a channel can actually give — the bot's own membership of
  the chat, which the poll already knows.
- Once a channel message has an import row, the audience recompute, the thread
  decision and `held_by_others` follow with no further change: each of them
  reads import rows and asks nothing about a transport.
- The posture question is separate and genuinely open: a mail posture is a
  property of a mailbox, and a workspace bot is not one seat's mailbox. That is
  a product decision, not a refactor — say whose bot's traffic is whose before
  writing a posture column for it.

Whatever is decided, decide it in one place. Two spellings of "may the workspace
read this" — one for mail, one for channels — is the shape that drifts until the
two disagree in front of a customer.

## Checking this page against the tree

- `rg 'Kind != "email"' backend/internal/modules/capture` — the birth gate.
- `rg -n 'capture_import' backend/internal` — every reader of the row the mail
  path writes and the channel path does not.
- `backend/internal/compose/threadaudience_integration_test.go` — what the share
  decision is held to today, all of it mail.
