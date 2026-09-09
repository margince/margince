// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim

// Putting a stored part's encoded body back where the stanza says it came from.
//
// The inverse of partslim.go, and byte-exact by the same means: the stanza
// records the ORIGINAL encoded body's digest, length and wrap width, so the
// re-encoding is compared against what was removed rather than assumed to match
// it. A restore that cannot reproduce those bytes fails and says so — handing
// back a message that merely resembles the provider's would forfeit the one
// claim raw_capture exists to make.
//
// Removal is exact for the same reason the strip was: the fields the strip
// appended are a contiguous run of X-Margince-Part-* lines immediately before
// the header block's blank line, so taking that run out restores the block the
// provider sent, field for field and in order.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// storedStanza is one slimmed part found in a message.
type storedStanza struct {
	// fieldsStart and fieldsEnd bound the appended run of X- fields.
	fieldsStart, fieldsEnd int
	// bodyStart and bodyEnd bound the substitute body.
	bodyStart, bodyEnd int
	ref                PartRef
	encodedSha256      string
	encodedBytes       int
	wrap               int
}

// PartRef is what a restore needs to fetch one part's octets.
type PartRef struct {
	Ordinal    int
	Sha256     string
	Bytes      int64
	StorageKey string
}

// IsSlimmed reports whether a payload carries any reference stanza. Cheap
// enough to call before the work of a restore, and it is the test a caller uses
// to leave an unslimmed original entirely untouched.
func IsSlimmed(raw []byte) bool {
	return bytes.Contains(raw, []byte(PartStoredHeader+":"))
}

// RestoreStoredParts rebuilds the provider's original from a slimmed one.
//
// fetch is called once per stanza with the reference it names. A payload
// carrying no stanza comes back unchanged, so this is safe to call on every row
// rather than only the slimmed ones.
func RestoreStoredParts(raw []byte, fetch func(PartRef) ([]byte, error)) ([]byte, error) {
	stanzas, err := findStanzas(raw)
	if err != nil {
		return nil, err
	}
	if len(stanzas) == 0 {
		return raw, nil
	}
	splices := make([]splice, 0, len(stanzas)*2)
	for _, stanza := range stanzas {
		body, err := fetch(stanza.ref)
		if err != nil {
			return nil, fmt.Errorf("partslim: fetching %s: %w",
				PartIdentity(stanza.ref.Ordinal), err)
		}
		encoded, err := reencode(stanza, body)
		if err != nil {
			return nil, err
		}
		// Two regions per stanza: the appended fields come out, and the
		// substitute body is replaced by the original encoding.
		splices = append(splices,
			splice{start: stanza.fieldsStart, end: stanza.fieldsEnd, with: nil},
			splice{start: stanza.bodyStart, end: stanza.bodyEnd, with: encoded})
	}
	return applySplices(raw, splices), nil
}

// reencode rebuilds the removed encoded body and proves it is the one that was
// removed.
func reencode(stanza storedStanza, body []byte) ([]byte, error) {
	name := PartIdentity(stanza.ref.Ordinal)
	if int64(len(body)) != stanza.ref.Bytes {
		return nil, fmt.Errorf("partslim: %s is %d bytes, the stanza says %d",
			name, len(body), stanza.ref.Bytes)
	}
	if got := sha256Hex(body); got != stanza.ref.Sha256 {
		return nil, fmt.Errorf("partslim: %s hashes to %s, the stanza says %s",
			name, got, stanza.ref.Sha256)
	}
	encoded := wrapBase64WithEOL(
		[]byte(base64.StdEncoding.EncodeToString(body)), stanza.wrap, []byte("\r\n"))
	if len(encoded) != stanza.encodedBytes {
		return nil, fmt.Errorf("partslim: %s re-encodes to %d bytes, the stanza says %d",
			name, len(encoded), stanza.encodedBytes)
	}
	if got := sha256Hex(encoded); got != stanza.encodedSha256 {
		return nil, fmt.Errorf("partslim: %s re-encodes to digest %s, the stanza says %s",
			name, got, stanza.encodedSha256)
	}
	return encoded, nil
}

// findStanzas locates every reference stanza in raw.
func findStanzas(raw []byte) ([]storedStanza, error) {
	var out []storedStanza
	marker := []byte(PartStoredHeader + ":")
	for at := 0; ; {
		found := bytes.Index(raw[at:], marker)
		if found < 0 {
			return out, nil
		}
		found += at
		stanza, err := readStanza(raw, found)
		if err != nil {
			return nil, err
		}
		out = append(out, stanza)
		at = stanza.bodyEnd
	}
}

// readStanza reads one stanza whose first field begins at the line containing
// markerAt.
func readStanza(raw []byte, markerAt int) (storedStanza, error) {
	fieldsStart := lineStart(raw, markerAt)
	fieldsEnd := fieldsStart
	fields := map[string]string{}
	for fieldsEnd < len(raw) {
		lineEnd, next := lineBounds(raw, fieldsEnd)
		line := raw[fieldsEnd:lineEnd]
		if !bytes.HasPrefix(line, []byte(partFieldPrefix)) {
			break
		}
		name, value, ok := splitField(string(line))
		if !ok {
			return storedStanza{}, fmt.Errorf("partslim: unreadable stanza field %q", line)
		}
		fields[name] = value
		fieldsEnd = next
	}
	bodyStart, err := blankLineEnd(raw, fieldsEnd)
	if err != nil {
		return storedStanza{}, err
	}
	stanza, err := stanzaFromFields(fields)
	if err != nil {
		return storedStanza{}, err
	}
	stanza.fieldsStart, stanza.fieldsEnd = fieldsStart, fieldsEnd
	stanza.bodyStart = bodyStart
	stanza.bodyEnd = bodyStart + stanza.substituteLen()
	if stanza.bodyEnd > len(raw) {
		return storedStanza{}, fmt.Errorf("partslim: the substitute body of %s runs past the message",
			PartIdentity(stanza.ref.Ordinal))
	}
	return stanza, nil
}

// substituteLen is how many bytes the strip's substitute body occupies.
//
// Derived from the constant the strip wrote rather than restated as a number,
// so editing the notice moves both halves of it in one place.
func (s storedStanza) substituteLen() int {
	return len(wrapBase64([]byte(partStoredNotice), base64WrapWidth))
}

// stanzaFromFields turns the parsed fields into a stanza, refusing one that is
// missing anything a restore needs.
func stanzaFromFields(fields map[string]string) (storedStanza, error) {
	var s storedStanza
	ordinal, err := strconv.Atoi(strings.TrimPrefix(fields[PartStoredHeader], "part:"))
	if err != nil {
		return s, fmt.Errorf("partslim: the stanza's ordinal %q is unreadable: %w",
			fields[PartStoredHeader], err)
	}
	size, err := strconv.ParseInt(fields[PartBytesHeader], 10, 64)
	if err != nil {
		return s, fmt.Errorf("partslim: the stanza's byte count is unreadable: %w", err)
	}
	if s.encodedBytes, err = strconv.Atoi(fields[PartEncodedBytesHeader]); err != nil {
		return s, fmt.Errorf("partslim: the stanza's encoded byte count is unreadable: %w", err)
	}
	if s.wrap, err = strconv.Atoi(fields[PartWrapHeader]); err != nil {
		return s, fmt.Errorf("partslim: the stanza's wrap width is unreadable: %w", err)
	}
	s.encodedSha256 = fields[PartEncodedSha256Header]
	s.ref = PartRef{
		Ordinal:    ordinal,
		Sha256:     fields[PartSha256Header],
		Bytes:      size,
		StorageKey: fields[PartStorageKeyHeader],
	}
	if s.ref.Sha256 == "" || s.ref.StorageKey == "" || s.encodedSha256 == "" {
		return s, fmt.Errorf("partslim: the stanza for part:%d is missing a field it needs", ordinal)
	}
	return s, nil
}

// lineStart walks back to the first byte of the line containing at.
func lineStart(raw []byte, at int) int {
	for at > 0 && raw[at-1] != '\n' {
		at--
	}
	return at
}

// lineBounds returns where the line beginning at start ends (excluding its
// terminator) and where the next line begins.
func lineBounds(raw []byte, start int) (lineEnd, next int) {
	nl := bytes.IndexByte(raw[start:], '\n')
	if nl < 0 {
		return len(raw), len(raw)
	}
	next = start + nl + 1
	lineEnd = next - 1
	if lineEnd > start && raw[lineEnd-1] == '\r' {
		lineEnd--
	}
	return lineEnd, next
}

// blankLineEnd expects the header block's terminating blank line at at, and
// returns where the body begins.
func blankLineEnd(raw []byte, at int) (int, error) {
	if bytes.HasPrefix(raw[at:], []byte("\r\n")) {
		return at + 2, nil
	}
	if bytes.HasPrefix(raw[at:], []byte("\n")) {
		return at + 1, nil
	}
	return 0, fmt.Errorf("partslim: a stanza is not followed by the header's blank line")
}

// splitField splits "Name: value" without trimming the value's own spacing
// beyond the single separating space a field is written with.
func splitField(line string) (name, value string, ok bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", "", false
	}
	return line[:colon], strings.TrimSpace(line[colon+1:]), true
}
