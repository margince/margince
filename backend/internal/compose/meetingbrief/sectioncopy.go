// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

//nolint:dupl // These distinct translations share a table shape, not behavior.
var sectionWords = struct {
	AnswerQuestion  langcopy.Phrase
	GetDecision     langcopy.Phrase
	CloseOverdue    langcopy.Phrase
	ClosePromise    langcopy.Phrase
	TheyWrote       langcopy.Phrase
	WeWrote         langcopy.Phrase
	LastCaptured    langcopy.Phrase
	Agreed          langcopy.Phrase
	OpenObjection   langcopy.Phrase
	OweOverdue      langcopy.Phrase
	AnswerObjection langcopy.Phrase
	CheckObjection  langcopy.Phrase
	DecisionProcess langcopy.Phrase
	Success         langcopy.Phrase
	TheirPromise    langcopy.Phrase
	Priority        langcopy.Phrase
}{
	AnswerQuestion:  langcopy.Phrase{En: "Answer the open question from %s: %s", De: "Beantworte die offene Frage von %s: %s", Vi: "Trả lời câu hỏi còn mở của %s: %s"},
	GetDecision:     langcopy.Phrase{En: "Get the decision %s is holding: %s", De: "Hole die ausstehende Entscheidung von %s ein: %s", Vi: "Lấy quyết định đang chờ từ %s: %s"},
	CloseOverdue:    langcopy.Phrase{En: "Close out what we owe %s, overdue since %s: %s", De: "Erledige, was wir %s schulden und seit %s überfällig ist: %s", Vi: "Hoàn tất điều chúng ta còn nợ %s, đã quá hạn từ %s: %s"},
	ClosePromise:    langcopy.Phrase{En: "Close out what we promised %s: %s", De: "Erfülle, was wir %s zugesagt haben: %s", Vi: "Hoàn tất điều chúng ta đã hứa với %s: %s"},
	TheyWrote:       langcopy.Phrase{En: "They wrote last, about %q.", De: "Die letzte Nachricht kam von ihnen und betraf %q.", Vi: "Tin nhắn gần nhất đến từ họ, về %q."},
	WeWrote:         langcopy.Phrase{En: "We wrote last, about %q.", De: "Die letzte Nachricht kam von uns und betraf %q.", Vi: "Tin nhắn gần nhất do chúng ta gửi, về %q."},
	LastCaptured:    langcopy.Phrase{En: "The last thing captured was %q.", De: "Zuletzt wurde %q erfasst.", Vi: "Nội dung được ghi nhận gần nhất là %q."},
	Agreed:          langcopy.Phrase{En: "Agreed with %s: %s", De: "Mit %s vereinbart: %s", Vi: "Đã thống nhất với %s: %s"},
	OpenObjection:   langcopy.Phrase{En: "%s's objection is still open: %s", De: "Der Einwand von %s ist noch offen: %s", Vi: "Ý kiến phản đối của %s vẫn chưa được giải quyết: %s"},
	OweOverdue:      langcopy.Phrase{En: "We are past due to %s on: %s", De: "Unsere Zusage an %s ist überfällig: %s", Vi: "Chúng ta đã quá hạn với %s về: %s"},
	AnswerObjection: langcopy.Phrase{En: "%s objected to %s and we have not answered — bring the answer, or say when.", De: "%s hatte einen Einwand zu %s, den wir noch nicht beantwortet haben. Bringe die Antwort mit oder nenne einen Termin.", Vi: "%s đã phản đối %s và chúng ta chưa trả lời — hãy mang theo câu trả lời hoặc cho biết khi nào sẽ trả lời."},
	CheckObjection:  langcopy.Phrase{En: "%s once objected to %s — confirm it is settled before moving on.", De: "%s hatte einen Einwand zu %s. Kläre, ob er erledigt ist, bevor du fortfährst.", Vi: "%s từng phản đối %s — xác nhận vấn đề đã được giải quyết trước khi tiếp tục."},
	DecisionProcess: langcopy.Phrase{En: "%s described how they decide: %s — walk the next step of it in the room.", De: "%s hat den Entscheidungsprozess beschrieben: %s. Besprich im Meeting den nächsten Schritt.", Vi: "%s đã mô tả cách họ ra quyết định: %s — cùng làm rõ bước tiếp theo trong cuộc họp."},
	Success:         langcopy.Phrase{En: "%s calls success %s — tie what you show to it.", De: "%s definiert Erfolg als %s. Richte deine Präsentation daran aus.", Vi: "%s định nghĩa thành công là %s — gắn nội dung trình bày với tiêu chí đó."},
	TheirPromise:    langcopy.Phrase{En: "%s owes us %s — ask where it stands.", De: "%s hat uns %s zugesagt. Frage nach dem aktuellen Stand.", Vi: "%s còn nợ chúng ta %s — hỏi về tình trạng hiện tại."},
	Priority:        langcopy.Phrase{En: "%s said %s matters — lead with it.", De: "%s hat %s als wichtig bezeichnet. Beginne damit.", Vi: "%s đã nói %s là quan trọng — hãy bắt đầu từ đó."},
}
