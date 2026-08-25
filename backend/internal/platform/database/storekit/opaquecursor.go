// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
// It returns an error, and the error is reachable — which is not obvious and is
// why it is written down here. A position that carries a time.Time cannot be
// marshalled if that instant falls outside year 0..9999, and Postgres
// timestamps reach year 294276: a row with an absurd but storable created_at
// makes a keyset built from it unencodable. Swallowing that would hand the
// caller an empty token beside HasMore = true, which is a page the client can
// ask for and never receive.
func EncodeOpaque[T any](position T) (string, error) {
	raw, err := json.Marshal(position)
	if err != nil {
		return "", fmt.Errorf("store: rendering a continuation token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
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
