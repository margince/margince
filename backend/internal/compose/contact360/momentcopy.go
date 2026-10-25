// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type momentCopy struct {
	langcopy.Spoken
	zone *time.Location
}

func momentCopyFor(language string) momentCopy {
	return momentCopy{Spoken: langcopy.For(language), zone: time.UTC}
}

var momentWords = struct {
	Meeting           langcopy.Phrase
	Prepare           langcopy.Phrase
	PrepareWhy        langcopy.Phrase
	Brief             langcopy.Phrase
	Agenda            langcopy.Phrase
	Replied           langcopy.Phrase
	RepliedWhy        langcopy.Phrase
	Reply             langcopy.Phrase
	Quiet             langcopy.Phrase
	QuietWhy          langcopy.Phrase
	FollowUp          langcopy.Phrase
	AskUnavailable    langcopy.Phrase
	Ask               langcopy.Phrase
	Log               langcopy.Phrase
	QuietNothing      langcopy.Phrase
	HiddenNothing     langcopy.Phrase
	Nothing           langcopy.Phrase
	MissingEvidence   langcopy.Phrase
	MissingHeadline   langcopy.Phrase
	MissingWhy        langcopy.Phrase
	OpenDeal          langcopy.Phrase
	Thin              langcopy.Phrase
	ThinWhy           langcopy.Phrase
	Book              langcopy.Phrase
	TheirMessage      langcopy.Phrase
	LastSentMessage   langcopy.Phrase
	Stakeholder       langcopy.Phrase
	RecordedRole      langcopy.Phrase
	OpenTask          langcopy.Phrase
	Write             langcopy.Phrase
	TeamOwes          langcopy.Phrase
	YouOwe            langcopy.Phrase
	TaskUndated       langcopy.Phrase
	PromisedToday     langcopy.Phrase
	PromisedAgo       langcopy.Phrase
	PromisedYesterday langcopy.Phrase
	Send              langcopy.Phrase
	ClaimUndated      langcopy.Phrase
}{
	Meeting:           langcopy.Phrase{En: "Meeting", De: "die Besprechung", Vi: "Cuộc họp"},
	Prepare:           langcopy.Phrase{En: "Prepare for %s", De: "Auf %s vorbereiten", Vi: "Chuẩn bị cho %s"},
	PrepareWhy:        langcopy.Phrase{En: "Prepare before the meeting, not after.", De: "Vor der Besprechung vorbereiten, nicht danach.", Vi: "Chuẩn bị trước cuộc họp, không phải sau đó."},
	Brief:             langcopy.Phrase{En: "Open meeting brief", De: "Besprechungsvorbereitung öffnen", Vi: "Mở bản chuẩn bị cuộc họp"},
	Agenda:            langcopy.Phrase{En: "Draft agenda", De: "Tagesordnung entwerfen", Vi: "Soạn chương trình họp"},
	Replied:           langcopy.Phrase{En: "They wrote back; your last message was %d days earlier", De: "Eine Antwort ist eingegangen; Ihre letzte Nachricht lag %d Tage davor", Vi: "Họ đã trả lời; tin nhắn gần nhất của bạn được gửi trước đó %d ngày"},
	RepliedWhy:        langcopy.Phrase{En: "They're waiting on a reply, and your last message to them is older than the re-engagement rule allows.", De: "Eine Antwort steht aus. Ihre letzte Nachricht liegt länger zurück, als die Regel zur erneuten Kontaktaufnahme vorsieht.", Vi: "Họ đang chờ phản hồi, và tin nhắn gần nhất của bạn đã quá thời hạn theo quy tắc liên hệ lại."},
	Reply:             langcopy.Phrase{En: "Draft a reply", De: "Antwort entwerfen", Vi: "Soạn phản hồi"},
	Quiet:             langcopy.Phrase{En: "No reply for %d days", De: "Seit %d Tagen keine Antwort", Vi: "Chưa có phản hồi trong %d ngày"},
	QuietWhy:          langcopy.Phrase{En: "The last message to them went out %d days ago. The rule flags no reply after %d days.", De: "Die letzte Nachricht an diesen Kontakt wurde vor %d Tagen gesendet. Die Regel meldet eine ausstehende Antwort nach %d Tagen.", Vi: "Tin nhắn gần nhất gửi cho liên hệ này đã đi cách đây %d ngày. Quy tắc đánh dấu khi chưa có phản hồi sau %d ngày."},
	FollowUp:          langcopy.Phrase{En: "Draft a follow-up", De: "Nachfassnachricht entwerfen", Vi: "Soạn tin nhắn theo dõi"},
	AskUnavailable:    langcopy.Phrase{En: "Asking a colleague for context isn't available yet", De: "Das Einholen von Kontext bei Kollegen ist noch nicht verfügbar", Vi: "Chức năng hỏi đồng nghiệp về bối cảnh chưa khả dụng"},
	Ask:               langcopy.Phrase{En: "Ask for context", De: "Nach Kontext fragen", Vi: "Hỏi thêm bối cảnh"},
	Log:               langcopy.Phrase{En: "Log an interaction", De: "Interaktion erfassen", Vi: "Ghi lại tương tác"}, //nolint:misspell // German translation uses German spelling.
	QuietNothing:      langcopy.Phrase{En: "No meeting coming up, nothing owed, nobody waiting on a reply.", De: "Keine Besprechung steht an, keine Zusage ist offen, keine Antwort steht aus.", Vi: "Không có cuộc họp sắp tới, cam kết chưa thực hiện hay phản hồi đang chờ."},
	HiddenNothing:     langcopy.Phrase{En: "Nothing needs you in what you can see. Parts of this record are hidden from you.", De: "In den sichtbaren Daten ist nichts zu tun. Teile dieses Datensatzes sind für Sie nicht sichtbar.", Vi: "Không có việc cần làm trong dữ liệu bạn được xem. Một số phần của hồ sơ bị ẩn với bạn."},
	Nothing:           langcopy.Phrase{En: "Nothing needs you today", De: "Heute steht nichts an", Vi: "Hôm nay không có việc cần bạn xử lý"},
	MissingEvidence:   langcopy.Phrase{En: "%s has no next step with them", De: "Für %s ist kein nächster Schritt mit diesem Kontakt geplant", Vi: "%s chưa có bước tiếp theo với họ"},
	MissingHeadline:   langcopy.Phrase{En: "No next step on the open deal", De: "Kein nächster Schritt für den offenen Deal", Vi: "Giao dịch đang mở chưa có bước tiếp theo"},
	MissingWhy:        langcopy.Phrase{En: "The deal is open and nothing is scheduled with them. They're %s on it.", De: "Der Deal ist offen, aber mit diesem Kontakt ist nichts geplant. Beteiligung: %s.", Vi: "Giao dịch đang mở nhưng chưa có lịch hẹn với họ. Vai trò: %s."},
	OpenDeal:          langcopy.Phrase{En: "Open the deal", De: "Deal öffnen", Vi: "Mở giao dịch"},
	Thin:              langcopy.Phrase{En: "No interactions recorded", De: "Keine Interaktionen erfasst", Vi: "Chưa ghi nhận tương tác"},                                                                                                                                      //nolint:misspell // German translation uses German spelling.
	ThinWhy:           langcopy.Phrase{En: "No interactions or colleagues found in the records you can see.", De: "In den sichtbaren Daten wurden keine Interaktionen oder Kollegen gefunden.", Vi: "Không tìm thấy tương tác hay đồng nghiệp trong các hồ sơ bạn được xem."}, //nolint:misspell // German translation uses German spelling.
	Book:              langcopy.Phrase{En: "Book a meeting", De: "Besprechung buchen", Vi: "Đặt lịch họp"},
	TheirMessage:      langcopy.Phrase{En: "Their last message", De: "Letzte Nachricht des Kontakts", Vi: "Tin nhắn gần nhất của họ"},
	LastSentMessage:   langcopy.Phrase{En: "Last message to them", De: "Letzte Nachricht an den Kontakt", Vi: "Tin nhắn gần nhất gửi cho họ"},
	Stakeholder:       langcopy.Phrase{En: "a stakeholder", De: "ein beteiligter Kontakt", Vi: "người liên quan"},
	RecordedRole:      langcopy.Phrase{En: "the recorded %s", De: "erfasste Rolle %s", Vi: "được ghi nhận là %s"},
	OpenTask:          langcopy.Phrase{En: "an open task", De: "eine offene Aufgabe", Vi: "một nhiệm vụ đang mở"},
	Write:             langcopy.Phrase{En: "Write to them", De: "Nachricht schreiben", Vi: "Viết cho họ"},
	TeamOwes:          langcopy.Phrase{En: "Your team owes them: %s", De: "Ihr Team schuldet ihnen: %s", Vi: "Nhóm của bạn cần thực hiện cho họ: %s"},
	YouOwe:            langcopy.Phrase{En: "You owe them: %s", De: "Sie schulden ihnen: %s", Vi: "Việc đã hứa với họ: %s"},
	TaskUndated:       langcopy.Phrase{En: "Task from %s with no due date.", De: "Aufgabe vom %s ohne Fälligkeitsdatum.", Vi: "Nhiệm vụ từ ngày %s chưa có hạn."},
	PromisedToday:     langcopy.Phrase{En: "Promised for a date earlier today.", De: "Für einen früheren Zeitpunkt heute zugesagt.", Vi: "Đã cam kết thực hiện vào thời điểm trước đó trong hôm nay."},
	PromisedAgo:       langcopy.Phrase{En: "Promised for a date %d days ago.", De: "Für einen Termin vor %d Tagen zugesagt.", Vi: "Đã cam kết thực hiện cách đây %d ngày."},
	PromisedYesterday: langcopy.Phrase{En: "Promised for yesterday.", De: "Für gestern zugesagt.", Vi: "Đã cam kết thực hiện vào hôm qua."},
	Send:              langcopy.Phrase{En: "Send it", De: "Senden", Vi: "Gửi đi"},
	ClaimUndated:      langcopy.Phrase{En: "No due date agreed.", De: "Kein Fälligkeitsdatum vereinbart.", Vi: "Chưa thống nhất hạn."},
}
