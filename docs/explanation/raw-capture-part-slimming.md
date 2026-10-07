# raw_capture part slimming: what the stored original promises

`raw_capture` holds the provider's original for every captured message. Without slimming it would
hold every attachment twice: once as the object the attachment row points at, and once again as
base64 inside the original. A periodic sweep removes the second copy and leaves a reference to the
first. That **narrows a guarantee**, and the reasoning is collected here so it is argued in one place
rather than discovered in a comment.

## What the column holds

`raw_capture.payload` holds the provider's octet stream with each **provably durable** attachment
part's encoded body replaced by a stanza:

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

The provider's own fields keep their values **and their order**. The stanza is appended as a
contiguous run immediately before the header block's blank line. `Content-Transfer-Encoding` is left
alone: the substitute body is itself valid base64, so a reader that decodes the slimmed message gets
a sentence explaining where the bytes went rather than noise.

## Why the duplication exists

Two writers, neither aware of the other. `capture/sinkraw.go`'s `storeRawCapture` writes `rec.Raw`
(the whole RFC822 message) inside the capture transaction. `capture/sinkparts.go` then stages the
same attachment bytes to the object store, and `activities/capturedfiles.go` puts them there before
the row that records them (*"ORDER IS THE DESIGN"*).

Measured on one staging installation, 2026-09-09:

| Measurement | Value |
|---|---|
| `raw_capture` total | 6737 MB, of which 6732 MB is TOAST |
| Share of the whole database | 92% |
| Rows | 12,638 |
| `attachment.byte_size` sum | 4343 MB across 17,705 rows |
| Same bytes, base64-inflated | ~5.8 GB, about 86% of the table |
| TOAST compression ratio | **1.0** |
| Extracted `subject` + `body` vs the raw original | 0.30% to 2.78% per mailbox |

The compression ratio is the tell: base64 of an already-compressed container (PDF, zip, JPEG) does
not compress. The duplicate cost full price on disk, in every base backup, in every `pg_dump` and in
every vacuum. Across four mailboxes, 42 MB of text was being kept alive by 6.5 GB of MIME.

## Why a sweep, and not the parser

The obvious place to strip is `mailmap.ToRecord`, so `rec.Raw` never carries the bytes. That does not
work: where `storeRawCapture` writes the original, `stageParts` has not yet put the attachment bytes
in the object store. Stripping there would mean one of two bad options. One is reordering the sink so
the stored original depends on an outbound call succeeding, on the path that exists so correspondence
lands. The other is writing a reference to bytes that may never arrive.

A later sweep can *prove* durability before removing anything, and the same code drains rows captured
before the sweep existed. One path, no reorder, and no window in which the only copy is gone.

## Why by byte offset, and not by parsing the MIME

Walking the message with `go-message` and re-emitting it is unsound for this table, and measurably so:

- The library **decodes on read and re-encodes on write**. Base64 comes back re-wrapped at its own
  76-character width, header fields inside a rewritten part change order, and charsets are
  converted to UTF-8.
- The tree does not import `go-message/charset`, so a `windows-1252` message fails the walk
  outright: 257 rows / 143 MB of the staging corpus.
- Worst, for the rows it *could* walk, every part's body is re-encoded on the way out. That is the
  defect class `sinkraw.go` already records as paid for once: *"a body in a non-UTF-8 charset, or a
  malformed header, was stored ALTERED. The insert succeeded and the row looked fine."*

So the strip never interprets the message. The caller supplies octets it has already verified
against the attachment row; `partslim` encodes those octets and looks for that byte string in the
payload. If it appears **once**, those bytes are spliced out and everything around them is
copied verbatim. If it appears zero times or more than once, nothing is removed.

The approach was validated against 40 real attachments from staging. Thirty were located at width 76
with CRLF and restored byte for byte. Ten were refused because the same signature logo appeared two to four times
down a quoted thread, and bytes cannot tell one copy from another. Those ten ran 760 B to 30 kB, a
rounding error against the megabyte attachments the sweep targets. Refusing them means the sweep
never splices a guess.

## Proof before removal

Every part the sweep removes has cleared all of:

1. an `attachment` row joined on `(external_source_id, external_part_id)`, i.e.
   `<source_system>:<source_id>` and `part:<n>`;
2. a non-empty `storage_key`;
3. an object at that key whose length equals the row's `byte_size`;
4. octets that hash to the row's `checksum`;
5. an encoding of those octets found once, and only once, in the payload.

A part failing any of them keeps its bytes. That makes the sweep safe for a part dropped for size
(it has an ordinal but no object) and safe on a deployment with no object store.

**With no object store the sweep does nothing.** It stamps nothing either. A stamp without proof
would consume rows that a store wired later should still process. `parts_slimmed_at` may only ever
mean *considered and found wanting*, never *nothing to prove with*.

## What still holds

- **Byte-exact reconstruction.** `partslim.RestoreStoredParts` rebuilds the provider's original octet
  stream. The stanza records the *original encoded body's* digest, length and wrap width, so the
  re-encoding is compared against what was removed rather than assumed to match it. A restore that
  cannot reproduce those bytes fails and says so.
- **The dedupe natural key.** `(source_system, source_id)` is untouched, so a replay still
  tombstones.
- **Forensic replay.** The three sweeps that re-read stored originals
  (`compose/participantreplay.go`, `meetingattendeerepair.go`, `participantnamerecover.go`) read
  participants, attendees and display names, all of which live in headers. Nothing in the tree
  parses attachment parts back out of `raw_capture`.
- **Art. 17 erasure.** The `ILIKE` over `payload::text` never saw inside a base64 part anyway, since
  the plaintext is not there to match, and `privacy/erasure_attachments.go` already purges the
  objects by `storage_key`.
- **Art. 15 access.** `compose/sarrestore.go` restores before disclosing. A missing object
  withholds that one payload and keeps the row listed.
- **The retention sweep.** It erases the whole payload when the activity's window closes, stanza
  and all.

## What no longer holds

**The column alone cannot reproduce the message.** A restore needs the object store. This trades a
self-contained column for a two-place guarantee. The digest guards the second place: a replaced
object cannot pass as the provider's bytes, because the stanza records their hash.

One consequence is latent rather than current. Re-canonicalisation is avoided, so a **DKIM signature
over a slimmed message still verifies once the part is restored**, but only if the object is there.
Nothing in the product verifies DKIM today (the `dkim` references in `compose/techenrich.go` are DNS
selector probes for tech enrichment, not signature checks). So this costs nothing now, and is written
down for whoever reaches for DKIM verification next.

## What is still owed

- **Orphaned-object reclamation.** `activities/capturedfiles.go` names it as owed by both writers of
  the `attachment` table. This sweep does not change that: it removes a copy from the database and
  never touches an object.
- **A retention policy bounds growth; slimming does not.** Slimming cuts the slope of `raw_capture`'s
  growth by roughly twentyfold and does not make it bounded. The `raw_capture` retention scope does,
  on a clock of its own rather than on the activity's. It does not reach an original with no
  activity row (an internal-only drop), where the raw_capture row is the only tombstone against a
  replay re-ingesting a message the pipeline already judged.
- **The ten-in-forty miss rate.** A repeated inline logo is never slimmed. Fixing it needs an
  identity for a part that survives duplication: an ordinal resolved against the MIME structure
  rather than against the bytes, which is the parsing approach this design avoids. It is worth
  revisiting only if small repeated images ever become a material share of the table.
