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
	resetLines(line)
	inviteLines(line)
	weeklyLines(line)

	return map[Language]Copy{English: *en, German: *de, Vietnamese: *vi}
}

// writeLine is what each section below is handed: the field to write, and its
// three renderings in the order the languages are declared.
type writeLine = func(field func(*Copy) *string, english, german, vietnamese string)

// resetLines is the password reset a person asked for.
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

// weeklyLines is the Monday retrospective.
func weeklyLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklySubject },
		"Your week: ",
		"Deine Woche: ",
		"Tuần của bạn: ")
	line(func(c *Copy) *string { return &c.WeeklyHeading },
		"Your week of ",
		"Deine Woche ab ",
		"Tuần của bạn từ ")
	line(func(c *Copy) *string { return &c.WeeklyPromised },
		"Promised, delivered",
		"Zugesagt, erledigt",
		"Đã hứa, đã xong")
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
	line(func(c *Copy) *string { return &c.WeeklyYes },
		"yes",
		"ja",
		"đồng ý")
	line(func(c *Copy) *string { return &c.WeeklyNo },
		"no",
		"nein",
		"từ chối")
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
	line(func(c *Copy) *string { return &c.WeeklyWhatMoved },
		"What moved:",
		"Was sich bewegt hat:",
		"Những gì đã chuyển động:")
	line(func(c *Copy) *string { return &c.WeeklyAndMore },
		"… and %d more, on Home",
		"… weitere auf Home: %d",
		"… và %d mục nữa, trên Home")
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
