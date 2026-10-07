# Whose correspondence a chat is, and what follows from the answer

A Telegram message and a Gmail message travel the same pipeline: one
`connector.Sink`, one activity row, one audit + outbox commit, the same RBAC
gates. Who may read a captured message depends on whose credential carried it
(`channel_provider.credential_model`), not on its kind. The registry records
that column from the transport's own declaration.

## The rule

> If the transport spends a **global** credential (a bot, an Official Account,
> anything an administrator binds once for everybody), the traffic is
> **workspace** business and every seat reads it. If it spends a credential
> **bound to one member**, that member's chats are their own correspondence and
> behave like their mail: the workspace floor, their counterparty holds, a
> sender's own marker.

`workspace_bot` is the only sound answer for a shared credential. A hold needs
somebody to hold the message for, and a bot's message names no member: its
`captured_by` carries no user id, no mailbox imported it, and no participant row
names one. Holding it would satisfy no arm of the audience gate and leave a row
that no human can open and nobody can widen back. Held by
`TestEveryCredentialModelIsBornWithAReader`
(`capture/credentialmodelcensus_integration_test.go`), which reads its corpus
from the column's own CHECK so a third model cannot join the ladder untested.

## What decides a captured message's birth

`decideBirthTx` (`capture/birthdecision.go`) runs five rungs, strictest first.
Which of them a record faces depends on the credential model, not the kind:

| Rung | Keyed on | Mail | Chat on a member-bound credential | Chat on a workspace bot |
| --- | --- | --- | --- | --- |
| 1. workspace mail-sharing floor | the workspace | yes | **yes** | no |
| 2. counterparty hold | the correspondent, stored per seat | yes | **yes** | no |
| 3. explicit marker on the message | the message's own subject | yes | **yes** | no |
| 4. inherited thread verdict | the delivering mailbox | yes | no | no |
| 5. mailbox posture | the delivering mailbox | yes | no | no |

Rungs 4 and 5 read `capture_connection` (one human's own mailbox, its posture,
the verdicts taken against its threads), and a channel transport has no row in
it. Whether a member may set a standing posture for their own transport is an
open product question; mail's answer does not carry over.

A transport declaring `per_member` whose capture names no member is a
**misdeclaration**, and the capture is refused with the transport named. Both
ways of carrying on are wrong: publishing defies whatever the floor was set to,
and holding writes the unreadable row.

## The import row, and what it unlocks

`recordThisImport` (`capture/importrow.go`) writes a `capture_import` row only
for a seat whose own credential delivered the message. Mail proves that with an
address: one of the seat's own among `rec.Addresses`, the evidence that a
provider delivered it and that somebody did not just type its `Message-ID`. A chat carries
none of the seat's addresses, so that arm answers no for every channel message
there has ever been.

For a member-bound transport the **credential is the evidence**, and it is the
stronger claim. The extension ingress establishes both halves before capture
runs: the member holds one of the unit's user-scoped secrets (depositing it is
the act that says "act for me here"), and the unit called `Ingest` for that
member. Neither fact is in the record, so neither is a unit's to assert.

It is bounded to the seat the row's provenance names. A second member of the
same unit replaying the same key earns no import row. The core cannot tell a
colleague whose own credential also carried the message from one who guessed the
first member's source id, and refusing grants nothing.
That colleague reads the conversation through their own capture of it.

Every later decision hangs on the import row. It carries the seat's posture and
reason, `activities.RecomputeAudienceTx` derives from it, and
`capture.ThreadActivityIDsTx` joins through it. A member-bound chat reaches all
of them, and a workspace bot's message reaches none.

## What each side gets

| Capability | Mail | Chat, member-bound credential | Chat, workspace bot |
| --- | --- | --- | --- |
| Activity row, links, audit image, `activity.captured` event | yes | yes | yes |
| Per-seat import row (`capture_import`) | yes | **yes** | no (no member to name) |
| Workspace mail-sharing floor | yes | **yes** | no |
| Counterparty hold | yes | **yes**, where the record names addresses | no |
| Confidentiality marker in the subject | yes | **yes**, where the record carries a subject | no |
| Mailbox posture (`shared` / `classified` / `held`) | yes | no | no |
| Thread verdict, inherited by the next message | yes | no | no |
| Owner shares or re-holds a thread | yes | **yes**: the selection joins the seat's import rows | no: answers 404 |
| Direct per-message audience write | no: refused as captured | **no**: refused, for the same reason | yes |
| Audience recompute across every importing seat | yes | **yes** | no |
| Auto-create the counterparty | tiered ladder T0–T4, disposition ledger | own seam, always creates, ownerless | same |
| Attachments | yes | whatever the transport supplies | no |
| Waiting queue, "not sales", reader state, replying from the CRM | yes | yes | yes |
| Art. 17 erasure | address suppression | channel-identity suppression | channel-identity suppression |

## What a captured chat is born as

A message on a workspace bot: `workspace`, with a NULL `audience_reason`, by
design. The link-less
limiter still never fires for it, because `limitLinkLessAudience` returns as soon
as the counterparty decision says a record will be created and the channel seam
always says so.

A message on a member-bound transport: whatever rungs 1 to 3 conclude, recorded
on the member's import row so every later sync of the conversation derives the
same answer, and so the message appears wherever a seat's own held mail does.

Not every hold is then liftable, and the difference is the same one mail has. A
`counterparty` hold is the seat's own decision, so the widening pass reaches it
(`capture/widenhistory.go`, which matches a hold whose only reason is
counterparty). A `workspace_floor` hold is an admin's, and raising the floor
again does not re-open what it caught: already-captured correspondence keeps the
audience it has, for chat as for mail.

The manual per-message audience write follows the import row, so the two kinds
of chat differ here. `refuseCapturedAudienceWrite` refuses a direct audience set
on any row some seat imported, because such a row's audience is derived from its
importers. A member-bound chat has an import row, so the write is refused: the
decision is made about the conversation, through the thread share or hold path,
not message by message. A workspace bot's message has no import row, so the
per-message write still reaches it. That write cannot erase the message for
everybody: an audience write leaving no reader is refused
(`activities.SetAudience`, `auth.ActivityHasAReaderTx`).

## Still open

- **A member's own posture for their own transport** (rungs 4 and 5). It needs a
  place for a member to say it, which `capture_connection` is not.
- **No audience shown on a chat row.** The audience and its reason are not
  rendered on a timeline chat row. A member-bound chat can be held, so a
  reader can meet a message they cannot see and be told nothing about why.
- **Rows already narrowed before the orphan refusal shipped.** No sweep exists,
  and the census that would find them has to run as the system principal,
  because by construction no human can see them.

## Checking this page against the tree

- `rg 'memberBound' backend/internal/modules/capture`: the axis, at both sites.
- `rg -n 'credential_model' backend/`: the declaration, the registry, the
  published contract.
- `backend/internal/compose/extingressfloor_integration_test.go`: the whole
  behaviour, driven through the real ingress on two transports that differ only
  in what they declared.
