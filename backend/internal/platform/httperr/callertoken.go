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
// Sized for a real identifier and nothing more, so a token that needs
// truncating is not a name anybody has. That every member of every closed
// vocabulary is inside it is not an assumption: compose's closed-set census
// asserts it, because a member longer than this would be truncated when a caller
// named it back and the refusal would quote a name matching nothing in the set
// beside it.
//
// BYTES, like MaxFaultText beside it. A rune bound would let a multi-byte
// token spend three times the budget for the same count, and what the figure
// protects is the length of the answer rather than how many characters a
// caller managed to fit in it.
const MaxCallerToken = 80

// QuoteCaller renders one caller-supplied name for a refusal: escaped, quoted,
// and bounded so the RENDERED token never exceeds MaxCallerToken.
//
// It escapes, and that is deliberate rather than incidental. `%q` renders the
// token as a Go string literal, so a newline inside a caller's name arrives as
// `\n` and not as what reads like a new line of conversation — which matters
// most on the tool surface, where the refusal lands in a transcript whose later
// prompts the same model reads, and where the name being quoted was written by
// that model. A refusal is not the place to discover that a caller chose a
// control character.
//
// TWO PLACES DECIDE WHAT IS SAFE TO ECHO, and the second is named here because
// spellings that drifted would mean the protection depended on which door the
// text came through. agents.echoSafe re-escapes whatever reaches the tool
// surface, by the same test (unicode.IsPrint) `%q` applies here — so a token
// quoted here passes through it unchanged, and one arriving by another route is
// still escaped. Bounding is the half that must happen HERE: echoSafe bounds the
// whole message, and by then a caller's token has already spent the budget the
// vocabulary needs.
//
// The bound is applied AFTER quoting, not before, because quoting is what
// decides the length: eighty bytes of newlines escape to a hundred and sixty,
// so a token bounded before it is rendered is not bounded at all. Cutting the
// rendered form costs a mangled escape at the seam in the pathological case,
// which is the right trade — the alternative is a refusal whose size a caller
// chooses.
func QuoteCaller(s string) string {
	quoted := fmt.Sprintf("%q", s)
	if len(quoted) <= MaxCallerToken {
		return quoted
	}
	// Room for the ellipsis and the closing quote the cut throws away, so the
	// answer is a quoted token a reader can see the end of.
	cut := MaxCallerToken - len("…\"")
	for cut > 0 && !utf8.RuneStart(quoted[cut]) {
		cut--
	}
	// A cut that lands just after a backslash would emit a dangling escape —
	// `"abc\` — which reads as a quoting bug in this function rather than as a
	// truncated name. Drop the orphan.
	for cut > 0 && trailingBackslashes(quoted[:cut])%2 == 1 {
		cut--
	}
	return quoted[:cut] + "…\""
}

// trailingBackslashes counts the unbroken run of backslashes at the end of s,
// which is what says whether a final one escapes the next byte or is itself
// escaped.
func trailingBackslashes(s string) int {
	n := 0
	for n < len(s) && s[len(s)-1-n] == '\\' {
		n++
	}
	return n
}
