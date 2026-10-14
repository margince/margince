// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var frontendReason = regexp.MustCompile(`"import\.reason\.([a-z_0-9]+)":\s*("(?:[^"\\]|\\.)*")`)

// The English the screen shows for a code and the English a downloaded
// problems file writes are one sentence: the file is read where the screen is
// not, so a second wording would disagree with what the reader saw.
func TestEveryReasonCodesEnglishIsTheSentenceTheScreenShows(t *testing.T) {
	source, err := os.ReadFile("../../../../frontend/src/i18n/en.ts")
	if err != nil {
		t.Fatal(err)
	}
	screen := map[ReasonCode]string{}
	for _, m := range frontendReason.FindAllStringSubmatch(string(source), -1) {
		var text string
		if err := json.Unmarshal([]byte(m[2]), &text); err != nil {
			t.Fatalf("%s: %v", m[1], err)
		}
		screen[ReasonCode(m[1])] = text
	}
	for _, code := range ReasonCodes() {
		want, ok := screen[code]
		if !ok {
			t.Errorf("%s has no sentence in frontend/src/i18n/en.ts", code)
			continue
		}
		template := reasonCatalog[code].text[textlang.English]
		if got := placeholder.ReplaceAllString(template, "{$1}"); got != want {
			t.Errorf("%s: problems file says %q, the screen says %q", code, got, want)
		}
	}
}
