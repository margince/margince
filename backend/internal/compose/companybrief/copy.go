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

// phrase is one sentence in every language, kept together so a translator reads
// the three side by side. Keyed per language instead, each sentence sat in a
// different block a hundred lines from its siblings.
type phrase struct{ en, de, vi string }

func (p phrase) in(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return p.de
	case textlang.Vietnamese:
		return p.vi
	default:
		return p.en
	}
}

// spoken is the floor resolved to one language.
type spoken struct{ lang textlang.Lang }

func (s spoken) say(p phrase) string { return p.in(s.lang) }

// nouns answers a keyed noun in this language, falling back to the stored key —
// a kind or field this build has no word for says only that it exists.
func (s spoken) noun(table map[string]phrase, key string) (string, bool) {
	p, ok := table[key]
	if !ok {
		return key, false
	}
	return p.in(s.lang), true
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
			en: "What they sell", de: "Was sie verkaufen", vi: "Họ bán gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldIcp): {
			en: "Who they sell to", de: "An wen sie verkaufen", vi: "Họ bán cho ai",
		},
		string(crmcontracts.CompanyProfileFieldFieldValueProposition): {
			en: "What they promise", de: "Was sie versprechen", vi: "Họ cam kết điều gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldUsp): {
			en: "How they differentiate", de: "Wodurch sie sich unterscheiden",
			vi: "Điều gì làm họ khác biệt",
		},
		string(crmcontracts.CompanyProfileFieldFieldCustomerPains): {
			en: "What they solve", de: "Welches Problem sie lösen",
			vi: "Họ giải quyết vấn đề gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldDesiredOutcomes): {
			en: "What their customers want", de: "Was ihre Kunden erreichen wollen",
			vi: "Khách hàng của họ muốn đạt được gì",
		},
		string(crmcontracts.CompanyProfileFieldFieldBuyingCenter): {
			en: "Who decides there", de: "Wer dort entscheidet",
			vi: "Ai là người quyết định bên đó",
		},
		string(crmcontracts.CompanyProfileFieldFieldSalesMotion): {
			en: "How they sell", de: "Wie sie verkaufen", vi: "Họ bán theo cách nào",
		},
	},
	KindNouns: map[string]phrase{
		kindEmail:   {en: "an email", de: "eine E-Mail", vi: "một email"},
		kindCall:    {en: "a call", de: "ein Anruf", vi: "một cuộc gọi"},
		kindMeeting: {en: "a meeting", de: "ein Termin", vi: "một cuộc họp"},
		kindNote:    {en: "a note", de: "eine Notiz", vi: "một ghi chú"},
		kindTask:    {en: "a task", de: "eine Aufgabe", vi: "một công việc"},
		kindMessage: {en: "a message", de: "eine Nachricht", vi: "một tin nhắn"},
	},

	ContactsSuffix: phrase{en: "%s contacts", de: "%s Kontakte", vi: "%s liên hệ"},
	StrengthOverOne: phrase{
		en: " Relationship strength %d across 1 known contact.",
		de: " Beziehungsstärke %d über einen bekannten Kontakt.",
		vi: " Mức độ quan hệ %d trên 1 liên hệ đã biết.",
	},
	StrengthOverContacts: phrase{
		en: " Relationship strength %d across %d known contacts.",
		de: " Beziehungsstärke %d über %d bekannte Kontakte.",
		vi: " Mức độ quan hệ %d trên %d liên hệ đã biết.",
	},

	OpenDealOne:  phrase{en: "1 open deal", de: "1 offener Deal", vi: "1 cơ hội đang mở"},
	OpenDealMany: phrase{en: "%d open deals", de: "%d offene Deals", vi: "%d cơ hội đang mở"},
	WorthAbout: phrase{
		en: " worth about %s %s", de: " im Wert von etwa %s %s",
		vi: " trị giá khoảng %s %s",
	},
	WonToDate: phrase{
		en: "; %s %s won to date", de: "; %s %s bisher gewonnen",
		vi: "; đã thắng %s %s đến nay",
	},
	StalledDeal: phrase{
		en: "%s is stalled with no recent activity.",
		de: "%s stockt, ohne jüngste Aktivität.",
		vi: "%s đang chững lại, không có hoạt động gần đây.",
	},

	LastContactPlain: phrase{
		en: "Last contact was %s.", de: "Der letzte Kontakt war %s.",
		vi: "Lần liên hệ gần nhất là %s.",
	},
	LastContactDated: phrase{
		en: "Last contact was %s on %s.", de: "Der letzte Kontakt war %s am %s.",
		vi: "Lần liên hệ gần nhất là %s vào %s.",
	},
	LastContactSubject: phrase{
		en: "Last contact was %s: %q.", de: "Der letzte Kontakt war %s: %q.",
		vi: "Lần liên hệ gần nhất là %s: %q.",
	},
	LastContactFull: phrase{
		en: "Last contact was %s on %s: %q.", de: "Der letzte Kontakt war %s am %s: %q.",
		vi: "Lần liên hệ gần nhất là %s vào %s: %q.",
	},

	OpenTaskOne:  phrase{en: "1 open task", de: "1 offene Aufgabe", vi: "1 công việc đang mở"},
	OpenTaskMany: phrase{en: "%d open tasks", de: "%d offene Aufgaben", vi: "%d công việc đang mở"},
	TasksStarting: phrase{
		en: "%s, starting with %q.", de: "%s, beginnend mit %q.",
		vi: "%s, bắt đầu với %q.",
	},

	KnownContactOne: phrase{
		en: "1 known contact", de: "1 bekannter Kontakt", vi: "1 liên hệ đã biết",
	},
	KnownContactMany: phrase{
		en: "%d known contacts", de: "%d bekannte Kontakte", vi: "%d liên hệ đã biết",
	},
	KnownContactLine: phrase{
		en: "Known contact: %s.", de: "Bekannter Kontakt: %s.", vi: "Liên hệ đã biết: %s.",
	},
	TasksEarliestDue: phrase{
		en: "%s, the earliest due %s.", de: "%s, die früheste fällig am %s.",
		vi: "%s, sớm nhất đến hạn %s.",
	},
	OpenTaskNamed: phrase{
		en: "Open task: %q.", de: "Offene Aufgabe: %q.", vi: "Công việc đang mở: %q.",
	},
	OpenTaskDue: phrase{
		en: "Open task: %q, due %s.", de: "Offene Aufgabe: %q, fällig am %s.",
		vi: "Công việc đang mở: %q, đến hạn %s.",
	},
	OpenDealNamed: phrase{
		en: "Open deal: %s", de: "Offener Deal: %s", vi: "Cơ hội đang mở: %s",
	},
	DealStalledMark: phrase{en: "stalled", de: "stockend", vi: "đang chững lại"},

	DateLayout: phrase{en: "2 Jan 2006", de: "2.1.2006", vi: "2/1/2006"},
}

// companyPhrasesFor answers the floor's language for a code, falling back to
// English for one this build does not speak.
func companyPhrasesFor(lang string) spoken {
	if textlang.Known(lang) {
		return spoken{lang: textlang.Lang(lang)}
	}
	return spoken{lang: textlang.English}
}
