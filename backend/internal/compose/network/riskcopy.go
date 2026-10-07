// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// What a coverage risk says, in each language the product speaks.
//
// These go out on the deal page as the finding itself, in the installation's
// base language, beside a kind label the frontend translates from the enum.
//
// The table is its own file because risk.go holds the rules; a reader auditing
// a threshold should not page through nine translations to reach the next one.
//
// Not intronotewrite.go's noteTable shape: a note's fields are assembled by
// draftfloor.Fill and chosen by branches, and a finding is one whole sentence.

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type phrase = langcopy.Phrase

// riskSentences is the finding set. Every field is answered in all three
// languages by riskWords below, which TestEveryShippedLanguageWritesEveryRisk
// holds — a keyed literal may omit a field and Go fills it with "", so an
// unanswered sentence goes missing rather than failing to build.
type riskSentences struct {
	SingleThreaded   phrase
	CoverageGap      phrase
	ChampionDeparted phrase
	StakeholderLeft  phrase
	GoingCold        phrase // days
	OurSideDominance phrase
}

var riskWords = riskSentences{
	SingleThreaded: phrase{
		En: "fewer than two engaged contacts — the deal rests on one relationship",
		De: "weniger als zwei aktive Kontakte — der Deal hängt an einer einzigen Beziehung",
		Vi: "chưa tới hai người liên hệ đang tương tác — thương vụ chỉ dựa vào một mối quan hệ",
	},
	CoverageGap: phrase{
		En: "no engaged champion — nobody inside the account is carrying this",
		De: "kein aktiver Fürsprecher — niemand im Unternehmen trägt das mit",
		Vi: "không có người ủng hộ đang tương tác — không ai bên trong công ty đang dẫn dắt việc này",
	},
	ChampionDeparted: phrase{
		En: "the champion has left the account — the contact arguing for this deal no longer works there",
		De: "der Fürsprecher hat das Unternehmen verlassen — der Kontakt, der für diesen Deal eingetreten ist, arbeitet dort nicht mehr",
		Vi: "người ủng hộ đã rời công ty — người liên hệ từng vận động cho thương vụ này không còn làm việc ở đó",
	},
	StakeholderLeft: phrase{
		En: "a stakeholder has left the account — the seat is still on the deal, the relationship is not",
		De: "ein Beteiligter hat das Unternehmen verlassen — die Rolle steht noch am Deal, die Beziehung nicht mehr",
		Vi: "một bên liên quan đã rời công ty — vị trí vẫn còn trong thương vụ, còn quan hệ thì không",
	},
	GoingCold: phrase{
		En: "no captured touch for %d days — the deal is open and nobody is talking",
		De: "seit %d Tagen kein erfasster Kontakt — der Deal ist offen und niemand spricht miteinander",
		Vi: "đã %d ngày không có tương tác nào được ghi nhận — thương vụ vẫn mở và không ai trao đổi",
	},
	OurSideDominance: phrase{
		En: "one colleague carries almost all the contact — the deal depends on their availability",
		De: "ein Teammitglied trägt fast den gesamten Kontakt — der Deal hängt an seiner Verfügbarkeit",
		Vi: "một đồng nghiệp đang gánh gần như toàn bộ liên hệ — thương vụ phụ thuộc vào thời gian của họ",
	},
}
