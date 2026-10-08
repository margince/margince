// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

// The Monday retrospective's copy, kept apart from catalog.go because it is the
// part of the catalog that grows with the weekly panel.

// weeklyLines is the Monday retrospective: what the week did, and then what a
// reader does next with it. It is split where the mail itself changes subject
// (the counted figures, then the movement and the two links), because the list
// is one call per line and grows every time the mail says something new.
func weeklyLines(line writeLine) {
	weeklyFigureLines(line)
	weeklyMovementLines(line)
	weeklyCoverageLines(line)
}

// weeklyCoverageLines are what a figure says in place of a count its source
// never measured, and the line a week before every source began opens with.
func weeklyCoverageLines(line writeLine) {
	line(func(c *Copy) *string { return &c.WeeklyNotRecorded },
		"Not recorded",
		"Nicht erfasst",
		"Không được ghi nhận")
	line(func(c *Copy) *string { return &c.WeeklyRecordedFrom },
		"Recorded from {date}",
		"Erfasst ab {date}",
		"Ghi nhận từ {date}")
	line(func(c *Copy) *string { return &c.WeeklyNoRecords },
		"No records from this source",
		"Keine Datensätze aus dieser Quelle",
		"Nguồn này chưa có bản ghi")
	line(func(c *Copy) *string { return &c.WeeklyPartialFrom },
		"Partial week: counted from {date}",
		"Teilwoche: gezählt ab {date}",
		"Một phần tuần: tính từ {date}")
	line(func(c *Copy) *string { return &c.WeeklyPartialValue },
		"{value} (partial)",
		"{value} (teilweise)",
		"{value} (một phần)")
	line(func(c *Copy) *string { return &c.WeeklyBeforeHistory },
		"This week falls before the first records, so it has no figures to report.",
		"Diese Woche liegt vor den ersten Datensätzen und hat daher keine Zahlen.",
		"Tuần này nằm trước các bản ghi đầu tiên nên không có số liệu để báo cáo.")
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
		"Proposals decided",
		"Entschiedene Vorschläge",
		"Bạn đã quyết")
	weeklyDecisionLines(line)
}

// weeklyDecisionLines is the retrospective's second half: what the reader did
// with their queue, and how each thing turned out.
//
// The seam is the one the mail itself has. The top of the message reports the
// week's numbers and from here down it reports the reader's own decisions, so
// the two are edited for different reasons and read as two blocks on the page.
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
		"Übertragen",
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
		"This week’s commitments",
		"Zusagen dieser Woche",
		"Cam kết tuần này")
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
