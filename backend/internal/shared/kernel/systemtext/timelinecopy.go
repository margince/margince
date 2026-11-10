// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var timelineDefinitions = map[string]Definition{
	"activity.source_body":             {Words: langcopy.Phrase{En: "%s", De: "%s", Vi: "%s"}, Arguments: []string{argumentText}},
	"activity.html_alternative":        {Words: langcopy.Phrase{En: "This message was also sent as HTML.", De: "Diese Nachricht wurde auch als HTML gesendet.", Vi: "Tin nhắn này cũng được gửi dưới dạng HTML."}},
	"activity.html_alternative_marker": {Words: langcopy.Phrase{En: bracketedText, De: bracketedText, Vi: bracketedText}, Arguments: []string{argumentText}},

	"activity.access_requested":   {Words: langcopy.Phrase{En: "Access requested: %s", De: "Zugriff angefordert: %s", Vi: "Đã yêu cầu quyền truy cập: %s"}, Arguments: []string{argumentText}},
	"commitment.transcript_line":  {Words: langcopy.Phrase{En: "Promiser: %s. Commitment in the meeting transcript (line %s).", De: "Zugesagt von: %s. Zusage im Besprechungsprotokoll (Zeile %s).", Vi: "Người cam kết: %s. Cam kết trong bản ghi cuộc họp (dòng %s)."}, Arguments: []string{argumentText, argumentText}},
	"commitment.transcript_lines": {Words: langcopy.Phrase{En: "Promiser: %s. Commitment in the meeting transcript (lines %s).", De: "Zugesagt von: %s. Zusage im Besprechungsprotokoll (Zeilen %s).", Vi: "Người cam kết: %s. Cam kết trong bản ghi cuộc họp (các dòng %s)."}, Arguments: []string{argumentText, argumentText}},

	"room.seller":           {Words: langcopy.Phrase{En: "The seller", De: "Der Verkäufer", Vi: "Bên bán"}, Arguments: []string{}},
	"room.buyer":            {Words: langcopy.Phrase{En: "The buyer", De: "Der Käufer", Vi: "Bên mua"}, Arguments: []string{}},
	"room.reply":            {Words: langcopy.Phrase{En: "%s replied in the Deal Room", De: "%s hat im Deal Room geantwortet", Vi: "%s đã trả lời trong Deal Room"}, Arguments: []string{argumentText}},
	"room.question":         {Words: langcopy.Phrase{En: "%s asked a question in the Deal Room", De: "%s hat im Deal Room eine Frage gestellt", Vi: "%s đã đặt câu hỏi trong Deal Room"}, Arguments: []string{argumentText}},
	"room.whole":            {Words: langcopy.Phrase{En: "About the room as a whole.", De: "Zum gesamten Raum.", Vi: "Về toàn bộ phòng."}, Arguments: []string{}},
	"room.document":         {Words: langcopy.Phrase{En: "About a document in the room.", De: "Zu einem Dokument im Raum.", Vi: "Về một tài liệu trong phòng."}, Arguments: []string{}},
	"room.named_document":   {Words: langcopy.Phrase{En: "About %s.", De: "Zu %s.", Vi: "Về %s."}, Arguments: []string{argumentText}},
	"room.changes":          {Words: langcopy.Phrase{En: "The buyer asked for changes to a Deal Room document", De: "Der Käufer hat Änderungen an einem Deal-Room-Dokument angefragt", Vi: "Bên mua đã yêu cầu sửa tài liệu trong Deal Room"}, Arguments: []string{}},
	"room.confirmed":        {Words: langcopy.Phrase{En: "The buyer confirmed a Deal Room document", De: "Der Käufer hat ein Deal-Room-Dokument bestätigt", Vi: "Bên mua đã xác nhận tài liệu trong Deal Room"}, Arguments: []string{}},
	"room.decision":         {Words: langcopy.Phrase{En: "A working decision inside the room, not a signature.", De: "Eine Arbeitsentscheidung im Raum, keine Unterschrift.", Vi: "Đây là quyết định làm việc trong phòng, không phải chữ ký."}, Arguments: []string{}},
	"extraction.accepted":   {Words: langcopy.Phrase{En: "Extraction accepted: %s", De: "Übernommene Extraktion: %s", Vi: "Đã chấp nhận trích xuất: %s"}, Arguments: []string{argumentText}},
	"lead.unnamed":          {Words: langcopy.Phrase{En: "A lead", De: "Ein Lead", Vi: "Một khách hàng tiềm năng"}, Arguments: []string{}},
	"lead.arrival_name":     {Words: langcopy.Phrase{En: "Name: %s", De: "Name: %s", Vi: "Tên: %s"}, Arguments: []string{argumentText}},
	"lead.arrival_email":    {Words: langcopy.Phrase{En: "Email: %s", De: "E-Mail: %s", Vi: "Email: %s"}, Arguments: []string{argumentText}},
	"lead.arrival_phone":    {Words: langcopy.Phrase{En: "Phone: %s", De: "Telefon: %s", Vi: "Điện thoại: %s"}, Arguments: []string{argumentText}},
	"lead.arrival_title":    {Words: langcopy.Phrase{En: "Title: %s", De: "Position: %s", Vi: "Chức danh: %s"}, Arguments: []string{argumentText}},
	"lead.arrival_company":  {Words: langcopy.Phrase{En: "Company: %s", De: "Unternehmen: %s", Vi: "Công ty: %s"}, Arguments: []string{argumentText}},
	"lead.arrival_linkedin": {Words: langcopy.Phrase{En: linkedInText, De: linkedInText, Vi: linkedInText}, Arguments: []string{argumentText}},
	"lead.arrival_origin":   {Words: langcopy.Phrase{En: "Came from: %s", De: "Herkunft: %s", Vi: "Nguồn: %s"}, Arguments: []string{argumentText}},
	"lead.arrival_next":     {Words: langcopy.Phrase{En: "No lead was created. If this is a new opportunity, open a deal with the contact.", De: "Es wurde kein Lead angelegt. Wenn dies eine neue Verkaufschance ist, lege einen Deal mit dem Kontakt an.", Vi: "Không tạo khách hàng tiềm năng. Nếu đây là cơ hội mới, hãy mở deal với liên hệ này."}, Arguments: []string{}},
}
