<!-- prose:plain -->
# Whose mail a chat is, and what follows from the answer

A Telegram message and a Gmail message travel the same pipeline: one `connector.Sink`, one activity row,
one audit + outbox commit, and the same RBAC gates. Who may read a captured message depends on whose
credential delivered it (`channel_provider.credential_model`), not on its kind. The registry records
that column from what the transport itself declares.

## The rule

> Say the transport spends a **shared** credential: a bot, an Official Account, or anything an admin
> binds once for all users. Then the messages are **workspace** business, and every seat reads them. Say
> it spends a credential **bound to one member**. Then that member's chats are their own mail, and act
> like their mail: the workspace floor, their counterparty holds, a marker the sender set.

`workspace_bot` is the only safe answer for a shared credential. A hold needs someone to hold the
message for, and a bot's message names no member. Its `captured_by` holds no user id, no mailbox
imported it, and no participant row names one. Holding it would meet no arm of the audience gate. It
would leave a row that no human can open, and that nobody can open up again.

`TestEveryCredentialModelIsBornWithAReader` (`capture/credentialmodelcensus_integration_test.go`) holds
this. It reads its corpus from the column's own CHECK, so a third model cannot join the ladder without a
test.

## What decides how a captured message starts

`decideBirthTx` (`capture/birthdecision.go`) runs five rungs, the strictest first. Which of them a record
meets depends on the credential model, not the kind:

| Rung | Keyed on | Mail | Chat on a member-bound credential | Chat on a workspace bot |
| --- | --- | --- | --- | --- |
| 1. workspace mail-sharing floor | the workspace | yes | **yes** | no |
| 2. counterparty hold | the other side, stored per seat | yes | **yes** | no |
| 3. a marker set on the message | the message's own subject | yes | **yes** | no |
| 4. thread verdict taken over from the thread | the delivering mailbox | yes | no | no |
| 5. mailbox posture | the delivering mailbox | yes | no | no |

Rungs 4 and 5 read `capture_connection`: one human's own mailbox, its posture, and the verdict taken on
each of its threads. A channel transport has no row in it. Whether a member may set a standing posture
for their own transport is an open product question. The answer for mail does not apply to it.

A transport that declares `per_member` while its capture names no member is **declared wrong**. The
capture is refused, with the transport named. Both ways to go on are wrong. Publishing goes against
what the floor was set to, and holding writes the row nobody can read.

## The import row, and what it opens up

`recordThisImport` (`capture/importrow.go`) writes a `capture_import` row only for a seat whose own
credential delivered the message. Mail proves that with an address. One of the seat's own addresses is
among `rec.Addresses`. That is the evidence that a provider delivered it, and that someone did not just
type its `Message-ID`. A chat holds none of the seat's addresses, so that arm answers no for every
channel message there has ever been.

For a transport bound to a member, **the credential is the evidence**, and it is the stronger claim. The
extension ingress sets up both facts before capture runs. The member holds one of the unit's secrets
with user scope (putting it there is the act that says `act for me here`). And the unit called `Ingest`
for that member. Neither fact is in the record, so neither is a unit's to state.

It is bounded to the seat that the row's provenance names. A second member of the same unit, sending the
same key again, gets no import row. The core cannot tell a colleague whose own credential also delivered
the message from one who made up the first member's source id. And refusing grants nothing. That
colleague reads the chat through their own capture of it.

Every later decision depends on the import row. It holds the seat's posture and reason, and
`activities.RecomputeAudienceTx` works from it. `capture.ThreadActivityIDsTx` joins through it. A
member-bound chat reaches all of them, and a workspace bot's message reaches none.

## What each side gets

| Feature | Mail | Chat, member-bound credential | Chat, workspace bot |
| --- | --- | --- | --- |
| Activity row, links, audit image, `activity.captured` event | yes | yes | yes |
| Import row per seat (`capture_import`) | yes | **yes** | no (no member to name) |
| Workspace mail-sharing floor | yes | **yes** | no |
| Counterparty hold | yes | **yes**, where the record names addresses | no |
| A Confidentiality marker in the subject | yes | **yes**, where the record holds a subject | no |
| Mailbox posture (`shared` / `classified` / `held`) | yes | no | no |
| Thread verdict, taken over by the next message | yes | no | no |
| Owner shares a thread, or holds it again | yes | **yes**: the choice joins the seat's import rows | no: answers 404 |
| Direct audience write per message | no: refused as captured | **no**: refused, for the same reason | yes |
| The audience, worked out again across every importing seat | yes | **yes** | no |
| Make the counterparty on its own | tiered ladder `T0`–`T4`, disposition log | own seam, always makes one, with no owner | same |
| Attached files | yes | what the transport gives | no |
| Waiting queue, "not sales", reader state, replying from the CRM | yes | yes | yes |
| Art. 17 erasure | address suppression | channel identity suppression | channel identity suppression |

## What a captured chat starts as

A message on a workspace bot: `workspace`, with a NULL `audience_reason`, by design. The limiter for
records with no link still never fires for it. That is because `limitLinkLessAudience` returns as soon as
the counterparty decision says a record will be made, and the channel seam always says so.

A message on a transport bound to a member: what rungs 1 to 3 decide. It is recorded on the member's
import row. So every later sync of the chat reaches the same answer, and the message shows up where a
seat's own held mail does.

Not every hold can then be opened again, and the split is the same one mail has. A `counterparty` hold is
the seat's own decision, so the pass that opens history again reaches it.
That pass (`capture/widenhistory.go`) matches a hold whose only reason is counterparty. A `workspace_floor` hold belongs to an admin. Raising the floor
again does not open what it held: mail already captured keeps the audience it has, for chat as for
mail.

The audience write by hand, per message, follows the import row, so the two kinds of chat differ here.
`refuseCapturedAudienceWrite` refuses a direct audience set on any row some seat imported, because such a
row's audience comes from its importers. A member-bound chat has an import row, so the write is refused.
The decision is made about the chat as a whole, through the thread share or hold path, not message by
message.

A workspace bot's message has no import row, so the write per message still reaches it. That write
cannot erase the message for every user: an audience write that leaves no reader is refused
(`activities.SetAudience`, `auth.ActivityHasAReaderTx`).

## Still open

- **A member's own posture for their own transport** (rungs 4 and 5). It needs a place for a member to
  say it, and `capture_connection` is not that place.
- **No audience shown on a chat row.** The audience and its reason are not shown on a chat row in the
  timeline. A member-bound chat can be held. So a reader can meet a message they cannot see, and be told
  nothing about why.
- **Rows with no reader left.** Some were left so before the refusal of such rows shipped. No sweep exists. The
  census that would find them has to run as the system principal, because by the way it is built no
  human can see them.

## Checking this page against the tree

- `rg 'memberBound' backend/internal/modules/capture`: the switch, at both sites.
- `rg -n 'credential_model' backend/`: what the transport declares, the registry, the published contract.
- `backend/internal/compose/extingressfloor_integration_test.go`: the whole flow, run through
  the real ingress on two transports that differ only in what they declared.
