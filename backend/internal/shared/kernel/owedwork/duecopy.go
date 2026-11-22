// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var dueWords = struct{ Ahead, Tomorrow, Today langcopy.Phrase }{
	Ahead:    langcopy.Phrase{En: "Due in %d days.", De: "Fällig in %d Tagen.", Vi: "Đến hạn sau %d ngày."},
	Tomorrow: langcopy.Phrase{En: "Due tomorrow.", De: "Morgen fällig.", Vi: "Đến hạn ngày mai."},
	Today:    langcopy.Phrase{En: "Due today.", De: "Heute fällig.", Vi: "Đến hạn hôm nay."},
}

// DeadlineWords gives both record cards the same explanation of a dated task.
func DeadlineWords(due time.Time, now time.Time, loc *time.Location, language string) string {
	if words, late := LateWords(&due, now, loc, language); late {
		return words
	}
	return UpcomingWords(due, now, loc, language)
}

// UpcomingWords counts calendar days in loc, as the task table's badge beside
// it does, so a task due at 01:00 tomorrow reads "tomorrow" at 23:00 today.
func UpcomingWords(due time.Time, now time.Time, loc *time.Location, language string) string {
	said := langcopy.For(language)
	days := elapsed.Days(now, due, loc)
	if days <= 0 {
		return said.Say(dueWords.Today)
	}
	if days == 1 {
		return said.Say(dueWords.Tomorrow)
	}
	return fmt.Sprintf(said.Say(dueWords.Ahead), days)
}
