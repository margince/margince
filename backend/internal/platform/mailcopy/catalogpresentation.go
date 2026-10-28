// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

func presentationLines(line writeLine) {
	workPresentationLines(line)
	roomPresentationLines(line)
	previewLines(line)
	weeklyPresentationLines(line)
	controllerPresentationLines(line)
}

func workPresentationLines(line writeLine) {
	line(func(c *Copy) *string { return &c.MailCurrent }, "Needs attention now", "Jetzt zu prüfen", "Cần xem xét lúc này")
	line(func(c *Copy) *string { return &c.MailStale }, "No activity for %d days", "Seit %d Tagen ohne Aktivität", "Chưa có hoạt động trong %d ngày")
	line(func(c *Copy) *string { return &c.MailMembersCounted }, "Figures cover %d of %d team members.", "Die Auswertung umfasst %d von %d Teammitgliedern.", "Số liệu bao gồm %d trên %d thành viên nhóm.")
	line(func(c *Copy) *string { return &c.MailTeamOpen }, "View the team’s week", "Teamwoche ansehen", "Xem tổng kết tuần của nhóm")
	line(func(c *Copy) *string { return &c.MailTeamDay }, "View the team’s current work", "Teamübersicht ansehen", "Xem công việc hiện tại của nhóm")
	line(func(c *Copy) *string { return &c.MailPersonal }, "Your work", "Deine Arbeit", "Công việc của bạn")
	line(func(c *Copy) *string { return &c.MailTeam }, "Team view", "Teamüberblick", "Tổng quan nhóm")
	line(func(c *Copy) *string { return &c.MailHelp }, "Requests for help", "Bitten um Unterstützung", "Yêu cầu hỗ trợ")
	line(func(c *Copy) *string { return &c.MailMissing },
		"Some team information is missing. Check the team view for details.",
		"Ein Teil der Teaminformationen fehlt. Prüfe die Details in der Teamübersicht.",
		"Thiếu một phần thông tin nhóm. Kiểm tra trong tổng quan nhóm.")
	line(func(c *Copy) *string { return &c.InviteBy }, "%s invited you to join %s.", "%s hat dich zu %s eingeladen.", "%s đã mời bạn tham gia %s.")
	line(func(c *Copy) *string { return &c.MailWhy }, "You receive this because email delivery is enabled in your notification settings.", "Du erhältst diese E-Mail, weil der E-Mail-Versand in deinen Benachrichtigungseinstellungen aktiviert ist.", "Bạn nhận được thư này vì đã bật gửi email trong cài đặt thông báo.")
	line(func(c *Copy) *string { return &c.MailCoverage },
		"This overview is incomplete. Some tasks or meetings may be missing. Check the current details in Margince.",
		"Diese Übersicht ist unvollständig. Aufgaben oder Termine können fehlen. Prüfe den aktuellen Stand in Margince.",
		"Bản tổng quan chưa đầy đủ. Một số công việc hoặc cuộc họp có thể còn thiếu. Kiểm tra thông tin hiện tại trong Margince.")
	line(func(c *Copy) *string { return &c.MailReview }, "Review", "Prüfen", "Xem xét")
	line(func(c *Copy) *string { return &c.MailReply }, "Reply", "Antworten", "Trả lời")
	line(func(c *Copy) *string { return &c.MailPrepare }, "Prepare for the meeting", "Gespräch vorbereiten", "Chuẩn bị cuộc họp")
	line(func(c *Copy) *string { return &c.MailTask }, "Complete the next step", "Nächsten Schritt erledigen", "Hoàn thành bước tiếp theo")
	line(func(c *Copy) *string { return &c.MailOwner }, "Owner", "Verantwortlich", "Người chịu trách nhiệm")
	line(func(c *Copy) *string { return &c.MailDue }, "Due", "Fällig", "Đến hạn")
	line(func(c *Copy) *string { return &c.MailDeadline }, "Decision deadline", "Frist", "Hạn quyết định")
	line(func(c *Copy) *string { return &c.MailAbout }, "About", "Betrifft", "Về")
	line(func(c *Copy) *string { return &c.MailProposal }, "Proposal", "Vorschlag", "Đề xuất")
	line(func(c *Copy) *string { return &c.MailBurstCount }, "%d requests", "%d Anfragen", "%d yêu cầu")
	line(func(c *Copy) *string { return &c.MailSetOnApproval }, "set when approved", "wird bei Freigabe gesetzt", "được đặt khi phê duyệt")
	line(func(c *Copy) *string { return &c.MailReadFrom }, "Read from", "Gelesen aus", "Đọc từ")
	line(func(c *Copy) *string { return &c.MailPromisedBy }, "Promised by", "Zugesagt von", "Người hứa")
	line(func(c *Copy) *string { return &c.MailInTheirWords }, "In their words", "Im Wortlaut", "Nguyên văn")
	line(func(c *Copy) *string { return &c.MailWhyYou }, "Why you", "Warum du", "Vì sao là bạn")
	line(func(c *Copy) *string { return &c.MailTranscriptOf },
		"a meeting transcript of", "ein Gesprächsprotokoll vom", "biên bản cuộc họp ngày")
	line(func(c *Copy) *string { return &c.MailReasonRequested },
		"You were asked to review this proposal.",
		"Du wurdest gebeten, diesen Vorschlag zu prüfen.",
		"Bạn được yêu cầu xem xét đề xuất này.")
	line(func(c *Copy) *string { return &c.MailReasonOnBehalf },
		"The agent proposed this change on your behalf.",
		"Der Agent hat diese Änderung in deinem Namen vorgeschlagen.",
		"Trợ lý AI đã đề xuất thay đổi này thay mặt bạn.")
	line(func(c *Copy) *string { return &c.MailReasonAuthority },
		"You have permission to make this change.",
		"Du darfst diese Änderung vornehmen.",
		"Bạn có quyền thực hiện thay đổi này.")
	line(func(c *Copy) *string { return &c.MailOpenNow }, "Still open now", "Noch offen", "Hiện vẫn chưa hoàn tất")
	line(func(c *Copy) *string { return &c.MailDecisions }, "Requests to review", "Offene Anfragen", "Yêu cầu cần xem xét")
	line(func(c *Copy) *string { return &c.MailUpdates }, "For your information", "Zur Information", "Thông tin cập nhật")
}

//nolint:dupl // Translation rows repeat the catalog schema, not message content or behavior.
func roomPresentationLines(line writeLine) {
	line(func(c *Copy) *string { return &c.RoomSubject },
		"Documents have been shared with you",
		"Unterlagen für Sie freigegeben",
		"Tài liệu được chia sẻ với quý khách")
	line(func(c *Copy) *string { return &c.RoomIntro },
		"You have been invited to view the following shared documents:",
		"Sie wurden eingeladen, die folgenden Unterlagen anzusehen:",
		"Quý khách được mời xem các tài liệu sau:")
	line(func(c *Copy) *string { return &c.RoomAction },
		"View the documents",
		"Unterlagen ansehen",
		"Xem tài liệu")
	line(func(c *Copy) *string { return &c.RoomPersonal },
		"This access link is for your use only and can be used once. It is valid until %s.",
		"Dieser Zugangslink ist nur für Sie bestimmt und kann einmal verwendet werden. Er ist bis %s gültig.",
		"Liên kết truy cập chỉ dành cho quý khách và dùng được một lần. Liên kết có hiệu lực đến %s.")
	line(func(c *Copy) *string { return &c.RoomIgnore },
		"If the link has expired, request a new link on the access page. If you did not expect this email, you can ignore it.",
		"Wenn der Link abgelaufen ist, können Sie auf der Zugangsseite einen neuen Link anfordern. Falls Sie diese E-Mail nicht erwartet haben, können Sie sie ignorieren.",
		"Nếu liên kết hết hạn, quý khách có thể yêu cầu liên kết mới trên trang truy cập. Nếu không chờ email này, quý khách có thể bỏ qua.")
	line(func(c *Copy) *string { return &c.RoomDisclosure },
		"Those managing these documents can see when you sign in and which documents you download.",
		"Wer diese Unterlagen verwaltet, kann Ihre Anmeldungen und Downloads sehen.",
		"Người quản lý tài liệu có thể xem thời điểm quý khách đăng nhập và tài liệu đã tải xuống.")
	line(func(c *Copy) *string { return &c.RoomSharedBy }, "%s from %s shared documents with you.", "%s von %s hat Unterlagen für Sie freigegeben.", "%s từ %s đã chia sẻ tài liệu với quý khách.")
}

func previewLines(line writeLine) {
	line(func(c *Copy) *string { return &c.MorningPreview },
		"Here is the work to focus on today.",
		"Das steht heute für dich an.",
		"Đây là những việc cần tập trung hôm nay.")
	line(func(c *Copy) *string { return &c.WeeklyPreview }, "Review your recorded results and choose your next priorities.", "Sieh dir deine erfassten Ergebnisse an und setze die nächsten Prioritäten.", "Xem lại kết quả đã ghi nhận và chọn ưu tiên tiếp theo.")
	line(func(c *Copy) *string { return &c.DigestPreview }, "Catch up on the updates you asked to receive together.", "Hier sind die Neuigkeiten, die du gesammelt erhalten möchtest.", "Xem các cập nhật bạn đã chọn nhận cùng nhau.")
	line(func(c *Copy) *string { return &c.NotificationPreview }, "Review the current request and decide the next step.", "Prüfe die aktuelle Anfrage und entscheide über den nächsten Schritt.", "Xem yêu cầu hiện tại và quyết định bước tiếp theo.")
	line(func(c *Copy) *string { return &c.ResetPreview }, "Use this personal link to choose a new password.", "Wähle über diesen persönlichen Link ein neues Passwort.", "Dùng liên kết cá nhân này để chọn mật khẩu mới.")
	line(func(c *Copy) *string { return &c.InvitePreview }, "See who invited you and choose whether to accept.", "Sieh nach, wer dich eingeladen hat, und entscheide, ob du annimmst.", "Xem ai đã mời bạn và quyết định có chấp nhận hay không.")
	line(func(c *Copy) *string { return &c.RoomPreview },
		"View the shared documents and the details of your personal access.",
		"Sehen Sie sich die freigegebenen Unterlagen und die Hinweise zu Ihrem Zugang an.",
		"Xem tài liệu được chia sẻ và thông tin về quyền truy cập cá nhân.")
	line(func(c *Copy) *string { return &c.NoticePreview },
		"See why we hold your details and how to contact us.",
		"Erfahren Sie, warum wir Ihre Kontaktdaten speichern und wie Sie uns erreichen.",
		"Tìm hiểu vì sao chúng tôi lưu thông tin liên hệ và cách liên hệ với chúng tôi.")
	line(func(c *Copy) *string { return &c.ConfirmRecordPreview },
		"Review your details; receiving news is a separate choice.",
		"Prüfen Sie Ihre Kontaktdaten. Über Neuigkeiten entscheiden Sie unabhängig davon.",
		"Kiểm tra thông tin liên hệ; nhận tin là lựa chọn riêng.")
	line(func(c *Copy) *string { return &c.ConfirmConsentPreview },
		"Confirm only if you want this subscription.",
		"Bestätigen Sie nur, wenn Sie diese Nachrichten abonnieren möchten.",
		"Chỉ xác nhận nếu muốn đăng ký nhận tin này.")
	line(func(c *Copy) *string { return &c.OptOutAckPreview },
		"No reply or further confirmation is needed.",
		"Sie müssen weder antworten noch erneut bestätigen.",
		"Quý khách không cần trả lời hay xác nhận thêm.")
}

func weeklyPresentationLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyLeads }, "Leads assigned", "Zugewiesene Leads", "Lead được phân công")
	line(func(c *Copy) *string { return &c.WeeklyAnswered }, "First replies recorded by week close", "Erste Antworten bis Wochenende erfasst", "Phản hồi đầu tiên ghi nhận trước khi kết thúc tuần")
	line(func(c *Copy) *string { return &c.WeeklyBreached }, "First replies overdue", "Überfällige erste Antworten", "Phản hồi đầu tiên quá hạn")
	line(func(c *Copy) *string { return &c.WeeklyMeetings }, "Meetings held", "Durchgeführte Termine", "Cuộc họp đã diễn ra")
	line(func(c *Copy) *string { return &c.WeeklyNextSteps }, "Meetings with a next step", "Termine mit nächstem Schritt", "Cuộc họp có bước tiếp theo")
	line(func(c *Copy) *string { return &c.WeeklyPromises }, "Commitments kept", "Eingehaltene Zusagen", "Cam kết đã thực hiện")
	line(func(c *Copy) *string { return &c.WeeklyCompleted }, "Tasks completed", "Aufgaben erledigt", "Việc cần làm đã hoàn tất")
	line(func(c *Copy) *string { return &c.WeeklyResults },
		"Results for the week",
		"Ergebnisse dieser Woche",
		"Kết quả trong tuần")
	line(func(c *Copy) *string { return &c.WeeklyWonRevenue }, "Value of deals won", "Wert gewonnener Deals", "Giá trị deal đã chốt")
	line(func(c *Copy) *string { return &c.WeeklyOverview }, "Here are your recorded results for this week.", "Hier findest du deine erfassten Ergebnisse dieser Woche.", "Đây là kết quả đã ghi nhận của bạn trong tuần.")
	line(func(c *Copy) *string { return &c.WeeklyWonOne }, "You won one deal.", "Du hast einen Deal gewonnen.", "Bạn đã chốt một deal.")
	line(func(c *Copy) *string { return &c.WeeklyWonMany }, "You won %d deals.", "Du hast %d Deals gewonnen.", "Bạn đã chốt %d deal.")
	line(func(c *Copy) *string { return &c.WeeklyDoneOne }, "You completed one task.", "Du hast eine Aufgabe erledigt.", "Bạn đã hoàn thành một công việc.")
	line(func(c *Copy) *string { return &c.WeeklyDoneMany }, "You completed %d tasks.", "Du hast %d Aufgaben erledigt.", "Bạn đã hoàn thành %d công việc.")
}

func controllerPresentationLines(line writeLine) {
	line(func(c *Copy) *string { return &c.BusinessPurpose }, "Direct correspondence", "Direkte Korrespondenz", "Thư từ trực tiếp")
	line(func(c *Copy) *string { return &c.MarketingPurpose }, "Product news", "Produktneuigkeiten", "Tin tức sản phẩm")
	line(func(c *Copy) *string { return &c.TransactionalPurpose }, "Security and service messages", "Sicherheits- und Servicenachrichten", "Thông báo về bảo mật và dịch vụ")
	line(func(c *Copy) *string { return &c.MailManagerResponse }, "Manager response", "Antwort der Führungskraft", "Phản hồi của người quản lý")
	line(func(c *Copy) *string { return &c.ConfirmRecordAction }, "Review contact details", "Kontaktdaten ansehen", "Xem thông tin liên hệ")
	line(func(c *Copy) *string { return &c.ConfirmConsentAction }, "Confirm subscription", "Abonnement bestätigen", "Xác nhận đăng ký")
	line(func(c *Copy) *string { return &c.NoticeAction }, "View privacy information", "Datenschutzinformationen ansehen", "Xem thông tin bảo vệ dữ liệu")
	line(func(c *Copy) *string { return &c.PreviewLinkMarker }, "[Personal link, created when you send]", "[Persönlicher Link, wird beim Senden erstellt]", "[Liên kết cá nhân, được tạo khi bạn gửi]")
	line(func(c *Copy) *string { return &c.ConsentPurposeLabel }, "Subscription", "Thema", "Đăng ký nhận")
	line(func(c *Copy) *string { return &c.ControllerFrom }, "This message is from %s.", "Diese Nachricht erhalten Sie von %s.", "Email này được gửi từ %s.")
}
