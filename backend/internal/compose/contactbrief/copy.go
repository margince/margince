// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

// What the deterministic floor says, in each language the product speaks.
//
// The floor is what a deployment with no model lane serves, and what every
// deployment serves when its lane fails. Written in English alone it made the
// same card arrive in two languages depending on whether a model answered —
// stored identically, and indistinguishable to the reader afterwards.
//
// Whole sentences per language, not clauses joined at runtime. The English
// assembly read "They wrote last, " + about + ", and it is unanswered", which
// survives translation only by accident: German puts the verb where English
// puts the object, and a glued sentence cannot move it. The one part that does
// slot is the ABOUT phrase, because it is a noun phrase in all three.

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// briefPhrases carries the floor's sentences. A field added here is a compile
// error in the other languages' entries, which is the point: a sentence written
// in one language and not another is the defect this table replaces.
//
// Held by: TestEveryShippedLanguageWritesTheContactFloor (backend/internal/compose/contactbrief/copy_test.go)
type briefPhrases struct {
	IdentityTitleEmployer string // name, title, employer
	IdentityEmployer      string // name, employer
	IdentityTitle         string // name, title
	IdentityBare          string // name

	AnsweredAfterDays string // days
	AnsweredAfterLong string
	QuietForDays      string // days
	GoneQuiet         string
	BandMoved         string // from, to
	RelationshipMoved string // kind
	UnrecordedBand    string

	RecordedRoleOnDeal string // role, deal
	OnDealNoRole       string // deal

	CaresPriority  string // priority
	CaresObjection string // objection
	CaresBoth      string // priority, objection

	WithheldMessage string

	TheyWroteLastOpen   string // about
	TheyWroteLastClosed string // about
	YouWroteLastOpen    string // about
	YouWroteLastClosed  string // about
	LastCaptured        string // about

	AboutSaying  string // preview
	AboutSubject string // subject
	AboutKind    string // kind
}

// briefCopy is keyed by every language in textlang.Shipped, held by
// TestEveryShippedLanguageWritesTheContactFloor.
var briefCopy = map[textlang.Lang]briefPhrases{
	textlang.English: {
		IdentityTitleEmployer: "%s is %s at %s.",
		IdentityEmployer:      "%s works at %s.",
		IdentityTitle:         "%s is %s.",
		IdentityBare:          "%s is recorded here with no title or employer.",

		AnsweredAfterDays: "They answered after %d days of silence.",
		AnsweredAfterLong: "They answered after a long silence.",
		QuietForDays:      "This relationship has been quiet for %d days.",
		GoneQuiet:         "This relationship has gone quiet.",
		BandMoved:         "The relationship moved from %s to %s.",
		RelationshipMoved: "The relationship changed: %s.",
		UnrecordedBand:    "unrecorded",

		RecordedRoleOnDeal: "They are the recorded %s on %s.",
		OnDealNoRole:       "They sit on %s, with no buying role recorded.",

		CaresPriority:  "They are focused on %s.",
		CaresObjection: "They have %s still unresolved.",
		CaresBoth:      "They are focused on %s, with %s still unresolved.",

		WithheldMessage: "The most recent message on this contact is one you may not read.",

		TheyWroteLastOpen:   "They wrote last, %s, and it is unanswered.",
		TheyWroteLastClosed: "They wrote last, %s.",
		YouWroteLastOpen:    "You wrote last, %s, with no reply yet.",
		YouWroteLastClosed:  "You wrote last, %s.",
		LastCaptured:        "The last thing captured was %s.",

		AboutSaying:  "saying %q",
		AboutSubject: "about %q",
		AboutKind:    "a %s",
	},
	textlang.German: {
		IdentityTitleEmployer: "%s ist %s bei %s.",
		IdentityEmployer:      "%s arbeitet bei %s.",
		IdentityTitle:         "%s ist %s.",
		IdentityBare:          "%s ist hier ohne Position und ohne Arbeitgeber erfasst.",

		AnsweredAfterDays: "Nach %d Tagen ohne Kontakt kam eine Antwort.",
		AnsweredAfterLong: "Nach langer Zeit ohne Kontakt kam eine Antwort.",
		QuietForDays:      "Diese Beziehung ist seit %d Tagen ruhig.",
		GoneQuiet:         "Diese Beziehung ist ruhig geworden.",
		BandMoved:         "Die Beziehung hat sich von %s zu %s verändert.",
		RelationshipMoved: "Die Beziehung hat sich verändert: %s.",
		UnrecordedBand:    "nicht erfasst",

		RecordedRoleOnDeal: "Erfasst als %s bei %s.",
		OnDealNoRole:       "Beteiligt an %s, ohne erfasste Rolle im Kaufprozess.",

		CaresPriority:  "Im Mittelpunkt steht %s.",
		CaresObjection: "Offen ist weiterhin %s.",
		CaresBoth:      "Im Mittelpunkt steht %s, offen ist weiterhin %s.",

		WithheldMessage: "Die jüngste Nachricht zu diesem Kontakt dürfen Sie nicht lesen.",

		TheyWroteLastOpen:   "Zuletzt kam eine Nachricht von dort, %s, und sie ist unbeantwortet.",
		TheyWroteLastClosed: "Zuletzt kam eine Nachricht von dort, %s.",
		YouWroteLastOpen:    "Zuletzt ging eine Nachricht von hier hinaus, %s, bisher ohne Antwort.",
		YouWroteLastClosed:  "Zuletzt ging eine Nachricht von hier hinaus, %s.",
		LastCaptured:        "Zuletzt erfasst wurde %s.",

		AboutSaying:  "mit dem Wortlaut %q",
		AboutSubject: "zum Thema %q",
		AboutKind:    "%s",
	},
	textlang.Vietnamese: {
		IdentityTitleEmployer: "%s là %s tại %s.",
		IdentityEmployer:      "%s làm việc tại %s.",
		IdentityTitle:         "%s là %s.",
		IdentityBare:          "%s được ghi nhận ở đây mà không có chức danh hay nơi làm việc.",

		AnsweredAfterDays: "Họ đã trả lời sau %d ngày im lặng.",
		AnsweredAfterLong: "Họ đã trả lời sau một thời gian dài im lặng.",
		QuietForDays:      "Mối quan hệ này đã im ắng %d ngày.",
		GoneQuiet:         "Mối quan hệ này đã trở nên im ắng.",
		BandMoved:         "Mối quan hệ chuyển từ %s sang %s.",
		RelationshipMoved: "Mối quan hệ đã thay đổi: %s.",
		UnrecordedBand:    "chưa ghi nhận",

		RecordedRoleOnDeal: "Được ghi nhận là %s trong %s.",
		OnDealNoRole:       "Có tham gia %s, nhưng chưa ghi nhận vai trò mua hàng.",

		CaresPriority:  "Họ đang tập trung vào %s.",
		CaresObjection: "Họ vẫn còn vướng mắc về %s.",
		CaresBoth:      "Họ đang tập trung vào %s, và vẫn còn vướng mắc về %s.",

		WithheldMessage: "Tin nhắn gần đây nhất của liên hệ này là tin bạn không được phép đọc.",

		TheyWroteLastOpen:   "Họ là bên viết gần đây nhất, %s, và vẫn chưa được trả lời.",
		TheyWroteLastClosed: "Họ là bên viết gần đây nhất, %s.",
		YouWroteLastOpen:    "Bạn là bên viết gần đây nhất, %s, và chưa có hồi âm.",
		YouWroteLastClosed:  "Bạn là bên viết gần đây nhất, %s.",
		LastCaptured:        "Điều được ghi nhận gần đây nhất là %s.",

		AboutSaying:  "với nội dung %q",
		AboutSubject: "về chủ đề %q",
		AboutKind:    "%s",
	},
}

// phrasesFor answers the floor's sentences for a language code, falling back to
// English for anything this build does not speak.
//
// The fallback is not a guess: an unknown code reaches here only from a stored
// setting this build no longer ships, and English is what BaseLanguageForPrompt
// itself falls back to. Writing the floor in English is worse than writing it
// in the reader's language and better than writing nothing.
func phrasesFor(lang string) briefPhrases {
	if p, ok := briefCopy[textlang.Lang(lang)]; ok {
		return p
	}
	return briefCopy[textlang.English]
}
