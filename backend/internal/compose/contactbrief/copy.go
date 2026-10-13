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

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

// The tables below are read by the writers in this package; the primitive they
// are built from is shared, because six private copies of one three-field
// struct is six places for a fallback to differ.
type phrase = langcopy.Phrase

type spoken = langcopy.Spoken

func phrasesFor(lang string) spoken { return langcopy.For(lang) }

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
		En: "%s is %s at %s.",
		De: "%s ist %s bei %s.",
		Vi: "%s là %s tại %s.",
	},
	IdentityEmployer: phrase{
		En: "%s works at %s.",
		De: "%s arbeitet bei %s.",
		Vi: "%s làm việc tại %s.",
	},
	IdentityTitle: phrase{
		En: "%s is %s.",
		De: "%s ist %s.",
		Vi: "%s là %s.",
	},
	IdentityBare: phrase{
		En: "%s is recorded here with no title or employer.",
		De: "%s ist hier ohne Position und ohne Arbeitgeber erfasst.",
		Vi: "%s được ghi nhận ở đây mà không có chức danh hay nơi làm việc.",
	},

	AnsweredAfterADay: phrase{
		En: "They answered after a day of silence.",
		De: "Nach einem Tag ohne Kontakt kam eine Antwort.",
		Vi: "Họ đã trả lời sau một ngày im lặng.",
	},
	AnsweredAfterDays: phrase{
		En: "They answered after %d days of silence.",
		De: "Nach %d Tagen ohne Kontakt kam eine Antwort.",
		Vi: "Họ đã trả lời sau %d ngày im lặng.",
	},
	AnsweredAfterLong: phrase{
		En: "They answered after a long silence.",
		De: "Nach langer Zeit ohne Kontakt kam eine Antwort.",
		Vi: "Họ đã trả lời sau một thời gian dài im lặng.",
	},
	QuietForADay: phrase{
		En: "This relationship has been quiet for a day.",
		De: "Diese Beziehung ist seit einem Tag ruhig.",
		Vi: "Mối quan hệ này đã im ắng một ngày.",
	},
	QuietForDays: phrase{
		En: "This relationship has been quiet for %d days.",
		De: "Diese Beziehung ist seit %d Tagen ruhig.",
		Vi: "Mối quan hệ này đã im ắng %d ngày.",
	},
	GoneQuiet: phrase{
		En: "This relationship has gone quiet.",
		De: "Diese Beziehung ist ruhig geworden.",
		Vi: "Mối quan hệ này đã trở nên im ắng.",
	},
	BandMoved: phrase{
		En: "The relationship moved from %s to %s.",
		De: "Die Beziehung hat sich von %s zu %s verändert.",
		Vi: "Mối quan hệ chuyển từ %s sang %s.",
	},
	RelationshipMoved: phrase{
		En: "The relationship changed: %s.",
		De: "Die Beziehung hat sich verändert: %s.",
		Vi: "Mối quan hệ đã thay đổi: %s.",
	},
	UnrecordedBand: phrase{En: "unrecorded", De: "nicht erfasst", Vi: "chưa ghi nhận"},

	RecordedRoleOnDeal: phrase{
		En: "They are the recorded %s on %s.",
		De: "Erfasst als %s bei %s.",
		Vi: "Được ghi nhận là %s trong %s.",
	},
	OnDealNoRole: phrase{
		En: "They sit on %s, with no buying role recorded.",
		De: "Beteiligt an %s, ohne erfasste Rolle im Kaufprozess.",
		Vi: "Có tham gia %s, nhưng chưa ghi nhận vai trò mua hàng.",
	},

	CaresPriority: phrase{
		En: "They are focused on %s.",
		De: "Im Mittelpunkt steht %s.",
		Vi: "Họ đang tập trung vào %s.",
	},
	CaresObjection: phrase{
		En: "They have %s still unresolved.",
		De: "Offen ist weiterhin %s.",
		Vi: "Họ vẫn còn vướng mắc về %s.",
	},
	CaresBoth: phrase{
		En: "They are focused on %s, with %s still unresolved.",
		De: "Im Mittelpunkt steht %s, offen ist weiterhin %s.",
		Vi: "Họ đang tập trung vào %s, và vẫn còn vướng mắc về %s.",
	},

	WithheldMessage: phrase{
		En: "The most recent message on this contact is one you may not read.",
		De: "Die jüngste Nachricht zu diesem Kontakt dürfen Sie nicht lesen.",
		Vi: "Tin nhắn gần đây nhất của liên hệ này là tin bạn không được phép đọc.",
	},

	TheyWroteLastOpen: phrase{
		En: "They wrote last, %s, and it is unanswered.",
		De: "Zuletzt kam eine Nachricht von dort, %s, und sie ist unbeantwortet.",
		Vi: "Họ là bên viết gần đây nhất, %s, và vẫn chưa được trả lời.",
	},
	TheyWroteLastClosed: phrase{
		En: "They wrote last, %s.",
		De: "Zuletzt kam eine Nachricht von dort, %s.",
		Vi: "Họ là bên viết gần đây nhất, %s.",
	},
	YouWroteLastOpen: phrase{
		En: "You wrote last, %s, with no reply yet.",
		De: "Zuletzt ging eine Nachricht von hier hinaus, %s, bisher ohne Antwort.",
		Vi: "Bạn là bên viết gần đây nhất, %s, và chưa có hồi âm.",
	},
	YouWroteLastClosed: phrase{
		En: "You wrote last, %s.",
		De: "Zuletzt ging eine Nachricht von hier hinaus, %s.",
		Vi: "Bạn là bên viết gần đây nhất, %s.",
	},
	LastCaptured: phrase{
		En: "The last thing captured was %s.",
		De: "Zuletzt erfasst wurde %s.",
		Vi: "Điều được ghi nhận gần đây nhất là %s.",
	},

	AboutSaying:  phrase{En: "saying %q", De: "mit dem Wortlaut %q", Vi: "với nội dung %q"},
	AboutSubject: phrase{En: "about %q", De: "zum Thema %q", Vi: "về chủ đề %q"},
	AboutKind:    phrase{En: "a %s", De: "%s", Vi: "%s"},
}
