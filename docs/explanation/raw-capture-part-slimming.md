<!-- prose:plain -->
# raw_capture part slimming: what the stored original promises

`raw_capture` holds the provider's original for every captured message. Without slimming it would
hold every attachment twice: once as the object the attachment row points at, and once again as
`base64` inside the original. A sweep that runs from time to time removes the second copy and leaves
a reference to the first. That **cuts back a promise**, so the reasons are kept here, in one place,
rather than learned one at a time from a comment.

## What the column holds

`raw_capture.payload` holds the provider's bytes. The sweep cuts out an attachment part's encoded
body only when its stored copy **is proved safe in the object store**. That encoding must also show
up once, and only once, in the payload. A block such as this one takes its place:

```
Content-Type: application/pdf
Content-Disposition: attachment; filename="figures.pdf"
Content-Transfer-Encoding: base64
X-Margince-Part-Stored: part:1
X-Margince-Part-Sha256: e5eea006…
X-Margince-Part-Bytes: 23184815
X-Margince-Part-Storage-Key: <workspace>/attachment/01a082df-…
X-Margince-Part-Encoded-Sha256: 7c1a44b9…
X-Margince-Part-Encoded-Bytes: 31352096
X-Margince-Part-Wrap: 76
```

The provider's own fields keep their values **and their order**. The block is added as one run of
lines right before the empty line that ends the header block. `Content-Transfer-Encoding` stays as
it is, because the new body is itself `base64` that decodes. So a reader that decodes the slimmed
message gets a line of text that says where the bytes are now, rather than noise.

## Why the copy exists

Two writers, and each one does not know about the other. `storeRawCapture` in `capture/sinkraw.go`
writes `rec.Raw` (the whole `RFC822` message) inside the capture transaction. `capture/sinkparts.go`
then gets the same attachment bytes ready for the object store. `activities/capturedfiles.go` puts
them there before the row that records them (*"ORDER IS THE DESIGN"*).

Measured on one staging installation, 2026-09-09:

| Measure | Value |
|---|---|
| `raw_capture`, whole table | 6737 MB, of which 6732 MB is `TOAST` |
| Share of the whole database | 92% |
| Rows | 12,638 |
| `attachment.byte_size`, added up | 4343 MB across 17,705 rows |
| Same bytes, as `base64` | ~5.8 GB, about 86% of the table |
| `TOAST` bytes against data bytes | **1.0** |
| `subject` + `body` alone, against the whole original | 0.30% to 2.78% per mailbox |

That 1.0 tells the story. The `base64` of a file that is already compressed (PDF, ZIP, JPEG) does
not compress again. So the copy costs full price in the database files, in every full database copy,
in every `pg_dump` and in every `VACUUM`. Across four mailboxes, 42 MB of text kept 6.5 GB of MIME
in the database.

## Why a sweep, and not a cut at parse time

The first place you would cut the bytes is `mailmap.ToRecord`, so `rec.Raw` never carries them. That
does not work.

At the point where `storeRawCapture` writes the original, `stageParts` has not yet put the
attachment bytes in the object store. Cutting there leaves two ways to do it, both wrong. One is to
change the order of steps in the capture `sink`, so the stored original needs an outbound call to
work. That is on the path whose job is to get the mail stored. The other is to write a reference to
bytes that may never reach the store.

A later sweep can *prove* the bytes are safe before it removes anything. The same code also works
through rows captured before the sweep existed. One path, no change of order, and no window in which
the only copy is removed.

## Why match bytes, and not parse the MIME

Reading the message with `go-message` and writing it out again is not safe for this table, and we
can measure why:

- `go-message` **decodes on read and encodes again on write**. `base64` comes back with lines of its
  own length (76). Header fields inside a part written again change order. The `charset` of the text
  changes to `UTF-8`.
- The tree does not import `go-message/charset`, so a `windows-1252` message fails the read: 257
  rows / 143 MB of the staging data.
- And for the rows it *could* read, the body of every part is encoded again on the way out.
  `sinkraw.go` records that this kind of bug already cost us once:
  `"a body in a non-UTF-8 charset, or a malformed header, was stored ALTERED.`
  `The insert succeeded and the row looked fine."`

So the cut never reads the meaning of the message. The caller gives it bytes it has already checked
against the attachment row. `partslim` encodes those bytes and looks for the encoded bytes in the
payload. If they show up **once**, those bytes are cut out and the rest of the payload is copied as
it is. If they do not show up, or show up more than once, nothing is removed.

We checked this way of working against 40 real attachments from staging. Of those, 30 matched at
line length 76 with `CRLF` and restored byte for byte. The sweep refused the other 10, because the
same signature image showed up two to four times in the replies below the message. Bytes cannot tell
one copy from another. Those 10 run from 760 B to 30 kB, too small to count next to the attachments
of many MB that the sweep is for. Refusing them means the sweep never cuts bytes it cannot prove.

## Proof before the sweep removes a part

Every part the sweep removes has passed all of these checks:

1. an `attachment` row matched on `(external_source_id, external_part_id)`, that is
   `<source_system>:<source_id>` and `part:<n>`;
2. a `storage_key` that is not empty;
3. an object at that key whose length is the same as the row's `byte_size`;
4. bytes whose hash matches the row's `checksum`;
5. an encoding of those bytes that shows up once, and only once, in the payload.

A part that fails any of them keeps its bytes. That makes the sweep safe for a part dropped for
being over the limit, which has a number but no object. It also makes the sweep safe on an
installation with no object store.

**With no object store the sweep does nothing.** It sets no mark either. A mark without proof would
use up rows that a store connected later must still work on. `parts_slimmed_at` may only mean
*checked and failed*, never *nothing to prove with*.

## What still holds

- **The original, byte for byte.** `partslim.RestoreStoredParts` builds the provider's original
  bytes again. The block records the hash, length and line length of the *original encoded body*. So
  the new encoding is checked against the removed body, rather than trusted to match it. A restore
  that cannot make those bytes again fails and says so.
- **The dedupe key.** `(source_system, source_id)` does not change, so a replay still meets the row
  and is dropped.
- **Replay for review.** The three sweeps that read stored originals again
  (`compose/participantreplay.go`, `meetingattendeerepair.go`, `participantnamerecover.go`) read
  participants, attendees and the names shown for them. All of those live in headers, so these
  sweeps never read attachment parts back out of `raw_capture`. Art. 15 access and the private-thread
  files path (`capture/privatethreadfiles.go`) do, and both build the parts again first.
- **Art. 17 erasure.** The `ILIKE` over `payload::text` never looks inside a `base64` part, since
  the text itself is not there to match. `privacy/erasure_attachments.go` already removes the
  objects by `storage_key`.
- **Art. 15 access.** `compose/sarrestore.go` restores before it shows the data. A missing object
  holds back that one payload and keeps the row listed.
- **The retention sweep.** It erases the whole payload when the activity's window ends, block and
  all.

## What does not hold any more

**The column alone cannot make the message again.** A restore needs the object store. This gives up
a column that stands alone, for a promise kept in two places. The hash covers the second place: a
different object cannot pass as the provider's bytes, because the block records their hash.

One more cost is not live today, but may be later. The bytes are never put into a new form. So a
DKIM signature over a slimmed message **still checks out once the part is restored**, if the object
is there.

Nothing in the product checks DKIM today. The `dkim` references in `compose/techenrich.go` are DNS
checks for DKIM `selector` records, run to learn which tools a company uses. They are not signature
checks. So this costs nothing now, and it is written down for the next one who builds DKIM checks.

## What is still to do

- **Taking back objects no row points at.** `activities/capturedfiles.go` names this as a job both
  writers of the `attachment` table still leave open. This sweep does not change that: it removes a
  copy from the database and never touches an object.
- **A retention policy limits the table.** Slimming does not. Slimming makes `raw_capture` take on
  new bytes about 20 times more slowly, and does not put a limit on it. The `raw_capture` retention
  scope does, on a clock of its own rather than on the activity's. It does not reach an original
  with no activity row (an internal-only drop). There, the `raw_capture` row is the only mark that
  stops a replay from taking in a message again that the pipeline already handled.
- **The 10 in 40 it misses.** An image that shows up more than once in a message is never slimmed.
  Fixing it needs a way to name a part that stays the same across copies. That name would be a
  number read from the MIME tree rather than from the bytes, which means parsing, and this design
  does not parse. Look at it again only if small images that show up more than once come to make up
  a real share of the table.
