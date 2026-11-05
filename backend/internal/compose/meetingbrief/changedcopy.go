// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var changedWords = struct {
	First     langcopy.Phrase
	Today     langcopy.Phrase
	Yesterday langcopy.Phrase
	Days      langcopy.Phrase
	One       langcopy.Phrase
	Many      langcopy.Phrase
	Unchanged langcopy.Phrase
	Ours      langcopy.Phrase
	Theirs    langcopy.Phrase
	Objection langcopy.Phrase
	Agreed    langcopy.Phrase
	Asked     langcopy.Phrase
	Said      langcopy.Phrase
}{
	First:     langcopy.Phrase{En: "First contact: you have not dealt with anyone in this room before.", De: "Erster Kontakt: Du hattest bisher mit niemandem in dieser Runde zu tun.", Vi: "Liên hệ lần đầu: bạn chưa từng làm việc với ai trong cuộc họp này."},
	Today:     langcopy.Phrase{En: "You last dealt with this room earlier today.", De: "Du hattest heute bereits Kontakt mit dieser Runde.", Vi: "Bạn đã làm việc với nhóm này trước đó trong hôm nay."},
	Yesterday: langcopy.Phrase{En: "You last dealt with this room yesterday.", De: "Dein letzter Kontakt mit dieser Runde war gestern.", Vi: "Lần gần nhất bạn làm việc với nhóm này là hôm qua."},
	Days:      langcopy.Phrase{En: "You last dealt with this room %d days ago.", De: "Dein letzter Kontakt mit dieser Runde war vor %d Tagen.", Vi: "Lần gần nhất bạn làm việc với nhóm này là cách đây %d ngày."},
	One:       langcopy.Phrase{En: "1 conversation since then, the latest %q.", De: "Seitdem gab es ein Gespräch, zuletzt %q.", Vi: "Có một cuộc trao đổi kể từ đó, gần nhất là %q."},
	Many:      langcopy.Phrase{En: "%d conversations since then, the latest %q.", De: "Seitdem gab es %d Gespräche, zuletzt %q.", Vi: "Có %d cuộc trao đổi kể từ đó, gần nhất là %q."},
	Unchanged: langcopy.Phrase{En: "Nothing captured has changed since.", De: "Seitdem wurde keine Änderung erfasst.", Vi: "Không có thay đổi nào được ghi nhận kể từ đó."},
	Ours:      langcopy.Phrase{En: "Since then we promised %s: %s", De: "Seitdem haben wir %s zugesagt: %s", Vi: "Kể từ đó, chúng ta đã hứa với %s: %s"},
	Theirs:    langcopy.Phrase{En: "Since then %s promised: %s", De: "Seitdem hat %s zugesagt: %s", Vi: "Kể từ đó, %s đã hứa: %s"},
	Objection: langcopy.Phrase{En: "Since then %s objected: %s", De: "Seitdem hat %s eingewandt: %s", Vi: "Kể từ đó, %s đã phản đối: %s"},
	Agreed:    langcopy.Phrase{En: "Since then it was agreed with %s: %s", De: "Seitdem wurde mit %s vereinbart: %s", Vi: "Kể từ đó, đã thống nhất với %s: %s"},
	Asked:     langcopy.Phrase{En: "Since then %s asked: %s", De: "Seitdem hat %s gefragt: %s", Vi: "Kể từ đó, %s đã hỏi: %s"},
	Said:      langcopy.Phrase{En: "Since then %s said: %s", De: "Seitdem hat %s gesagt: %s", Vi: "Kể từ đó, %s đã nói: %s"},
}

func (said briefCopy) lastDealtLine(now time.Time, at Occurred, loc *time.Location) string {
	days := at.daysAgo(now, loc)
	switch {
	case days <= 0:
		return said.Say(changedWords.Today)
	case days == 1:
		return said.Say(changedWords.Yesterday)
	default:
		return fmt.Sprintf(said.Say(changedWords.Days), days)
	}
}

func (said briefCopy) recentConversations(count int, subject string) string {
	if count == 1 {
		return fmt.Sprintf(said.Say(changedWords.One), subject)
	}
	return fmt.Sprintf(said.Say(changedWords.Many), count, subject)
}
