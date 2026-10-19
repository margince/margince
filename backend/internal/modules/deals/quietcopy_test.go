// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestQuietReceiptLanguagePreservesItsCorrespondenceFacts(t *testing.T) {
	langcopytest.Census(t, quietWords)
	contact := ids.NewV7()
	for _, language := range []string{"de", "vi"} {
		for _, kind := range []string{"call", "meeting", "email", "unknown"} {
			side := &QuietSide{At: quietAt(-21), ContactID: contact, Kind: kind}
			for _, facts := range []QuietFacts{{LastInbound: side}, {LastOutbound: side}} {
				got := quietCopyFor(language).quietReason(facts, QuietNames{contact: "Original Name"}, quietToday, time.UTC)
				for _, fact := range []string{"Original Name", "2026-08-05", "3"} {
					if !strings.Contains(got, fact) {
						t.Fatalf("%s lost %s in %s", language, fact, got)
					}
				}
				if got == quietCopyFor("en").quietReason(facts, QuietNames{contact: "Original Name"}, quietToday, time.UTC) {
					t.Fatalf("%s kept the English explanation", language)
				}
			}
		}
	}
}
