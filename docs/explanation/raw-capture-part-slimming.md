# raw_capture part slimming — what the stored original promises

`raw_capture` holds the provider's original for every captured message, and it held every
attachment twice: once as the object the attachment row points at, and once again as base64 inside
the original. A periodic sweep now removes the second copy and leaves a reference to the first. This
page is the reasoning, because it **narrows a guarantee** and a narrowed guarantee should be argued
in one place rather than discovered in a comment.

## What changed

`raw_capture.payload` held the provider's octet stream, attachment bodies included. It now holds
that stream with each **provably durable** attachment part's encoded body replaced by a stanza:

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
contiguous run immediately before the header block's blank line, and `Content-Transfer-Encoding` is
deliberately left alone: the substitute body is itself valid base64, so a reader that decodes the
slimmed message gets a sentence explaining where the bytes went rather than noise.

## Why the duplication existed

Two writers, neither aware of the other. `capture/sinkraw.go`'s `storeRawCapture` writes `rec.Raw`
— the whole RFC822 message — inside the capture transaction. `capture/sinkparts.go` then stages the
same attachment bytes to the object store, and `activities/capturedfiles.go` puts them there before
the row that records them (*"ORDER IS THE DESIGN"*).

Measured on one staging installation, 2026-09-09:

| Measurement | Value |
|---|---|
| `raw_capture` total | 6737 MB, of which 6732 MB is TOAST |
| Share of the whole database | 92% |
| Rows | 12,638 |
| `attachment.byte_size` sum | 4343 MB across 17,705 rows |
| Same bytes, base64-inflated | ~5.8 GB — about 86% of the table |
| TOAST compression ratio | **1.0** |
| Extracted `subject` + `body` vs the raw original | 0.30% – 2.78% per mailbox |

The compression ratio is the tell: base64 of an already-compressed container (PDF, zip, JPEG) does
not compress, so the duplicate cost full price on disk, in every base backup, in every `pg_dump`
and in every vacuum. Across four mailboxes, 42 MB of text was being kept alive by 6.5 GB of MIME.

## Why a sweep, and not the parser

The obvious place to strip is `mailmap.ToRecord`, so `rec.Raw` never carries the bytes. It does not
work: at `sink.go:180` where the original is written, the attachment bytes are **not yet in the
object store** — `stageParts` runs later, from `sinkactivity.go:116`. Stripping there would mean
either reordering the sink so the stored original depends on an outbound call succeeding, on the
path whose whole purpose is that correspondence lands, or writing a reference to bytes that may
never arrive.

A later sweep can *prove* durability before removing anything, and the same code then drains the
rows captured before it existed. One path, no reorder, and no window in which the only copy is
gone.

## Why by byte offset, and not by parsing the MIME

The first implementation walked the message with `go-message` and re-emitted it. That is unsound for
this table, and measurably so:

- The library **decodes on read and re-encodes on write**. Base64 comes back re-wrapped at its own
  76-character width, header fields inside a rewritten part change order, and charsets are
  converted to UTF-8.
- The tree does not import `go-message/charset`, so a `windows-1252` message fails the walk
  outright — 257 rows / 143 MB of the staging corpus.
- Worst, for the rows it *could* walk, every part's body was re-encoded on the way out. That is the
  defect class `sinkraw.go` already records as paid for once: *"a body in a non-UTF-8 charset, or a
  malformed header, was stored ALTERED. The insert succeeded and the row looked fine."*

So the strip never interprets the message. The caller supplies octets it has already verified
against the attachment row; `partslim` encodes those octets and looks for that byte string in the
payload. If it appears **exactly once**, those bytes are spliced out and everything around them is
copied verbatim. If it appears zero times or more than once, nothing is removed.

Validated against 40 real attachments from staging: 30 located at width 76 with CRLF and restored
byte-exactly; 10 refused because the same signature logo appeared two to four times down a quoted
thread and bytes cannot tell one copy from another. Those ten ran 760 B to 30 kB — a rounding error
against the megabyte attachments this exists for, and the right trade for never splicing a guess.

## Proof before removal

Every part the sweep removes has cleared all of:

1. an `attachment` row joined on `(external_source_id, external_part_id)`, i.e.
   `<source_system>:<source_id>` and `part:<n>`;
2. a non-empty `storage_key`;
3. an object at that key whose length equals the row's `byte_size`;
4. octets that hash to the row's `checksum`;
5. an encoding of those octets found exactly once in the payload.

A part failing any of them keeps its bytes. That is what makes the sweep safe for a part dropped
for size — it has an ordinal but no object — and safe on a deployment with no object store.

**With no object store the sweep does nothing at all, including no stamping.** This was a real hole
during implementation: a pass that marked rows "considered" while proving nothing would consume the
backlog a later-wired store would have worked, permanently and silently. `parts_slimmed_at` may
only ever mean *considered and found wanting*, never *nothing to prove with*.

## What still holds

- **Byte-exact reconstruction.** `partslim.RestoreStoredParts` rebuilds the provider's exact octet
  stream. The stanza records the *original encoded body's* digest, length and wrap width, so the
  re-encoding is compared against what was removed rather than assumed to match it. A restore that
  cannot reproduce those bytes fails and says so.
- **The dedupe natural key.** `(source_system, source_id)` is untouched, so a replay still
  tombstones.
- **Forensic replay.** The three sweeps that re-read stored originals —
  `compose/participantreplay.go`, `meetingattendeerepair.go`, `participantnamerecover.go` — read
  participants, attendees and display names, all of which live in headers. Nothing in the tree
  parses attachment parts back out of `raw_capture`.
- **Art. 17 erasure.** The `ILIKE` over `payload::text` never saw inside a base64 part anyway —
  the plaintext is not there to match — and `privacy/erasure_attachments.go` already purges the
  objects by `storage_key`.
- **Art. 15 access.** `compose/sarrestore.go` restores before disclosing. A missing object
  withholds that one payload and keeps the row listed.
- **The retention sweep.** It erases the whole payload when the activity's window closes, stanza
  and all.

## What no longer holds

**The column alone is no longer sufficient to reproduce the message.** A restore needs the object
store. This trades a self-contained column for a two-place guarantee, and the digest is what keeps
the second place honest: an object silently replaced cannot pass as the provider's bytes, because
the stanza remembers what they hashed to.

One consequence is latent rather than current: re-canonicalisation is avoided, so a **DKIM
signature over a slimmed message still verifies once the part is restored** — but only if the
object is there. Nothing in the product verifies DKIM today (the `dkim` references in
`compose/techenrich.go` are DNS selector probes for tech enrichment, not signature checks), so this
costs nothing now and is written down because the next contact to reach for it should know.

## What is still owed

- **Orphaned-object reclamation.** `activities/capturedfiles.go` names it as owed by both writers of
  the `attachment` table. This sweep does not change that: it removes a copy from the database and
  never touches an object.
- **`raw_capture` still has no retention sweep of its own.** It ages out only via the **activity**
  sweep joined on `(source_system, source_id)`, plus Art. 17 contact erasure. Slimming reduces the
  slope of its growth by roughly twentyfold; it does not make it bounded.
- **The ten-in-forty miss rate.** A repeated inline logo is never slimmed. Fixing it needs an
  identity for a part that survives duplication — an ordinal resolved against the MIME structure
  rather than against the bytes — and that is the design this deliberately avoided. It is worth
  revisiting only if small repeated images ever become a material share of the table.
