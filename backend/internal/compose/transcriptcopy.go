// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type transcriptCopy struct{ langcopy.Spoken }

func transcriptCopyFor(language string) transcriptCopy { return transcriptCopy{langcopy.For(language)} }

var transcriptWords = struct{ TasksOne, Tasks, WatchedOne, Watched, ProposalsOne, Proposals, Colleague, WithheldOne, Withheld, Empty, AlreadyKnown, History, Incomplete, SourceUnknown, Contradicted, Cancelled, Pending, SourceMissing, Completed, Conditional, Optional, Request, Unclear, NoPromiser, NotTranscript, TooLong, Blank, Missing, Refused, NoModel langcopy.Phrase }{
	TasksOne:      langcopy.Phrase{En: "added %d task for %s", De: "%d Aufgabe für %s erstellt", Vi: "Đã thêm %d công việc cho %s"},
	Tasks:         langcopy.Phrase{En: "added %d tasks for %s", De: "%d Aufgaben für %s erstellt", Vi: "Đã thêm %d công việc cho %s"},
	WatchedOne:    langcopy.Phrase{En: "noted %d commitment the customer made", De: "%d Zusage des Kunden erfasst", Vi: "Đã ghi nhận %d cam kết của khách hàng"},
	Watched:       langcopy.Phrase{En: "noted %d commitments the customer made", De: "%d Zusagen des Kunden erfasst", Vi: "Đã ghi nhận %d cam kết của khách hàng"},
	ProposalsOne:  langcopy.Phrase{En: "sent %d suggestion to %s to confirm", De: "%d Vorschlag zur Bestätigung an %s gesendet", Vi: "Đã gửi %d đề xuất đến %s để xác nhận"},
	Colleague:     langcopy.Phrase{En: "a colleague", De: "ein Teammitglied", Vi: "một đồng nghiệp"},
	Proposals:     langcopy.Phrase{En: "sent %d suggestions to %s to confirm", De: "%d Vorschläge zur Bestätigung an %s gesendet", Vi: "Đã gửi %d đề xuất đến %s để xác nhận"},
	WithheldOne:   langcopy.Phrase{En: "%d commitment: %s", De: "%d Zusage: %s", Vi: "%d cam kết: %s"},
	Withheld:      langcopy.Phrase{En: "%d commitments: %s", De: "%d Zusagen: %s", Vi: "%d cam kết: %s"},
	Empty:         langcopy.Phrase{En: "this transcript states no next steps clearly enough to propose one", De: "Dieses Transkript nennt keine nächsten Schritte eindeutig genug für einen Vorschlag.", Vi: "Bản chép lời không nêu rõ bước tiếp theo nào để đề xuất."},  //nolint:misspell // German translation uses German spelling.
	AlreadyKnown:  langcopy.Phrase{En: "every next step this transcript states has already been put to you", De: "Alle nächsten Schritte aus diesem Transkript wurden dir bereits vorgelegt.", Vi: "Mọi bước tiếp theo trong bản chép lời đã được trình bày để xem xét."}, //nolint:misspell // German translation uses German spelling.
	History:       langcopy.Phrase{En: "kept as history without creating new work", De: "als Verlauf gespeichert, ohne neue Arbeit zu erstellen", Vi: "được lưu vào lịch sử, không tạo công việc mới"},
	Incomplete:    langcopy.Phrase{En: "needs later conversation evidence before creating work", De: "benötigt spätere Gesprächsbelege, bevor Arbeit erstellt werden kann", Vi: "cần bằng chứng từ trao đổi sau đó trước khi tạo công việc"},
	SourceUnknown: langcopy.Phrase{En: "needs a verified source date before creating work", De: "benötigt ein bestätigtes Quelldatum, bevor Arbeit erstellt werden kann", Vi: "cần ngày nguồn đã xác minh trước khi tạo công việc"},
	Contradicted:  langcopy.Phrase{En: "has conflicting message dates; review the source before creating work", De: "enthält widersprüchliche Nachrichtendaten; prüfe die Quelle, bevor Arbeit erstellt wird", Vi: "có ngày tin nhắn mâu thuẫn; kiểm tra nguồn trước khi tạo công việc"},
	Cancelled:     langcopy.Phrase{En: "belongs to a canceled meeting; no work created", De: "gehört zu einem abgesagten Termin; keine Arbeit erstellt", Vi: "thuộc cuộc họp đã hủy; không tạo công việc"},
	Pending:       langcopy.Phrase{En: "belongs to a meeting that has not taken place; no work created", De: "gehört zu einem noch nicht abgehaltenen Termin; keine Arbeit erstellt", Vi: "thuộc cuộc họp chưa diễn ra; không tạo công việc"},
	SourceMissing: langcopy.Phrase{En: "source is no longer available; no work created", De: "Quelle nicht mehr verfügbar; keine Arbeit erstellt", Vi: "nguồn không còn khả dụng; không tạo công việc"},
	Completed:     langcopy.Phrase{En: "describes work already completed; no new work created", De: "beschreibt bereits erledigte Arbeit; keine neue Arbeit erstellt", Vi: "mô tả công việc đã hoàn thành; không tạo công việc mới"},
	Conditional:   langcopy.Phrase{En: "depends on an unmet condition; no work created", De: "hängt von einer noch offenen Bedingung ab; keine Arbeit erstellt", Vi: "phụ thuộc vào điều kiện chưa đáp ứng; không tạo công việc"},
	Optional:      langcopy.Phrase{En: "is an optional offer, not an agreed task", De: "ist ein unverbindliches Angebot, keine vereinbarte Aufgabe", Vi: "là đề nghị tùy chọn, chưa phải công việc đã thống nhất"},
	Request:       langcopy.Phrase{En: "is a request nobody has accepted; no work created", De: "ist eine noch nicht angenommene Anfrage; keine Arbeit erstellt", Vi: "là yêu cầu chưa được chấp nhận; không tạo công việc"},
	Unclear:       langcopy.Phrase{En: "does not establish an open commitment; no work created", De: "belegt keine offene Zusage; keine Arbeit erstellt", Vi: "chưa xác định được cam kết còn mở; không tạo công việc"},
	NoPromiser:    langcopy.Phrase{En: "names no colleague who made it; kept on the meeting, nobody was asked", De: "nennt niemanden aus dem Team, der die Zusage gemacht hat; bleibt beim Termin, niemand wurde gefragt", Vi: "không nêu đồng nghiệp đã hứa; giữ lại trên cuộc họp, không hỏi ai"},
	NotTranscript: langcopy.Phrase{En: "only an activity logged with source_system=transcript can be read for next steps", De: "Nur eine als Transkript erfasste Aktivität kann auf nächste Schritte gelesen werden.", Vi: "Chỉ hoạt động được ghi nhận dưới dạng bản chép lời mới có thể được đọc để tìm bước tiếp theo."},                                                                                                                                                                                                                         //nolint:misspell // German translation uses German spelling.
	TooLong:       langcopy.Phrase{En: "this transcript is %d lines / %d characters, and one reading addresses at most %d lines / %d characters; log the meeting as more than one transcript and read each", De: "Dieses Transkript enthält %d Zeilen / %d Zeichen. Ein Lesevorgang erfasst höchstens %d Zeilen / %d Zeichen. Teile den Termin in mehrere Transkripte auf und lies jedes einzeln.", Vi: "Bản chép lời có %d dòng / %d ký tự; mỗi lần đọc xử lý tối đa %d dòng / %d ký tự. Chia cuộc họp thành nhiều bản chép lời và đọc từng bản."}, //nolint:misspell // German translation uses German spelling.
	Blank:         langcopy.Phrase{En: "this transcript is empty, so there is nothing to read", De: "Dieses Transkript ist leer. Es gibt nichts zu lesen.", Vi: "Bản chép lời trống nên không có nội dung để đọc."},                                                                                                                                                                                                                                                                                                                                 //nolint:misspell // German translation uses German spelling.
	Missing:       langcopy.Phrase{En: "this transcript is no longer available to read", De: "Dieses Transkript ist nicht mehr zum Lesen verfügbar.", Vi: "Bản chép lời không còn khả dụng để đọc."},                                                                                                                                                                                                                                                                                                                                                //nolint:misspell // German translation uses German spelling.
	Refused:       langcopy.Phrase{En: "the model's reading of this transcript could not be used; the transcript is unchanged and can be read again", De: "Das Leseergebnis des Modells konnte nicht verwendet werden. Das Transkript ist unverändert und kann erneut gelesen werden.", Vi: "Không sử dụng được kết quả đọc của mô hình. Bản chép lời không thay đổi và có thể đọc lại."},                                                                                                                                                           //nolint:misspell // German translation uses German spelling.
	NoModel:       langcopy.Phrase{En: "this installation has no AI model configured for reading transcripts", De: "Es ist kein KI-Modell zum Lesen von Transkripten konfiguriert.", Vi: "Chưa cấu hình mô hình AI để đọc bản chép lời."},                                                                                                                                                                                                                                                                                                           //nolint:misspell // German translation uses German spelling.
}

func (said transcriptCopy) count(n int, one, many langcopy.Phrase, rest ...any) string {
	phrase := many
	if n == 1 {
		phrase = one
	}
	return fmt.Sprintf(said.Say(phrase), append([]any{n}, rest...)...)
}
