// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

// The copy itself, one line at a time, in every language the contract's
// `base_language` enum admits.
//
// KEY-MAJOR rather than one block per language, and it is the layout the job
// asks for: the three renderings of one string sit together, so a reader
// comparing them — or checking one against the panel it has to match — reads
// three adjacent lines instead of scrolling between three blocks. The drift
// this catalog exists to stop is exactly the kind that is invisible when the
// versions are far apart.
//
// The weekly labels are the frontend catalog's own strings, character for
// character, and TestEveryMailLabelMatchesTheScreenThatShowsIt holds them
// there. The transactional copy has no screen behind it and lives only here.

var catalog = buildCatalog()

// buildCatalog writes each line into all three copies at once.
//
// The field is named through an accessor rather than a string key, so a typo
// does not compile — which a map of key to translations would have allowed,
// and which nothing else here would catch.
func buildCatalog() map[Language]Copy {
	en, de, vi := &Copy{Language: English}, &Copy{Language: German}, &Copy{Language: Vietnamese}
	line := func(field func(*Copy) *string, english, german, vietnamese string) {
		*field(en), *field(de), *field(vi) = english, german, vietnamese
	}
	// One function per message rather than one long one, because the length
	// cap is a real reading limit here: a translator opens the section they are
	// working on, and a hundred lines of unrelated copy above it is noise.
	unsubscribeLines(line)
	resetLines(line)
	inviteLines(line)
	weeklyLines(line)
	morningLines(line)
	notificationLines(line)
	digestLines(line)
	confirmLines(line)
	noticeLines(line)
	presentationLines(line)

	return map[Language]Copy{English: *en, German: *de, Vietnamese: *vi}
}

// writeLine is what each section below is handed: the field to write, and its
// three renderings in the order the languages are declared.
type writeLine = func(field func(*Copy) *string, english, german, vietnamese string)

// resetLines is the password reset a colleague asked for.
// unsubscribeLines is what a send adds beneath the message: the closing for a
// sender with no signature, and the unsubscribe footer.
func unsubscribeLines(line writeLine) {
	line(func(c *Copy) *string { return &c.UnsubscribeLabel },
		"Unsubscribe",
		"Abmelden",
		"Hủy đăng ký")
	line(func(c *Copy) *string { return &c.ManagePreferencesLabel },
		"Manage your preferences",
		"E-Mail-Einstellungen verwalten",
		"Quản lý tùy chọn email")
	line(func(c *Copy) *string { return &c.SignOffClosing },
		"Best regards,",
		"Viele Grüße,",
		"Trân trọng,")
}

func resetLines(line writeLine) {
	line(func(c *Copy) *string { return &c.ResetSubject },
		"Reset your Margince password",
		"Margince-Passwort zurücksetzen",
		"Đặt lại mật khẩu Margince")
	line(func(c *Copy) *string { return &c.ResetIntro },
		"Use the link below to choose a new password for your Margince account. The link is valid for one hour.",
		"Mit diesem Link kannst du ein neues Passwort für dein Margince-Konto festlegen. Der Link ist eine Stunde gültig.",
		"Dùng liên kết dưới đây để đặt mật khẩu mới cho tài khoản Margince. Liên kết có hiệu lực trong một giờ.")
	line(func(c *Copy) *string { return &c.ResetAction },
		"Choose a new password",
		"Neues Passwort festlegen",
		"Đặt mật khẩu mới")
	line(func(c *Copy) *string { return &c.ResetIgnore },
		"If this wasn't you, ignore this email — your password is unchanged.",
		"Warst du das nicht, ignoriere diese E-Mail. Dein Passwort bleibt unverändert.",
		"Nếu không phải bạn, hãy bỏ qua email này — mật khẩu của bạn không thay đổi.")
}

// inviteLines is the invitation an administrator sent on somebody's behalf.
func inviteLines(line writeLine) {
	line(func(c *Copy) *string { return &c.InviteSubject },
		"You're invited to Margince",
		"Du bist zu Margince eingeladen",
		"Bạn được mời vào Margince")
	line(func(c *Copy) *string { return &c.InviteIntro },
		"You have been invited to your team’s shared CRM in Margince.",
		"Du wurdest zum gemeinsamen CRM deines Teams in Margince eingeladen.",
		"Bạn được mời tham gia CRM dùng chung của nhóm trên Margince.")
	line(func(c *Copy) *string { return &c.InviteAction },
		"Set up your access",
		"Zugang einrichten",
		"Thiết lập quyền truy cập")
	line(func(c *Copy) *string { return &c.InviteIgnore },
		"This invitation is valid for seven days. If you did not expect it, you can ignore this email.",
		"Diese Einladung ist sieben Tage gültig. Wenn du sie nicht erwartet hast, kannst du diese E-Mail ignorieren.",
		"Lời mời có hiệu lực trong bảy ngày. Nếu không chờ lời mời này, bạn có thể bỏ qua email.")
}

// morningLines is the daily brief.
func morningLines(line writeLine) {
	line(func(c *Copy) *string { return &c.MorningSubject },
		"Your priorities: ",
		"Dein Überblick: ",
		"Việc ưu tiên của bạn: ")
	line(func(c *Copy) *string { return &c.MorningTop },
		"What to start with:",
		"Womit du anfängst:",
		"Bắt đầu với:")
	line(func(c *Copy) *string { return &c.MorningAndMore },
		"and %d more on Home",
		"und %d weitere auf der Home-Seite",
		"và %d mục khác trên Trang chủ")
	line(func(c *Copy) *string { return &c.MorningQuiet },
		"Nothing is waiting on you this morning.",
		"Heute Morgen wartet nichts auf dich.",
		"Sáng nay không có gì đang chờ bạn.")
	line(func(c *Copy) *string { return &c.MorningOpenDay },
		"View today’s work",
		"Tagesübersicht öffnen",
		"Xem công việc hôm nay")
}

// notificationLines is the one notice mailed the moment it is raised: an
// approval waiting on the colleague who can answer it.
//
// It names the READER's part rather than the product's. "An approval is
// waiting for your decision" is the sentence the Worklist card already makes,
// and the message exists to carry that sentence out of a tab nobody has open —
// so a wording about the notification itself would describe the envelope
// instead of what is in it.
//
// The German is INFORMAL, like the reset, invite and brief copy above and
// unlike the confirm sections below: this message goes to a colleague who works
// here, and a product that says "du" every morning and "Sie" when a decision is
// waiting is two voices in one mailbox.
func notificationLines(line writeLine) {
	line(func(c *Copy) *string { return &c.NotificationSubject },
		"Your decision is needed: ",
		"Deine Entscheidung ist gefragt: ",
		"Cần quyết định của bạn: ")
	line(func(c *Copy) *string { return &c.NotificationIntro },
		"Please review this request. You can approve or decline it in Margince.",
		"Prüfe diese Anfrage. In Margince kannst du sie freigeben oder ablehnen.",
		"Xem yêu cầu này. Có thể duyệt hoặc từ chối trong Margince.")
	line(func(c *Copy) *string { return &c.NotificationOpen },
		"Review the request",
		"Anfrage prüfen",
		"Xem yêu cầu")
}

// digestLines is the once-a-day batch: what is waiting on a colleague's
// worklist that they have not opened yet.
//
// It names the QUEUE and not the product's own machinery. "Here is what is
// still waiting on your worklist" is a sentence about the reader's work; "your
// notification digest is ready" describes the envelope, which is the wording
// every product reaches for and nobody reads twice.
//
// The German is INFORMAL, like the brief and the immediate notice above and
// unlike the confirm sections below: this goes to a colleague who works here,
// and a product that says "du" every morning and "Sie" in the daily summary of
// the same morning is two voices in one mailbox.
func digestLines(line writeLine) {
	line(func(c *Copy) *string { return &c.DigestSubject },
		"What is waiting on your worklist",
		"Das wartet in deiner Worklist",
		"Những việc đang chờ trong danh sách công việc của bạn")
	line(func(c *Copy) *string { return &c.DigestIntro },
		"These updates need your attention. Open an item to see its current status.",
		"Diese Neuigkeiten solltest du dir ansehen. Öffne einen Eintrag, um den aktuellen Stand zu prüfen.",
		"Bạn nên xem các cập nhật này. Mở từng mục để kiểm tra trạng thái hiện tại.")
	line(func(c *Copy) *string { return &c.DigestAndMore },
		"…and %d more",
		"…weitere Einträge: %d",
		"…và %d mục khác")
	line(func(c *Copy) *string { return &c.DigestOpen },
		"View the updates",
		"Neuigkeiten ansehen",
		"Xem cập nhật")
}

// confirmLines is the copy for a contact the installation holds a record about.
// The English is pinned by hash on every consent proof, so moving a word or
// reordering these registrations is a version bump, not a translation.
// German and Vietnamese use the formal address (Sie; quý khách). This
// copy speaks to a stranger about their own rights, and the reset and invite
// copy above to a colleague.
func confirmLines(line writeLine) {
	confirmAskLines(line)
	recordNoticeLines(line)
	confirmConsentLines(line)
}

// confirmAskLines is what the confirm SCREEN asks, and the two answers it
// offers. A consent proof names the row a reader was shown, so this is the
// published wording rather than whatever arrived alongside the answer.
func confirmAskLines(line writeLine) {
	// THE QUESTION THE PAGE ASKS, which is what a consent is given TO. Its
	// translations are the ones the confirm screen shipped, moved here so the
	// proof can name a published row rather than quoting whatever arrived with
	// the answer.
	line(func(c *Copy) *string { return &c.ConfirmMarketingAsk },
		"News from time to time, roughly once a month. Your choice is respected.",
		"Sie erhalten von Zeit zu Zeit Neuigkeiten, etwa einmal im Monat. Ihre Entscheidung wird respektiert.",
		"Thỉnh thoảng gửi tin tức, khoảng mỗi tháng một lần. Lựa chọn của quý khách được tôn trọng.")
	line(func(c *Copy) *string { return &c.ConfirmMarketingYes },
		"Subscribe to news",
		"Neuigkeiten abonnieren",
		"Đăng ký nhận tin")
	line(func(c *Copy) *string { return &c.ConfirmMarketingNo },
		"Do not send news",
		"Keine Neuigkeiten senden",
		"Không gửi tin cho tôi")
	// THE DEDICATED SUBSCRIPTION LINK'S OWN QUESTION, which names the purpose
	// rather than describing a frequency. A grant through that door binds this;
	// one through the record-confirmation door binds the pair above.
	line(func(c *Copy) *string { return &c.ConfirmSubscriptionAsk },
		"Confirm that you want to receive {purpose}.",
		"Bestätigen Sie, dass Sie {purpose} erhalten möchten.",
		"Xác nhận quý khách muốn nhận {purpose}.")
	line(func(c *Copy) *string { return &c.ConfirmSubscriptionConfirm },
		"Confirm subscription",
		"Abonnement bestätigen",
		"Xác nhận đăng ký")
}

// recordNoticeLines is the copy for the messages the installation sends about a
// record it already holds: the invitation to check it, the notice that it
// exists, and the acknowledgement that advertising has stopped. None of them
// asks the reader for anything.
func recordNoticeLines(line writeLine) {
	line(func(c *Copy) *string { return &c.ConfirmRecordSubject },
		"Review your contact details",
		"Ihre Kontaktdaten prüfen",
		"Kiểm tra thông tin liên hệ của quý khách")
	line(func(c *Copy) *string { return &c.ConfirmRecordBody },
		"You can review the contact details we hold about you and correct any mistakes.\nChoosing to receive news from us is optional and separate from checking your details.",
		"Sie können Ihre bei uns gespeicherten Kontaktdaten prüfen und Fehler korrigieren.\nOb Sie außerdem Neuigkeiten von uns erhalten möchten, entscheiden Sie unabhängig davon.",
		"Quý khách có thể kiểm tra thông tin liên hệ chúng tôi lưu và sửa thông tin chưa đúng.\nViệc đăng ký nhận tin là tùy chọn và độc lập với việc kiểm tra thông tin.")
	line(func(c *Copy) *string { return &c.NoticeSubject },
		"Information about your personal data",
		"Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
		"Thông tin về việc xử lý dữ liệu cá nhân của quý khách")
	line(func(c *Copy) *string { return &c.NoticeBody },
		"We hold personal data about you. The link below explains how to review the details, where they came from, how they are used, and your privacy rights.\nYou can also contact us directly using the contact details in this message.",
		"Wir haben personenbezogene Daten von Ihnen gespeichert. Über den folgenden Link erfahren Sie, welche Daten das sind, woher sie stammen, wofür wir sie verwenden und welche Rechte Sie haben.\nBei Fragen können Sie sich auch direkt über die Kontaktdaten in dieser E-Mail an uns wenden.",
		"Chúng tôi đang lưu dữ liệu cá nhân của quý khách. Liên kết dưới đây cho biết dữ liệu nào được lưu, nguồn cung cấp, cách sử dụng và các quyền về dữ liệu của quý khách.\nQuý khách cũng có thể liên hệ trực tiếp theo thông tin trong email này.")
	line(func(c *Copy) *string { return &c.NoticeIgnore },
		"No reply or confirmation is required.",
		"Sie müssen weder antworten noch etwas bestätigen.",
		"Quý khách không cần trả lời hay xác nhận.")
	line(func(c *Copy) *string { return &c.OptOutAckSubject },
		"Your advertising opt-out is confirmed",
		"Ihre Abmeldung von Werbung ist bestätigt",
		"Yêu cầu ngừng nhận quảng cáo đã được xác nhận")
	line(func(c *Copy) *string { return &c.OptOutAckBody },
		"Your opt-out is recorded. We will not send further messages for the advertising purposes you opted out of. Messages already sent may still reach you.",
		"Ihre Abmeldung ist gespeichert. Zu den abgemeldeten Werbezwecken erhalten Sie keine weiteren Nachrichten. Bereits versendete Nachrichten können Sie noch erreichen.",
		"Yêu cầu ngừng nhận đã được ghi nhận. Chúng tôi sẽ không gửi thêm thư cho các mục đích quảng cáo quý khách đã từ chối. Những thư đã gửi vẫn có thể đến sau đó.")
	line(func(c *Copy) *string { return &c.OptOutAckIgnore },
		"No reply or further action is needed.",
		"Sie müssen nichts weiter tun und nicht antworten.",
		"Quý khách không cần trả lời hay thực hiện thêm thao tác nào.")
}

// confirmConsentLines is the message that asks a reader to turn a request into
// a permission, and the lines both confirmation links share — what the link is,
// how long it lasts, and what ignoring it does.
func confirmConsentLines(line writeLine) {
	line(func(c *Copy) *string { return &c.ConfirmConsentSubject },
		"Confirm your subscription",
		"Bitte bestätigen Sie Ihr Abonnement",
		"Vui lòng xác nhận đăng ký của quý khách")
	line(func(c *Copy) *string { return &c.ConfirmConsentBody },
		"Confirm this subscription only if you want to receive these messages. We will send messages for this purpose only after you confirm. You can withdraw your choice later.",
		"Bestätigen Sie das Abonnement nur, wenn Sie diese Nachrichten erhalten möchten. Erst danach senden wir Ihnen Nachrichten zu diesem Zweck. Sie können Ihre Einwilligung später widerrufen.",
		"Chỉ xác nhận đăng ký nếu muốn nhận các thư này. Thư cho mục đích này chỉ được gửi sau khi quý khách xác nhận. Quý khách có thể rút lại sự đồng ý sau đó.")
	line(func(c *Copy) *string { return &c.ConfirmPersonal },
		"This link is for your use only.",
		"Dieser Link ist nur für Sie bestimmt.",
		"Liên kết này chỉ dành cho quý khách.")
	line(func(c *Copy) *string { return &c.ConfirmExpiry },
		" It is valid until %s.",
		" Er ist bis %s gültig.",
		" Liên kết có hiệu lực đến %s.")
	line(func(c *Copy) *string { return &c.ConfirmRecordIgnore },
		"Reviewing your details is optional. Leaving this email unanswered does not subscribe you to anything.",
		"Die Prüfung Ihrer Daten ist freiwillig. Wenn Sie nichts tun, abonnieren Sie keine Nachrichten.",
		"Việc kiểm tra thông tin là tùy chọn. Không trả lời email này không đăng ký quý khách nhận bất kỳ nội dung nào.")
	line(func(c *Copy) *string { return &c.ConfirmConsentIgnore },
		"If you do not want this subscription, you can ignore this email. Without your confirmation, the subscription will not be activated.",
		"Wenn Sie diese Nachrichten nicht abonnieren möchten, können Sie diese E-Mail ignorieren. Ohne Ihre Bestätigung wird das Abonnement nicht aktiviert.",
		"Nếu không muốn đăng ký, quý khách có thể bỏ qua email này. Đăng ký sẽ không được kích hoạt khi chưa xác nhận.")
}
