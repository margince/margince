// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// locateEncoded answers what counting each folded candidate answers: the first
// width and line ending found once, as the folded text itself, or not located.
func TestLocatingAPartAgreesWithCountingEachFoldedCandidate(t *testing.T) {
	body := make([]byte, 700)
	for i := range body {
		body[i] = byte(i*31 + 7)
	}
	b64 := []byte(base64.StdEncoding.EncodeToString(body))
	fold := func(width int, eol string) []byte { return wrapBase64WithEOL(b64, width, []byte(eol)) }
	broken := bytes.Replace(fold(76, "\r\n"), []byte("\r\n"), []byte("\r\r"), 1)
	for name, raw := range map[string][]byte{
		"once at 76 with CRLF":      concat([]byte("head\r\n\r\n"), fold(76, "\r\n"), []byte("\r\n--b--")),
		"once at 72 with CRLF":      concat([]byte("head\r\n\r\n"), fold(72, "\r\n")),
		"once at 64 with LF":        concat([]byte("head\n\n"), fold(64, "\n"), []byte("\n")),
		"twice":                     concat(fold(76, "\r\n"), []byte("\r\n"), fold(76, "\r\n")),
		"one line break is damaged": concat([]byte("x"), broken),
		"cut short":                 fold(76, "\r\n")[:500],
		"unwrapped":                 concat([]byte("<"), b64, []byte(">")),
	} {
		var want []byte
		var wantWidth int
	search:
		for _, width := range candidateWraps {
			for _, eol := range []string{"\r\n", "\n"} {
				if bytes.Count(raw, fold(width, eol)) == 1 {
					want, wantWidth = fold(width, eol), width
					break search
				}
			}
		}
		got, width, _, err := locateEncoded(raw, body)
		if (err == nil) != (want != nil) || !bytes.Equal(got, want) || (want != nil && width != wantWidth) {
			t.Errorf("%s: located %d bytes at width %d (err %v), counting finds %d bytes at width %d", name, len(got), width, err, len(want), wantWidth)
		}
	}
}
