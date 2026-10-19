// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type quietCopy struct{ langcopy.Spoken }

func quietCopyFor(language string) quietCopy { return quietCopy{langcopy.For(language)} }

var quietWords = struct{ Fallback, PacedOne, Paced, InMessage, InCall, InMeeting, OutMessage, OutCall, OutMeeting, NoCorrespondence, Contact, ContactObject, ContactRecipient, Today, Yesterday, Days, Weeks langcopy.Phrase }{
	Fallback:         langcopy.Phrase{En: "This deal has gone quiet and we could not read its correspondence — check the account.", De: "Dieser Deal ist still geworden. Die Korrespondenz konnte nicht gelesen werden. Prüfe das Konto.", Vi: "Deal này đã im ắng và không đọc được trao đổi. Hãy kiểm tra tài khoản."},
	PacedOne:         langcopy.Phrase{En: "Based on how long deals like this usually take to clear their last stage.", De: "Grundlage ist die übliche Dauer der letzten Phase bei vergleichbaren Deals.", Vi: "Dựa trên thời gian các deal tương tự thường cần để hoàn tất giai đoạn cuối."},
	Paced:            langcopy.Phrase{En: "Based on how long deals like this usually take, with %d stages still to go.", De: "Grundlage ist die übliche Dauer vergleichbarer Deals; es verbleiben %d Phasen.", Vi: "Dựa trên thời gian thông thường của các deal tương tự, còn %d giai đoạn."},
	InMessage:        langcopy.Phrase{En: "%s wrote on %s and nobody has answered since — %s.", De: "%s hat am %s geschrieben. Seitdem hat niemand geantwortet — %s.", Vi: "%s đã viết ngày %s và từ đó chưa có ai trả lời — %s."},
	InCall:           langcopy.Phrase{En: "%s called on %s and nobody has followed up since — %s.", De: "%s hat am %s angerufen. Seitdem hat niemand nachgefasst — %s.", Vi: "%s đã gọi ngày %s và từ đó chưa có ai liên hệ tiếp — %s."},
	InMeeting:        langcopy.Phrase{En: "%s met us on %s and nobody has followed up since — %s.", De: "%s hat uns am %s getroffen. Seitdem hat niemand nachgefasst — %s.", Vi: "%s đã gặp nhóm ngày %s và từ đó chưa có ai liên hệ tiếp — %s."},
	OutMessage:       langcopy.Phrase{En: "We wrote to %s on %s and there has been no reply — %s.", De: "Wir haben %s am %s geschrieben und keine Antwort erhalten — %s.", Vi: "Nhóm đã viết cho %s ngày %s và chưa nhận được trả lời — %s."},
	OutCall:          langcopy.Phrase{En: "We called %s on %s and nothing has happened since — %s.", De: "Wir haben %s am %s angerufen. Seitdem ist nichts passiert — %s.", Vi: "Nhóm đã gọi %s ngày %s và từ đó chưa có hoạt động nào — %s."},
	OutMeeting:       langcopy.Phrase{En: "We met %s on %s and nothing has happened since — %s.", De: "Wir haben %s am %s getroffen. Seitdem ist nichts passiert — %s.", Vi: "Nhóm đã gặp %s ngày %s và từ đó chưa có hoạt động nào — %s."},
	NoCorrespondence: langcopy.Phrase{En: "Nobody has been in touch on this deal either way — there is no correspondence to judge it by.", De: "Zu diesem Deal hat noch kein Austausch stattgefunden. Es gibt keine Korrespondenz als Grundlage für eine Einschätzung.", Vi: "Chưa có bên nào liên hệ về deal này; không có trao đổi để đánh giá."},
	Contact:          langcopy.Phrase{En: "The contact", De: "Der Kontakt", Vi: "Người liên hệ"},
	ContactObject:    langcopy.Phrase{En: "the contact", De: "den Kontakt", Vi: "người liên hệ"},
	ContactRecipient: langcopy.Phrase{En: "the contact", De: "dem Kontakt", Vi: "người liên hệ"},
	Today:            langcopy.Phrase{En: "that was today", De: "das war heute", Vi: "đó là hôm nay"},
	Yesterday:        langcopy.Phrase{En: "that was yesterday", De: "das war gestern", Vi: "đó là hôm qua"},
	Days:             langcopy.Phrase{En: "%d days ago", De: "vor %d Tagen", Vi: "%d ngày trước"},
	Weeks:            langcopy.Phrase{En: "%d weeks ago", De: "vor %d Wochen", Vi: "%d tuần trước"},
}
