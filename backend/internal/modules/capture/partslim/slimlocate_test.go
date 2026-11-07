// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// soleWrappedAt answers what counting the folded text answers, without
// building it: found once where bytes.Count finds it once, nowhere otherwise.
func TestFindingAFoldedBodyInPlaceAgreesWithCountingTheFoldedText(t *testing.T) {
	body := make([]byte, 700)
	for i := range body {
		body[i] = byte(i*31 + 7)
	}
	b64 := []byte(base64.StdEncoding.EncodeToString(body))
	folded := func(width int, eol string) []byte { return wrapBase64WithEOL(b64, width, []byte(eol)) }
	broken := bytes.Replace(folded(76, "\r\n"), []byte("\r\n"), []byte("\r\r"), 1)
	for name, raw := range map[string][]byte{
		"once at 76 with CRLF":      concat([]byte("head\r\n\r\n"), folded(76, "\r\n"), []byte("\r\n--b--")),
		"once at 64 with LF":        concat([]byte("head\n\n"), folded(64, "\n"), []byte("\n")),
		"twice":                     concat(folded(76, "\r\n"), []byte("\r\n"), folded(76, "\r\n")),
		"one line break is damaged": concat([]byte("x"), broken),
		"cut short":                 folded(76, "\r\n")[:500],
		"unwrapped":                 concat([]byte("<"), b64, []byte(">")),
	} {
		for _, width := range candidateWraps {
			for _, eol := range []string{"\r\n", "\n"} {
				want := bytes.Count(raw, folded(width, eol)) == 1
				at, length, got := soleWrappedAt(raw, b64, width, []byte(eol))
				if got != want {
					t.Errorf("%s, width %d, eol %q: found once = %v, counting says %v", name, width, eol, got, want)
				}
				if got && !bytes.Equal(raw[at:at+length], folded(width, eol)) {
					t.Errorf("%s, width %d, eol %q: located %q", name, width, eol, raw[at:at+length])
				}
			}
		}
	}
}
