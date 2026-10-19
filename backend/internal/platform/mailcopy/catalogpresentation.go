// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

func presentationLines(line writeLine) {
	line(func(c *Copy) *string { return &c.MailPersonal }, "Your work", "Deine Arbeit", "Công việc của bạn")
	line(func(c *Copy) *string { return &c.MailTeam }, "Team view", "Teamüberblick", "Tổng quan nhóm")
	line(func(c *Copy) *string { return &c.MailHelp }, "Requests for help", "Bitten um Unterstützung", "Yêu cầu hỗ trợ")
	line(func(c *Copy) *string { return &c.MailMissing }, "Team coverage is incomplete.", "Der Teamüberblick ist unvollständig.", "Thông tin nhóm chưa đầy đủ.")
	line(func(c *Copy) *string { return &c.InviteBy }, "%s invited you to join %s.", "%s hat dich zu %s eingeladen.", "%s đã mời bạn tham gia %s.")
	line(func(c *Copy) *string { return &c.MailWhy }, "You receive this because email delivery is enabled in your notification settings.", "Du erhältst diese E-Mail, weil der E-Mail-Versand in deinen Benachrichtigungseinstellungen aktiviert ist.", "Bạn nhận được thư này vì đã bật gửi email trong cài đặt thông báo.")
	line(func(c *Copy) *string { return &c.MailCoverage }, "Some sources could not be read. This is a partial view, not an all-clear.", "Einige Quellen konnten nicht gelesen werden. Diese Übersicht ist unvollständig und keine Entwarnung.", "Không thể đọc một số nguồn. Đây là bản xem chưa đầy đủ, không phải xác nhận mọi việc đã ổn.")
	line(func(c *Copy) *string { return &c.MailReview }, "Review", "Prüfen", "Xem xét")
	line(func(c *Copy) *string { return &c.MailReply }, "Reply", "Antworten", "Trả lời")
	line(func(c *Copy) *string { return &c.MailPrepare }, "Prepare for the meeting", "Gespräch vorbereiten", "Chuẩn bị cuộc họp")
	line(func(c *Copy) *string { return &c.MailTask }, "Complete the next step", "Nächsten Schritt erledigen", "Hoàn thành bước tiếp theo")
	line(func(c *Copy) *string { return &c.MailOwner }, "Owner", "Verantwortlich", "Người phụ trách")
	line(func(c *Copy) *string { return &c.MailDue }, "Due", "Fällig", "Đến hạn")
	line(func(c *Copy) *string { return &c.WeeklyCompleted }, "Tasks completed", "Aufgaben erledigt", "Việc cần làm đã hoàn tất")
	line(func(c *Copy) *string { return &c.WeeklyResults }, "Recorded results", "Erfasste Ergebnisse", "Kết quả đã ghi nhận")
	line(func(c *Copy) *string { return &c.RoomSubject }, "Your Deal Room is ready", "Dein Deal Room ist bereit", "Deal Room của bạn đã sẵn sàng")
	line(func(c *Copy) *string { return &c.RoomIntro }, "You have been invited to review the material shared in your Deal Room.", "Du wurdest eingeladen, die in deinem Deal Room geteilten Unterlagen anzusehen.", "Bạn được mời xem tài liệu chia sẻ trong Deal Room.")
	line(func(c *Copy) *string { return &c.RoomAction }, "Open your Deal Room", "Deal Room öffnen", "Mở Deal Room")
	line(func(c *Copy) *string { return &c.RoomPersonal }, "This personal link works once and expires on %s.", "Dieser persönliche Link funktioniert einmal und läuft am %s ab.", "Liên kết cá nhân này dùng một lần và hết hạn vào %s.")
	line(func(c *Copy) *string { return &c.RoomIgnore }, "If you did not expect this invitation, you can ignore it.", "Wenn du diese Einladung nicht erwartet hast, kannst du sie ignorieren.", "Nếu không chờ lời mời này, bạn có thể bỏ qua.")
	line(func(c *Copy) *string { return &c.RoomDisclosure }, "The room managers can see your sign-ins and which documents you download.", "Die Raumverantwortlichen sehen deine Anmeldungen und heruntergeladenen Dokumente.", "Người quản lý phòng có thể thấy các lần đăng nhập và tài liệu bạn tải xuống.")
	line(func(c *Copy) *string { return &c.MorningPreview }, "Start with the customer work that needs your attention today.", "Starte mit der Kundenarbeit, die heute deine Aufmerksamkeit braucht.", "Bắt đầu với công việc khách hàng cần bạn chú ý hôm nay.")
	line(func(c *Copy) *string { return &c.WeeklyPreview }, "Review your recorded results and choose your next priorities.", "Sieh dir deine erfassten Ergebnisse an und setze die nächsten Prioritäten.", "Xem lại kết quả đã ghi nhận và chọn ưu tiên tiếp theo.")
	line(func(c *Copy) *string { return &c.DigestPreview }, "Catch up on the updates you asked to receive together.", "Hier sind die Neuigkeiten, die du gesammelt erhalten möchtest.", "Xem các cập nhật bạn đã chọn nhận cùng nhau.")
	line(func(c *Copy) *string { return &c.NotificationPreview }, "Review the current request and decide the next step.", "Prüfe die aktuelle Anfrage und entscheide über den nächsten Schritt.", "Xem yêu cầu hiện tại và quyết định bước tiếp theo.")
	line(func(c *Copy) *string { return &c.ResetPreview }, "Use this personal link to choose a new password.", "Wähle über diesen persönlichen Link ein neues Passwort.", "Dùng liên kết cá nhân này để chọn mật khẩu mới.")
	line(func(c *Copy) *string { return &c.InvitePreview }, "See who invited you and choose whether to accept.", "Sieh nach, wer dich eingeladen hat, und entscheide, ob du annimmst.", "Xem ai đã mời bạn và quyết định có chấp nhận hay không.")
	line(func(c *Copy) *string { return &c.RoomPreview }, "Review your room access and how sign-ins and downloads are recorded.", "Sieh dir deinen Raumzugang an und erfahre, wie Anmeldungen und Downloads erfasst werden.", "Xem quyền truy cập phòng và cách ghi nhận đăng nhập, tải xuống.")
	line(func(c *Copy) *string { return &c.NoticePreview }, "Learn why we hold your details and how to reach us.", "Erfahre, warum wir deine Daten speichern und wie du uns erreichst.", "Tìm hiểu vì sao chúng tôi lưu thông tin của bạn và cách liên hệ.")
	line(func(c *Copy) *string { return &c.ConfirmRecordPreview }, "Check your details and tell us what needs correcting.", "Prüfe deine Daten und teile uns mit, was korrigiert werden muss.", "Kiểm tra thông tin của bạn và cho biết điều cần sửa.")
	line(func(c *Copy) *string { return &c.ConfirmConsentPreview }, "Confirm your choice only if you want these messages.", "Bestätige deine Wahl nur, wenn du diese Nachrichten erhalten möchtest.", "Chỉ xác nhận lựa chọn nếu bạn muốn nhận những thư này.")
	line(func(c *Copy) *string { return &c.OptOutAckPreview }, "No reply or further confirmation is needed.", "Eine Antwort oder weitere Bestätigung ist nicht nötig.", "Bạn không cần trả lời hay xác nhận thêm.")
}
