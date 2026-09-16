// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Where an attachment's ENCODED bytes sit in a marshalled request body, so the
// secret rules can be kept out of them.
//
// The stripper runs over the marshalled body at the last moment before egress,
// which is what makes it unbypassable and is deliberate. An attachment rides
// that body base64-encoded, and two of the rules are alphanumeric enough to
// match inside a blob by coincidence: a run of encoded bytes that happens to
// spell `AKIA` followed by sixteen uppercase characters was replaced with
// `[SECRET-REMOVED:aws_access_key]`, which injects characters base64 has no
// alphabet for into the middle of a file. The attachment reaches the vendor
// undecodable, the error comes back as a provider-side complaint about a
// malformed image, the call looks ordinary in the trace, and the reading is
// charged and lost.
//
// STRUCTURE, not pattern. Narrowing the two regexes was the smaller change and
// it fixes two instances of a class — "an alphanumeric-heavy pattern matched
// against encoded bytes" — that the next rule added walks straight back into.
// The exclusion is by POSITION in the document the adapter built, so a rule
// added tomorrow inherits it.
//
// And it is what the stripper's own scope statement already says. That comment
// is explicit that an attachment's bytes are base64 and nothing here can find a
// credential inside them: a pass that cannot see into an attachment should not
// be reaching into one, which is all of the risk and none of the benefit.

import "bytes"

// encodedKeys are the object keys whose STRING value is an attachment's
// encoded bytes, across the four wires this module speaks.
//
//   - `data`   — Anthropic's source.data and Gemini's inline_data.data.
//   - `images` — Ollama's array of bare base64 strings, whose ELEMENTS are the
//     blobs; the key names the array rather than a value, handled below.
//   - `url`    — OpenAI's image_url.url, but ONLY when it is a data: URL. A
//     plain https URL under the same key is ordinary text a credential can
//     legitimately hide in, so the value is tested rather than the key alone.
var encodedKeys = map[string]bool{"data": true, "images": true, "url": true}

// dataURLPrefix marks the one `url` value that carries bytes rather than an
// address.
var dataURLPrefix = []byte("data:")

// span is a half-open byte range of the payload.
type span struct{ from, to int }

// encodedSpans reports the ranges holding encoded attachment bytes, in order
// and non-overlapping.
//
// A minimal JSON scanner rather than a parser: it walks the bytes tracking only
// what it needs — whether it is inside a string, and the key most recently
// read — because the alternative is unmarshalling and re-marshalling the body,
// and the bytes that leave must be the bytes the adapter built. Re-marshalling
// would reorder keys and re-escape strings, which changes a request nobody
// asked to change.
//
// A body that is not JSON at all yields no spans, so the scan falls back to
// covering everything — the behaviour before this file, which is the safe
// direction: a rule that runs everywhere can corrupt an attachment, and one
// that runs nowhere misses a credential. Only the first is possible here
// because a non-JSON body carries no attachment.
func encodedSpans(payload []byte) []span {
	var spans []span
	var key []byte
	inArray := false
	for i := 0; i < len(payload); i++ {
		switch payload[i] {
		case '"':
			start := i
			i = stringEnd(payload, i)
			if i >= len(payload) {
				return spans
			}
			value := payload[start+1 : i]
			// A string is a KEY when the next non-space byte is a colon.
			if next := skipSpace(payload, i+1); next < len(payload) && payload[next] == ':' {
				key = value
				inArray = false
				continue
			}
			if encodedValue(key, value, inArray) {
				spans = append(spans, span{start + 1, i})
			}
		case '[':
			inArray = true
		case ']', '}', ',':
			// A value ended. The key no longer governs what follows, except
			// inside an array, where every element belongs to the same key.
			if payload[i] != ',' || !inArray {
				inArray = false
				key = nil
			}
		}
	}
	return spans
}

// encodedValue answers whether one string value holds encoded bytes.
func encodedValue(key, value []byte, inArray bool) bool {
	if !encodedKeys[string(key)] {
		return false
	}
	switch string(key) {
	case "images":
		// Ollama: the key names an array and the ELEMENTS are the blobs, so a
		// bare string under this key outside an array is not one.
		return inArray
	case "url":
		return bytes.HasPrefix(value, dataURLPrefix)
	default:
		return true
	}
}

// stringEnd is the index of the quote closing the string opened at `from`,
// honouring backslash escapes. It answers len(payload) for an unterminated
// string, which a marshalled body cannot contain.
func stringEnd(payload []byte, from int) int {
	for i := from + 1; i < len(payload); i++ {
		switch payload[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return len(payload)
}

// skipSpace is the index of the next byte that is not JSON whitespace.
func skipSpace(payload []byte, from int) int {
	for i := from; i < len(payload); i++ {
		switch payload[i] {
		case ' ', '\t', '\n', '\r':
		default:
			return i
		}
	}
	return len(payload)
}

// scannableSegments is the complement of the encoded spans: the parts of the
// body the rules may run over.
//
// Every boundary falls INSIDE a JSON string, between its opening quote and its
// content, which is what makes splitting safe: the rules are documented never
// to match across a double quote, so no match this splitting breaks could have
// existed in the whole body either.
func scannableSegments(payload []byte, spans []span) []span {
	segments := make([]span, 0, len(spans)+1)
	at := 0
	for _, s := range spans {
		if s.from > at {
			segments = append(segments, span{at, s.from})
		}
		at = s.to
	}
	if at < len(payload) {
		segments = append(segments, span{at, len(payload)})
	}
	return segments
}
