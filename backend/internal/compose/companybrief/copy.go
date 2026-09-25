// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// What the deterministic floor says about a company, in each language.
//
// The floor is what a deployment with no model lane serves, so writing it in
// English alone made the card change language whenever the lane failed.
//
// Two English habits could not be carried across and are gone. An article
// chosen by first letter ("a"/"an") is English grammar; German picks by gender
// and Vietnamese uses none, so each language names the activity kind as a whole
// noun phrase instead. A plural formed by appending "s" is the same mistake:
// German inflects irregularly and Vietnamese does not inflect at all, so a
// count carries its own singular and plural sentence per language.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The activity kinds the floor can name, as constants so the three language
// tables key on the same words rather than on loose literals that can drift
// apart a character at a time.
const (
	kindEmail   = "email"
	kindCall    = "call"
	kindMeeting = "meeting"
	kindNote    = "note"
	kindTask    = "task"
	kindMessage = "message"
)

// companyPhrases carries the company floor's sentences. Adding a field fails to
// compile in the languages that have not answered it.
//
// Held by: TestEveryShippedLanguageWritesTheCompanyFloor (backend/internal/compose/companybrief/copy_test.go)
type companyPhrases struct {
	// ProfileLabels turn a stored profile field into the question it answers.
	// Joined to the stored value with a colon, never grammatically: the values
	// are whatever a human accepted off a site read, in the company's own
	// language, and only a colon is true of both a noun phrase and a sentence.
	ProfileLabels map[string]string

	// KindNouns name an activity kind as a whole noun phrase, article and all
	// where the language has one. A kind absent here renders as its stored key,
	// which says only that something happened.
	KindNouns map[string]string

	ContactsSuffix string // size band
	StrengthClause string // strength, contact count

	OpenDealOne  string
	OpenDealMany string // count
	WorthAbout   string // amount, currency
	WonToDate    string // amount, currency

	StalledDeal string // deal name

	LastContactPlain   string // noun phrase
	LastContactDated   string // noun phrase, date
	LastContactSubject string // noun phrase, subject
	LastContactFull    string // noun phrase, date, subject

	OpenTaskOne   string
	OpenTaskMany  string // count
	TasksStarting string // task phrase, first task name

	// The ask answers' floor, which is the same floor under another question.
	KnownContactOne  string
	KnownContactMany string // count
	KnownContactLine string // contact name
	TasksEarliestDue string // task phrase, date
	OpenTaskNamed    string // task name
	OpenTaskDue      string // task name, date
	OpenDealNamed    string // deal name
	DealStalledMark  string

	// DateLayout is a Go reference layout, so each language writes a date the
	// way its readers do. The year is always named: these writers hold no
	// clock, and a bare day and month on last year's task reads as this year.
	DateLayout string
}

// companyCopy is keyed by every language in textlang.Shipped, held by
// TestEveryShippedLanguageWritesTheCompanyFloor.
//
//nolint:dupl // three translations of one table are structurally identical by construction; that is what makes the census above able to compare them field by field
var companyCopy = map[textlang.Lang]companyPhrases{
	textlang.English: {
		ProfileLabels: map[string]string{
			string(crmcontracts.CompanyProfileFieldFieldOfferSummary):     "What they sell",
			string(crmcontracts.CompanyProfileFieldFieldIcp):              "Who they sell to",
			string(crmcontracts.CompanyProfileFieldFieldValueProposition): "What they promise",
			string(crmcontracts.CompanyProfileFieldFieldUsp):              "How they differentiate",
			string(crmcontracts.CompanyProfileFieldFieldCustomerPains):    "What they solve",
			string(crmcontracts.CompanyProfileFieldFieldDesiredOutcomes):  "What their customers want",
			string(crmcontracts.CompanyProfileFieldFieldBuyingCenter):     "Who decides there",
			string(crmcontracts.CompanyProfileFieldFieldSalesMotion):      "How they sell",
		},
		KindNouns: map[string]string{
			kindEmail: "an email", kindCall: "a call", kindMeeting: "a meeting",
			kindNote: "a note", kindTask: "a task", kindMessage: "a message",
		},
		ContactsSuffix: "%s contacts",
		StrengthClause: " Relationship strength %d across %d known contact(s).",
		OpenDealOne:    "1 open deal",
		OpenDealMany:   "%d open deals",
		WorthAbout:     " worth about %s %s",
		WonToDate:      "; %s %s won to date",
		StalledDeal:    "%s is stalled with no recent activity.",

		LastContactPlain:   "Last contact was %s.",
		LastContactDated:   "Last contact was %s on %s.",
		LastContactSubject: "Last contact was %s: %q.",
		LastContactFull:    "Last contact was %s on %s: %q.",

		OpenTaskOne:      "1 open task",
		OpenTaskMany:     "%d open tasks",
		TasksStarting:    "%s, starting with %q.",
		KnownContactOne:  "1 known contact",
		KnownContactMany: "%d known contacts",
		KnownContactLine: "Known contact: %s.",
		TasksEarliestDue: "%s, the earliest due %s.",
		OpenTaskNamed:    "Open task: %q.",
		OpenTaskDue:      "Open task: %q, due %s.",
		OpenDealNamed:    "Open deal: %s",
		DealStalledMark:  "stalled",
		DateLayout:       "2 Jan 2006",
	},
	textlang.German: {
		ProfileLabels: map[string]string{
			string(crmcontracts.CompanyProfileFieldFieldOfferSummary):     "Was sie verkaufen",
			string(crmcontracts.CompanyProfileFieldFieldIcp):              "An wen sie verkaufen",
			string(crmcontracts.CompanyProfileFieldFieldValueProposition): "Was sie versprechen",
			string(crmcontracts.CompanyProfileFieldFieldUsp):              "Wodurch sie sich unterscheiden",
			string(crmcontracts.CompanyProfileFieldFieldCustomerPains):    "Welches Problem sie lösen",
			string(crmcontracts.CompanyProfileFieldFieldDesiredOutcomes):  "Was ihre Kunden erreichen wollen",
			string(crmcontracts.CompanyProfileFieldFieldBuyingCenter):     "Wer dort entscheidet",
			string(crmcontracts.CompanyProfileFieldFieldSalesMotion):      "Wie sie verkaufen",
		},
		KindNouns: map[string]string{
			kindEmail: "eine E-Mail", kindCall: "ein Anruf", kindMeeting: "ein Termin",
			kindNote: "eine Notiz", kindTask: "eine Aufgabe", kindMessage: "eine Nachricht",
		},
		ContactsSuffix: "%s Kontakte",
		StrengthClause: " Beziehungsstärke %d über %d bekannte Kontakte.",
		OpenDealOne:    "1 offener Deal",
		OpenDealMany:   "%d offene Deals",
		WorthAbout:     " im Wert von etwa %s %s",
		WonToDate:      "; %s %s bisher gewonnen",
		StalledDeal:    "%s stockt, ohne jüngste Aktivität.",

		LastContactPlain:   "Der letzte Kontakt war %s.",
		LastContactDated:   "Der letzte Kontakt war %s am %s.",
		LastContactSubject: "Der letzte Kontakt war %s: %q.",
		LastContactFull:    "Der letzte Kontakt war %s am %s: %q.",

		OpenTaskOne:      "1 offene Aufgabe",
		OpenTaskMany:     "%d offene Aufgaben",
		TasksStarting:    "%s, beginnend mit %q.",
		KnownContactOne:  "1 bekannter Kontakt",
		KnownContactMany: "%d bekannte Kontakte",
		KnownContactLine: "Bekannter Kontakt: %s.",
		TasksEarliestDue: "%s, die früheste fällig am %s.",
		OpenTaskNamed:    "Offene Aufgabe: %q.",
		OpenTaskDue:      "Offene Aufgabe: %q, fällig am %s.",
		OpenDealNamed:    "Offener Deal: %s",
		DealStalledMark:  "stockend",
		DateLayout:       "2. Jan. 2006",
	},
	textlang.Vietnamese: {
		ProfileLabels: map[string]string{
			string(crmcontracts.CompanyProfileFieldFieldOfferSummary):     "Họ bán gì",
			string(crmcontracts.CompanyProfileFieldFieldIcp):              "Họ bán cho ai",
			string(crmcontracts.CompanyProfileFieldFieldValueProposition): "Họ cam kết điều gì",
			string(crmcontracts.CompanyProfileFieldFieldUsp):              "Điều gì làm họ khác biệt",
			string(crmcontracts.CompanyProfileFieldFieldCustomerPains):    "Họ giải quyết vấn đề gì",
			string(crmcontracts.CompanyProfileFieldFieldDesiredOutcomes):  "Khách hàng của họ muốn đạt được gì",
			string(crmcontracts.CompanyProfileFieldFieldBuyingCenter):     "Ai là người quyết định bên đó",
			string(crmcontracts.CompanyProfileFieldFieldSalesMotion):      "Họ bán theo cách nào",
		},
		KindNouns: map[string]string{
			kindEmail: "một email", kindCall: "một cuộc gọi", kindMeeting: "một cuộc họp",
			kindNote: "một ghi chú", kindTask: "một công việc", kindMessage: "một tin nhắn",
		},
		ContactsSuffix: "%s liên hệ",
		StrengthClause: " Mức độ quan hệ %d trên %d liên hệ đã biết.",
		OpenDealOne:    "1 cơ hội đang mở",
		OpenDealMany:   "%d cơ hội đang mở",
		WorthAbout:     " trị giá khoảng %s %s",
		WonToDate:      "; đã thắng %s %s đến nay",
		StalledDeal:    "%s đang chững lại, không có hoạt động gần đây.",

		LastContactPlain:   "Lần liên hệ gần nhất là %s.",
		LastContactDated:   "Lần liên hệ gần nhất là %s vào %s.",
		LastContactSubject: "Lần liên hệ gần nhất là %s: %q.",
		LastContactFull:    "Lần liên hệ gần nhất là %s vào %s: %q.",

		OpenTaskOne:      "1 công việc đang mở",
		OpenTaskMany:     "%d công việc đang mở",
		TasksStarting:    "%s, bắt đầu với %q.",
		KnownContactOne:  "1 liên hệ đã biết",
		KnownContactMany: "%d liên hệ đã biết",
		KnownContactLine: "Liên hệ đã biết: %s.",
		TasksEarliestDue: "%s, sớm nhất đến hạn %s.",
		OpenTaskNamed:    "Công việc đang mở: %q.",
		OpenTaskDue:      "Công việc đang mở: %q, đến hạn %s.",
		OpenDealNamed:    "Cơ hội đang mở: %s",
		DealStalledMark:  "đang chững lại",
		DateLayout:       "2 thg 1 2006",
	},
}

// companyPhrasesFor answers the floor's sentences for a language code, falling
// back to English for one this build does not speak.
func companyPhrasesFor(lang string) companyPhrases {
	if p, ok := companyCopy[textlang.Lang(lang)]; ok {
		return p
	}
	return companyCopy[textlang.English]
}
