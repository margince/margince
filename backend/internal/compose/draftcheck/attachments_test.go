// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestDraftsCannotClaimAnOutgoingAttachmentFromHistoricalMail(t *testing.T) {
	for _, body := range []string{
		"Please find attached the document.",
		"Anbei sende ich dir die Lebensgeschichte.",
		"Tôi đính kèm tài liệu.",
		"Attached you’ll find the document.",
		"I've enclosed the document.",
		"Anbei finden Sie die Unterlagen.",
		"Im Anhang sende ich die Unterlagen.",
	} {
		t.Run(body, func(t *testing.T) {
			found := false
			for _, finding := range Body(body, textlang.English, convstate.BandFresh, Grounds{Threaded: true}) {
				found = found || finding.Rule == RuleUnattachedDocument
			}
			if !found {
				t.Fatal("a reply must not turn a historical attachment into an outgoing one")
			}
		})
	}
	for _, body := range []string{"Could you send the document?", "I can attach a copy after you confirm.", "Danke für deine Unterlagen.", "Thanks — the contract you attached is signed.", "The files you attached are clear."} {
		if ClaimsAttachment(body) {
			t.Errorf("a request or future offer claims no attachment: %q", body)
		}
	}
}
