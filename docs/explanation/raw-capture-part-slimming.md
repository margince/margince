<!-- prose:plain -->
<!-- describes: backend/internal/modules/capture/partslim backend/internal/modules/capture/partslimstore.go backend/internal/modules/capture/sinkoriginal.go backend/internal/compose/jobs_partslim.go backend/internal/compose/jobs_storedobject.go backend/internal/modules/capture/sinkraw.go backend/internal/compose/sarrestore.go backend/internal/modules/privacy/erasure_attachments.go backend/internal/platform/originalstore backend/internal/modules/capture/originaloffload.go backend/internal/modules/privacy/saroriginals.go backend/internal/modules/privacy/raworiginalsubjects.go backend/internal/modules/privacy/retentionrowerasure.go backend/internal/modules/activities/capturedfilecopies.go -->
# raw_capture part slimming: what the stored original promises

`raw_capture` holds the provider's original for every captured message. Without slimming it would
hold every attachment twice: once as the object the attachment row points at, and once again as
`base64` inside the original. A sweep that runs from time to time removes the second copy and leaves
a reference to the first. That **cuts back a promise**, so the reasons are kept here, in one place,
rather than learned one at a time from a comment.

## What the column holds

The encoded original stays inline up to the limit in
[`originalstore.InlineLimit`](../../backend/internal/platform/originalstore/originalstore.go).
Above it, the existing object store holds the encoded JSON. `payload_object` names its key,
SHA-256 and byte count. The inline value keeps only the JSON type. Readers check the object and
return the provider value.

Provider JSON cannot supply a storage reference. Without an object
store, a large capture fails before its transaction commits.

The existing part sweep also moves older large originals in bounded batches. Its partial index
marks which inline rows have been checked.

The stored original holds the provider's bytes. The sweep cuts out an attachment part's encoded
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

Two writers, and each one does not know about the other. Where an original is stored whole,
`storeRawCapture` in `capture/sinkraw.go` writes `rec.Raw` (the whole `RFC822` message) inside the
capture transaction. `capture/sinkparts.go` gets the same attachment bytes ready for the object store. `activities/capturedfiles.go` puts
them there before the row that records them (*"ORDER IS THE DESIGN"*).

Without slimming, every attachment sits in `raw_capture` a second time as `base64`, which takes 4
bytes for every 3 of the file. The `base64` of a file that is already compressed (PDF, ZIP, JPEG) does
not compress again. So the copy costs full price in the database files, in every full database copy,
in every `pg_dump` and in every `VACUUM`.

## A new message is stored slim; the sweep handles the rest

A new message stages its files in the object store, inside the capture transaction, before the
rows that point at them exist. `capture/sinkoriginal.go` holds back the original of such a message
until then. Then it writes the original once, with each stored file replaced by the block above, and
stamps the row `slimmed`. The file's bytes never reach `raw_capture`. The cut comes after the object
store took the bytes, and after the attachment row names that object. So the block never names
bytes that are not there.

A file whose bytes an attachment row already holds is not put again. It takes that row's key
(`activities/capturedfilecopies.go`), so identical files share one object.

The cut is not made at parse time (`mailmap.ToRecord`), because the bytes are not stored yet
there. The original is stored whole, and the sweep below meets it as before, when:

- the message is a replay, or the row turns out to be one capture took over;
- its file was kept back, as for a private thread;
- the attachment row already existed and names an older object, which only the sweep can prove;
- nothing can come out of it.

The sweep is also what works through rows captured before this existed.

## How fast it drains

Until the sweep reaches a row, the row holds its attachments in full. So the rows still waiting, not
the size after slimming, set how much disk new mail needs.

A pass (`compose/jobs_partslim.go`) runs every five minutes and takes batches of `PartSlimBatch`
rows. It keeps taking them while they come back full, for up to four minutes. A quiet mailbox leaves
the sweep with one short batch a pass. A mailbox backfill can write many large messages in an hour.
The pass keeps up with it rather than leave them whole for hours.

A pass stops early when the object store cannot prove a part. Such a row is still marked as looked
at, so more batches while the store is not answering would use up rows they cannot slim. The pass
stops after the first batch that holds such a part. A row with no attachment parts needs no call to
the store. So a pass can still take several full batches of those rows while the store is down.

## Why match bytes, and not parse the MIME

Reading the message with `go-message` and writing it out again is not safe for this table:

- `go-message` **decodes on read and encodes again on write**. `base64` comes back with lines of its
  own length (76). Header fields inside a part written again change order. The `charset` of the text
  changes to `UTF-8`.
- The tree does not import `go-message/charset`, so a `windows-1252` message fails the read.
- And for the rows it *could* read, the body of every part is encoded again on the way out.
  `sinkraw.go` records that this kind of bug already cost us once:
  `"a body in a non-UTF-8 charset, or a malformed header, was stored ALTERED.`
  `The insert succeeded and the row looked fine."`

So the cut never reads the meaning of the message. The caller gives it bytes it has already checked
against the attachment row. `partslim` encodes those bytes and looks for the encoded bytes in the
payload. Each part carrying those bytes needs one copy of them, and no copy may be left over. Then
every copy is cut out and the rest of the payload is copied as it is. Otherwise nothing is removed.

A signature image repeated down a quoted thread is one part per copy. Bytes cannot tell one copy
from another. So the copies go to the parts in order, the first copy to the first part. Each copy
must also sit under a part header that says `base64`. A copy that is not a part, such as the same
bytes quoted as text, leaves every copy in place. So the sweep never cuts bytes it cannot prove.

## Proof before the sweep removes a part

Every part the sweep removes has passed all of these checks:

1. an `attachment` row matched on `(external_source_id, external_part_id)`, that is
   `<source_system>:<source_id>` and `part:<n>`;
2. a `storage_key` that is not empty;
3. an object at that key whose length is the same as the row's `byte_size`;
4. bytes whose hash matches the row's `checksum`;
5. an encoding of those bytes that shows up once for each part that carries them, and no more.

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
  the text itself is not there to match. `privacy/erasure_attachments.go` queues the objects by
  `storage_key` for the `stored_object_reap` job. One object holds the bytes of every row with an
  identical file. Erasure keeps an object while a row outside the erased set still names it.
- **Art. 15 access.** `compose/sarrestore.go` restores before it shows the data. A missing object
  holds back that one payload and keeps the row listed.
- **The retention sweep.** It erases the whole payload when the activity's window ends, block and
  all.
- **Objects no row points at.** Both writers of the `attachment` table mark a key as provisional
  before they put the bytes. The hourly `stored_object_reap` job (`compose/jobs_storedobject.go`)
  deletes a provisional object whose row never came, once it is 24 hours old. This sweep removes a
  copy from the database and never touches an object.

## What does not hold any more

**The column alone cannot make the message again.** A restore needs the object store. This gives up
a column that stands alone, for a promise kept in two places. The hash covers the second place: a
different object cannot pass as the provider's bytes, because the block records their hash.

One more cost is not live today, but may be later. The bytes are never put into a new form. So a
DKIM signature over a slimmed message **still checks out once the part is restored**, if the object
is there.

Nothing in the product checks DKIM today. The `dkim` references in `compose/techenrich.go` are DNS
checks for DKIM `selector` records, run to learn whether a company has DKIM set up. They are not signature
checks. So this costs nothing now, and it is written down for the next one who builds DKIM checks.

## Whole-original objects

The existing object ledger records an upload before it starts. The capture transaction clears
that intent when it takes the reference. A replaced or deleted reference records its old object
in the same transaction. The existing job removes objects no row holds.

A bounded hash filter rules out absent email windows before reading the object. Possible matches
still use the same SQL content test. Unknown formats read the original. Repairs pass over missing
or broken evidence, but retry when the object store cannot be reached.

Subject erasure and retention delete no object. The row delete queues its key in the same
transaction, and the hourly `stored_object_reap` job deletes the bytes after the commit.

Erasure first records a matched subject in trusted metadata.
A retry can then find its original if the object is gone and the database change failed.
A changed reference makes erasure check the object again. Export keeps its linked-activity access
rule and reads only content it may show. Storage metadata never enters the export.

For retention, every linked activity must be past its correspondence floor and free of legal hold.
This holds for archived activities too. Originals with no linked activity keep the existing replay
boundary.

## What is still to do

- **A retention policy limits the table.** Slimming does not. Slimming makes `raw_capture` take on
  new bytes more slowly, and does not put a limit on it. The `raw_capture` retention
  scope does, on a clock of its own rather than on the activity's. It does not reach an original
  with no activity row (an internal-only drop). There, the `raw_capture` row is the only mark that
  stops a replay from taking in a message again that the pipeline already handled.
- A file that also shows up outside its parts: bytes quoted as text beside the parts that carry
  them keep every copy. Fixing it needs a way to tell a part from a quote, which means
  parsing, and this design does not parse.
