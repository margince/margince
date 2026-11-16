// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"strings"
	"testing"
)

// A reason is read away from its record, on a home page or a notice. So every
// sentence saying why work exists takes a text argument: the record, the
// sender or the finding it is about.
func TestEveryReasonSentenceNamesWhatItIsAbout(t *testing.T) {
	t.Parallel()
	reasons := 0
	for code, definition := range catalog {
		if !isReason(code) {
			continue
		}
		reasons++
		named := false
		for _, kind := range definition.Arguments {
			named = named || kind == argumentText
		}
		if !named {
			t.Errorf("%s says why work exists without naming what it is about", code)
		}
	}
	if reasons == 0 {
		t.Fatal("the census found no reason sentences")
	}
}

// isReason is a code whose sentence is a piece of work's why.
func isReason(code string) bool {
	return strings.Contains(code, ".why") || strings.HasSuffix(code, "_why") || strings.HasPrefix(code, "request.asked")
}
