// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"unicode/utf8"
)

// MaxCallerToken bounds ONE name a refusal quotes back from the caller's own
// request — a population that resolves to nothing, a block kind outside the
// grammar, a field no entity carries.
//
// It exists so the rest of a refusal does not have to compete with it. A
// refusal that names a closed set spends most of its length on the set, which
// is the half the caller acts on; the echoed token is the half that says which
// input was wrong, and a caller choosing a long one must not be able to push
// the set out of the answer. Bounding the echo at its SOURCE is what lets the
// surface downstream carry the set whole: the caller-influenced share of the
// message is a known quantity rather than whatever was sent.
//
// Sized for a real identifier and nothing more. Every name in every closed
// vocabulary this refuses against is comfortably inside it, so a token that
// needs truncating is not a name anybody has.
//
// BYTES, like MaxFaultText beside it. A rune bound would let a multi-byte
// token spend three times the budget for the same count, and what the figure
// protects is the length of the answer rather than how many characters a
// caller managed to fit in it.
const MaxCallerToken = 80

// QuoteCaller renders one caller-supplied name for a refusal: quoted, so an
// empty or space-padded token is still visible, and bounded at MaxCallerToken.
//
// It does NOT escape. A refusal crosses more than one boundary and each has its
// own rule about what a control character does there — a tool transcript needs
// them rendered visible, an HTTP body needs them JSON-encoded — so escaping
// belongs at the boundary that knows, and doing it here would double-escape on
// one of the two.
func QuoteCaller(s string) string {
	if len(s) > MaxCallerToken {
		cut := MaxCallerToken
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return fmt.Sprintf("%q", s)
}
