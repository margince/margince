// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// storeRawCapture appends the provider's original bytes under the natural
// key. Raw capture is EVIDENCE: append-once, and rewritten by exactly two
// sweeps, both of which NARROW what it holds rather than replace it. A replay
// carrying different bytes for the same natural key keeps the original —
// silently replacing provenance would gut lineage and forensic replay. A
// record that arrived with no original stores nothing.
//
// The two sanctioned rewrites. Retention's erase (privacy/retention.go) takes
// the whole payload once the activity's window closes. The part sweep
// (partslimstore.go) takes a stored attachment's OCTETS once it can prove them
// durable in the object store, leaving a stanza naming the object, its length
// and its digest — so what the column still promises afterwards is the message
// verbatim and its attachments by reference, and partslim.RestoreStoredParts
// rebuilds the provider's exact bytes or refuses to hand any back. What it no
// longer promises is that the column ALONE holds them.
//
// For mail that key is transport-independent, so the FIRST connector to deliver
// a message supplies the bytes on file and a second connector's copy of the
// same message is not stored. That is the append-once rule meeting one
// identity, not a new policy — but it does mean the stored original is one
// provider's rendering. Equal Message-IDs do not promise equal bytes: delivery
// headers differ per mailbox, and a Bcc survives only on the sender's copy.
func storeRawCapture(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	if len(rec.Raw) == 0 {
		return nil
	}
	payload, err := rawCapturePayload(rec.Raw)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO raw_capture (source_system, source_id, payload)
		VALUES ($1, $2, $3)
		ON CONFLICT (source_system, source_id) DO NOTHING`,
		rec.NaturalKey.SourceSystem, rec.NaturalKey.SourceID, payload); err != nil {
		return fmt.Errorf("capture: raw store: %w", err)
	}
	return nil
}

// RawCaptureBase64Encoding names the envelope rawCapturePayload uses for a
// provider original that jsonb cannot hold as text. It is exported so the one
// reader that unwraps a stored original reads the name rather than repeating
// the string.
const RawCaptureBase64Encoding = "base64"

// rawCaptureEnvelope carries bytes jsonb will not take, and says so. The
// encoding travels WITH the payload rather than being inferred, because a
// reader guessing at a blob is how a re-parse silently reads the wrong bytes.
type rawCaptureEnvelope struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

// rawCapturePayload turns a provider's original into something the jsonb
// column can hold without ALTERING a byte of it. raw_capture is the copy a
// re-parse or a dispute reads, so what it holds has to be what the provider
// sent — the one thing this table exists for.
//
// The old spelling was json.Marshal(string(raw)) for anything non-JSON, and it
// broke on the two shapes mail actually produces:
//
//   - Go's encoder replaces every invalid UTF-8 byte with U+FFFD, so a body in
//     a non-UTF-8 charset, or a malformed header, was stored ALTERED. The
//     insert succeeded and the row looked fine; only a byte comparison against
//     the provider would have shown it.
//   - A NUL becomes the escape \u0000, which jsonb refuses outright
//     (22P05). That one does not corrupt quietly — it fails the capture
//     transaction, and the connectors treat a non-skip error as fatal to the
//     whole pull.
//
// So: JSON that jsonb will take is stored as itself; text that is valid UTF-8
// and NUL-free keeps the JSON-string spelling every existing row uses; and
// anything else travels base64 in a self-describing envelope. Only the third
// case is new, and it is exactly the case that was being lost.
func rawCapturePayload(raw []byte) ([]byte, error) {
	if json.Valid(raw) && !jsonEscapesNUL(raw) {
		return raw, nil
	}
	if utf8.Valid(raw) && !bytes.ContainsRune(raw, 0) {
		return json.Marshal(string(raw))
	}
	return json.Marshal(rawCaptureEnvelope{
		Encoding: RawCaptureBase64Encoding,
		Data:     base64.StdEncoding.EncodeToString(raw),
	})
}

// jsonEscapesNUL reports whether a JSON document decodes to a string carrying a
// NUL. That escape is legal JSON and illegal jsonb, so such a document has to
// leave the as-itself path even though it parses.
//
// Parity, not substring. The bytes `\\u0000` are a valid JSON document that
// decodes to a BACKSLASH followed by the text u0000 — no NUL anywhere — and a
// plain search would send it down the base64 path for nothing. Only a `u0000`
// preceded by an ODD run of backslashes is an escape rather than the text of
// one, because the run's last backslash is what escapes the u.
func jsonEscapesNUL(raw []byte) bool {
	lower := bytes.ToLower(raw)
	for at := 0; ; {
		found := bytes.Index(lower[at:], []byte("u0000"))
		if found < 0 {
			return false
		}
		found += at
		slashes := 0
		for k := found - 1; k >= 0 && lower[k] == '\\'; k-- {
			slashes++
		}
		if slashes%2 == 1 {
			return true
		}
		at = found + len("u0000")
	}
}

// DecodeStoredOriginal unwraps what storeRawCapture put in raw_capture.payload,
// and is the exact inverse of rawCapturePayload above.
//
// It lives here rather than at each reader because the three spellings are this
// file's own invention: a JSON payload (a calendar event resource) stored as
// itself, text (an RFC822 message) as a JSON *string*, and bytes jsonb cannot
// hold as text in a base64 envelope naming its own encoding. A reader keeping
// its own copy of that list is a second answer to what the column means, and
// the copy is what falls behind when a fourth spelling arrives.
//
// The envelope is checked before the string case because it IS a JSON object,
// and it is checked by its DECLARED encoding rather than by shape, so a
// provider payload that happens to carry those two keys cannot be mistaken for
// one.
func DecodeStoredOriginal(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("capture: the stored original is empty")
	}
	var envelope rawCaptureEnvelope
	if err := json.Unmarshal(payload, &envelope); err == nil &&
		envelope.Encoding == RawCaptureBase64Encoding {
		raw, err := base64.StdEncoding.DecodeString(envelope.Data)
		if err != nil {
			return nil, fmt.Errorf("capture: decoding the stored original: %w", err)
		}
		return raw, nil
	}
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte(`"`)) {
		return payload, nil
	}
	var text string
	if err := json.Unmarshal(payload, &text); err != nil {
		return nil, fmt.Errorf("capture: unwrapping the stored original: %w", err)
	}
	return []byte(text), nil
}

// EncodeStoredOriginal is rawCapturePayload under a name another package can
// reach. The part sweep needs it: what it writes back has to be spelled the way
// the sink would have spelled it, or the next reader decodes something the
// column does not hold.
func EncodeStoredOriginal(raw []byte) ([]byte, error) { return rawCapturePayload(raw) }
