// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

func TestAStoredBodyIsAssessedOnlyWhereTheOriginalVouchesForIt(t *testing.T) {
	broken := mailOf(`multipart/mixed; boundary="A"`,
		"", "--A", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64",
		"", "aGFsbG8gd2VsdA==", "!!!not base64!!!", "--A--", "")
	whole := alternative("base64", "aGFsbG8=")
	headerOnly := "From: a@example.org\nTo: " + walkOwner
	longText := strings.Repeat("a long line of text ", 600)
	long := mailOf("text/plain; charset=utf-8", "", longText, "")
	longCut := textcut.BytesMarked(headerOnly+"\n\n"+strings.TrimSpace(longText), legacyMaxBodyLen, "…")
	for name, tc := range map[string]struct {
		raw    []byte
		stored string
		want   capture.BodyCompleteness
		ok     bool
	}{
		"stored as capture stores it now":     {whole, headerOnly + "\n\nhallo", capture.BodyComplete, true},
		"a broken part stored as capture now": {broken, headerOnly + "\n\nhallo welt", capture.BodyPartial, true},
		"text an older parser lost":           {broken, headerOnly, capture.BodyUnreadable, true},
		"a body somebody edited":              {whole, headerOnly + "\n\nhallo, edited", "", false},
		"a long body an older cap cut":        {long, longCut, capture.BodyPartial, true},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := AssessStoredBody(tc.raw, tc.stored)
			if got != tc.want || ok != tc.ok {
				t.Errorf("AssessStoredBody = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAShortStoredBodyIsRecoveredFromItsOriginal(t *testing.T) {
	headerOnly := "From: a@example.org\nTo: " + walkOwner
	whole := alternative("base64", "aGFsbG8=")
	longText := strings.Repeat("a long line of text ", 600)
	long := mailOf("text/plain; charset=utf-8", "", longText, "")
	longCut := textcut.BytesMarked(headerOnly+"\n\n"+strings.TrimSpace(longText), legacyMaxBodyLen, "…")
	for name, tc := range map[string]struct {
		raw          []byte
		stored, want string
		completeness capture.BodyCompleteness
		ok           bool
	}{
		"text an older parser lost": {whole, headerOnly, headerOnly + "\n\nhallo", capture.BodyComplete, true},
		"text the older cap cut":    {long, longCut, headerOnly + "\n\n" + strings.TrimSpace(longText), capture.BodyComplete, true},
		"a body somebody edited":    {whole, headerOnly + "\n\nedited", "", "", false},
		"already whole":             {whole, headerOnly + "\n\nhallo", "", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			got, completeness, ok := RecoverStoredBody(tc.raw, tc.stored)
			if ok != tc.ok || (ok && (got != tc.want || completeness != tc.completeness)) {
				t.Errorf("RecoverStoredBody = %q, %q, %v; want %q, %q, %v", got, completeness, ok, tc.want, tc.completeness, tc.ok)
			}
		})
	}
}
