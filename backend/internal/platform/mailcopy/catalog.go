// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

// The copy itself, one entry per language the contract's `base_language` enum
// admits.
//
// The weekly labels are the frontend catalog's own strings, character for
// character, and TestEveryMailLabelMatchesTheScreenThatShowsIt holds them there.
// The transactional copy has no screen behind it and lives only here.
var catalog = map[Language]Copy{
	English: {
		ResetSubject: "Reset your Margince password",
		ResetIntro:   "Someone requested a password reset for your Margince account.",
		ResetAction:  "Reset your password within one hour:",
		ResetIgnore:  "If this wasn't you, ignore this email — your password is unchanged.",

		InviteSubject: "You're invited to Margince",
		InviteIntro:   "You've been invited to Margince.",
		InviteAction:  "Set your password within seven days to sign in:",
		InviteIgnore:  "If you weren't expecting this, you can ignore this email.",

		WeeklySubject:      "Your week: ",
		WeeklyHeading:      "Your week of ",
		WeeklyPromised:     "Promised, delivered",
		WeeklyDealsWon:     "Won",
		WeeklyDealsLost:    "Lost",
		WeeklyMoved:        "Moved",
		WeeklyDecided:      "You decided",
		WeeklyYes:          "yes",
		WeeklyNo:           "no",
		WeeklyQueue:        "Morning queue",
		WeeklyActed:        "acted",
		WeeklyDismissed:    "dismissed",
		WeeklyCarried:      "Carried over",
		WeeklyWhatMoved:    "What moved:",
		WeeklyAndMore:      "… and %d more, on Home",
		WeeklyFullWeek:     "The full week, and the ones before it:",
		WeeklyOutcomeWon:   "won",
		WeeklyOutcomeLost:  "lost",
		WeeklyOutcomeMoved: "moved",
	},
	German: {
		ResetSubject: "Margince-Passwort zurücksetzen",
		ResetIntro:   "Jemand hat für dein Margince-Konto eine Passwort-Zurücksetzung angefordert.",
		ResetAction:  "Setze dein Passwort innerhalb einer Stunde zurück:",
		ResetIgnore:  "Warst du das nicht, ignoriere diese E-Mail — dein Passwort bleibt unverändert.",

		InviteSubject: "Du bist zu Margince eingeladen",
		InviteIntro:   "Du wurdest zu Margince eingeladen.",
		InviteAction:  "Setze innerhalb von sieben Tagen dein Passwort, um dich anzumelden:",
		InviteIgnore:  "Hast du das nicht erwartet, kannst du diese E-Mail ignorieren.",

		WeeklySubject:      "Deine Woche: ",
		WeeklyHeading:      "Deine Woche ab ",
		WeeklyPromised:     "Zugesagt, erledigt",
		WeeklyDealsWon:     "Gewonnen",
		WeeklyDealsLost:    "Verloren",
		WeeklyMoved:        "Bewegt",
		WeeklyDecided:      "Von dir entschieden",
		WeeklyYes:          "ja",
		WeeklyNo:           "nein",
		WeeklyQueue:        "Morgen-Liste",
		WeeklyActed:        "bearbeitet",
		WeeklyDismissed:    "weggeklickt",
		WeeklyCarried:      "Übernommen",
		WeeklyWhatMoved:    "Was sich bewegt hat:",
		WeeklyAndMore:      "… und %d weitere, auf Home",
		WeeklyFullWeek:     "Die ganze Woche, und die davor:",
		WeeklyOutcomeWon:   "gewonnen",
		WeeklyOutcomeLost:  "verloren",
		WeeklyOutcomeMoved: "bewegt",
	},
	Vietnamese: {
		ResetSubject: "Đặt lại mật khẩu Margince",
		ResetIntro:   "Có người yêu cầu đặt lại mật khẩu cho tài khoản Margince của bạn.",
		ResetAction:  "Đặt lại mật khẩu trong vòng một giờ:",
		ResetIgnore:  "Nếu không phải bạn, hãy bỏ qua email này — mật khẩu của bạn không thay đổi.",

		InviteSubject: "Bạn được mời vào Margince",
		InviteIntro:   "Bạn đã được mời vào Margince.",
		InviteAction:  "Đặt mật khẩu trong vòng bảy ngày để đăng nhập:",
		InviteIgnore:  "Nếu bạn không mong đợi điều này, bạn có thể bỏ qua email này.",

		WeeklySubject:      "Tuần của bạn: ",
		WeeklyHeading:      "Tuần của bạn từ ",
		WeeklyPromised:     "Đã hứa, đã xong",
		WeeklyDealsWon:     "Thắng",
		WeeklyDealsLost:    "Thua",
		WeeklyMoved:        "Đã chuyển",
		WeeklyDecided:      "Bạn đã quyết",
		WeeklyYes:          "đồng ý",
		WeeklyNo:           "từ chối",
		WeeklyQueue:        "Danh sách buổi sáng",
		WeeklyActed:        "đã xử lý",
		WeeklyDismissed:    "đã bỏ qua",
		WeeklyCarried:      "Chuyển tiếp",
		WeeklyWhatMoved:    "Những gì đã chuyển động:",
		WeeklyAndMore:      "… và %d mục nữa, trên Home",
		WeeklyFullWeek:     "Cả tuần, và những tuần trước:",
		WeeklyOutcomeWon:   "thắng",
		WeeklyOutcomeLost:  "thua",
		WeeklyOutcomeMoved: "đã chuyển",
	},
}
