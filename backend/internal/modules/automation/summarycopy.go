// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The inbox lines an automation writes when it stages something for a human.
//
// An approval's summary is shared-record text: stored once and read by every
// seat, so it follows the installation's base language, resolved when the
// approval is staged. Rows already stored keep the words they were written in.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// summaryCopy is one language's set. Every field is required, and the census
// (summarycopy_test.go) refuses a partial set.
type summaryCopy struct {
	// stagedAction names a generic staged action. %s, %s, %s are the action
	// kind, the target's record type and its id, in that order; all three are
	// identifiers and pass through untranslated.
	stagedAction string
	// heldDraft says a composed reply waits for review. %s is the addressee.
	heldDraft string
}

// summaryByLang is the census, keyed by textlang.Lang so the test can walk
// textlang.Shipped and ask this map directly.
var summaryByLang = map[textlang.Lang]summaryCopy{
	textlang.English: {
		stagedAction: "automation wants to %s on %s %s",
		heldDraft:    "an automation drafted a reply to %s — read it before it goes",
	},
	textlang.German: {
		stagedAction: "Eine Automatisierung möchte %s für %s %s ausführen",
		heldDraft:    "Eine Automatisierung hat eine Antwort an %s entworfen. Lies sie, bevor sie versendet wird.",
	},
	textlang.Vietnamese: {
		stagedAction: "Một tự động hóa muốn thực hiện %s trên %s %s",
		heldDraft:    "Một tự động hóa đã soạn thư trả lời gửi %s, hãy đọc trước khi thư được gửi đi.",
	},
}

// summaryIn answers the set for the installation's base language, and English
// for a language this table has not learned: the language comes off a settings
// row, and a sentence in English beats no sentence.
func summaryIn(ctx context.Context, language baselanguage.Resolver) summaryCopy {
	if said, ok := summaryByLang[language.Resolve(ctx)]; ok {
		return said
	}
	return summaryByLang[textlang.English]
}
