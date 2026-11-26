// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package fieldcatalog

import (
	"fmt"
	"time"
)

// ProjectAnnualOccurrence picks the occurrence in the requested annual window.
func ProjectAnnualOccurrence(month time.Month, day int, from, to time.Time) time.Time {
	fromMMDD := from.Format("0102")
	toMMDD := to.Format("0102")
	if fromMMDD < toMMDD {
		return time.Date(to.Year(), month, day, 0, 0, 0, 0, to.Location())
	}
	candMMDD := fmt.Sprintf("%02d%02d", int(month), day)
	if candMMDD >= fromMMDD {
		return time.Date(from.Year(), month, day, 0, 0, 0, 0, from.Location())
	}
	return time.Date(to.Year(), month, day, 0, 0, 0, 0, to.Location())
}
