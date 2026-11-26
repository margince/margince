// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var approvalDefinitions = map[string]Definition{
	"capture.duplicateLead": {Words: langcopy.Phrase{En: "Captured %s/%s duplicates an existing lead", De: "Der erfasste Datensatz %s/%s ist ein Duplikat eines bestehenden Leads", Vi: "Hồ sơ thu thập %s/%s trùng với một lead hiện có"}, Arguments: []string{argumentText, argumentText}},

	"approval.taskCompleted":                {Words: langcopy.Phrase{En: "Done because you wrote to %s on %s", De: "Erledigt, weil du %s am %s geschrieben hast", Vi: "Đã hoàn tất vì bạn đã gửi email cho %s vào %s"}, Arguments: []string{argumentText, argumentText}},
	"approval.taskBooked":                   {Words: langcopy.Phrase{En: "Done because %s booked a meeting for %s", De: "Erledigt, weil %s ein Meeting für %s gebucht hat", Vi: "Đã hoàn tất vì %s đã đặt cuộc họp vào %s"}, Arguments: []string{argumentText, argumentText}},
	"approval.taskChatDone":                 {Words: langcopy.Phrase{En: "Done because the chat said “%s”", De: "Erledigt, weil im Chat stand: „%s“", Vi: "Đã hoàn tất vì cuộc trò chuyện có câu “%s”"}, Arguments: []string{argumentText}},
	"approval.taskChatMoved":                {Words: langcopy.Phrase{En: "Moved to %s because the chat said “%s”", De: "Auf %s verschoben, weil im Chat stand: „%s“", Vi: "Đã dời sang %s vì cuộc trò chuyện có câu “%s”"}, Arguments: []string{argumentText, argumentText}},
	"approval.taskChatDoneUnquoted":         {Words: langcopy.Phrase{En: "Done because of a filed chat", De: "Erledigt wegen eines abgelegten Chats", Vi: "Đã hoàn tất nhờ một cuộc trò chuyện đã lưu"}, Arguments: []string{}},
	"approval.taskChatMovedUnquoted":        {Words: langcopy.Phrase{En: "Moved to %s because of a filed chat", De: "Wegen eines abgelegten Chats auf %s verschoben", Vi: "Đã dời sang %s nhờ một cuộc trò chuyện đã lưu"}, Arguments: []string{argumentText}},
	"approval.taskContact":                  {Words: langcopy.Phrase{En: "the contact", De: "dem Kontakt", Vi: "liên hệ"}, Arguments: []string{}},
	"approval.taskBooker":                   {Words: langcopy.Phrase{En: "the contact", De: "der Kontakt", Vi: "liên hệ"}, Arguments: []string{}},
	"approval.signaturePrivate":             {Words: langcopy.Phrase{En: "The sender’s private signature cannot be shown to this reviewer. Ask the sender to review this message.", De: "Die private Signatur des Absenders ist für diese Prüfung nicht freigegeben. Den Absender um Prüfung dieser Nachricht bitten.", Vi: "Không thể hiển thị chữ ký riêng của người gửi cho người xem xét này. Hãy yêu cầu người gửi xem xét tin nhắn."}, Arguments: []string{}},
	"approval.draftedReplyWaiting":          {Words: langcopy.Phrase{En: "A reply to “%s” is drafted and waiting to be sent: the thread left no next step planned", De: "Eine Antwort auf „%s“ ist entworfen und wartet auf den Versand: Das Gespräch endete ohne geplanten nächsten Schritt", Vi: "Một thư trả lời cho “%s” đã được soạn và đang chờ gửi: chuỗi thư chưa có bước tiếp theo nào được lên kế hoạch"}, Arguments: []string{argumentText}, Retired: wordings{textlang.English: {"A reply to “%s” is drafted and waiting to be sent — the conversation left no next step planned"}}},
	"approval.why.signature_name":           {Words: langcopy.Phrase{En: "An email signature spells the company “%s”, and no second source confirms it yet.", De: "Eine E-Mail-Signatur schreibt die Firma „%s“, und noch keine zweite Quelle bestätigt das.", Vi: "Một chữ ký email ghi tên công ty là “%s”, và chưa có nguồn thứ hai nào xác nhận."}, Arguments: []string{argumentText}},
	"approval.why.reply_owed":               {Words: langcopy.Phrase{En: "The last message in “%s” came on %s, and nothing was planned after it.", De: "Die letzte Nachricht in „%s“ kam am %s, und danach wurde nichts geplant.", Vi: "Thư gần nhất trong “%s” đến vào ngày %s, và sau đó chưa có kế hoạch nào."}, Arguments: []string{argumentText, argumentDate}},
	"approval.counterpartyWorthKeeping":     {Words: langcopy.Phrase{En: "Is %s a contact worth keeping?", De: "Lohnt es sich, %s als Kontakt zu behalten?", Vi: "%s có phải là một liên hệ đáng giữ lại không?"}, Arguments: []string{argumentText}},
	"approval.contactShareAsked":            {Words: langcopy.Phrase{En: "%s asks you to share this contact. Approving gives write access, or read access when their seat or role cannot edit contacts.", De: "%s bittet dich, diesen Kontakt zu teilen. Wenn du zustimmst, gibt das Schreibzugriff, oder nur Lesezugriff, wenn Platz oder Rolle das Bearbeiten von Kontakten nicht erlauben.", Vi: "%s đề nghị bạn chia sẻ liên hệ này. Khi duyệt, quyền được cấp là quyền ghi, hoặc quyền đọc nếu tài khoản hoặc vai trò của họ không được sửa liên hệ."}, Arguments: []string{argumentText}},
	"approval.employerChangeAsked":          {Words: langcopy.Phrase{En: "%s may have moved from %s to %s.", De: "%s ist vielleicht von %s zu %s gewechselt.", Vi: "%s có thể đã chuyển từ %s sang %s."}, Arguments: []string{argumentText, argumentText, argumentText}},
	"approval.proposalStageAsked":           {Words: langcopy.Phrase{En: "A proposal went out on %s. Move the deal to “%s”?", De: "Zu %s ist ein Angebot rausgegangen. Den Deal in die Phase „%s“ verschieben?", Vi: "Một đề xuất đã được gửi cho %s. Chuyển deal sang giai đoạn “%s”?"}, Arguments: []string{argumentText, argumentText}},
	"approval.taskDueAsked":                 {Words: langcopy.Phrase{En: "The due date for “%s” looks far off. Keep it, change it or clear it.", De: "Das Fälligkeitsdatum für „%s“ wirkt weit weg. Behalten, ändern oder leeren.", Vi: "Hạn cho “%s” có vẻ quá xa. Giữ, đổi hoặc xóa."}, Arguments: []string{argumentText}},
	"approval.legalReviewAsked":             {Words: langcopy.Phrase{En: "Legal review requested for %s. Answer it under Legal reviews.", De: "Rechtsprüfung für %s angefragt. Beantworte sie unter Rechtsprüfungen.", Vi: "Đã yêu cầu rà soát pháp lý cho %s. Trả lời trong mục Rà soát pháp lý."}, Arguments: []string{argumentText}},
	"approval.companyRename":                {Words: langcopy.Phrase{En: "Rename %s to %s?", De: "%s in %s umbenennen?", Vi: "Đổi tên %s thành %s?"}, Arguments: []string{argumentText, argumentText}},
	"approval.siteLeadFound":                {Words: langcopy.Phrase{En: "Found on %s: %s (%s)", De: "Gefunden auf %s: %s (%s)", Vi: "Tìm thấy trên %s: %s (%s)"}, Arguments: []string{argumentText, argumentText, argumentText}, Retired: wordings{textlang.English: {"Found on %s: %s — %s"}}},
	"approval.enrichmentOf":                 {Words: langcopy.Phrase{En: "Enrichment of %s", De: "Anreicherung von %s", Vi: "Bổ sung dữ liệu cho %s"}, Arguments: []string{argumentText}},
	"approval.contractEndedMove":            {Words: langcopy.Phrase{En: "Their mail says the contract ended. Move this company from %s to %s?", De: "Laut ihrer E-Mail ist der Vertrag beendet. Dieses Unternehmen von %s nach %s verschieben?", Vi: "Email của họ cho biết hợp đồng đã kết thúc. Chuyển công ty này từ %s sang %s?"}, Arguments: []string{argumentText, argumentText}},
	"approval.linkedInLooksLike":            {Words: langcopy.Phrase{En: "%s at %s looks like %s", De: "%s bei %s scheint %s zu sein", Vi: "%s tại %s có vẻ là %s"}, Arguments: []string{argumentText, argumentText, argumentText}},
	"approval.unnamedEmployer":              {Words: langcopy.Phrase{En: "an unnamed employer", De: "einem nicht genannten Arbeitgeber", Vi: "một nơi làm việc không rõ tên"}, Arguments: []string{}},
	"approval.vcardResemblesContact":        {Words: langcopy.Phrase{En: "This card resembles an existing contact. Create %s anyway?", De: "Diese Visitenkarte ähnelt einem bestehenden Kontakt. %s trotzdem erstellen?", Vi: "Danh thiếp này giống một liên hệ đã có. Vẫn tạo %s?"}, Arguments: []string{argumentText}},
	"approval.vcardReadFromPhoto":           {Words: langcopy.Phrase{En: "A model read this card from a photo. Create %s?", De: "Ein Modell hat diese Visitenkarte aus einem Foto gelesen. %s erstellen?", Vi: "Một mô hình đã đọc danh thiếp này từ ảnh. Tạo %s?"}, Arguments: []string{argumentText}},
	"approval.heldNotSent":                  {Words: langcopy.Phrase{En: "“%s” was not sent: %s", De: "„%s“ wurde nicht gesendet: %s", Vi: "“%s” chưa được gửi: %s"}, Arguments: []string{argumentText, argumentText}},
	"approval.heldConsentWithdrawn":         {Words: langcopy.Phrase{En: "a recipient withdrew consent for this purpose after it was scheduled", De: "jemand unter den Empfangenden hat die Einwilligung für diesen Zweck nach der Planung widerrufen", Vi: "một người nhận đã rút lại sự đồng ý cho mục đích này sau khi tin nhắn được lên lịch"}, Arguments: []string{}},
	"approval.heldSenderInactive":           {Words: langcopy.Phrase{En: "the sending account or its mailbox is no longer active", De: "das sendende Konto oder sein Postfach ist nicht mehr aktiv", Vi: "tài khoản gửi hoặc hộp thư của nó không còn hoạt động"}, Arguments: []string{}},
	"approval.heldPassportRevoked":          {Words: langcopy.Phrase{En: "the passport it was scheduled under has been revoked or expired; your account is fine, so send it yourself if it should still go", De: "die Agent-Zugangsdaten, unter denen die Nachricht geplant wurde, wurden widerrufen oder sind abgelaufen. Dein Konto ist in Ordnung, sende sie also selbst, wenn sie noch rausgehen soll", Vi: "passport dùng để lên lịch đã bị thu hồi hoặc hết hạn; tài khoản của bạn vẫn bình thường, hãy tự gửi nếu tin nhắn vẫn cần được gửi"}, Arguments: []string{}, Retired: wordings{textlang.English: {"the agent credential it was scheduled under has been revoked or expired — your account is fine, so send it yourself if it should still go"}}},
	"approval.heldMissedWindow":             {Words: langcopy.Phrase{En: "its moment passed while nothing was running, and it is too late to be the message that was written", De: "der geplante Zeitpunkt ist verstrichen, während nichts lief, und für die Nachricht, wie sie geschrieben wurde, ist es jetzt zu spät", Vi: "thời điểm gửi đã trôi qua khi không có gì đang chạy, và giờ đã quá muộn để tin nhắn còn đúng như khi được viết"}, Arguments: []string{}},
	"approval.heldTimerExhausted":           {Words: langcopy.Phrase{En: "the job that wakes it ran out of attempts", De: "der Job, der den Versand auslöst, hat alle Versuche aufgebraucht", Vi: "tác vụ kích hoạt việc gửi đã hết số lần thử"}, Arguments: []string{}},
	"approval.heldSendRefused":              {Words: langcopy.Phrase{En: "a gate refused it at send time", De: "eine Prüfung hat den Versand zum Sendezeitpunkt abgelehnt", Vi: "một bước kiểm tra đã từ chối tin nhắn khi gửi"}, Arguments: []string{}},
	"approval.heldFollowupSettled":          {Words: langcopy.Phrase{En: "the request it followed up was answered or completed after it was scheduled", De: "die Anfrage, an die sie anschließt, wurde nach der Planung beantwortet oder erledigt", Vi: "yêu cầu mà thư liên hệ lại này nhắc đến đã được trả lời hoặc hoàn tất sau khi lên lịch"}, Arguments: []string{}},
	"approval.heldMeetingCanceled":          {Words: langcopy.Phrase{En: "the meeting it followed up was canceled after it was scheduled", De: "der Termin, an den sie anschließt, wurde nach der Planung abgesagt", Vi: "cuộc họp mà thư liên hệ lại này nhắc đến đã bị hủy sau khi lên lịch"}, Arguments: []string{}},
	"approval.heldRecipientStopped":         {Words: langcopy.Phrase{En: "a recipient stopped marketing after it was scheduled", De: "jemand unter den Empfangenden hat nach der Planung Werbung gestoppt", Vi: "một địa chỉ nhận đã dừng nhận tiếp thị sau khi lên lịch"}, Arguments: []string{}},
	"approval.heldRecipientReplied":         {Words: langcopy.Phrase{En: "new mail came in from a recipient after it was scheduled", De: "nach der Planung kam neue Mail von jemandem unter den Empfangenden", Vi: "có thư mới từ bên nhận sau khi lên lịch"}, Arguments: []string{}},
	"approval.reviewSendRefused":            {Words: langcopy.Phrase{En: "A send was refused for %s (%s) and somebody is asking whether it may go anyway", De: "Ein Versand an %s wurde abgelehnt (%s), und jemand fragt, ob er trotzdem rausgehen darf", Vi: "Một lần gửi tới %s đã bị từ chối (%s) và có người đang hỏi liệu vẫn có thể gửi đi không"}, Arguments: []string{argumentText, argumentText}},
	"approval.reviewOneRecipient":           {Words: langcopy.Phrase{En: "1 recipient", De: "1 Empfangsadresse", Vi: "1 người nhận"}, Arguments: []string{}},
	"approval.reviewRecipients":             {Words: langcopy.Phrase{En: "%d recipients", De: "%d Empfangsadressen", Vi: "%d người nhận"}, Arguments: []string{argumentCount}},
	"approval.coldStartFromURL":             {Words: langcopy.Phrase{En: "Cold-start read-back of %s", De: "Cold-Start-Auslesung aus %s", Vi: "Đọc dữ liệu khởi tạo từ %s"}, Arguments: []string{argumentText}},
	"approval.coldStartFromText":            {Words: langcopy.Phrase{En: "Cold-start read-back of pasted text", De: "Cold-Start-Auslesung aus eingefügtem Text", Vi: "Đọc dữ liệu khởi tạo từ văn bản đã dán"}, Arguments: []string{}},
	"approval.coldStartFromSelfDescription": {Words: langcopy.Phrase{En: "Cold-start read-back of a self-description", De: "Cold-Start-Auslesung aus einer Selbstbeschreibung", Vi: "Đọc dữ liệu khởi tạo từ một bản tự mô tả"}, Arguments: []string{}},
	"approval.fxRateChanged":                {Words: langcopy.Phrase{En: "%s → %s %s (was %s)", De: "%s → %s %s (bisher %s)", Vi: "%s → %s %s (trước đây %s)"}, Arguments: []string{argumentText, argumentText, argumentText, argumentText}},
	"approval.fxNoRateInForce":              {Words: langcopy.Phrase{En: "none in force today", De: "kein heute gültiger Kurs", Vi: "không có tỷ giá nào hiệu lực hôm nay"}, Arguments: []string{}},
	"approval.createdUnder":                 {Words: langcopy.Phrase{En: "under", De: "unter", Vi: "thuộc"}, Arguments: []string{}},
	"approval.nestedFields":                 {Words: langcopy.Phrase{En: "{%d fields}", De: "{%d Felder}", Vi: "{%d trường}"}, Arguments: []string{argumentCount}},
}

// TaskClosedKey names the receipt sentence for a task Margince closed, by
// whether a booking or a sent mail closed it.
func TaskClosedKey(booking bool) string {
	if booking {
		return "approval.taskBooked"
	}
	return "approval.taskCompleted"
}

// TaskClosedContactKey names the receipt's words for a contact with no readable
// name. The booking sentence needs the subject, the mail sentence the object.
func TaskClosedContactKey(booking bool) string {
	if booking {
		return "approval.taskBooker"
	}
	return "approval.taskContact"
}

// TaskChatKey names the receipt sentence that quotes the chat line for a task
// a filed chat changed: done, or moved to a new day.
func TaskChatKey(moved bool) string {
	if moved {
		return "approval.taskChatMoved"
	}
	return "approval.taskChatDone"
}

// TaskChatUnquotedKey is TaskChatKey's sentence for a reader who may not read
// the chat, which names no chat line.
func TaskChatUnquotedKey(moved bool) string {
	return TaskChatKey(moved) + "Unquoted"
}
