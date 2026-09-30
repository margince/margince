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
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
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

// The tables below are read by the writers in this package; the primitive they
// are built from is shared, because six private copies of one three-field
// struct is six places for a fallback to differ.
type phrase = langcopy.Phrase

type spoken = langcopy.Spoken

// nounFor answers a keyed noun in the resolved language, falling back to the
// stored key — a kind or field this build has no word for says only that it
// exists. A free function rather than a method, because Spoken's method set is
// fixed at its shared definition.
func nounFor(s spoken, table map[string]phrase, key string) (string, bool) {
	p, ok := table[key]
	if !ok {
		return key, false
	}
	return p.In(s.Lang()), true
}

// companyPhrases is the company floor's sentence set. Every field is answered
// in all three languages by floor below, which
// TestEveryShippedLanguageWritesTheCompanyFloor holds — a keyed literal may omit
// a field and Go fills it with "", so an unanswered sentence goes missing
// rather than failing to build.
type companyPhrases struct {
	// ProfileLabels turn a stored profile field into the question it answers.
	// Joined to the stored value with a colon, never grammatically: the values
	// are whatever a human accepted off a site read, in the company's own
	// language, and only a colon is true of both a noun phrase and a sentence.
	ProfileLabels map[string]phrase

	// KindNouns name an activity kind as a whole noun phrase, article and all
	// where the language has one.
	KindNouns map[string]phrase

	ContactsSuffix phrase // size band
	// The contact count takes its own sentence at one: "über 1 bekannte
	// Kontakte" is not German.
	StrengthOverOne      phrase // strength
	StrengthOverContacts phrase // strength, contact count

	OpenDealOne  phrase
	OpenDealMany phrase // count
	WorthAbout   phrase // amount, currency
	WonToDate    phrase // amount, currency

	StalledDeal phrase // deal name

	LastContactPlain   phrase // noun phrase
	LastContactDated   phrase // noun phrase, date
	LastContactSubject phrase // noun phrase, subject
	LastContactFull    phrase // noun phrase, date, subject

	OpenTaskOne   phrase
	OpenTaskMany  phrase // count
	TasksStarting phrase // task phrase, first task name

	KnownContactOne  phrase
	KnownContactMany phrase // count
	KnownContactLine phrase // contact name
	TasksEarliestDue phrase // task phrase, date
	OpenTaskNamed    phrase // task name
	OpenTaskDue      phrase // task name, date
	OpenDealNamed    phrase // deal name
	DealStalledMark  phrase

	// DateLayout is a Go reference layout. Only English may name a month in
	// words: time.Format writes the `Jan` token as an English abbreviation
	// whatever the reader's language, so a German layout spelling it out would
	// print "2. May. 2026". The other two are numeric for that reason, held by
	// TestNoLanguageWritesAnEnglishMonthName.
	DateLayout phrase
}

var floor = companyPhrases{
	ProfileLabels: map[string]phrase{
		string(crmcontracts.CompanyProfileFieldFieldOfferSummary): {
			En: "What they sell", De: "Was sie verkaufen", Vi: "Họ bán gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldIcp): {
			En: "Who they sell to", De: "An wen sie verkaufen", Vi: "Họ bán cho ai",
		},
		string(crmcontracts.CompanyProfileFieldFieldValueProposition): {
			En: "What they promise", De: "Was sie versprechen", Vi: "Họ cam kết điều gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldUsp): {
			En: "How they differentiate", De: "Wodurch sie sich unterscheiden",
			Vi: "Điều gì làm họ khác biệt",
		},
		string(crmcontracts.CompanyProfileFieldFieldCustomerPains): {
			En: "What they solve", De: "Welches Problem sie lösen",
			Vi: "Họ giải quyết vấn đề gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldDesiredOutcomes): {
			En: "What their customers want", De: "Was ihre Kunden erreichen wollen",
			Vi: "Khách hàng của họ muốn đạt được gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldBuyingCenter): {
			En: "Who decides there", De: "Wer dort entscheidet",
			Vi: "Ai là người quyết định bên đó",
		},
		string(crmcontracts.CompanyProfileFieldFieldSalesMotion): {
			En: "How they sell", De: "Wie sie verkaufen", Vi: "Họ bán theo cách nào",
		},
	},
	KindNouns: map[string]phrase{
		kindEmail:   {En: "an email", De: "eine E-Mail", Vi: "một email"},
		kindCall:    {En: "a call", De: "ein Anruf", Vi: "một cuộc gọi"},
		kindMeeting: {En: "a meeting", De: "ein Termin", Vi: "một cuộc họp"},
		kindNote:    {En: "a note", De: "eine Notiz", Vi: "một ghi chú"},
		kindTask:    {En: "a task", De: "eine Aufgabe", Vi: "một công việc"},
		kindMessage: {En: "a message", De: "eine Nachricht", Vi: "một tin nhắn"},
	},

	ContactsSuffix: phrase{En: "%s contacts", De: "%s Kontakte", Vi: "%s liên hệ"},
	StrengthOverOne: phrase{
		En: " Relationship strength %d across 1 known contact.",
		De: " Beziehungsstärke %d über einen bekannten Kontakt.",
		Vi: " Mức độ quan hệ %d trên 1 liên hệ đã biết.",
	},
	StrengthOverContacts: phrase{
		En: " Relationship strength %d across %d known contacts.",
		De: " Beziehungsstärke %d über %d bekannte Kontakte.",
		Vi: " Mức độ quan hệ %d trên %d liên hệ đã biết.",
	},

	OpenDealOne:  phrase{En: "1 open deal", De: "1 offener Deal", Vi: "1 cơ hội đang mở"},
	OpenDealMany: phrase{En: "%d open deals", De: "%d offene Deals", Vi: "%d cơ hội đang mở"},
	WorthAbout: phrase{
		En: " worth about %s %s", De: " im Wert von etwa %s %s",
		Vi: " trị giá khoảng %s %s",
	},
	WonToDate: phrase{
		En: "; %s %s won to date", De: "; %s %s bisher gewonnen",
		Vi: "; đã thắng %s %s đến nay",
	},
	StalledDeal: phrase{
		En: "%s is stalled with no recent activity.",
		De: "%s stockt, ohne jüngste Aktivität.",
		Vi: "%s đang chững lại, không có hoạt động gần đây.",
	},

	LastContactPlain: phrase{
		En: "Last contact was %s.", De: "Der letzte Kontakt war %s.",
		Vi: "Lần liên hệ gần nhất là %s.",
	},
	LastContactDated: phrase{
		En: "Last contact was %s on %s.", De: "Der letzte Kontakt war %s am %s.",
		Vi: "Lần liên hệ gần nhất là %s vào %s.",
	},
	LastContactSubject: phrase{
		En: "Last contact was %s: %q.", De: "Der letzte Kontakt war %s: %q.",
		Vi: "Lần liên hệ gần nhất là %s: %q.",
	},
	LastContactFull: phrase{
		En: "Last contact was %s on %s: %q.", De: "Der letzte Kontakt war %s am %s: %q.",
		Vi: "Lần liên hệ gần nhất là %s vào %s: %q.",
	},

	OpenTaskOne:  phrase{En: "1 open task", De: "1 offene Aufgabe", Vi: "1 công việc đang mở"},
	OpenTaskMany: phrase{En: "%d open tasks", De: "%d offene Aufgaben", Vi: "%d công việc đang mở"},
	TasksStarting: phrase{
		En: "%s, starting with %q.", De: "%s, beginnend mit %q.",
		Vi: "%s, bắt đầu với %q.",
	},

	KnownContactOne: phrase{
		En: "1 known contact", De: "1 bekannter Kontakt", Vi: "1 liên hệ đã biết",
	},
	KnownContactMany: phrase{
		En: "%d known contacts", De: "%d bekannte Kontakte", Vi: "%d liên hệ đã biết",
	},
	KnownContactLine: phrase{
		En: "Known contact: %s.", De: "Bekannter Kontakt: %s.", Vi: "Liên hệ đã biết: %s.",
	},
	TasksEarliestDue: phrase{
		En: "%s, the earliest due %s.", De: "%s, die früheste fällig am %s.",
		Vi: "%s, sớm nhất đến hạn %s.",
	},
	OpenTaskNamed: phrase{
		En: "Open task: %q.", De: "Offene Aufgabe: %q.", Vi: "Công việc đang mở: %q.",
	},
	OpenTaskDue: phrase{
		En: "Open task: %q, due %s.", De: "Offene Aufgabe: %q, fällig am %s.",
		Vi: "Công việc đang mở: %q, đến hạn %s.",
	},
	OpenDealNamed: phrase{
		En: "Open deal: %s", De: "Offener Deal: %s", Vi: "Cơ hội đang mở: %s",
	},
	DealStalledMark: phrase{En: "stalled", De: "stockend", Vi: "đang chững lại"},

	DateLayout: phrase{En: "2 Jan 2006", De: "2.1.2006", Vi: "2/1/2006"},
}

// companyPhrasesFor answers the floor's language for a code, falling back to
// English for one this build does not speak.
func companyPhrasesFor(lang string) spoken { return langcopy.For(lang) }
