// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// raw_capture is the copy a re-parse or a dispute reads, so what it holds has
// to be what the provider sent. These pin that: every shape a provider can
// send comes back byte-identical, and the two the old spelling lost are named.

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// roundTrip is what a reader does with a stored payload, in the order
// compose's decodeStoredOriginal does it.
func roundTrip(t *testing.T, stored []byte) []byte {
	t.Helper()
	var envelope rawCaptureEnvelope
	if err := json.Unmarshal(stored, &envelope); err == nil && envelope.Encoding == RawCaptureBase64Encoding {
		raw, err := base64.StdEncoding.DecodeString(envelope.Data)
		if err != nil {
			t.Fatalf("decoding the envelope: %v", err)
		}
		return raw
	}
	if !strings.HasPrefix(strings.TrimSpace(string(stored)), `"`) {
		return stored
	}
	var text string
	if err := json.Unmarshal(stored, &text); err != nil {
		t.Fatalf("unwrapping the stored string: %v", err)
	}
	return []byte(text)
}

func TestARawCapturePayloadComesBackByteIdentical(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"a JSON resource, stored as itself", []byte(`{"id":"evt_1","summary":"Kickoff"}`)},
		{"an RFC822 message, stored as a JSON string", []byte("From: a@b.test\r\nSubject: Hallo\r\n\r\nGruesse")},
		// The two the old spelling lost. A body in a non-UTF-8 charset came
		// back with U+FFFD where its bytes had been; a NUL never came back at
		// all, because jsonb refused the row and took the whole capture with it.
		{"a body that is not valid UTF-8", []byte("Subject: Gr\xfc\xdfe\r\n\r\nlatin-1 \xff\xfe bytes")},
		{"a body carrying a NUL", []byte("Subject: quarterly update\r\n\r\nHello\x00World")},
		{"JSON whose own string escapes a NUL", []byte(`{"body":"Hello\u0000World"}`)},
		// The lookalike: an escaped BACKSLASH followed by the text u0000. It
		// decodes to a backslash and five characters, carries no NUL, and jsonb
		// takes it — so it must stay stored as itself rather than be sent down
		// the base64 path by a search that cannot tell the two apart.
		{"JSON carrying the TEXT of a NUL escape", []byte(`{"body":"Hello\\u0000World"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored, err := rawCapturePayload(tc.raw)
			if err != nil {
				t.Fatalf("storing: %v", err)
			}
			// Whatever shape it took, jsonb has to accept it: a payload
			// carrying a NUL escape is refused by the column, and the
			// connectors treat that refusal as fatal to the whole pull.
			if !json.Valid(stored) {
				t.Fatalf("the stored payload is not JSON at all: %q", stored)
			}
			// What jsonb refuses is a NUL in a decoded string, not the
			// characters that spell one — a payload that escapes its own
			// backslash carries the text and no NUL, and is accepted.
			var decoded any
			if err := json.Unmarshal(stored, &decoded); err != nil {
				t.Fatalf("the stored payload does not decode: %q", stored)
			}
			if carriesNUL(decoded) {
				t.Errorf("the stored payload decodes to a string carrying a NUL, which jsonb refuses: %q", stored)
			}
			if got := roundTrip(t, stored); string(got) != string(tc.raw) {
				t.Errorf("read back %q, want the provider's own %q", got, tc.raw)
			}
		})
	}
}

// The common shapes keep the spelling every existing row uses, so this is a fix
// for the broken case rather than a rewrite of the table.
func TestTheUnbrokenShapesKeepTheirOldSpelling(t *testing.T) {
	resource := []byte(`{"id":"evt_1"}`)
	if stored, err := rawCapturePayload(resource); err != nil || string(stored) != string(resource) {
		t.Errorf("a JSON resource stored as %q (err %v), want it unchanged", stored, err)
	}
	text := []byte("From: a@b.test\r\n\r\nHallo")
	stored, err := rawCapturePayload(text)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(stored), `"`) {
		t.Errorf("text stored as %q, want the JSON-string spelling", stored)
	}
}

// carriesNUL walks a decoded payload for the one thing jsonb will not store:
// a NUL inside a string.
//
//craft:ignore naked-any a decoded JSON document has no shape to name - this walks whatever the provider sent, which is the point
func carriesNUL(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.ContainsRune(t, 0)
	case map[string]any:
		for _, e := range t {
			if carriesNUL(e) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if carriesNUL(e) {
				return true
			}
		}
	}
	return false
}

// The lookalike above decides between two spellings, so it gets its own
// assertion rather than only riding the round-trip.
func TestTheTextOfANULEscapeIsStoredAsJSON(t *testing.T) {
	lookalike := []byte(`{"body":"Hello\\u0000World"}`)
	stored, err := rawCapturePayload(lookalike)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(lookalike) {
		t.Errorf("stored as %q, want the document unchanged — it carries the TEXT of an escape, not a NUL", stored)
	}
	escaped := []byte(`{"body":"Hello\u0000World"}`)
	if stored, err := rawCapturePayload(escaped); err != nil || string(stored) == string(escaped) {
		t.Errorf("a real NUL escape stored as %q (err %v), want it off the as-itself path", stored, err)
	}
}
