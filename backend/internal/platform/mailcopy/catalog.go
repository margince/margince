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
	en, de, vi := &Copy{}, &Copy{}, &Copy{}
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
	confirmLines(line)

	return map[Language]Copy{English: *en, German: *de, Vietnamese: *vi}
}

// writeLine is what each section below is handed: the field to write, and its
// three renderings in the order the languages are declared.
type writeLine = func(field func(*Copy) *string, english, german, vietnamese string)

// resetLines is the password reset a person asked for.
// unsubscribeLines is the footer beneath an outgoing message.
func unsubscribeLines(line writeLine) {
	line(func(c *Copy) *string { return &c.UnsubscribeLabel },
		"Unsubscribe",
		"Abmelden",
		"Hủy đăng ký")
	line(func(c *Copy) *string { return &c.ManagePreferencesLabel },
		"Manage your preferences",
		"E-Mail-Einstellungen verwalten",
		"Quản lý tùy chọn email")
}

func resetLines(line writeLine) {
	line(func(c *Copy) *string { return &c.ResetSubject },
		"Reset your Margince password",
		"Margince-Passwort zurücksetzen",
		"Đặt lại mật khẩu Margince")
	line(func(c *Copy) *string { return &c.ResetIntro },
		"Someone requested a password reset for your Margince account.",
		"Jemand hat für dein Margince-Konto eine Passwort-Zurücksetzung angefordert.",
		"Có người yêu cầu đặt lại mật khẩu cho tài khoản Margince của bạn.")
	line(func(c *Copy) *string { return &c.ResetAction },
		"Reset your password within one hour:",
		"Setze dein Passwort innerhalb einer Stunde zurück:",
		"Đặt lại mật khẩu trong vòng một giờ:")
	line(func(c *Copy) *string { return &c.ResetIgnore },
		"If this wasn't you, ignore this email — your password is unchanged.",
		"Warst du das nicht, ignoriere diese E-Mail — dein Passwort bleibt unverändert.",
		"Nếu không phải bạn, hãy bỏ qua email này — mật khẩu của bạn không thay đổi.")
}

// inviteLines is the invitation an administrator sent on somebody's behalf.
func inviteLines(line writeLine) {
	line(func(c *Copy) *string { return &c.InviteSubject },
		"You're invited to Margince",
		"Du bist zu Margince eingeladen",
		"Bạn được mời vào Margince")
	line(func(c *Copy) *string { return &c.InviteIntro },
		"You've been invited to Margince.",
		"Du wurdest zu Margince eingeladen.",
		"Bạn đã được mời vào Margince.")
	line(func(c *Copy) *string { return &c.InviteAction },
		"Set your password within seven days to sign in:",
		"Setze innerhalb von sieben Tagen dein Passwort, um dich anzumelden:",
		"Đặt mật khẩu trong vòng bảy ngày để đăng nhập:")
	line(func(c *Copy) *string { return &c.InviteIgnore },
		"If you weren't expecting this, you can ignore this email.",
		"Hast du das nicht erwartet, kannst du diese E-Mail ignorieren.",
		"Nếu bạn không mong đợi điều này, bạn có thể bỏ qua email này.")
}

// weeklyLines is the Monday retrospective: what the week did, and then what a
// reader does next with it. Split where the mail itself changes subject — the
// counted figures above, the movement and the two links below — because the
// list is one call per line and grows every time the mail says something new.
func weeklyLines(line writeLine) {
	weeklyFigureLines(line)
	weeklyMovementLines(line)
}

// weeklyFigureLines are the counted outcomes: what was delivered, won, lost,
// moved, decided, and how much of the morning queue was answered.
func weeklyFigureLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklySubject },
		"Your week: ",
		"Deine Woche: ",
		"Tuần của bạn: ")
	line(func(c *Copy) *string { return &c.WeeklyHeading },
		"Your week of ",
		"Deine Woche ab ",
		"Tuần của bạn từ ")
	line(func(c *Copy) *string { return &c.WeeklyTasksDelivered },
		"Tasks delivered",
		"Aufgaben erledigt",
		"Công việc đã hoàn thành")
	line(func(c *Copy) *string { return &c.WeeklyOfDue },
		"%d of %d",
		"%d von %d",
		"%d trên %d")
	line(func(c *Copy) *string { return &c.WeeklyDealsWon },
		"Won",
		"Gewonnen",
		"Thắng")
	line(func(c *Copy) *string { return &c.WeeklyDealsLost },
		"Lost",
		"Verloren",
		"Thua")
	line(func(c *Copy) *string { return &c.WeeklyMoved },
		"Moved",
		"Bewegt",
		"Đã chuyển")
	line(func(c *Copy) *string { return &c.WeeklyDecided },
		"You decided",
		"Von dir entschieden",
		"Bạn đã quyết")
	weeklyDecisionLines(line)
}

// weeklyDecisionLines is the retrospective's second half: what the reader did
// with their queue, and how each thing turned out.
//
// Split from weeklyLines, which had grown past the function ceiling. The seam
// is the one the mail itself has — the top of the message reports the week's
// numbers, and from here down it reports the reader's own decisions — so the
// two are edited for different reasons and read as two blocks on the page.
func weeklyDecisionLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyYes },
		"yes",
		"ja",
		"đồng ý")
	line(func(c *Copy) *string { return &c.WeeklyNo },
		"no",
		"nein",
		"từ chối")
	weeklyQueueLines(line)
	weeklyClosingLines(line)
}

// weeklyQueueLines is the retrospective's middle: what the morning list did
// with the week, and what it carried into the next one.
func weeklyQueueLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyQueue },
		"Morning queue",
		"Morgen-Liste",
		"Danh sách buổi sáng")
	line(func(c *Copy) *string { return &c.WeeklyActed },
		"acted",
		"bearbeitet",
		"đã xử lý")
	line(func(c *Copy) *string { return &c.WeeklyDismissed },
		"dismissed",
		"weggeklickt",
		"đã bỏ qua")
	line(func(c *Copy) *string { return &c.WeeklyCarried },
		"Carried over",
		"Übernommen",
		"Chuyển tiếp")
}

// weeklyMovementLines are what actually moved, the way on to the rest of it,
// and the words a single row's outcome is written with.
func weeklyMovementLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyWhatMoved },
		"What moved:",
		"Was sich bewegt hat:",
		"Những gì đã chuyển động:")
	line(func(c *Copy) *string { return &c.WeeklyAndMore },
		"… and %d more, on Home",
		"… weitere auf Home: %d",
		"… và %d mục nữa, trên Home")
}

// weeklyClosingLines is what the retrospective asks for once it has reported:
// next week's plan, the archive behind it, and the outcome words the movement
// list is written in.
func weeklyClosingLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyPlanAhead },
		"Plan your week",
		"Ihre Woche planen",
		"Lập kế hoạch tuần của bạn")
	line(func(c *Copy) *string { return &c.WeeklyFullWeek },
		"The full week, and the ones before it:",
		"Die ganze Woche, und die davor:",
		"Cả tuần, và những tuần trước:")
	line(func(c *Copy) *string { return &c.WeeklyOutcomeWon },
		"won",
		"gewonnen",
		"thắng")
	line(func(c *Copy) *string { return &c.WeeklyOutcomeLost },
		"lost",
		"verloren",
		"thua")
	line(func(c *Copy) *string { return &c.WeeklyOutcomeMoved },
		"moved",
		"bewegt",
		"đã chuyển")
}

// morningLines is the daily brief.
func morningLines(line writeLine) {
	line(func(c *Copy) *string { return &c.MorningSubject },
		"Your morning: ",
		"Dein Morgen: ",
		"Buổi sáng của bạn: ")
	line(func(c *Copy) *string { return &c.MorningHeading },
		"Your morning, ",
		"Dein Morgen, ",
		"Buổi sáng của bạn, ")
	line(func(c *Copy) *string { return &c.MorningTop },
		"What to start with:",
		"Womit du anfängst:",
		"Bắt đầu với:")
	line(func(c *Copy) *string { return &c.MorningAndMore },
		"and %d more in the brief",
		"und %d weitere im Briefing",
		"và %d mục khác trong bản tóm tắt")
	line(func(c *Copy) *string { return &c.MorningQuiet },
		"Nothing is waiting on you this morning.",
		"Heute Morgen wartet nichts auf dich.",
		"Sáng nay không có gì đang chờ bạn.")
	line(func(c *Copy) *string { return &c.MorningOpenDay },
		"Open your day:",
		"Öffne deinen Tag:",
		"Mở ngày của bạn:")
}

// confirmLines is the two links the installation sends as itself.
//
// The English is the wording that shipped, unchanged: it is pinned by hash and
// recorded on every consent proof, so moving a word here is a version bump, not
// a translation.
//
// The German and Vietnamese use the formal address (Sie / quý vị), unlike the
// reset and invite copy above. Those speak to a colleague who works here; these
// speak to a stranger the installation holds a record about, and about their
// own rights.
func confirmLines(line writeLine) {
	// THE QUESTION THE PAGE ASKS, which is what a consent is given TO. Its
	// translations are the ones the confirm screen shipped, moved here so the
	// proof can name a published row rather than quoting whatever arrived with
	// the answer.
	line(func(c *Copy) *string { return &c.ConfirmMarketingAsk },
		"News from time to time, roughly once a month. You decide, and I will hold to it.",
		"Neuigkeiten ab und zu, etwa einmal im Monat. Sie entscheiden, ich halte mich daran.",
		"Tin tức thỉnh thoảng, khoảng mỗi tháng một lần. Bạn quyết định, và tôi sẽ tuân theo.")
	line(func(c *Copy) *string { return &c.ConfirmMarketingYes },
		"Yes, keep me posted",
		"Ja, halten Sie mich auf dem Laufenden",
		"Có, hãy gửi tin cho tôi")
	line(func(c *Copy) *string { return &c.ConfirmMarketingNo },
		"No thanks, just keep my details correct",
		"Nein danke, nur meine Daten korrekt halten",
		"Không, chỉ cần giữ thông tin của tôi chính xác")
	// THE DEDICATED SUBSCRIPTION LINK'S OWN QUESTION, which names the purpose
	// rather than describing a frequency. A grant through that door binds this;
	// one through the record-confirmation door binds the pair above.
	line(func(c *Copy) *string { return &c.ConfirmSubscriptionAsk },
		"Confirm that you want to receive {purpose}.",
		"Bestätigen Sie, dass Sie {purpose} erhalten möchten.",
		"Xác nhận rằng bạn muốn nhận {purpose}.")
	line(func(c *Copy) *string { return &c.ConfirmSubscriptionConfirm },
		"Yes, subscribe me",
		"Ja, ich möchte das Abo",
		"Có, đăng ký cho tôi")
	line(func(c *Copy) *string { return &c.ConfirmRecordSubject },
		"Your details, and whether we may stay in touch",
		"Ihre Daten, und ob wir in Kontakt bleiben dürfen",
		"Thông tin của quý vị, và liệu chúng tôi có thể giữ liên lạc")
	line(func(c *Copy) *string { return &c.ConfirmRecordBody },
		"You can see what we have on file about you, correct anything that is wrong,\n"+
			"and tell us whether you want to hear from us.",
		"Sie können sehen, was wir über Sie gespeichert haben, Falsches korrigieren\n"+
			"und uns sagen, ob Sie von uns hören möchten.",
		"Quý vị có thể xem chúng tôi lưu giữ thông tin gì về mình, sửa những gì chưa đúng,\n"+
			"và cho chúng tôi biết quý vị có muốn nhận tin từ chúng tôi hay không.")
	line(func(c *Copy) *string { return &c.ConfirmConsentSubject },
		"Please confirm you want to hear from us",
		"Bitte bestätigen Sie, dass Sie von uns hören möchten",
		"Vui lòng xác nhận quý vị muốn nhận tin từ chúng tôi")
	line(func(c *Copy) *string { return &c.ConfirmConsentBody },
		"You asked to hear from us. Confirming below is what turns that into a\n"+
			"permission we will act on — until you do, we will not write to you about it.",
		"Sie haben darum gebeten, von uns zu hören. Erst Ihre Bestätigung unten macht\n"+
			"daraus eine Einwilligung, auf die wir uns stützen — bis dahin schreiben wir\n"+
			"Ihnen dazu nicht.",
		"Quý vị đã yêu cầu nhận tin từ chúng tôi. Việc xác nhận bên dưới mới biến điều đó\n"+
			"thành sự đồng ý mà chúng tôi dựa vào — cho đến lúc đó, chúng tôi sẽ không\n"+
			"viết cho quý vị về việc này.")
	line(func(c *Copy) *string { return &c.ConfirmPersonal },
		"This link is personal to you.",
		"Dieser Link ist persönlich für Sie.",
		"Liên kết này dành riêng cho quý vị.")
	// The date arrives ISO-formatted, so each language frames it rather than
	// inflecting it: German "bis zum" would want an ordinal with a period
	// ("bis zum 9. März"), which no formatter in this tree produces.
	line(func(c *Copy) *string { return &c.ConfirmExpiry },
		" It works until %s.",
		" Er gilt bis einschließlich %s.",
		" Liên kết có hiệu lực đến hết ngày %s.")
	line(func(c *Copy) *string { return &c.ConfirmRecordIgnore },
		"You do not have to do anything. Ignoring this changes nothing.",
		"Sie müssen nichts tun. Ignorieren ändert nichts.",
		"Quý vị không cần làm gì cả. Bỏ qua thư này thì không có gì thay đổi.")
	line(func(c *Copy) *string { return &c.ConfirmConsentIgnore },
		"If you did not ask for this, ignore it. Nothing happens until you confirm.",
		"Falls Sie darum nicht gebeten haben, ignorieren Sie diese E-Mail. Ohne Ihre\n"+
			"Bestätigung passiert nichts.",
		"Nếu quý vị không yêu cầu điều này, hãy bỏ qua. Không có gì xảy ra cho đến khi\n"+
			"quý vị xác nhận.")
}
