// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var touchWords = struct {
	None           langcopy.Phrase
	Today          langcopy.Phrase
	Yesterday      langcopy.Phrase
	Days           langcopy.Phrase
	First          langcopy.Phrase
	SpokeToday     langcopy.Phrase
	SpokeYesterday langcopy.Phrase
	SpokeDays      langcopy.Phrase
	Meeting        langcopy.Phrase
	MetToday       langcopy.Phrase
	MetYesterday   langcopy.Phrase
	MetDays        langcopy.Phrase
}{
	None:           langcopy.Phrase{En: "Nothing has been captured with anyone in this room before.", De: "Bisher wurde kein Austausch mit den Teilnehmenden erfasst.", Vi: "Chưa ghi nhận trao đổi nào với những người tham gia cuộc họp này."},
	Today:          langcopy.Phrase{En: "Last touch was today.", De: "Der letzte Kontakt war heute.", Vi: "Lần liên hệ gần nhất là hôm nay."},
	Yesterday:      langcopy.Phrase{En: "Last touch was yesterday.", De: "Der letzte Kontakt war gestern.", Vi: "Lần liên hệ gần nhất là hôm qua."},
	Days:           langcopy.Phrase{En: "Last touch was %d days ago.", De: "Der letzte Kontakt war vor %d Tagen.", Vi: "Lần liên hệ gần nhất cách đây %d ngày."},
	First:          langcopy.Phrase{En: "%s — first time you are meeting them.", De: "%s: Ihr trefft euch zum ersten Mal.", Vi: "%s — đây là lần đầu bạn gặp người này."},
	SpokeToday:     langcopy.Phrase{En: "%s — last spoke today.", De: "%s: Zuletzt heute gesprochen.", Vi: "%s — lần trao đổi gần nhất là hôm nay."},
	SpokeYesterday: langcopy.Phrase{En: "%s — last spoke yesterday.", De: "%s: Zuletzt gestern gesprochen.", Vi: "%s — lần trao đổi gần nhất là hôm qua."},
	SpokeDays:      langcopy.Phrase{En: "%s — last spoke %d days ago.", De: "%s: Zuletzt vor %d Tagen gesprochen.", Vi: "%s — lần trao đổi gần nhất cách đây %d ngày."},
	Meeting:        langcopy.Phrase{En: "A meeting", De: "Eine Besprechung", Vi: "Một cuộc họp"},
	MetToday:       langcopy.Phrase{En: "You met earlier today: %s.", De: "Ihr habt euch heute bereits getroffen: %s.", Vi: "Các bạn đã gặp nhau trước đó trong hôm nay: %s."},
	MetYesterday:   langcopy.Phrase{En: "You met yesterday: %s.", De: "Ihr habt euch gestern getroffen: %s.", Vi: "Các bạn đã gặp nhau hôm qua: %s."},
	MetDays:        langcopy.Phrase{En: "You met %d days ago: %s.", De: "Ihr habt euch vor %d Tagen getroffen: %s.", Vi: "Các bạn đã gặp nhau cách đây %d ngày: %s."},
}
