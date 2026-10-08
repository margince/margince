// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The sentences the deterministic signal producers write onto an account.
//
// A signal summary is shared-record text: it is stored once and read by
// everyone who can see the account, so it follows the installation's base
// language rather than the language of whatever produced it. Hardcoding one
// language reads correctly on exactly the installations that chose it and
// wrongly on the rest, which is what a German summary on an English screen
// already looked like in production.
//
// The model-written producers need none of this — they are instructed through
// promptlang.Rule, which already answers for every shipped language. What lives
// here is the copy this product writes itself, in the shape onboardingcopy.go
// established: a table keyed by the language, with the shipped set as the
// census, so a language the product gains fails a test rather than a reader.

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// signalSummaryCopy is one language's set. Every field is required — a partial
// set is a silent English fallback in miniature, and the census refuses one.
type signalSummaryCopy struct {
	// ghostedThread says we wrote and nobody answered. %d is how many days ago.
	ghostedThread string
	// projectQuiet says a project has had nothing filed on it. %s is the
	// project's name, %d how many days — in that order, and the census holds
	// both holes present so a translation cannot drop the number it is about.
	projectQuiet string
}

// signalSummaryByLang is the census, keyed by textlang.Lang so the test can
// walk textlang.Shipped and ask this map directly.
var signalSummaryByLang = map[textlang.Lang]signalSummaryCopy{
	textlang.English: {
		ghostedThread: "We wrote %d days ago and nobody has answered.",
		projectQuiet:  "Nothing has been filed on %s for %d days.",
	},
	textlang.German: {
		ghostedThread: "Wir haben vor %d Tagen geschrieben und niemand hat geantwortet.",
		projectQuiet:  "Zu %s wurde seit %d Tagen nichts abgelegt.",
	},
	textlang.Vietnamese: {
		ghostedThread: "Chúng ta đã viết cách đây %d ngày và chưa ai trả lời.",
		projectQuiet:  "Không có gì được lưu cho %s trong %d ngày.",
	},
}

// signalSummaryCopyFor answers the installation's language, and English for
// anything else.
//
// The fallback is unreachable for a language the product ships — the census
// holds that — and it is still here because the language comes off a settings
// row a contact can edit. Answering in a language beats answering in none.
func signalSummaryCopyFor(lang textlang.Lang) signalSummaryCopy {
	if said, ok := signalSummaryByLang[lang]; ok {
		return said
	}
	return signalSummaryByLang[textlang.English]
}
