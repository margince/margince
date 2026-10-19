// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var dueWords = struct{ Ahead, One, Today langcopy.Phrase }{
	Ahead: langcopy.Phrase{En: "Due in %d days.", De: "Fällig in %d Tagen.", Vi: "Đến hạn sau %d ngày."},
	One:   langcopy.Phrase{En: "Due in one day.", De: "In einem Tag fällig.", Vi: "Đến hạn sau một ngày."},
	Today: langcopy.Phrase{En: "Due today.", De: "Heute fällig.", Vi: "Đến hạn hôm nay."},
}

// DeadlineWords gives both record cards the same explanation of a dated task.
func DeadlineWords(due time.Time, now time.Time, language string) string {
	if words, late := LateWords(&due, now, language); late {
		return words
	}
	return UpcomingWords(due, now, language)
}

// UpcomingWords counts remaining whole days, rather than calendar boundaries.
func UpcomingWords(due time.Time, now time.Time, language string) string {
	said := langcopy.For(language)
	days := elapsed.FullDaysUntil(now, due)
	if days <= 0 {
		return said.Say(dueWords.Today)
	}
	if days == 1 {
		return said.Say(dueWords.One)
	}
	return fmt.Sprintf(said.Say(dueWords.Ahead), days)
}
