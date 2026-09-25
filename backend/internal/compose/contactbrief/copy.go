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

// phrase is one sentence in every language, kept together so a translator reads
// the three side by side and a reviewer can see at a glance that they say the
// same thing. Keyed per language instead, each sentence sat in a different
// block a hundred lines from its siblings.
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

// briefPhrases is the floor's sentence set. Every field is answered in all
// three languages by floor below, which
// TestEveryShippedLanguageWritesTheContactFloor holds — a keyed literal may omit
// a field and Go fills it with "", so an unanswered sentence goes missing
// rather than failing to build.
type briefPhrases struct {
	IdentityTitleEmployer phrase // name, title, employer
	IdentityEmployer      phrase // name, employer
	IdentityTitle         phrase // name, title
	IdentityBare          phrase // name

	// A count of one takes its own sentence: English wrote "1 days" and German
	// "1 Tagen", both of which read as a machine talking.
	AnsweredAfterADay phrase
	AnsweredAfterDays phrase // days
	AnsweredAfterLong phrase
	QuietForADay      phrase
	QuietForDays      phrase // days
	GoneQuiet         phrase
	BandMoved         phrase // from, to
	RelationshipMoved phrase // kind
	UnrecordedBand    phrase

	RecordedRoleOnDeal phrase // role, deal
	OnDealNoRole       phrase // deal

	CaresPriority  phrase // priority
	CaresObjection phrase // objection
	CaresBoth      phrase // priority, objection

	WithheldMessage phrase

	TheyWroteLastOpen   phrase // about
	TheyWroteLastClosed phrase // about
	YouWroteLastOpen    phrase // about
	YouWroteLastClosed  phrase // about
	LastCaptured        phrase // about

	AboutSaying  phrase // preview
	AboutSubject phrase // subject
	AboutKind    phrase // kind
}

var floor = briefPhrases{
	IdentityTitleEmployer: phrase{
		en: "%s is %s at %s.",
		de: "%s ist %s bei %s.",
		vi: "%s là %s tại %s.",
	},
	IdentityEmployer: phrase{
		en: "%s works at %s.",
		de: "%s arbeitet bei %s.",
		vi: "%s làm việc tại %s.",
	},
	IdentityTitle: phrase{
		en: "%s is %s.",
		de: "%s ist %s.",
		vi: "%s là %s.",
	},
	IdentityBare: phrase{
		en: "%s is recorded here with no title or employer.",
		de: "%s ist hier ohne Position und ohne Arbeitgeber erfasst.",
		vi: "%s được ghi nhận ở đây mà không có chức danh hay nơi làm việc.",
	},

	AnsweredAfterADay: phrase{
		en: "They answered after a day of silence.",
		de: "Nach einem Tag ohne Kontakt kam eine Antwort.",
		vi: "Họ đã trả lời sau một ngày im lặng.",
	},
	AnsweredAfterDays: phrase{
		en: "They answered after %d days of silence.",
		de: "Nach %d Tagen ohne Kontakt kam eine Antwort.",
		vi: "Họ đã trả lời sau %d ngày im lặng.",
	},
	AnsweredAfterLong: phrase{
		en: "They answered after a long silence.",
		de: "Nach langer Zeit ohne Kontakt kam eine Antwort.",
		vi: "Họ đã trả lời sau một thời gian dài im lặng.",
	},
	QuietForADay: phrase{
		en: "This relationship has been quiet for a day.",
		de: "Diese Beziehung ist seit einem Tag ruhig.",
		vi: "Mối quan hệ này đã im ắng một ngày.",
	},
	QuietForDays: phrase{
		en: "This relationship has been quiet for %d days.",
		de: "Diese Beziehung ist seit %d Tagen ruhig.",
		vi: "Mối quan hệ này đã im ắng %d ngày.",
	},
	GoneQuiet: phrase{
		en: "This relationship has gone quiet.",
		de: "Diese Beziehung ist ruhig geworden.",
		vi: "Mối quan hệ này đã trở nên im ắng.",
	},
	BandMoved: phrase{
		en: "The relationship moved from %s to %s.",
		de: "Die Beziehung hat sich von %s zu %s verändert.",
		vi: "Mối quan hệ chuyển từ %s sang %s.",
	},
	RelationshipMoved: phrase{
		en: "The relationship changed: %s.",
		de: "Die Beziehung hat sich verändert: %s.",
		vi: "Mối quan hệ đã thay đổi: %s.",
	},
	UnrecordedBand: phrase{en: "unrecorded", de: "nicht erfasst", vi: "chưa ghi nhận"},

	RecordedRoleOnDeal: phrase{
		en: "They are the recorded %s on %s.",
		de: "Erfasst als %s bei %s.",
		vi: "Được ghi nhận là %s trong %s.",
	},
	OnDealNoRole: phrase{
		en: "They sit on %s, with no buying role recorded.",
		de: "Beteiligt an %s, ohne erfasste Rolle im Kaufprozess.",
		vi: "Có tham gia %s, nhưng chưa ghi nhận vai trò mua hàng.",
	},

	CaresPriority: phrase{
		en: "They are focused on %s.",
		de: "Im Mittelpunkt steht %s.",
		vi: "Họ đang tập trung vào %s.",
	},
	CaresObjection: phrase{
		en: "They have %s still unresolved.",
		de: "Offen ist weiterhin %s.",
		vi: "Họ vẫn còn vướng mắc về %s.",
	},
	CaresBoth: phrase{
		en: "They are focused on %s, with %s still unresolved.",
		de: "Im Mittelpunkt steht %s, offen ist weiterhin %s.",
		vi: "Họ đang tập trung vào %s, và vẫn còn vướng mắc về %s.",
	},

	WithheldMessage: phrase{
		en: "The most recent message on this contact is one you may not read.",
		de: "Die jüngste Nachricht zu diesem Kontakt dürfen Sie nicht lesen.",
		vi: "Tin nhắn gần đây nhất của liên hệ này là tin bạn không được phép đọc.",
	},

	TheyWroteLastOpen: phrase{
		en: "They wrote last, %s, and it is unanswered.",
		de: "Zuletzt kam eine Nachricht von dort, %s, und sie ist unbeantwortet.",
		vi: "Họ là bên viết gần đây nhất, %s, và vẫn chưa được trả lời.",
	},
	TheyWroteLastClosed: phrase{
		en: "They wrote last, %s.",
		de: "Zuletzt kam eine Nachricht von dort, %s.",
		vi: "Họ là bên viết gần đây nhất, %s.",
	},
	YouWroteLastOpen: phrase{
		en: "You wrote last, %s, with no reply yet.",
		de: "Zuletzt ging eine Nachricht von hier hinaus, %s, bisher ohne Antwort.",
		vi: "Bạn là bên viết gần đây nhất, %s, và chưa có hồi âm.",
	},
	YouWroteLastClosed: phrase{
		en: "You wrote last, %s.",
		de: "Zuletzt ging eine Nachricht von hier hinaus, %s.",
		vi: "Bạn là bên viết gần đây nhất, %s.",
	},
	LastCaptured: phrase{
		en: "The last thing captured was %s.",
		de: "Zuletzt erfasst wurde %s.",
		vi: "Điều được ghi nhận gần đây nhất là %s.",
	},

	AboutSaying:  phrase{en: "saying %q", de: "mit dem Wortlaut %q", vi: "với nội dung %q"},
	AboutSubject: phrase{en: "about %q", de: "zum Thema %q", vi: "về chủ đề %q"},
	AboutKind:    phrase{en: "a %s", de: "%s", vi: "%s"},
}

// spoken is the floor resolved to one language, so the writers below read a
// sentence rather than a lookup.
type spoken struct {
	lang textlang.Lang
}

func (s spoken) say(p phrase) string { return p.in(s.lang) }

// phrasesFor answers the floor's language for a code, falling back to English
// for anything this build does not speak.
//
// The fallback is not a guess: an unknown code reaches here only from a stored
// setting this build no longer ships, and English is what BaseLanguageForPrompt
// itself falls back to. Writing the floor in English is worse than writing it
// in the reader's language and better than writing nothing.
func phrasesFor(lang string) spoken {
	if textlang.Known(lang) {
		return spoken{lang: textlang.Lang(lang)}
	}
	return spoken{lang: textlang.English}
}
