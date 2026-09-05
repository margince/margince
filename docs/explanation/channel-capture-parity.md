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
| Per-seat import row (`capture_import`) | yes | **no** | `mailboxWasARecipientTx` matches an address, and a bot has no seat behind it to name anyway |
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
posture that holds it and no verdict that judges it, and the thread share/hold
endpoint cannot see it in either direction — it selects through the import row
that is never written.

What is left is the manual per-message audience control, and it is available in
exactly the wrong half of the cases. `refuseCapturedAudienceWrite` refuses a
direct audience write only for a message some mailbox imported, so the server
accepts one on any channel message; the timeline row offers it for every kind
but `email`, withholding it where `audience_reason` says the audience was
derived (`frontend/src/screens/timelineactions.tsx`). So a channel message filed
under a record — audience `workspace`, no reason — can be narrowed by hand, and
a link-less one, which `limitLinkLessAudience` already held to its participants,
offers no control at all. The hold a channel message can actually get is the one
nobody can lift.

The same conversation arriving by email is gated by the workspace floor, then by
that mailbox's posture, and is shareable or re-holdable by its owner at any time.

## Why closing it is a product decision, not a refactor

The obvious fix — "write a `capture_import` row for a channel message too" — does
not typecheck against the model. `capture_import` is keyed `(activity_id,
user_id)`, and a channel message has no seat to name:

- a channel connection is one **workspace-wide bot binding**, not a seat's
  mailbox. It lives in its own table (`channel_connection`, unique per provider
  while live) precisely because `capture_connection` models "one human's own
  mailbox" and a bot is not that. `capture/channelconn.go` says so in its opening
  comment.
- `connected_by` on that row is **audit-only, never an owner**, restated in
  `capture/channelconn.go`, `capture/sinkchannel.go` and
  `compose/telegramingest.go`.
- the connector principal the poll builds sets neither `UserID` nor
  `OnBehalfOf` (`compose/telegramingest.go`), deliberately: reusing the
  connecting admin would make every captured message look like that admin's own
  row-scoped activity.
- there is no chat→seat mapping anywhere. `person_channel_identity` maps a chat
  identity to a **person** — the outside human — and carries no owner column.

So the mail question — *which of our seats received this, and what does each of
them ask of it?* — has no answer for a bot. Parity therefore needs a decision
about **whose privacy a channel conversation is** before any code moves:

- a workspace-level posture on the bot binding (one answer for all of it), or
- a per-chat owner, which the product does not model today, or
- a deliberate "channel traffic is workspace business", written down and held by
  a test rather than left as an omission.

One thing is worth settling whichever answer wins: the manual audience write
above is a per-message answer on a conversation, and a chat is a conversation.
Deciding it message by message is how one thread ends up half shared.

Whatever is decided, decide it in one place. Two spellings of "may the workspace
read this" — one for mail, one for channels — is the shape that drifts until the
two disagree in front of a customer.

## Checking this page against the tree

- `rg 'Kind != "email"' backend/internal/modules/capture` — the birth gate.
- `rg -n 'capture_import' backend/internal` — every reader of the row the mail
  path writes and the channel path does not.
- `backend/internal/compose/threadaudience_integration_test.go` — what the share
  decision is held to today, all of it mail.
