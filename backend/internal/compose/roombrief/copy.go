// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

// What the deterministic floor says, as whole sentences per language: a clause
// glued at runtime survives translation only by accident.

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type phrase = langcopy.Phrase

// briefPhrases is the floor's sentence set. A keyed literal may omit a field
// and Go fills it with "", so TestEveryShippedLanguageWritesTheRoomFloor holds
// every field answered in every language.
type briefPhrases struct {
	WaitingAbout phrase // author, document, quote
	WaitingRoom  phrase // author, quote
	MoreWaiting  phrase // count
	OneMore      phrase
	AskedForLink phrase // name
	NotSignedIn  phrase // names
	NothingOwed  phrase
}

var floor = briefPhrases{
	WaitingAbout: phrase{
		En: "%s is waiting on your answer about %s: “%s”",
		De: "%s wartet auf eine Antwort zu %s: „%s“",
		Vi: "%s đang chờ bạn trả lời về %s: “%s”",
	},
	WaitingRoom: phrase{
		En: "%s is waiting on your answer: “%s”",
		De: "%s wartet auf eine Antwort: „%s“",
		Vi: "%s đang chờ bạn trả lời: “%s”",
	},
	MoreWaiting: phrase{
		En: "%s more threads are waiting on you.",
		De: "%s weitere Threads warten auf eine Antwort.",
		Vi: "Còn %s chủ đề khác đang chờ bạn.",
	},
	OneMore: phrase{
		En: "One more thread is waiting on you.",
		De: "Ein weiterer Thread wartet auf eine Antwort.",
		Vi: "Còn một chủ đề khác đang chờ bạn.",
	},
	AskedForLink: phrase{
		En: "%s asked for a new link.",
		De: "%s hat einen neuen Link angefordert.",
		Vi: "%s đã yêu cầu một liên kết mới.",
	},
	NotSignedIn: phrase{
		En: "Not signed in yet: %s.",
		De: "Noch nicht angemeldet: %s.",
		Vi: "Chưa đăng nhập: %s.",
	},
	NothingOwed: phrase{
		En: "Nothing in the room is waiting on you.",
		De: "Im Raum wartet nichts auf eine Antwort.",
		Vi: "Không có gì trong phòng đang chờ bạn.",
	},
}
