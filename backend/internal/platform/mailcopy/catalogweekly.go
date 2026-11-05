// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

// The Monday retrospective's copy, kept apart from catalog.go because it is the
// part of the catalog that grows with the weekly panel.

// weeklyLines is the Monday retrospective: what the week did, and then what a
// reader does next with it. It is split where the mail changes subject: the
// counted figures, the movement and the two links, and the words for a figure
// nobody measured. The list is one call per line and grows with the mail.
func weeklyLines(line writeLine) {
	weeklyFigureLines(line)
	weeklyMovementLines(line)
	weeklyCoverageLines(line)
}

// weeklyCoverageLines are what a figure says in place of a count nobody
// measured. They include the line a week before every source opens with.
func weeklyCoverageLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyNotRecorded },
		"Not recorded",
		"Nicht erfasst",
		"Không ghi nhận")
	line(func(c *Copy) *string { return &c.WeeklyRecordedFrom },
		"Recorded from {date}",
		"Erfasst ab {date}",
		"Ghi nhận từ {date}")
	line(func(c *Copy) *string { return &c.WeeklyNoRecords },
		"No information recorded for this part of the report",
		"Für diesen Teil der Auswertung liegen keine Angaben vor",
		"Chưa có thông tin cho phần báo cáo này")
	line(func(c *Copy) *string { return &c.WeeklyPartialFrom },
		"Figures cover the period from {date} only",
		"Die Auswertung umfasst nur den Zeitraum ab {date}",
		"Số liệu chỉ bao gồm thời gian từ {date}")
	line(func(c *Copy) *string { return &c.WeeklyPartialValue },
		"{value} (partial)",
		"{value} (teilweise)",
		"{value} (một phần)")
	line(func(c *Copy) *string { return &c.WeeklyBeforeHistory },
		"This week falls before the first records, so it has no figures to report.",
		"Für diese Woche gibt es keine Zahlen, da sie vor den ersten Datensätzen liegt.",
		"Tuần này nằm trước thời điểm có dữ liệu đầu tiên nên không có số liệu để báo cáo.")
	line(func(c *Copy) *string { return &c.WeeklyUnavailable },
		"Unavailable",
		"Nicht verfügbar",
		"Không khả dụng")
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
		"Deine Woche vom ",
		"Tuần của bạn từ ")
	line(func(c *Copy) *string { return &c.WeeklyTasksDelivered },
		"Due tasks completed",
		"Fällige Aufgaben erledigt",
		"Việc đến hạn đã hoàn tất")
	line(func(c *Copy) *string { return &c.WeeklyOfDue },
		"%d of %d",
		"%d von %d",
		"%d trên %d")
	line(func(c *Copy) *string { return &c.WeeklyDealsWon },
		"Deals won",
		"Gewonnene Deals",
		"Deal đã chốt")
	line(func(c *Copy) *string { return &c.WeeklyDealsLost },
		"Deals lost",
		"Verlorene Deals",
		"Deal đã mất")
	line(func(c *Copy) *string { return &c.WeeklyMoved },
		"Deals with stage changes",
		"Deals mit Phasenwechsel",
		"Deal đã đổi giai đoạn")
	line(func(c *Copy) *string { return &c.WeeklyDecided },
		"Proposals decided",
		"Entschiedene Vorschläge",
		"Đề xuất đã có quyết định")
	weeklyDecisionLines(line)
}

// weeklyDecisionLines is the retrospective's second half: what the reader did
// with their queue, and how each thing turned out.
//
// The seam is the one the mail itself has. The top of the message reports the
// week's numbers. From here down it reports the reader's own decisions. The two
// are edited for different reasons and read as two blocks on the page.
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
		"Morning brief items",
		"Einträge im Morgenbericht",
		"Việc trong Bản tin sáng")
	line(func(c *Copy) *string { return &c.WeeklyActed },
		"acted",
		"bearbeitet",
		"đã xử lý")
	line(func(c *Copy) *string { return &c.WeeklyDismissed },
		"dismissed",
		"weggeklickt",
		"đã bỏ qua")
	line(func(c *Copy) *string { return &c.WeeklyCarried },
		"Open at week’s end",
		"Zum Wochenende offen",
		"Việc còn mở cuối tuần")
}

// weeklyMovementLines are what actually moved, the way on to the rest of it,
// and the words a single row's outcome is written with.
func weeklyMovementLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyWhatMoved },
		"Deals that changed this week:",
		"Diese Deals haben sich verändert:",
		"Các deal thay đổi trong tuần:")
	line(func(c *Copy) *string { return &c.WeeklyAndMore },
		"… and %d more, on Home",
		"… weitere auf Home: %d",
		"… và %d mục nữa, trên Home")
}

// weeklyClosingLines is what the retrospective asks for once it has reported.
// That is next week's plan and the archive behind it. The outcome words the
// movement list is written in close it.
func weeklyClosingLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyFullWeek },
		"Review the results and plan your next step",
		"Ergebnisse ansehen und den nächsten Schritt planen",
		"Xem kết quả và lên kế hoạch bước tiếp theo")
	line(func(c *Copy) *string { return &c.WeeklyOutcomeWon },
		"won",
		"gewonnen",
		"đã chốt")
	line(func(c *Copy) *string { return &c.WeeklyOutcomeLost },
		"lost",
		"verloren",
		"đã mất")
	line(func(c *Copy) *string { return &c.WeeklyOutcomeMoved },
		"stage changed",
		"Verkaufsphase geändert",
		"đã đổi giai đoạn")
}
