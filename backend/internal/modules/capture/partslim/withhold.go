// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim

import (
	"bytes"
	"encoding/base64"
	"errors"
	"sort"
	"strconv"
)

// PartWithheldHeader marks bytes capture never kept: one part (`part:N`), or
// the whole message body (`all`) when no part could be cut out alone. Not under
// partFieldPrefix: a restore reads that run as a stanza to fetch, and there is
// nothing to fetch here.
const PartWithheldHeader = "X-Margince-Withheld-Part"

// partWithheldNotice is what a withheld part's substitute body decodes to.
const partWithheldNotice = "This part was not kept: the message is private to its " +
	"mailbox owner, so Margince stored its name, size and type only.\r\n"

// WithheldPart is one part capture will not keep: its ordinal, so the marker
// names the part the message numbered, and its decoded bytes, so it can be
// located in the original.
type WithheldPart struct {
	Ordinal int
	Body    []byte
}

// WithholdParts removes each part's encoded bytes from a stored original, and
// reports whether every one was found.
//
// When one cannot be located exactly once, the whole message is cut to its top
// header block instead. Minimisation is the point of withholding, so a
// part left in place because the locator gave up would defeat it; losing the
// stored body text costs nothing the activity row does not already hold.
func WithholdParts(raw []byte, parts []WithheldPart) ([]byte, bool) {
	if len(parts) == 0 {
		return raw, true
	}
	splices := make([]splice, 0, len(parts))
	for _, part := range parts {
		s, err := withholdSplice(raw, part.Ordinal, part.Body)
		if err != nil {
			return HeadersOnly(raw), false
		}
		splices = append(splices, s)
	}
	ordered, err := orderSplices(splices)
	if err != nil {
		return HeadersOnly(raw), false
	}
	out := applySplices(raw, ordered)
	// A part sent 7bit or 8bit is in the original verbatim, and a base64 copy
	// of it elsewhere could have been the one located. Its bytes surviving the
	// splice say so.
	for _, part := range parts {
		if len(part.Body) > 0 && bytes.Contains(out, part.Body) {
			return HeadersOnly(raw), false
		}
	}
	return out, true
}

func withholdSplice(raw []byte, ordinal int, body []byte) (splice, error) {
	encoded, _, eol, err := locateEncoded(raw, body)
	if err != nil {
		return splice{}, err
	}
	at := bytes.Index(raw, encoded)
	header, err := headerBlockBefore(raw, at)
	if err != nil {
		return splice{}, err
	}
	if !declaresBase64(raw[:header.fieldsEnd]) {
		return splice{}, ErrPartNotLocated
	}
	field := concat([]byte(PartWithheldHeader+": "+PartIdentity(ordinal)+"; bytes="+
		strconv.Itoa(len(body))), header.eol)
	return splice{
		start: header.fieldsEnd,
		end:   at + len(encoded),
		with: concat(field, header.eol,
			wrapBase64WithEOL([]byte(base64.StdEncoding.EncodeToString([]byte(partWithheldNotice))),
				base64WrapWidth, eol)),
	}, nil
}

// declaresBase64 reports whether the MIME header block ending the given bytes
// says its body is base64. The block runs from the part's boundary line, so a
// header written into an earlier part's text does not count.
func declaresBase64(upToFieldsEnd []byte) bool {
	start := bytes.LastIndex(upToFieldsEnd, []byte("\n--"))
	block := upToFieldsEnd[start+1:]
	for line := range bytes.SplitSeq(block, []byte("\n")) {
		name, value, found := bytes.Cut(bytes.TrimSpace(line), []byte(":"))
		if found && bytes.EqualFold(bytes.TrimSpace(name), []byte("Content-Transfer-Encoding")) {
			return bytes.EqualFold(bytes.TrimSpace(value), []byte("base64"))
		}
	}
	return false
}

// errSplicesOverlap says two parts resolved to one region, so one was located
// wrongly and splicing both would corrupt the message.
var errSplicesOverlap = errors.New("partslim: two parts overlap in the original")

func orderSplices(splices []splice) ([]splice, error) {
	ordered := append([]splice(nil), splices...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].start < ordered[j].start })
	for i := 1; i < len(ordered); i++ {
		if ordered[i].start < ordered[i-1].end {
			return nil, errSplicesOverlap
		}
	}
	return ordered, nil
}

// HeadersOnly is the message's top header block and the blank line ending it.
// A message with no blank line cannot be told header from body, so it keeps
// only the withheld marker.
func HeadersOnly(raw []byte) []byte {
	end := -1
	for _, sep := range [][]byte{[]byte("\r\n\r\n"), []byte("\n\n")} {
		if at := bytes.Index(raw, sep); at >= 0 && (end < 0 || at+len(sep) < end) {
			end = at + len(sep)
		}
	}
	if end < 0 {
		return []byte(PartWithheldHeader + ": all\r\n\r\n")
	}
	return append([]byte(nil), raw[:end]...)
}
