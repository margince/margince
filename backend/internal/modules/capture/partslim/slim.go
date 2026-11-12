// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package partslim replaces a stored attachment's encoded body inside a
// provider original with a reference to the object holding those bytes, and
// puts them back byte-exactly on demand.
//
// The bytes of an attachment are already durable in the object store before
// this runs — capture/sinkparts.go stages them, activities/capturedfiles.go
// puts them — so the copy inside the provider original is a second copy of the
// same octets, base64-inflated and incompressible.
//
// LOCATED BY ENCODING, NEVER BY PARSING. The caller supplies the verified
// octets; this encodes them and finds that byte string in the message. So the
// only bytes ever removed are bytes positively identified as the encoding of an
// object whose digest already matched — a message this cannot locate exactly
// once is left alone rather than guessed at.
//
// That is why there is no MIME walk here. Reading the message would mean
// decoding it, and the decoders alter what they read: charsets are converted to
// UTF-8, base64 is re-wrapped at the writer's own width, and a body in a
// charset the library does not know fails outright. sinkraw.go's own comment
// records that class of corruption as a defect already paid for once; a sweep
// that reintroduced it would be a worse bug than the disk it reclaims.
//
// The header block keeps every field it had, in order. The stanza's fields are
// APPENDED as a contiguous run immediately before the block's terminating blank
// line, and Content-Transfer-Encoding is deliberately left in place: the
// substitute body is itself valid base64, so the slimmed message still decodes,
// and a restore removes exactly the run it added.
package partslim

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// The stanza's fields. A reader identifies a slimmed part by the first one
// alone; the rest are what a restore needs to prove it put back what was taken.
const (
	PartStoredHeader        = "X-Margince-Part-Stored"
	PartSha256Header        = "X-Margince-Part-Sha256"
	PartBytesHeader         = "X-Margince-Part-Bytes"
	PartStorageKeyHeader    = "X-Margince-Part-Storage-Key"
	PartEncodedSha256Header = "X-Margince-Part-Encoded-Sha256"
	PartEncodedBytesHeader  = "X-Margince-Part-Encoded-Bytes"
	PartWrapHeader          = "X-Margince-Part-Wrap"
	// PartWrapEOLHeader names the line terminator the original encoded body
	// used. Recorded because locateEncoded matches EITHER terminator, so a
	// stanza carrying only the width leaves a restore to guess — and a bare-LF
	// body rebuilt with CRLF is one byte longer per line, which fails its own
	// digest check and makes that message unreproducible for good.
	PartWrapEOLHeader = "X-Margince-Part-Wrap-EOL"
)

// The terminator spellings the stanza uses. A header value cannot carry a
// literal CR or LF, so the two are named rather than written.
const (
	wrapEOLCRLF = "crlf"
	wrapEOLLF   = "lf"
)

// eolBytes turns a stanza's terminator name back into the bytes it stands for.
// An unnamed terminator is CRLF: it is what RFC 2045 requires and what every
// message in this corpus uses, so it is the honest default for a stanza written
// before this field existed.
func eolBytes(name string) []byte {
	if name == wrapEOLLF {
		return []byte("\n")
	}
	return []byte("\r\n")
}

// eolName is eolBytes' inverse, for the strip.
func eolName(eol []byte) string {
	if string(eol) == "\n" {
		return wrapEOLLF
	}
	return wrapEOLCRLF
}

// partFieldPrefix is what every field the stanza adds begins with, and is how a
// restore finds exactly the run to remove.
const partFieldPrefix = "X-Margince-Part-"

// partStoredNotice is what the substitute body decodes to. Encoded rather than
// written plainly because Content-Transfer-Encoding stays as the provider set
// it: a plaintext body under a base64 header would decode to noise, and a
// reader that decoded the slimmed message would see garbage instead of an
// explanation.
const partStoredNotice = "The bytes of this part are held in the object store, " +
	"not in this record. The X-Margince-Part-* fields above name the object, " +
	"its length and its digest.\r\n"

// base64WrapWidth is the line length a slimmed part's substitute body is
// wrapped at, and the first width tried when locating an original.
//
// 76 is RFC 2045's own limit and what every provider in this corpus emits.
// In a sample of forty attachments, thirty located at 76 with CRLF and
// restored byte for byte. The other ten were one logo repeated two to four
// times down a quoted thread, one part per copy. splicesFor cuts those when
// the copies and the parts agree in number.
//
// The other widths exist because "every provider we have seen" is not "every
// provider".
const base64WrapWidth = 76

// candidateWraps are the line lengths tried when locating an encoded body, in
// descending order of how often they occur. A width of 0 means unwrapped — one
// line, however long.
var candidateWraps = []int{base64WrapWidth, 72, 64, 998, 0}

// ErrPartNotLocated says the encoded body was not found once per part that
// carries it, so nothing was removed. A width this does not try, or a file
// whose bytes also appear outside its parts, lands here and is left alone.
var ErrPartNotLocated = errors.New("partslim: the part's encoded body was not located once per part")

// StoredPart is one part whose bytes are proved durable, with those bytes.
type StoredPart struct {
	// Ordinal is parts.go's ordinal, carried through so the stanza names the
	// part the attachment row names. Nothing here derives it — the caller reads
	// it from attachment.external_part_id, which is where it was written.
	Ordinal int
	// Sha256 and Bytes describe the DECODED octets, matching
	// attachment.checksum and attachment.byte_size.
	Sha256 string
	Bytes  int64
	// StorageKey addresses the object holding them.
	StorageKey string
	// Body is the verified octets themselves. Supplied rather than fetched
	// because the caller has already read them to prove the object matches its
	// row, and this locates the part by encoding them.
	Body []byte
}

// splice is one region of the original to replace, and what replaces it.
type splice struct {
	start, end int
	with       []byte
}

// StripStoredParts replaces each part's encoded body with a reference stanza,
// and reports how many parts it removed.
//
// A part whose encoded body cannot be located exactly once is skipped, not
// failed: the rest of the message is still worth slimming, and the count tells
// the caller how much of what it asked for actually happened.
func StripStoredParts(raw []byte, parts []StoredPart) ([]byte, int, error) {
	if len(parts) == 0 {
		return raw, 0, nil
	}
	splices := make([]splice, 0, len(parts))
	for _, copies := range partsByBytes(parts) {
		located, err := splicesFor(raw, copies)
		if errors.Is(err, ErrPartNotLocated) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		splices = append(splices, located...)
	}
	if len(splices) == 0 {
		return raw, 0, nil
	}
	sort.Slice(splices, func(i, j int) bool { return splices[i].start < splices[j].start })
	for i := 1; i < len(splices); i++ {
		if splices[i].start < splices[i-1].end {
			// Two parts resolving to overlapping regions means one of them was
			// located wrongly, and splicing both would corrupt the message.
			return nil, 0, errors.New("partslim: two stored parts overlap in the original")
		}
	}
	return applySplices(raw, splices), len(splices), nil
}

// partsByBytes groups the parts that carry identical bytes, each group in
// ordinal order, which is the order the parts appear in the message.
func partsByBytes(parts []StoredPart) [][]StoredPart {
	index := map[string]int{}
	var groups [][]StoredPart
	for _, part := range parts {
		at, seen := index[part.Sha256]
		if !seen {
			at = len(groups)
			index[part.Sha256] = at
			groups = append(groups, nil)
		}
		groups[at] = append(groups[at], part)
	}
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i].Ordinal < group[j].Ordinal })
	}
	return groups
}

// splicesFor locates the parts that carry one set of bytes and describes the
// replacement for each. k parts with the same bytes are cut only when their
// encoding occurs k times, and the occurrences go to them in order. A further
// copy, such as the file quoted as text, leaves them all in place.
func splicesFor(raw []byte, copies []StoredPart) ([]splice, error) {
	for _, part := range copies {
		if got := sha256Hex(part.Body); got != part.Sha256 {
			return nil, fmt.Errorf("partslim: part:%d hashes to %s, its row says %s",
				part.Ordinal, got, part.Sha256)
		}
	}
	encoded, width, eol, err := locateEncodedTimes(raw, copies[0].Body, len(copies))
	if err != nil {
		return nil, err
	}
	splices := make([]splice, 0, len(copies))
	from := 0
	for _, part := range copies {
		at := from + bytes.Index(raw[from:], encoded)
		header, err := headerBlockBefore(raw, at)
		if err != nil {
			return nil, err
		}
		if !declaresBase64(raw[:header.fieldsEnd]) {
			// A quoted text body holding the same encoding would otherwise be
			// cut in the place of a real part.
			return nil, ErrPartNotLocated
		}
		stanza := stanzaFields(part, encoded, width, eol, header.eol)
		splices = append(splices, splice{
			start: header.fieldsEnd,
			end:   at + len(encoded),
			// The region becomes the new fields, the header block's blank line
			// in that block's terminator, then the substitute body. The body
			// keeps the terminator the removed body used.
			with: concat(stanza, header.eol,
				wrapBase64WithEOL([]byte(base64.StdEncoding.EncodeToString([]byte(partStoredNotice))),
					base64WrapWidth, eol)),
		})
		from = at + len(encoded)
	}
	return splices, nil
}

// locateEncoded finds the one encoding of body that appears in raw once,
// returning it, the width it was wrapped at, and the terminator it used.
//
// The terminator is returned rather than assumed because both are tried: a
// restore that rebuilt an LF-wrapped body with CRLF would produce different
// bytes and fail the digest this same call recorded.
func locateEncoded(raw, body []byte) ([]byte, int, []byte, error) {
	return locateEncodedTimes(raw, body, 1)
}

// locateEncodedTimes finds the encoding of body that appears in raw `times`
// times, no more and no fewer.
func locateEncodedTimes(raw, body []byte, times int) ([]byte, int, []byte, error) {
	b64 := make([]byte, base64.StdEncoding.EncodedLen(len(body)))
	base64.StdEncoding.Encode(b64, body)
	// Sized for the narrowest candidate with CRLF, so no candidate grows it.
	folded := make([]byte, 0, len(b64)+2*(len(b64)/64+1))
	for _, width := range candidateWraps {
		for _, eol := range [][]byte{[]byte("\r\n"), []byte("\n")} {
			// One buffer serves every candidate, so a large attachment is held
			// twice while it is located, not once per width tried.
			folded = foldInto(folded[:0], b64, width, eol)
			if bytes.Count(raw, folded) == times {
				at := bytes.Index(raw, folded)
				return raw[at : at+len(folded)], width, eol, nil
			}
		}
	}
	return nil, 0, nil, ErrPartNotLocated
}

// foldInto appends b64 folded as wrapBase64WithEOL folds it to dst.
func foldInto(dst, b64 []byte, width int, eol []byte) []byte {
	if width <= 0 || len(b64) <= width {
		return append(dst, b64...)
	}
	for at := 0; at < len(b64); at += width {
		if at > 0 {
			dst = append(dst, eol...)
		}
		dst = append(dst, b64[at:min(at+width, len(b64))]...)
	}
	return dst
}

// headerSpan says where a part's header fields end, just before the blank line
// that terminates them.
type headerSpan struct {
	fieldsEnd int
	// eol is the terminator the header block's own lines use. The stanza's
	// fields and the blank line after them are written with it, so a bare-LF
	// message does not come back one byte longer per line than it went in.
	eol []byte
}

// headerBlockBefore locates the header block whose body begins at bodyAt.
//
// A MIME body begins immediately after its header's terminating blank line, so
// the four bytes before it are CRLFCRLF (or two, for a message using bare LF).
// A body that does not sit behind one is not a body this can reason about, and
// the part is refused rather than spliced at a guess.
func headerBlockBefore(raw []byte, bodyAt int) (headerSpan, error) {
	if bodyAt >= 4 && bytes.Equal(raw[bodyAt-4:bodyAt], []byte("\r\n\r\n")) {
		return headerSpan{fieldsEnd: bodyAt - 2, eol: []byte("\r\n")}, nil
	}
	if bodyAt >= 2 && bytes.Equal(raw[bodyAt-2:bodyAt], []byte("\n\n")) {
		return headerSpan{fieldsEnd: bodyAt - 1, eol: []byte("\n")}, nil
	}
	return headerSpan{}, fmt.Errorf(
		"partslim: the located body at %d does not follow a header's blank line", bodyAt)
}

// stanzaFields renders the fields appended to the part's header block. Each
// ends in CRLF, so the run splices in directly before the blank line.
func stanzaFields(part StoredPart, encoded []byte, width int, eol, fieldEOL []byte) []byte {
	var out bytes.Buffer
	field := func(name, value string) {
		out.WriteString(name)
		out.WriteString(": ")
		out.WriteString(value)
		out.Write(fieldEOL)
	}
	field(PartStoredHeader, PartIdentity(part.Ordinal))
	field(PartSha256Header, part.Sha256)
	field(PartBytesHeader, strconv.FormatInt(part.Bytes, 10))
	field(PartStorageKeyHeader, part.StorageKey)
	field(PartEncodedSha256Header, sha256Hex(encoded))
	field(PartEncodedBytesHeader, strconv.Itoa(len(encoded)))
	field(PartWrapHeader, strconv.Itoa(width))
	field(PartWrapEOLHeader, eolName(eol))
	return out.Bytes()
}

// applySplices rebuilds the message with each region replaced. The splices are
// ordered and non-overlapping, so one pass over the original suffices.
func applySplices(raw []byte, splices []splice) []byte {
	var out bytes.Buffer
	out.Grow(len(raw))
	at := 0
	for _, s := range splices {
		out.Write(raw[at:s.start])
		out.Write(s.with)
		at = s.end
	}
	out.Write(raw[at:])
	return out.Bytes()
}

// PartIdentity spells an ordinal the way sinkparts.partIdentity does, so the
// stanza and attachment.external_part_id carry the same string.
func PartIdentity(ordinal int) string { return "part:" + strconv.Itoa(ordinal) }

// wrapBase64WithEOL folds an already-encoded run at width. A width of 0 leaves
// it on one line.
func wrapBase64WithEOL(b64 []byte, width int, eol []byte) []byte {
	if width <= 0 || len(b64) <= width {
		return b64
	}
	var out bytes.Buffer
	out.Grow(len(b64) + len(eol)*(len(b64)/width+1))
	for at := 0; at < len(b64); at += width {
		if at > 0 {
			out.Write(eol)
		}
		end := min(at+width, len(b64))
		out.Write(b64[at:end])
	}
	return out.Bytes()
}

// concat joins runs without the caller spelling out an append chain.
func concat(runs ...[]byte) []byte {
	size := 0
	for _, run := range runs {
		size += len(run)
	}
	out := make([]byte, 0, size)
	for _, run := range runs {
		out = append(out, run...)
	}
	return out
}

// sha256Hex is the digest spelling attachment.checksum uses.
func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Sha256HexForTest exposes the digest spelling to this package's tests without
// making the helper part of the package's surface for anyone else.
func Sha256HexForTest(body []byte) string { return sha256Hex(body) }
