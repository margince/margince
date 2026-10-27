// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type statusCopy struct{ langcopy.Spoken }

func statusCopyFor(language string) statusCopy { return statusCopy{langcopy.For(language)} }

// English task subjects also define stored dismissal identities; their digests are pinned in tests.
var statusWords = struct {
	Call, Email, Message, Note, Task langcopy.Phrase
	Story                            langcopy.Phrase
	LastContact                      langcopy.Phrase
	NoContact                        langcopy.Phrase
	OpenTasksAtLeast                 langcopy.Phrase
	OpenTaskOne                      langcopy.Phrase
	OpenTasks                        langcopy.Phrase
	Meeting                          langcopy.Phrase
	Booked                           langcopy.Phrase
	Prepare                          langcopy.Phrase
	FarMeeting                       langcopy.Phrase
	Complete                         langcopy.Phrase
	Reminder                         langcopy.Phrase
	Request                          langcopy.Phrase
	OwnReply                         langcopy.Phrase
	ReviewReply                      langcopy.Phrase
	Closed                           langcopy.Phrase
	Agree                            langcopy.Phrase
	Book                             langcopy.Phrase
	BookNamed                        langcopy.Phrase
	BookRole                         langcopy.Phrase
	Never                            langcopy.Phrase
	OpeningOne                       langcopy.Phrase
	OpeningMany                      langcopy.Phrase
	SeatRole                         langcopy.Phrase
	SeatNamed                        langcopy.Phrase
	NoContactNext                    langcopy.Phrase
	QuietNext                        langcopy.Phrase
	LastEvidence                     langcopy.Phrase
	FollowUp                         langcopy.Phrase
	Met                              langcopy.Phrase
	Today                            langcopy.Phrase
	Tomorrow                         langcopy.Phrase
	Yesterday                        langcopy.Phrase
	InDays                           langcopy.Phrase
	DaysAgo                          langcopy.Phrase
	Open                             langcopy.Phrase
	Won                              langcopy.Phrase
	Lost                             langcopy.Phrase
}{
	Call:             langcopy.Phrase{En: "call", De: "Anruf", Vi: "cuộc gọi"},
	Email:            langcopy.Phrase{En: "email", De: "E-Mail", Vi: "email"},
	Message:          langcopy.Phrase{En: "message", De: "Nachricht", Vi: "tin nhắn"},
	Note:             langcopy.Phrase{En: "note", De: "Notiz", Vi: "ghi chú"},
	Task:             langcopy.Phrase{En: "task", De: "Aufgabe", Vi: "nhiệm vụ"},
	Story:            langcopy.Phrase{En: "%s is %s.", De: "%s ist %s.", Vi: "%s đang ở trạng thái %s."},
	LastContact:      langcopy.Phrase{En: "The last recorded exchange on this deal was %s: %s.", De: "Der letzte erfasste Austausch zu diesem Deal war %s: %s.", Vi: "Trao đổi gần nhất được ghi nhận trong deal này là %s: %s."},
	NoContact:        langcopy.Phrase{En: "No exchange is shown in this deal’s recent history.", De: "Im jüngsten Verlauf dieses Deals ist kein Austausch erfasst.", Vi: "Chưa có trao đổi nào được hiển thị trong lịch sử gần đây của deal này."},
	OpenTasksAtLeast: langcopy.Phrase{En: "At least %d tasks are still open on this deal.", De: "Für diesen Deal sind noch mindestens %d Aufgaben offen.", Vi: "Giao dịch này còn ít nhất %d nhiệm vụ đang mở."},
	OpenTaskOne:      langcopy.Phrase{En: "One task is still open on this deal.", De: "Für diesen Deal ist noch eine Aufgabe offen.", Vi: "Giao dịch này còn một nhiệm vụ đang mở."},
	OpenTasks:        langcopy.Phrase{En: "%d tasks are still open on this deal.", De: "Für diesen Deal sind noch %d Aufgaben offen.", Vi: "Giao dịch này còn %d nhiệm vụ đang mở."},
	Meeting:          langcopy.Phrase{En: "meeting", De: "Besprechung", Vi: "cuộc họp"},
	Booked:           langcopy.Phrase{En: "Booked: %s", De: "Geplant: %s", Vi: "Đã lên lịch: %s"},
	Prepare:          langcopy.Phrase{En: "A meeting is booked %s — read the brief before it.", De: "Eine Besprechung ist %s geplant – lesen Sie vorher die Vorbereitung.", Vi: "Cuộc họp đã được lên lịch %s – hãy đọc bản chuẩn bị trước đó."},
	FarMeeting:       langcopy.Phrase{En: "A meeting is booked %s — nothing is owed before it.", De: "Eine Besprechung ist %s geplant – bis dahin steht nichts an.", Vi: "Cuộc họp đã được lên lịch %s – không có việc cần làm trước đó."},
	Complete:         langcopy.Phrase{En: "Complete the existing task: %s", De: "Erledigen Sie die bestehende Aufgabe: %s", Vi: "Hoàn thành nhiệm vụ hiện có: %s"},
	Reminder:         langcopy.Phrase{En: "This request already has a reminder. The reply still needs to be handled.", De: "Für diese Anfrage gibt es bereits eine Erinnerung. Die Antwort steht noch aus.", Vi: "Yêu cầu này đã có lời nhắc. Vẫn cần xử lý câu trả lời."},
	Request:          langcopy.Phrase{En: "Request: %s", De: "Anfrage: %s", Vi: "Yêu cầu: %s"},
	OwnReply:         langcopy.Phrase{En: "Review and take responsibility for the reply still owed: %s", De: "Prüfen und übernehmen Sie die noch ausstehende Antwort: %s", Vi: "Xem xét và nhận trách nhiệm cho câu trả lời còn thiếu: %s"},
	ReviewReply:      langcopy.Phrase{En: "Review whether this conversation needs a follow-up: %s", De: "Prüfen Sie, ob dieses Gespräch eine Rückmeldung braucht: %s", Vi: "Xem xét liệu cuộc trao đổi này có cần phản hồi tiếp hay không: %s"},
	Closed:           langcopy.Phrase{En: "This deal is %s — there is no next step to take.", De: "Dieser Deal ist %s – es steht kein nächster Schritt an.", Vi: "Giao dịch này ở trạng thái %s – không có bước tiếp theo cần thực hiện."},
	Agree:            langcopy.Phrase{En: "Agree the next step on %s", De: "Vereinbaren Sie den nächsten Schritt für %s", Vi: "Thống nhất bước tiếp theo cho %s"},
	Book:             langcopy.Phrase{En: "The last contact was %s and nothing is booked. Book a meeting with %s.", De: "Der letzte Kontakt war %s und es ist nichts geplant. Vereinbaren Sie eine Besprechung mit %s.", Vi: "Lần liên hệ gần nhất là %s và chưa có lịch hẹn. Hãy đặt lịch họp với %s."},
	BookNamed:        langcopy.Phrase{En: "Book a meeting with %s", De: "Besprechung mit %s vereinbaren", Vi: "Đặt lịch họp với %s"},
	BookRole:         langcopy.Phrase{En: "Book a meeting with the %s on %s", De: "Besprechung mit dem zuständigen Kontakt (%s) für %s vereinbaren", Vi: "Đặt lịch họp với vai trò %s cho %s"},
	Never:            langcopy.Phrase{En: "never", De: "noch nie", Vi: "chưa từng"},
	OpeningOne:       langcopy.Phrase{En: "Nobody has been contacted yet. Open with %s.", De: "Es gab noch keinen Kontakt. Beginnen Sie mit %s.", Vi: "Chưa liên hệ với ai. Hãy bắt đầu với %s."},
	OpeningMany:      langcopy.Phrase{En: "%d contacts are named on this deal and none has been contacted. Open with %s.", De: "Für diesen Deal sind %d Kontakte erfasst, aber noch keiner wurde kontaktiert. Beginnen Sie mit %s.", Vi: "Giao dịch này có %d liên hệ nhưng chưa liên hệ với ai. Hãy bắt đầu với %s."},
	SeatRole:         langcopy.Phrase{En: "the %s", De: "dem zuständigen Kontakt (%s)", Vi: "vai trò %s"},
	SeatNamed:        langcopy.Phrase{En: "%s, the %s", De: "%s (%s)", Vi: "%s, vai trò %s"},
	NoContactNext:    langcopy.Phrase{En: "No exchange is shown in this deal’s recent history. Agree the next step.", De: "Im jüngsten Verlauf dieses Deals ist kein Austausch erfasst. Nächsten Schritt vereinbaren.", Vi: "Chưa có trao đổi nào được hiển thị trong lịch sử gần đây của deal này. Thống nhất bước tiếp theo."},
	QuietNext:        langcopy.Phrase{En: "The last recorded exchange on this deal was %s. Nothing is booked. Agree the next step.", De: "Der letzte erfasste Austausch zu diesem Deal war %s. Es ist nichts geplant. Nächsten Schritt vereinbaren.", Vi: "Trao đổi gần nhất được ghi nhận trong deal này là %s. Chưa có lịch hẹn. Thống nhất bước tiếp theo."},
	LastEvidence:     langcopy.Phrase{En: "Last recorded exchange on this deal: %s", De: "Letzter erfasster Austausch zu diesem Deal: %s", Vi: "Trao đổi gần nhất được ghi nhận trong deal này: %s"},
	FollowUp:         langcopy.Phrase{En: "You met %s and nothing has been sent since. Draft the follow-up.", De: "Die Besprechung war %s und seitdem wurde nichts gesendet. Entwerfen Sie die Rückmeldung.", Vi: "Cuộc họp diễn ra %s và từ đó chưa gửi gì. Hãy soạn thư phản hồi."},
	Met:              langcopy.Phrase{En: "Met: %s", De: "Besprechung: %s", Vi: "Đã họp: %s"},
	Today:            langcopy.Phrase{En: "today", De: "heute", Vi: "hôm nay"},
	Tomorrow:         langcopy.Phrase{En: "tomorrow", De: "morgen", Vi: "ngày mai"},
	Yesterday:        langcopy.Phrase{En: "yesterday", De: "gestern", Vi: "hôm qua"},
	InDays:           langcopy.Phrase{En: "in %d days", De: "in %d Tagen", Vi: "sau %d ngày"},
	DaysAgo:          langcopy.Phrase{En: "%d days ago", De: "vor %d Tagen", Vi: "%d ngày trước"},
	Open:             langcopy.Phrase{En: "open", De: "offen", Vi: "đang mở"},
	Won:              langcopy.Phrase{En: "won", De: "gewonnen", Vi: "thành công"},
	Lost:             langcopy.Phrase{En: "lost", De: "verloren", Vi: "thất bại"},
}

func (said statusCopy) dealStatus(value string) string {
	switch value {
	case "open":
		return said.Say(statusWords.Open)
	case "won":
		return said.Say(statusWords.Won)
	case "lost":
		return said.Say(statusWords.Lost)
	default:
		return value
	}
}
