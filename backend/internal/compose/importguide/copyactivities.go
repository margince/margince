// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// withActivityColumns adds the activity columns to the guide the other objects
// declare, and answers it. They sit apart to keep each file under the length cap.
func withActivityColumns(columns map[string]Column) map[string]Column {
	maps.Copy(columns, activityColumns)
	return columns
}

var activityColumns = map[string]Column{
	"type": {Description: langcopy.Phrase{
		En: "What the activity is: note, call, meeting or email. A message needs a channel and a task is a to-do, so neither is imported.",
		De: "Was die Aktivität ist: note, call, meeting oder email. Eine Nachricht braucht einen Kanal und eine Aufgabe ist eine To-do, daher wird keine von beiden importiert.",
		Vi: "Loại hoạt động: note, call, meeting hoặc email. Tin nhắn cần một kênh và nhiệm vụ là việc cần làm, nên cả hai đều không được nhập.",
	}, Example: same("note · call · meeting · email"), RequiredOnNew: true},
	"subject": {Description: langcopy.Phrase{
		En: "A short title for the activity, such as an email's subject line.",
		De: "Ein kurzer Titel für die Aktivität, etwa die Betreffzeile einer E-Mail.",
		Vi: "Tiêu đề ngắn của hoạt động, chẳng hạn dòng chủ đề của một email.",
	}, Example: langcopy.Phrase{En: "Pricing call", De: "Preisgespräch", Vi: "Cuộc gọi về giá"}},
	"body": {Description: langcopy.Phrase{
		En: "The text of the note, or what was said in the call, meeting or email.",
		De: "Der Text der Notiz oder was im Anruf, Termin oder in der E-Mail gesagt wurde.",
		Vi: "Nội dung ghi chú, hoặc những gì đã nói trong cuộc gọi, cuộc họp hoặc email.",
	}, Example: langcopy.Phrase{En: "Asked for a quote by Friday", De: "Wünscht ein Angebot bis Freitag", Vi: "Yêu cầu báo giá trước thứ Sáu"}},
	"occurred_at": {Description: langcopy.Phrase{
		En: "When it happened: YYYY-MM-DD, or a full timestamp with its time zone. A date alone means midnight UTC. A time without a zone, or in the future, is refused.",
		De: "Wann es stattfand: YYYY-MM-DD oder ein vollständiger Zeitstempel mit Zeitzone. Ein Datum allein bedeutet Mitternacht UTC. Eine Uhrzeit ohne Zeitzone oder in der Zukunft wird abgelehnt.",
		Vi: "Thời điểm xảy ra: YYYY-MM-DD, hoặc mốc thời gian đầy đủ kèm múi giờ. Chỉ có ngày nghĩa là nửa đêm UTC. Giờ không có múi giờ hoặc ở tương lai sẽ bị từ chối.",
	}, Example: same("2026-03-02 · 2026-03-02T10:00:00+07:00"), RequiredOnNew: true},
	"links": {Description: langcopy.Phrase{
		En: "The records the activity is about, as type:key pairs separated by semicolons. The type is company, contact, lead or deal; the key is a ref, an email, a domain or a Margince ID. A call or meeting links to a contact, not a company.",
		De: "Die Datensätze, um die es in der Aktivität geht, als Paare Typ:Schlüssel, durch Semikolons getrennt. Der Typ ist company, contact, lead oder deal; der Schlüssel ist eine Ref, eine E-Mail-Adresse, eine Domain oder eine Margince-ID. Ein Anruf oder Termin wird mit einem Kontakt verknüpft, nicht mit einem Unternehmen.",
		Vi: "Các hồ sơ mà hoạt động nói đến, dạng cặp loại:khóa, phân cách bằng dấu chấm phẩy. Loại là company, contact, lead hoặc deal; khóa là ref, email, tên miền hoặc ID Margince. Cuộc gọi hoặc cuộc họp gắn với liên hệ, không gắn với công ty.",
	}, Example: same("deal:D-1001; contact:jo@acme.com"), RequiredOnNew: true},
	"duration_minutes": {Description: langcopy.Phrase{
		En: "How long a call or meeting lasted, in whole minutes. Not for a note or an email.",
		De: "Wie lange ein Anruf oder Termin dauerte, in ganzen Minuten. Nicht für Notiz oder E-Mail.",
		Vi: "Cuộc gọi hoặc cuộc họp kéo dài bao lâu, tính bằng phút nguyên. Không dùng cho ghi chú hoặc email.",
	}, Example: same("30")},
	"meeting_status": {Description: langcopy.Phrase{
		En: "How a meeting went: booked, held, no_show or canceled. Only for a meeting. A no_show or canceled meeting does not count as the last contact.",
		De: "Wie ein Termin verlief: booked, held, no_show oder canceled. Nur für einen Termin. Ein Termin mit no_show oder canceled zählt nicht als letzter Kontakt.",
		Vi: "Cuộc họp diễn ra thế nào: booked, held, no_show hoặc canceled. Chỉ dùng cho cuộc họp. Cuộc họp no_show hoặc canceled không tính là lần liên hệ gần nhất.",
	}, Example: same("held")},
	"direction": {Description: langcopy.Phrase{
		En: "Which way it went: inbound if the contact reached you, outbound if you reached them. Written only when a row creates the activity.",
		De: "In welche Richtung es ging: inbound, wenn der Kontakt dich erreicht hat, outbound, wenn du ihn erreicht hast. Wird nur geschrieben, wenn eine Zeile die Aktivität anlegt.",
		Vi: "Chiều của hoạt động: inbound nếu liên hệ tìm bạn, outbound nếu bạn tìm họ. Chỉ được ghi khi dòng tạo hoạt động mới.",
	}, Example: same("outbound")},
	"activity.owner_email": {Description: langcopy.Phrase{
		En: "Email of the Margince user the activity is attributed to, and who hosts a meeting. If no user has this email, you become the author and the import preview says so. Written only when a row creates the activity.",
		De: "E-Mail-Adresse des Mitglieds, dem die Aktivität zugeschrieben wird und das einen Termin leitet. Hat kein Mitglied diese Adresse, wirst du Verfasser, und die Importvorschau weist darauf hin. Wird nur geschrieben, wenn eine Zeile die Aktivität anlegt.",
		Vi: "Email của thành viên Margince được ghi là tác giả hoạt động và là chủ trì cuộc họp. Nếu không thành viên nào có email này, bạn sẽ là tác giả và bản xem trước lần nhập sẽ báo điều này. Chỉ được ghi khi dòng tạo hoạt động mới.",
	}, Example: same("mai@yourcompany.com")},
}
