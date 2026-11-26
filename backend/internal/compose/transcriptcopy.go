// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type transcriptCopy struct{ langcopy.Spoken }

func transcriptCopyFor(language string) transcriptCopy { return transcriptCopy{langcopy.For(language)} }

var transcriptWords = struct{ TasksOne, Tasks, TasksYouOne, TasksYou, TasksForOne, TasksFor, ProposalsWaitOne, ProposalsWait, SentToOne, SentTo, AColleague, KnownYouOne, KnownYou, KnownToOne, KnownTo, KnownOne, Known, WatchedOne, Watched, ProposalsOne, Proposals, WithheldOne, Withheld, Empty, AlreadyKnown, History, Incomplete, SourceUnknown, Contradicted, Cancelled, Pending, SourceMissing, Completed, Conditional, Optional, Request, Unclear, NoPromiser, NotTranscript, TooLong, Blank, Missing, Refused, NoModel, Unanswered, Unfinished langcopy.Phrase }{
	TasksOne:         langcopy.Phrase{En: "added %d task", De: "%d Aufgabe erstellt", Vi: "Đã thêm %d công việc"},
	Tasks:            langcopy.Phrase{En: "added %d tasks", De: "%d Aufgaben erstellt", Vi: "Đã thêm %d công việc"},
	TasksYouOne:      langcopy.Phrase{En: "added %d task for you", De: "%d Aufgabe für dich erstellt", Vi: "Đã thêm %d công việc cho bạn"},
	TasksYou:         langcopy.Phrase{En: "added %d tasks for you", De: "%d Aufgaben für dich erstellt", Vi: "Đã thêm %d công việc cho bạn"},
	TasksForOne:      langcopy.Phrase{En: "added %d task for %s", De: "%d Aufgabe für %s erstellt", Vi: "Đã thêm %d công việc cho %s"},
	TasksFor:         langcopy.Phrase{En: "added %d tasks for %s", De: "%d Aufgaben für %s erstellt", Vi: "Đã thêm %d công việc cho %s"},
	ProposalsWaitOne: langcopy.Phrase{En: "staged %d suggestion", De: "%d Vorschlag vorgelegt", Vi: "Đã tạo %d đề xuất"},
	ProposalsWait:    langcopy.Phrase{En: "staged %d suggestions", De: "%d Vorschläge vorgelegt", Vi: "Đã tạo %d đề xuất"},
	SentToOne:        langcopy.Phrase{En: "sent %d suggestion to %s to confirm", De: "%d Vorschlag an %s zur Bestätigung gesendet", Vi: "Đã gửi %d đề xuất cho %s xác nhận"},
	SentTo:           langcopy.Phrase{En: "sent %d suggestions to %s to confirm", De: "%d Vorschläge an %s zur Bestätigung gesendet", Vi: "Đã gửi %d đề xuất cho %s xác nhận"},
	AColleague:       langcopy.Phrase{En: "a colleague", De: "ein Teammitglied", Vi: "một đồng nghiệp"},
	KnownYouOne:      langcopy.Phrase{En: "%d next step was already put to you", De: "%d nächster Schritt wurde dir bereits vorgelegt", Vi: "%d bước tiếp theo đã được trình cho bạn"},
	KnownYou:         langcopy.Phrase{En: "%d next steps were already put to you", De: "%d nächste Schritte wurden dir bereits vorgelegt", Vi: "%d bước tiếp theo đã được trình cho bạn"},
	KnownToOne:       langcopy.Phrase{En: "%d next step was already put to %s", De: "%d nächster Schritt wurde %s bereits vorgelegt", Vi: "%d bước tiếp theo đã được trình cho %s"},
	KnownTo:          langcopy.Phrase{En: "%d next steps were already put to %s", De: "%d nächste Schritte wurden %s bereits vorgelegt", Vi: "%d bước tiếp theo đã được trình cho %s"},
	KnownOne:         langcopy.Phrase{En: "%d next step was already known", De: "%d nächster Schritt war bereits bekannt", Vi: "%d bước tiếp theo đã được biết trước"},
	Known:            langcopy.Phrase{En: "%d next steps were already known", De: "%d nächste Schritte waren bereits bekannt", Vi: "%d bước tiếp theo đã được biết trước"},
	WatchedOne:       langcopy.Phrase{En: "noted %d commitment the customer made", De: "%d Zusage des Kunden erfasst", Vi: "Đã ghi nhận %d cam kết của khách hàng"},
	Watched:          langcopy.Phrase{En: "noted %d commitments the customer made", De: "%d Zusagen des Kunden erfasst", Vi: "Đã ghi nhận %d cam kết của khách hàng"},
	ProposalsOne:     langcopy.Phrase{En: "you were asked to confirm %d suggestion", De: "du wurdest gebeten, %d Vorschlag zu bestätigen", Vi: "Bạn được đề nghị xác nhận %d đề xuất"},
	Proposals:        langcopy.Phrase{En: "you were asked to confirm %d suggestions", De: "du wurdest gebeten, %d Vorschläge zu bestätigen", Vi: "Bạn được đề nghị xác nhận %d đề xuất"},
	WithheldOne:      langcopy.Phrase{En: "%d commitment: %s", De: "%d Zusage: %s", Vi: "%d cam kết: %s"},
	Withheld:         langcopy.Phrase{En: "%d commitments: %s", De: "%d Zusagen: %s", Vi: "%d cam kết: %s"},
	Empty:            langcopy.Phrase{En: "this transcript states no next steps clearly enough to propose one", De: "Dieses Transkript nennt keine nächsten Schritte eindeutig genug für einen Vorschlag.", Vi: "Bản chép lời không nêu rõ bước tiếp theo nào để đề xuất."}, //nolint:misspell // German translation uses German spelling.
	AlreadyKnown:     langcopy.Phrase{En: "every next step this transcript states was already known", De: "Alle nächsten Schritte aus diesem Transkript waren bereits bekannt.", Vi: "Mọi bước tiếp theo trong bản chép lời đã được biết trước."},                           //nolint:misspell // German translation uses German spelling.
	History:          langcopy.Phrase{En: "kept as history without creating new work", De: "als Verlauf gespeichert, ohne neue Arbeit zu erstellen", Vi: "được lưu vào lịch sử, không tạo công việc mới"},
	Incomplete:       langcopy.Phrase{En: "needs later conversation evidence before creating work", De: "benötigt spätere Gesprächsbelege, bevor Arbeit erstellt werden kann", Vi: "cần bằng chứng từ trao đổi sau đó trước khi tạo công việc"},
	SourceUnknown:    langcopy.Phrase{En: "needs a verified source date before creating work", De: "benötigt ein bestätigtes Quelldatum, bevor Arbeit erstellt werden kann", Vi: "cần ngày nguồn đã xác minh trước khi tạo công việc"},
	Contradicted:     langcopy.Phrase{En: "has conflicting message dates; review the source before creating work", De: "enthält widersprüchliche Nachrichtendaten, daher erst die Quelle prüfen, bevor Arbeit erstellt wird", Vi: "có ngày tin nhắn mâu thuẫn; kiểm tra nguồn trước khi tạo công việc"},
	Cancelled:        langcopy.Phrase{En: "belongs to a canceled meeting; no work created", De: "gehört zu einem abgesagten Termin, daher wurde keine Arbeit erstellt", Vi: "thuộc cuộc họp đã hủy; không tạo công việc"},
	Pending:          langcopy.Phrase{En: "belongs to a meeting that has not taken place; no work created", De: "gehört zu einem noch nicht abgehaltenen Termin, daher wurde keine Arbeit erstellt", Vi: "thuộc cuộc họp chưa diễn ra; không tạo công việc"},
	SourceMissing:    langcopy.Phrase{En: "source is no longer available; no work created", De: "Quelle nicht mehr verfügbar, daher wurde keine Arbeit erstellt", Vi: "nguồn không còn khả dụng; không tạo công việc"},
	Completed:        langcopy.Phrase{En: "describes work already completed; no new work created", De: "beschreibt bereits erledigte Arbeit, daher wurde keine neue Arbeit erstellt", Vi: "mô tả công việc đã hoàn thành; không tạo công việc mới"},
	Conditional:      langcopy.Phrase{En: "depends on an unmet condition; no work created", De: "hängt von einer noch offenen Bedingung ab, daher wurde keine Arbeit erstellt", Vi: "phụ thuộc vào điều kiện chưa đáp ứng; không tạo công việc"},
	Optional:         langcopy.Phrase{En: "is an optional offer, not an agreed task", De: "ist ein unverbindliches Angebot, keine vereinbarte Aufgabe", Vi: "là đề nghị tùy chọn, chưa phải công việc đã thống nhất"},
	Request:          langcopy.Phrase{En: "is a request nobody has accepted; no work created", De: "ist eine noch nicht angenommene Anfrage, daher wurde keine Arbeit erstellt", Vi: "là yêu cầu chưa được chấp nhận; không tạo công việc"},
	Unclear:          langcopy.Phrase{En: "does not establish an open commitment; no work created", De: "belegt keine offene Zusage, daher wurde keine Arbeit erstellt", Vi: "chưa xác định được cam kết còn mở; không tạo công việc"},
	NoPromiser:       langcopy.Phrase{En: "names no colleague who made it; kept on the meeting, nobody was asked", De: "nennt niemanden aus dem Team, der die Zusage gemacht hat, daher bleibt sie beim Termin und niemand wurde gefragt", Vi: "không nêu đồng nghiệp đã hứa; giữ lại trên cuộc họp, không hỏi ai"},
	NotTranscript:    langcopy.Phrase{En: "Only an activity logged as a transcript can be read for next steps.", De: "Nur eine als Transkript erfasste Aktivität kann auf nächste Schritte gelesen werden.", Vi: "Chỉ hoạt động được ghi nhận dưới dạng bản chép lời mới có thể được đọc để tìm bước tiếp theo."},                                                                                                                                                                                                                                       //nolint:misspell // German translation uses German spelling.
	TooLong:          langcopy.Phrase{En: "This transcript is %d lines / %d characters, and one reading addresses at most %d lines / %d characters. Log the meeting as more than one transcript and read each.", De: "Dieses Transkript enthält %d Zeilen / %d Zeichen. Ein Lesevorgang erfasst höchstens %d Zeilen / %d Zeichen. Teile den Termin in mehrere Transkripte auf und lies jedes einzeln.", Vi: "Bản chép lời có %d dòng / %d ký tự; mỗi lần đọc xử lý tối đa %d dòng / %d ký tự. Chia cuộc họp thành nhiều bản chép lời và đọc từng bản."}, //nolint:misspell // German translation uses German spelling.
	Blank:            langcopy.Phrase{En: "This transcript is empty, so there is nothing to read.", De: "Dieses Transkript ist leer. Es gibt nichts zu lesen.", Vi: "Bản chép lời trống nên không có nội dung để đọc."},                                                                                                                                                                                                                                                                                                                                 //nolint:misspell // German translation uses German spelling.
	Missing:          langcopy.Phrase{En: "This transcript is no longer available to read.", De: "Dieses Transkript ist nicht mehr zum Lesen verfügbar.", Vi: "Bản chép lời không còn khả dụng để đọc."},                                                                                                                                                                                                                                                                                                                                                //nolint:misspell // German translation uses German spelling.
	Refused:          langcopy.Phrase{En: "The model's reading of this transcript could not be used. The transcript is unchanged and can be read again.", De: "Das Leseergebnis des Modells konnte nicht verwendet werden. Das Transkript ist unverändert und kann erneut gelesen werden.", Vi: "Không sử dụng được kết quả đọc của mô hình. Bản chép lời không thay đổi và có thể đọc lại."},                                                                                                                                                           //nolint:misspell // German translation uses German spelling.
	NoModel:          langcopy.Phrase{En: "AI could not read this transcript because no AI provider is set up. An admin can add one under Settings → AI models → Providers.", De: "Die KI konnte dieses Transkript nicht lesen, da kein KI-Anbieter eingerichtet ist. Ein Admin kann einen unter Einstellungen → KI-Modelle → Anbieter hinzufügen.", Vi: "AI không đọc được bản chép lời này vì chưa thiết lập nhà cung cấp AI. Quản trị viên có thể thêm nhà cung cấp trong Cài đặt → Mô hình AI → Nhà cung cấp."},                                     //nolint:misspell // German translation uses German spelling.
	Unanswered:       langcopy.Phrase{En: "The AI model did not answer after several tries. The transcript is unchanged and can be read again.", De: "Das KI-Modell hat nach mehreren Versuchen nicht geantwortet. Das Transkript ist unverändert und kann erneut gelesen werden.", Vi: "Mô hình AI không phản hồi sau nhiều lần thử; bản chép lời không thay đổi và có thể đọc lại."},                                                                                                                                                                  //nolint:misspell // German translation uses German spelling.
	Unfinished:       langcopy.Phrase{En: "The reading could not finish after several tries. The transcript is unchanged and can be read again.", De: "Das Lesen konnte nach mehreren Versuchen nicht abgeschlossen werden. Das Transkript ist unverändert und kann erneut gelesen werden.", Vi: "Không thể hoàn tất việc đọc sau nhiều lần thử; bản chép lời không thay đổi và có thể đọc lại."},                                                                                                                                                       //nolint:misspell // German translation uses German spelling.
}

func (said transcriptCopy) count(n int, one, many langcopy.Phrase, rest ...any) string {
	phrase := many
	if n == 1 {
		phrase = one
	}
	return fmt.Sprintf(said.Say(phrase), append([]any{n}, rest...)...)
}
