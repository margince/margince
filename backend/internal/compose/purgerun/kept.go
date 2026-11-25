// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package purgerun

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// KeptBreakdown is the skipped count, by the reason each message survived.
// The four reasons are disjoint and sum to Skipped.
type KeptBreakdown struct {
	// Held is how many an erasure or a controller pinned by hand.
	Held int `json:"held"`
	// UnderStatute is how many sit inside their commercial-retention window.
	UnderStatute int `json:"under_statute"`
	// UnderRequest is how many a data-subject request is still about.
	UnderRequest int `json:"under_request"`
	// UnderUndeterminedFloor is how many were kept because this installation
	// could not say what the law requires. It is apart from UnderStatute
	// because the basis differs, and one count would claim a window was measured.
	UnderUndeterminedFloor int `json:"under_undetermined_floor"`
	// StatutoryClass names the retention class that shielded them. It is empty
	// when nothing was shielded by it, so no rule is shown beside a zero.
	StatutoryClass string `json:"statutory_class,omitempty"`
	// StatutoryYears is how long that class keeps its records, in whole years.
	// It is zero when the period is not whole years, because a rounded number
	// would misstate how long mail is held. The ISO 8601 form is not sent, since
	// a colleague reads this number.
	StatutoryYears int `json:"statutory_years,omitempty"`
	// StatutoryFromYearEnd says the period counts from the end of the calendar
	// year, which is the difference between "six years" and "up to seven".
	StatutoryFromYearEnd bool `json:"statutory_from_year_end,omitempty"`
}

// BreakdownOf says why a purge left what it left. The statutory class is named
// only when it kept something, so a reader is not shown a rule that applied to
// nothing of theirs.
func BreakdownOf(subject capture.PurgeSubject, policy privacy.FloorPolicy) KeptBreakdown {
	kept := KeptBreakdown{
		Held:                   len(subject.Held),
		UnderStatute:           len(subject.UnderStatute),
		UnderRequest:           len(subject.UnderRequest),
		UnderUndeterminedFloor: len(subject.UnderUndeterminedFloor),
	}
	if kept.UnderStatute > 0 {
		class := policy.StrictestClass(time.Now())
		keep := class.Keep
		kept.StatutoryClass = string(class.Name)
		kept.StatutoryFromYearEnd = class.Anchor == jurisdiction.AnchorCalendarYearEnd
		// Only a whole-year period is reported. A period in months or days has
		// no number to state truthfully, so the class is named alone.
		if keep.Months == 0 && keep.Days == 0 {
			kept.StatutoryYears = keep.Years
		}
	}
	return kept
}
