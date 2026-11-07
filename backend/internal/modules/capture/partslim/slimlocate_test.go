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

// An attachment of zeros folds to identical lines, so a run of all of them but
// the last matches deeply from every line start. The search stops at its
// budget before the true copy after that run, and leaves the part where it was.
func TestNearMissesInAMessageCannotMakeTheSearchQuadratic(t *testing.T) {
	b64 := []byte(base64.StdEncoding.EncodeToString(make([]byte, 60000)))
	eol := []byte("\r\n")
	nearMisses := wrapBase64WithEOL(b64[:len(b64)-76], 76, eol)
	raw := concat(nearMisses, []byte("\r\n!\r\n"), wrapBase64WithEOL(b64, 76, eol))
	if _, _, ok := soleWrappedAt(raw, b64, 76, eol); ok {
		t.Fatal("the search ran past its budget and located the part")
	}
	compared := 0
	for from := 0; ; {
		next := bytes.Index(nearMisses[from:], b64[:76])
		if next < 0 {
			break
		}
		_, n := foldedAt(nearMisses[from+next:], b64, 76, eol)
		compared += n
		from += next + 1
	}
	if budget := 2*len(raw) + len(b64); compared <= budget {
		t.Fatalf("the fixture compares %d bytes, under the %d budget, so it does not exercise the bound", compared, budget)
	}
}
