// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"encoding/base64"
	"encoding/json"
)

// EncodeOpaque and DecodeOpaque are the one envelope a keyset cursor travels
// in: base64url of the position as JSON.
//
// Held by: TestTheCursorEnvelopeIsSpelledOnce (backend/onecursorenvelope_test.go)
//
// Four modules wrote their own, and the KEYSETS differing is the whole reason
// they did — a dedupe queue pages by confidence, a lead queue by SLA band, a
// search by relevance score. Those are genuinely four positions. The envelope
// around them is not four things: it is base64url, JSON, and one error for "the
// client sent a token we did not mint", written four times with three different
// error types between them.
//
// The generic parameter is what lets the keyset stay each module's own while
// the envelope stops being. A caller declares its position type, and validating
// that a decoded position is a POSITION — not merely well-formed JSON — stays
// where the knowledge is: `null` and `{}` both unmarshal without error and
// leave every field at its zero value, which reads as a real position at the
// top of the list rather than as a refusal, and only the module knows which
// field being zero is the tell.
//
// The error is always MalformedCursorError, never a wrap. The token is
// client-supplied input on every surface that pages a store, so a token that
// does not decode is the caller's mistake and httperr must be able to answer
// 422 rather than falling through to a 500 and sending an admin looking for an
// outage that is not there. The base64 or JSON cause is deliberately dropped:
// it tells the client nothing it can act on beyond "not one we minted".

// EncodeOpaque renders a position as an opaque continuation token.
//
// It cannot fail. A cursor is a fixed-shape struct of scalars, which is the
// case json.Marshal has no error for — the shapes that can fail (channels,
// functions, NaN) are not positions, and a caller that reached for one would
// not compile past the constraint below.
func EncodeOpaque[T any](position T) string {
	raw, _ := json.Marshal(position) //nolint:errchkjson // a fixed-shape position of scalars has no failing case
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeOpaque reads a token back, or refuses it as the client's mistake.
//
// A well-formed token is not yet a valid position — see the package note above
// — so callers check the field that must be set before trusting the result.
func DecodeOpaque[T any](token string) (T, error) {
	var position T
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return position, &MalformedCursorError{}
	}
	if err := json.Unmarshal(raw, &position); err != nil {
		var zero T
		return zero, &MalformedCursorError{}
	}
	return position, nil
}
